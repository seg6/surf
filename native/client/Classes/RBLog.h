#import <Foundation/Foundation.h>

void RBLogEvent(NSString *component, NSString *level, NSDictionary *fields,
                NSString *format, ...) NS_FORMAT_FUNCTION(4, 5);
NSString *RBCurrentLogPath(void);
void RBClearLog(void);
// Flushes the serial log writer and returns the bounded predecessor plus
// current NDJSON snapshot without blocking the caller.
void RBLogSnapshot(void (^completion)(NSData *data));
void RBInstallCrashHandlers(void);
