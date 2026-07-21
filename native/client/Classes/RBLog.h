#import <Foundation/Foundation.h>

void RBLog(NSString *format, ...) NS_FORMAT_FUNCTION(1, 2);
NSString *RBCurrentLogPath(void);
void RBInstallCrashHandlers(void);
