#import <Foundation/Foundation.h>

@class RBMediaPipeline;
@class RBStreamView;

// One bounded five-second diagnostics sample. Rates are derived from
// cumulative media counters; the service never sits on the decode or display
// path.
@interface RBDiagnosticsReport : NSObject
@property(nonatomic, assign) double presentedFPS;
@property(nonatomic, assign) double imageFPS;
@property(nonatomic, assign) double AURate;
@property(nonatomic, assign) double decodeRate;
@property(nonatomic, assign) NSUInteger dropDelta;
@property(nonatomic, assign) NSUInteger errorDelta;
@property(nonatomic, assign) double recentGapMS;
@property(nonatomic, assign) double dropPercent;
@property(nonatomic, assign) double callbackMS;
@property(nonatomic, assign) double ageSeconds;
@property(nonatomic, assign) BOOL memoryWarning;
- (NSDictionary *)mediaStatsMessage;
@end

// Owns clock synchronization and the rolling diagnostic baselines. UIKit only
// asks for snapshots and decides whether to show them.
@interface RBDiagnostics : NSObject
@property(nonatomic, assign, readonly) double recentGapMS;
@property(nonatomic, assign, readonly) double lastRTTMS;

- (id)initWithMediaPipeline:(RBMediaPipeline *)pipeline streamView:(RBStreamView *)streamView;
- (void)resetConnection;
- (void)noteMemoryWarning;
- (NSDictionary *)clockProbeIfIdle;
- (BOOL)consumeControlMessage:(NSDictionary *)message;
- (RBDiagnosticsReport *)reportAtTime:(CFTimeInterval)now age:(double)age;
- (NSDictionary *)overlaySnapshotForServer:(NSString *)server
                                   version:(NSString *)version
                                     state:(NSString *)state
                                   latency:(double)latency
                                       age:(double)age;
@end
