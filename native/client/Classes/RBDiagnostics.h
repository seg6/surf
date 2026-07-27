#import <Foundation/Foundation.h>

// Bounded, payload-free native diagnostics. This service never stores URLs,
// text, credentials, PCM, encoded video, or decoded images.
@interface RBDiagnostics : NSObject
@property(nonatomic, assign, readonly) double lastRTTMS;
@property(nonatomic, assign, readonly, getter=isCapturing) BOOL capturing;
@property(nonatomic, assign, readonly) NSUInteger droppedTraceEvents;
- (NSDictionary *)latencyProbeIfIdle;
- (BOOL)consumeControlMessage:(NSDictionary *)message;
- (void)startCapture;
- (NSArray *)stopCapture;
- (void)traceName:(NSString *)name values:(NSDictionary *)values;
@end
