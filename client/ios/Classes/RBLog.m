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
static void (^RBLogRecordHandler)(NSDictionary *record);

static NSData *RBJSONLine(NSDictionary *record) {
    NSData *json = [NSJSONSerialization dataWithJSONObject:record options:0 error:nil];
    if (!json) return nil;
    NSMutableData *line = [json mutableCopy];
    [line appendBytes:"\n" length:1];
    return line;
}

static BOOL RBStructuredLogData(NSData *data) {
    if (![data length]) return YES;
    NSString *text = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
    if (!text) return NO;
    for (NSString *line in [text componentsSeparatedByCharactersInSet:[NSCharacterSet newlineCharacterSet]]) {
        if (![line length]) continue;
        NSData *lineData = [line dataUsingEncoding:NSUTF8StringEncoding];
        NSDictionary *record = [NSJSONSerialization JSONObjectWithData:lineData options:0 error:nil];
        if (![record isKindOfClass:[NSDictionary class]] || ![[record objectForKey:@"ts"] isKindOfClass:[NSString class]] ||
            ![[record objectForKey:@"level"] isKindOfClass:[NSString class]] ||
            ![[record objectForKey:@"component"] isKindOfClass:[NSString class]] ||
            ![[record objectForKey:@"message"] isKindOfClass:[NSString class]] ||
            ![[record objectForKey:@"fields"] isKindOfClass:[NSDictionary class]]) return NO;
    }
    return YES;
}

static void RBOpenLog(void) {
    NSFileManager *fm = [NSFileManager defaultManager];
    [fm createDirectoryAtPath:RBLogDirectory withIntermediateDirectories:YES attributes:nil error:nil];
    for (NSString *path in @[[RBLogFile stringByAppendingString:@".1"], RBLogFile]) {
        NSData *existing = [NSData dataWithContentsOfFile:path];
        if ([existing length] && !RBStructuredLogData(existing)) [fm removeItemAtPath:path error:nil];
    }
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
            [RBLogDateFormatter setLocale:[[NSLocale alloc] initWithLocaleIdentifier:@"en_US_POSIX"]];
            [RBLogDateFormatter setTimeZone:[NSTimeZone timeZoneForSecondsFromGMT:0]];
            [RBLogDateFormatter setDateFormat:@"yyyy-MM-dd'T'HH:mm:ss.SSS'Z'"];
            RBOpenLog();
        });
    });
}

NSString *RBCurrentLogPath(void) {
    return RBLogFile;
}

void RBClearLog(void) {
    RBClearLogWithCompletion(nil);
}

void RBClearLogWithCompletion(void (^completion)(void)) {
    RBInitializeLog();
    dispatch_async(RBLogQueue, ^{
        [RBLogHandle closeFile];
        RBLogHandle = nil;
        NSFileManager *fm = [NSFileManager defaultManager];
        [fm removeItemAtPath:RBLogFile error:nil];
        [fm removeItemAtPath:[RBLogFile stringByAppendingString:@".1"] error:nil];
        RBLogSize = 0;
        RBOpenLog();
        if (completion) dispatch_async(dispatch_get_main_queue(), completion);
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

void RBSetLogRecordHandler(void (^handler)(NSDictionary *record)) {
    RBInitializeLog();
    dispatch_async(RBLogQueue, ^{
        RBLogRecordHandler = [handler copy];
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
        if (RBLogRecordHandler) RBLogRecordHandler(record);
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
