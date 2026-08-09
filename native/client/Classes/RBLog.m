#import "RBLog.h"
#import "RBConfig.h"

#include <fcntl.h>
#include <signal.h>
#include <stdarg.h>
#include <stdio.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>

static dispatch_queue_t RBLogQueue;
static NSDateFormatter *RBLogDateFormatter;
static NSFileHandle *RBLogHandle;
static unsigned long long RBLogSize;

static NSString *RBMigratedLevelForMessage(NSString *message) {
    NSString *lower = [message lowercaseString], *level = @"info";
    if ([lower rangeOfString:@"error"].location != NSNotFound ||
        [lower rangeOfString:@"failed"].location != NSNotFound ||
        [lower rangeOfString:@"rejected"].location != NSNotFound ||
        [lower rangeOfString:@"fatal"].location != NSNotFound) level = @"error";
    else if ([lower rangeOfString:@"warning"].location != NSNotFound ||
             [lower rangeOfString:@"timeout"].location != NSNotFound ||
             [lower rangeOfString:@"stalled"].location != NSNotFound ||
             [lower rangeOfString:@"drop"].location != NSNotFound) level = @"warn";
    return level;
}

static NSString *RBMigratedComponentForMessage(NSString *message) {
    NSRange colon = [message rangeOfString:@":"];
    if (colon.location != NSNotFound && colon.location > 0 && colon.location <= 24) {
        return [[message substringToIndex:colon.location] lowercaseString];
    }
    NSArray *words = [message componentsSeparatedByCharactersInSet:[NSCharacterSet whitespaceCharacterSet]];
    NSString *first = [words count] ? [words objectAtIndex:0] : nil;
    return [first length] && [first length] <= 18 ? [first lowercaseString] : @"app";
}

static id RBMigratedTypedFieldValue(NSString *value) {
    if ([value isEqualToString:@"true"] || [value isEqualToString:@"yes"]) return @YES;
    if ([value isEqualToString:@"false"] || [value isEqualToString:@"no"]) return @NO;
    NSScanner *integerScanner = [NSScanner scannerWithString:value];
    long long integer = 0;
    if ([integerScanner scanLongLong:&integer] && [integerScanner isAtEnd]) return @(integer);
    NSScanner *doubleScanner = [NSScanner scannerWithString:value];
    double number = 0;
    if ([doubleScanner scanDouble:&number] && [doubleScanner isAtEnd]) return @(number);
    return value;
}

static NSDictionary *RBMigratedFieldsForMessage(NSString *message) {
    NSMutableDictionary *fields = [NSMutableDictionary dictionary];
    NSCharacterSet *validKey = [NSCharacterSet characterSetWithCharactersInString:@"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"];
    NSCharacterSet *trailing = [NSCharacterSet characterSetWithCharactersInString:@",;)"];
    for (NSString *token in [message componentsSeparatedByCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]]) {
        NSRange equals = [token rangeOfString:@"="];
        if (equals.location == NSNotFound || equals.location == 0 || equals.location + 1 >= [token length]) continue;
        NSString *key = [[token substringToIndex:equals.location] lowercaseString];
        if ([key rangeOfCharacterFromSet:[validKey invertedSet]].location != NSNotFound) continue;
        NSString *value = [token substringFromIndex:equals.location + 1];
        while ([value length] && [trailing characterIsMember:[value characterAtIndex:[value length] - 1]]) {
            value = [value substringToIndex:[value length] - 1];
        }
        if ([value length]) [fields setObject:RBMigratedTypedFieldValue(value) forKey:key];
    }
    return fields;
}

static NSData *RBJSONLine(NSDictionary *record) {
    NSData *json = [NSJSONSerialization dataWithJSONObject:record options:0 error:nil];
    if (!json) return nil;
    NSMutableData *line = [json mutableCopy];
    [line appendBytes:"\n" length:1];
    return line;
}

