#import "RBDiagnostics.h"
#import "RBClockSync.h"
#import "RBMediaPipeline.h"
#import "RBStreamView.h"

@implementation RBDiagnosticsReport

- (NSDictionary *)mediaStatsMessage {
    return @{
        @"t": @"media-stats",
        @"fps": @(self.imageFPS),
        @"auRate": @(self.AURate),
        @"callbackMs": @(self.callbackMS),
        @"gapMs": @(self.recentGapMS),
        @"dropPct": @(self.dropPercent),
        @"memoryWarn": @(self.memoryWarning)
    };
}

@end

@interface RBDiagnostics ()
@property(nonatomic, weak) RBMediaPipeline *pipeline;
@property(nonatomic, weak) RBStreamView *streamView;
@property(nonatomic, strong, readwrite) RBClockSync *clockSync;
@property(nonatomic, assign, readwrite) double recentGapMS;
@property(nonatomic, assign) CFTimeInterval lastReportAt;
@property(nonatomic, assign) NSUInteger lastPresentedFrames;
@property(nonatomic, assign) NSUInteger lastVideoAUs;
@property(nonatomic, assign) NSUInteger lastDecodedFrames;
@property(nonatomic, assign) NSUInteger lastDecodeErrors;
@property(nonatomic, assign) NSUInteger lastVideoDrops;
@property(nonatomic, assign) BOOL memoryWarning;
@end

@implementation RBDiagnostics

- (id)initWithMediaPipeline:(RBMediaPipeline *)pipeline streamView:(RBStreamView *)streamView {
    self = [super init];
    if (self) {
        self.pipeline = pipeline;
        self.streamView = streamView;
        self.clockSync = [[RBClockSync alloc] init];
    }
    return self;
}

- (void)resetConnection {
    [self.clockSync reset];
    self.lastReportAt = 0.0;
    self.lastPresentedFrames = self.streamView.presentedFrames;
    self.lastVideoAUs = self.pipeline.videoAUs;
    self.lastDecodedFrames = self.pipeline.decodedFrames;
    self.lastDecodeErrors = self.pipeline.decodeErrors;
    self.lastVideoDrops = self.pipeline.droppedAUs;
    self.recentGapMS = 0.0;
    self.memoryWarning = NO;
}

- (void)noteMemoryWarning { self.memoryWarning = YES; }
- (double)lastRTTMS { return self.clockSync.lastRTTMS; }
- (NSDictionary *)clockProbeIfIdle { return [self.clockSync probeIfIdle]; }
- (BOOL)consumeControlMessage:(NSDictionary *)message {
    return [self.clockSync consumeControlMessage:message];
}

- (RBDiagnosticsReport *)reportAtTime:(CFTimeInterval)now age:(double)age {
    if (self.lastReportAt <= 0.0) {
        self.lastReportAt = now;
        return nil;
    }
    if (now - self.lastReportAt < 5.0) return nil;

    double dt = MAX(0.001, now - self.lastReportAt);
    NSUInteger presented = self.streamView.presentedFrames;
    NSUInteger aus = self.pipeline.videoAUs;
    NSUInteger decoded = self.pipeline.decodedFrames;
    NSUInteger errors = self.pipeline.decodeErrors;
    NSUInteger drops = self.pipeline.droppedAUs;
    NSUInteger auDelta = aus >= self.lastVideoAUs ? aus - self.lastVideoAUs : 0;

    RBDiagnosticsReport *report = [[RBDiagnosticsReport alloc] init];
    report.presentedFPS = presented >= self.lastPresentedFrames ?
        (presented - self.lastPresentedFrames) / dt : 0.0;
    report.imageFPS = self.streamView.uniquePresentationFPS;
    report.AURate = auDelta / dt;
    report.decodeRate = decoded >= self.lastDecodedFrames ?
        (decoded - self.lastDecodedFrames) / dt : 0.0;
    report.dropDelta = drops >= self.lastVideoDrops ? drops - self.lastVideoDrops : drops;
    report.errorDelta = errors >= self.lastDecodeErrors ? errors - self.lastDecodeErrors : errors;
    report.recentGapMS = [self.streamView consumeRecentMaximumPresentationGapMS];
    report.dropPercent = auDelta > 0 ? 100.0 * report.dropDelta / (double)auDelta : 0.0;
    report.callbackMS = self.pipeline.averageCallbackMS;
    report.ageSeconds = age;
    report.memoryWarning = self.memoryWarning;

    self.recentGapMS = report.recentGapMS;
    self.memoryWarning = NO;
    self.lastReportAt = now;
    self.lastPresentedFrames = presented;
    self.lastVideoAUs = aus;
    self.lastDecodedFrames = decoded;
    self.lastDecodeErrors = errors;
    self.lastVideoDrops = drops;
    return report;
}

- (NSDictionary *)overlaySnapshotForServer:(NSString *)server
                                   version:(NSString *)version
                                     state:(NSString *)state
                                   latency:(double)latency
                                       age:(double)age {
    return @{
        @"server": server ?: @"Surf",
        @"version": version ?: @"",
        @"state": state ?: @"idle",
        @"imageFPS": @(self.streamView.uniquePresentationFPS),
        @"aus": @(self.pipeline.videoAUs),
        @"latency": @(latency),
        @"rtt": @(self.clockSync.lastRTTMS),
        @"age": @(age * 1000.0),
        @"maxGap": @(self.recentGapMS),
        @"queue": @(self.pipeline.queuedAUs),
        @"gaps": @(self.pipeline.sequenceGaps),
        @"overwritten": @(self.streamView.overwrittenVideoFrames),
        @"submitMS": @(self.pipeline.averageSubmitMS),
        @"callbackMS": @(self.pipeline.averageCallbackMS),
        @"wrapMS": @(self.pipeline.averageWrapMS),
        @"errors": @(self.pipeline.decodeErrors),
        @"drops": @(self.pipeline.droppedAUs),
        @"audioQueue": @(self.pipeline.audioQueuedBuffers),
        @"audioDrops": @(self.pipeline.audioDroppedPCM),
        @"audioUnderruns": @(self.pipeline.audioUnderruns),
        @"audioRestarts": @(self.pipeline.audioRestartCount)
    };
}

@end
