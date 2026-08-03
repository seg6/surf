#import <Foundation/Foundation.h>

@interface RBClockSync : NSObject
@property(nonatomic, assign, readonly) double lastRTTMS;
- (void)reset;
- (NSDictionary *)probeIfIdle;
- (BOOL)consumeControlMessage:(NSDictionary *)message;
@end