static void RBMigrateLegacyLog(NSFileManager *fm) {
    NSData *existing = [NSData dataWithContentsOfFile:RBLogFile];
    if (![existing length]) return;
    NSString *text = [[NSString alloc] initWithData:existing encoding:NSUTF8StringEncoding];
    if (![text length]) return;
    NSMutableData *migrated = [NSMutableData data];
    BOOL changed = NO;
    for (NSString *line in [text componentsSeparatedByCharactersInSet:[NSCharacterSet newlineCharacterSet]]) {
        if (![line length]) continue;
        NSData *lineData = [line dataUsingEncoding:NSUTF8StringEncoding];
        id decoded = [NSJSONSerialization JSONObjectWithData:lineData options:0 error:nil];
        if ([decoded isKindOfClass:[NSDictionary class]]) {
            [migrated appendData:lineData];
            [migrated appendBytes:"\n" length:1];
            continue;
        }
        changed = YES;
        NSString *timestamp = nil, *message = line;
        if ([line length] >= 24 && [line characterAtIndex:4] == '-' && [line characterAtIndex:10] == ' ') {
            timestamp = [line substringToIndex:23];
            message = [line substringFromIndex:24];
        }
        if (![timestamp length]) timestamp = [RBLogDateFormatter stringFromDate:[NSDate date]];
        NSMutableDictionary *fields = [NSMutableDictionary dictionaryWithDictionary:RBMigratedFieldsForMessage(message)];
        [fields setObject:@YES forKey:@"migrated"];
        [fields setObject:@"legacy-text" forKey:@"format"];
        NSDictionary *record = @{ @"ts": timestamp,
                                  @"level": RBMigratedLevelForMessage(message),
                                  @"component": RBMigratedComponentForMessage(message),
                                  @"message": message,
                                  @"fields": fields };
        NSData *recordLine = RBJSONLine(record);
        if (recordLine) [migrated appendData:recordLine];
    }
    if (!changed) return;
    NSString *temporary = [RBLogFile stringByAppendingString:@".migrating"];
    if ([migrated writeToFile:temporary atomically:YES]) {
        [fm removeItemAtPath:RBLogFile error:nil];
        [fm moveItemAtPath:temporary toPath:RBLogFile error:nil];
    } else {
        [fm removeItemAtPath:temporary error:nil];
    }
}

static void RBOpenLog(void) {
    NSFileManager *fm = [NSFileManager defaultManager];
    [fm createDirectoryAtPath:RBLogDirectory withIntermediateDirectories:YES attributes:nil error:nil];
    RBMigrateLegacyLog(fm);
    NSDictionary *attrs = [fm attributesOfItemAtPath:RBLogFile error:nil];
    RBLogSize = [[attrs objectForKey:NSFileSize] unsignedLongLongValue];
    if (RBLogSize > 1024 * 1024) {
        NSString *old = [RBLogFile stringByAppendingString:@".1"];
        [fm removeItemAtPath:old error:nil];
        [fm moveItemAtPath:RBLogFile toPath:old error:nil];
        RBLogSize = 0;
    }
    if (![fm fileExistsAtPath:RBLogFile]) {
        [fm createFileAtPath:RBLogFile contents:nil attributes:nil];
    }
    RBLogHandle = [NSFileHandle fileHandleForWritingAtPath:RBLogFile];
    [RBLogHandle seekToEndOfFile];
}

static void RBInitializeLog(void) {
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        RBLogQueue = dispatch_queue_create("surf.log", DISPATCH_QUEUE_SERIAL);
        dispatch_async(RBLogQueue, ^{
            RBLogDateFormatter = [[NSDateFormatter alloc] init];
            [RBLogDateFormatter setDateFormat:@"yyyy-MM-dd HH:mm:ss.SSS"];
            RBOpenLog();
        });
    });
}

NSString *RBCurrentLogPath(void) {
    return RBLogFile;
}

void RBClearLog(void) {
    RBInitializeLog();
    dispatch_async(RBLogQueue, ^{
        [RBLogHandle closeFile];
        RBLogHandle = nil;
        [[NSFileManager defaultManager] removeItemAtPath:RBLogFile error:nil];
        RBLogSize = 0;
        RBOpenLog();
    });
}

