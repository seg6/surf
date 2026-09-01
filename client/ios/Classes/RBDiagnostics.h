#import <Foundation/Foundation.h>

@class RBMediaPipeline;
@class RBStreamView;

typedef enum {
    RBDiagnosticsHealthOffline,
    RBDiagnosticsHealthSmooth,
    RBDiagnosticsHealthDelayed,
    RBDiagnosticsHealthUnstable
} RBDiagnosticsHealth;

// Immutable UI snapshot. Health is classified here rather than in the view so
// every diagnostics surface gives the same answer for the same pipeline state.
@interface RBDiagnosticsSnapshot : NSObject
@property(nonatomic, readonly, copy) NSString *server;
@property(nonatomic, readonly, copy) NSString *version;
@property(nonatomic, readonly, copy) NSString *compatibilityVersion;
@property(nonatomic, readonly, copy) NSString *streamState;
@property(nonatomic, readonly, copy) NSString *state;
@property(nonatomic, readonly, assign) RBDiagnosticsHealth health;
@property(nonatomic, readonly, copy) NSString *healthLabel;
@property(nonatomic, readonly, copy) NSString *healthReason;
@property(nonatomic, readonly, assign) double imageFPS;
@property(nonatomic, readonly, assign) double AURate;
@property(nonatomic, readonly, copy) NSString *rendererMode;
@property(nonatomic, readonly, assign) double rendererMS;
@property(nonatomic, readonly, assign) unsigned long long rendererBackpressure;
@property(nonatomic, readonly, assign) unsigned long long rendererRecoveries;
@property(nonatomic, readonly, assign) unsigned long long rendererFailures;
@property(nonatomic, readonly, assign) double latencyMS;
@property(nonatomic, readonly, assign) double RTTMS;
@property(nonatomic, readonly, assign) double ageMS;
@property(nonatomic, readonly, assign) double maxGapMS;
@property(nonatomic, readonly, assign) int queuedAUs;
@property(nonatomic, readonly, assign) unsigned long long sequenceGaps;
@property(nonatomic, readonly, assign) unsigned long long overwrittenFrames;
@property(nonatomic, readonly, assign) double submitMS;
@property(nonatomic, readonly, assign) double callbackMS;
@property(nonatomic, readonly, assign) double handoffMS;
@property(nonatomic, readonly, assign) unsigned long long decodeErrors;
@property(nonatomic, readonly, assign) unsigned long long droppedAUs;
@property(nonatomic, readonly, assign) int audioQueuedBuffers;
@property(nonatomic, readonly, assign) unsigned long long audioDroppedPCM;
@property(nonatomic, readonly, assign) unsigned long long audioUnderruns;
@property(nonatomic, readonly, assign) unsigned long long audioRestarts;
@end

// One bounded two-second diagnostics sample. Rates are derived from
// cumulative media counters; the service never sits on the decode or display
// path.
@interface RBDiagnosticsReport : NSObject
@property(nonatomic, assign) double presentedFPS;
@property(nonatomic, assign) double imageFPS;
@property(nonatomic, assign) double AURate;
@property(nonatomic, assign) double decodeRate;
@property(nonatomic, copy) NSString *rendererMode;
@property(nonatomic, assign) double rendererRate;
@property(nonatomic, assign) double rendererMS;
@property(nonatomic, assign) NSUInteger rendererBackpressure;
@property(nonatomic, assign) NSUInteger rendererRecoveries;
@property(nonatomic, assign) NSUInteger rendererFailures;
@property(nonatomic, assign) NSUInteger dropDelta;
@property(nonatomic, assign) NSUInteger errorDelta;
@property(nonatomic, assign) double recentGapMS;
@property(nonatomic, assign) double dropPercent;
@property(nonatomic, assign) double callbackMS;
@property(nonatomic, assign) double ageSeconds;
@property(nonatomic, assign) double windowMS;
@property(nonatomic, assign) int queueDepth;
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
- (void)resetVideoWindow;
- (void)noteMemoryWarning;
- (NSDictionary *)clockProbeIfIdle;
- (BOOL)consumeControlMessage:(NSDictionary *)message;
- (RBDiagnosticsReport *)reportAtTime:(CFTimeInterval)now age:(double)age;
- (RBDiagnosticsSnapshot *)overlaySnapshotForServer:(NSString *)server
                                            version:(NSString *)version
                                      compatibility:(NSString *)compatibilityVersion
                                             stream:(NSString *)streamState
                                              state:(NSString *)state
                                            latency:(double)latency
                                                age:(double)age;
@end
