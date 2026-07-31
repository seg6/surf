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

static void RBOpenLog(void) {
    NSFileManager *fm = [NSFileManager defaultManager];
    [fm createDirectoryAtPath:RBLogDirectory withIntermediateDirectories:YES attributes:nil error:nil];
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

void RBLog(NSString *format, ...) {
    if (!format) return;
    va_list ap;
    va_start(ap, format);
    NSString *message = [[NSString alloc] initWithFormat:format arguments:ap];
    va_end(ap);

    // NSLog is intentionally omitted in release operation: it is synchronous
    // on old iOS and made media error bursts contend with touch/display work.
    RBInitializeLog();
    dispatch_async(RBLogQueue, ^{
        if (!RBLogHandle) RBOpenLog();
        NSString *line = [NSString stringWithFormat:@"%@ %@\n",
                          [RBLogDateFormatter stringFromDate:[NSDate date]], message];
        NSData *data = [line dataUsingEncoding:NSUTF8StringEncoding];
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

static void RBWriteCrashLine(const char *line) {
    mkdir("/var/mobile/Library/Surf", 0755);
    int fd = open("/var/mobile/Library/Surf/surf.log", O_WRONLY | O_CREAT | O_APPEND, 0644);
    if (fd >= 0) {
        write(fd, line, strlen(line));
        close(fd);
    }
}

static void RBSignalHandler(int sig) {
    char buf[96];
    snprintf(buf, sizeof(buf), "fatal signal %d\n", sig);
    RBWriteCrashLine(buf);
    // Do not raise the signal from inside its handler. On iOS 6 that can
    // nominate an unrelated thread as the crash site and discard the useful
    // faulting stack. Restoring the default action and returning retries the
    // faulting instruction, producing an accurate device crash report.
    signal(sig, SIG_DFL);
}

static void RBExceptionHandler(NSException *exception) {
    RBLog(@"uncaught exception: %@ %@", [exception name], [exception reason]);
    RBLog(@"stack: %@", [[exception callStackSymbols] componentsJoinedByString:@" | "]);
}

void RBInstallCrashHandlers(void) {
    RBInitializeLog();
    NSSetUncaughtExceptionHandler(RBExceptionHandler);
    signal(SIGABRT, RBSignalHandler);
    signal(SIGILL, RBSignalHandler);
    signal(SIGSEGV, RBSignalHandler);
    signal(SIGBUS, RBSignalHandler);
    signal(SIGFPE, RBSignalHandler);
    RBLog(@"crash/log handlers installed at %@", RBLogFile);
}
