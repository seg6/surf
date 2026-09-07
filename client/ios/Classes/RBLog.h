#import <Foundation/Foundation.h>

void RBLogEvent(NSString *component, NSString *level, NSDictionary *fields,
                NSString *format, ...) NS_FORMAT_FUNCTION(4, 5);
NSString *RBCurrentLogPath(void);
void RBClearLog(void);
void RBClearLogWithCompletion(void (^completion)(void));
// Flushes the serial log writer and returns the bounded predecessor plus
// current NDJSON snapshot without blocking the caller.
void RBLogSnapshot(void (^completion)(NSData *data));
// Streams newly written structured records over the current authenticated
// session. Passing nil stops live mirroring; the file remains the durable
// bounded source used for reconnect snapshots.
void RBSetLogRecordHandler(void (^handler)(NSDictionary *record));
void RBInstallCrashHandlers(void);
