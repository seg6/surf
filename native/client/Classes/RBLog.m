#import "RBLog.h"
#import "RBConfig.h"

#include <fcntl.h>
#include <signal.h>
#include <stdarg.h>
#include <stdio.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>

static void RBEnsureLogDirectory(void) {
    NSFileManager *fm = [NSFileManager defaultManager];
    [fm createDirectoryAtPath:RBLogDirectory withIntermediateDirectories:YES attributes:nil error:nil];

    NSDictionary *attrs = [fm attributesOfItemAtPath:RBLogFile error:nil];
    unsigned long long size = [[attrs objectForKey:NSFileSize] unsignedLongLongValue];
    if (size > 1024 * 1024) {
        NSString *old = [RBLogFile stringByAppendingString:@".1"];
        [fm removeItemAtPath:old error:nil];
        [fm moveItemAtPath:RBLogFile toPath:old error:nil];
    }
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

    NSDateFormatter *df = [[NSDateFormatter alloc] init];
    [df setDateFormat:@"yyyy-MM-dd HH:mm:ss.SSS"];
    NSString *line = [NSString stringWithFormat:@"%@ %@\n", [df stringFromDate:[NSDate date]], message];

    @synchronized([NSFileManager class]) {
        RBEnsureLogDirectory();
        NSFileHandle *fh = [NSFileHandle fileHandleForWritingAtPath:RBLogFile];
        if (!fh) {
            [[NSData data] writeToFile:RBLogFile atomically:NO];
            fh = [NSFileHandle fileHandleForWritingAtPath:RBLogFile];
        }
        if (fh) {
            [fh seekToEndOfFile];
            [fh writeData:[line dataUsingEncoding:NSUTF8StringEncoding]];
            [fh closeFile];
        }
    }

    NSLog(@"%@", message);
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
    signal(sig, SIG_DFL);
    raise(sig);
}

static void RBExceptionHandler(NSException *exception) {
    RBLog(@"uncaught exception: %@ %@", [exception name], [exception reason]);
    RBLog(@"stack: %@", [[exception callStackSymbols] componentsJoinedByString:@" | "]);
}

void RBInstallCrashHandlers(void) {
    NSSetUncaughtExceptionHandler(&RBExceptionHandler);
    signal(SIGABRT, RBSignalHandler);
    signal(SIGILL, RBSignalHandler);
    signal(SIGSEGV, RBSignalHandler);
    signal(SIGBUS, RBSignalHandler);
    signal(SIGFPE, RBSignalHandler);
    RBLog(@"crash/log handlers installed at %@", RBLogFile);
}