void RBLogSnapshot(void (^completion)(NSData *data)) {
    RBInitializeLog();
    if (!completion) return;
    dispatch_async(RBLogQueue, ^{
        [RBLogHandle synchronizeFile];
        NSMutableData *snapshot = [NSMutableData data];
        NSData *older = [NSData dataWithContentsOfFile:[RBLogFile stringByAppendingString:@".1"]];
        NSData *current = [NSData dataWithContentsOfFile:RBLogFile];
        if ([older length]) [snapshot appendData:older];
        if ([current length]) [snapshot appendData:current];
        const NSUInteger maximum = 2 * 1024 * 1024;
        if ([snapshot length] > maximum) {
            snapshot = [[snapshot subdataWithRange:NSMakeRange([snapshot length] - maximum, maximum)] mutableCopy];
            const unsigned char *bytes = [snapshot bytes];
            NSUInteger firstLine = 0;
            while (firstLine < [snapshot length] && bytes[firstLine] != '\n') firstLine++;
            if (firstLine < [snapshot length]) [snapshot replaceBytesInRange:NSMakeRange(0, firstLine + 1) withBytes:NULL length:0];
        }
        completion(snapshot);
    });
}

static void RBWriteRecord(NSString *component, NSString *level, NSDictionary *fields, NSString *message) {
    RBInitializeLog();
    dispatch_async(RBLogQueue, ^{
        if (!RBLogHandle) RBOpenLog();
        NSDictionary *record = @{@"ts": [RBLogDateFormatter stringFromDate:[NSDate date]],
                                 @"level": level ?: @"info",
                                 @"component": component ?: @"app",
                                 @"message": message ?: @"",
                                 @"fields": fields ?: @{}};
        NSData *data = RBJSONLine(record);
        if (!data) {
            record = @{@"ts": [RBLogDateFormatter stringFromDate:[NSDate date]],
                       @"level": @"error", @"component": @"logging",
                       @"message": @"Could not encode structured log fields", @"fields": @{}};
            data = RBJSONLine(record);
        }
        if (RBLogSize + [data length] > 1024 * 1024) {
            [RBLogHandle closeFile];
            RBLogHandle = nil;
            NSFileManager *fm = [NSFileManager defaultManager];
            NSString *old = [RBLogFile stringByAppendingString:@".1"];
            [fm removeItemAtPath:old error:nil];
            [fm moveItemAtPath:RBLogFile toPath:old error:nil];
            RBLogSize = 0;
            RBOpenLog();
        }
        [RBLogHandle writeData:data];
        RBLogSize += [data length];
    });
}

void RBLogEvent(NSString *component, NSString *level, NSDictionary *fields, NSString *format, ...) {
    if (!format) return;
    va_list ap;
    va_start(ap, format);
    NSString *message = [[NSString alloc] initWithFormat:format arguments:ap];
    va_end(ap);
    RBWriteRecord([component length] ? [component lowercaseString] : @"app",
                  [level length] ? [level lowercaseString] : @"info",
                  [fields isKindOfClass:[NSDictionary class]] ? fields : @{}, message);
}

static void RBWriteCrashLine(const char *line) {
    mkdir("/var/mobile/Library/Surf", 0755);
    int fd = open("/var/mobile/Library/Surf/surf.log", O_WRONLY | O_CREAT | O_APPEND, 0644);
    if (fd >= 0) {
        write(fd, line, strlen(line));
        close(fd);
    }
}

static void RBSignalHandler(int sig) {
    char buf[256];
    snprintf(buf, sizeof(buf), "{\"ts\":\"\",\"level\":\"error\",\"component\":\"crash\",\"message\":\"Fatal signal received\",\"fields\":{\"signal\":%d}}\n", sig);
    RBWriteCrashLine(buf);
    // Do not raise the signal from inside its handler. On iOS 6 that can
    // nominate an unrelated thread as the crash site and discard the useful
    // faulting stack. Restoring the default action and returning retries the
    // faulting instruction, producing an accurate device crash report.
    signal(sig, SIG_DFL);
}

static void RBExceptionHandler(NSException *exception) {
    RBLogEvent(@"crash", @"error", @{@"name": [exception name] ?: @"",
               @"reason": [exception reason] ?: @"",
               @"stack": [[exception callStackSymbols] componentsJoinedByString:@" | "] ?: @""},
               @"Uncaught exception");
}

void RBInstallCrashHandlers(void) {
    RBInitializeLog();
    NSSetUncaughtExceptionHandler(RBExceptionHandler);
    signal(SIGABRT, RBSignalHandler);
    signal(SIGILL, RBSignalHandler);
    signal(SIGSEGV, RBSignalHandler);
    signal(SIGBUS, RBSignalHandler);
    signal(SIGFPE, RBSignalHandler);
    RBLogEvent(@"logging", @"info", @{@"path": RBLogFile ?: @""}, @"Crash and log handlers installed");
}
