#import <Foundation/Foundation.h>

void RBLogEvent(NSString *component, NSString *level, NSDictionary *fields,
                NSString *format, ...) NS_FORMAT_FUNCTION(4, 5);
NSString *RBCurrentLogPath(void);
void RBClearLog(void);
void RBInstallCrashHandlers(void);
