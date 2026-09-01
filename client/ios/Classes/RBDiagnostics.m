#import "RBDiagnostics.h"
#import "RBClockSync.h"
#import "RBMediaPipeline.h"
#import "RBStreamView.h"

#import <QuartzCore/QuartzCore.h>

@interface RBDiagnosticsSnapshot ()
@property(nonatomic, readwrite, copy) NSString *server;
@property(nonatomic, readwrite, copy) NSString *version;
@property(nonatomic, readwrite, copy) NSString *compatibilityVersion;
@property(nonatomic, readwrite, copy) NSString *streamState;
@property(nonatomic, readwrite, copy) NSString *state;
@property(nonatomic, readwrite, assign) RBDiagnosticsHealth health;
@property(nonatomic, readwrite, copy) NSString *healthLabel;
@property(nonatomic, readwrite, copy) NSString *healthReason;
@property(nonatomic, readwrite, assign) double imageFPS;
@property(nonatomic, readwrite, assign) double AURate;
@property(nonatomic, readwrite, copy) NSString *rendererMode;
@property(nonatomic, readwrite, assign) double rendererMS;
@property(nonatomic, readwrite, assign) unsigned long long rendererBackpressure;
@property(nonatomic, readwrite, assign) unsigned long long rendererRecoveries;
@property(nonatomic, readwrite, assign) unsigned long long rendererFailures;
@property(nonatomic, readwrite, assign) double latencyMS;
@property(nonatomic, readwrite, assign) double RTTMS;
@property(nonatomic, readwrite, assign) double ageMS;
@property(nonatomic, readwrite, assign) double maxGapMS;
@property(nonatomic, readwrite, assign) int queuedAUs;
@property(nonatomic, readwrite, assign) unsigned long long sequenceGaps;
@property(nonatomic, readwrite, assign) unsigned long long overwrittenFrames;
@property(nonatomic, readwrite, assign) double submitMS;
@property(nonatomic, readwrite, assign) double callbackMS;
@property(nonatomic, readwrite, assign) double handoffMS;
@property(nonatomic, readwrite, assign) unsigned long long decodeErrors;
@property(nonatomic, readwrite, assign) unsigned long long droppedAUs;
@property(nonatomic, readwrite, assign) int audioQueuedBuffers;
@property(nonatomic, readwrite, assign) unsigned long long audioDroppedPCM;
@property(nonatomic, readwrite, assign) unsigned long long audioUnderruns;
@property(nonatomic, readwrite, assign) unsigned long long audioRestarts;
@end

@implementation RBDiagnosticsSnapshot
@end

@implementation RBDiagnosticsReport

- (NSDictionary *)mediaStatsMessage {
    return @{
        @"t": @"media-stats",
        @"fps": @(self.imageFPS),
        @"presentedFps": @(self.presentedFPS),
        @"decodeFps": @(self.decodeRate),
        @"auRate": @(self.AURate),
        @"renderer": self.rendererMode ?: @"unknown",
        @"rendererFps": @(self.rendererRate),
        @"rendererMs": @(self.rendererMS),
        @"rendererBackpressure": @(self.rendererBackpressure),
        @"rendererRecoveries": @(self.rendererRecoveries),
        @"rendererFailures": @(self.rendererFailures),
        @"callbackMs": @(self.callbackMS),
        @"gapMs": @(self.recentGapMS),
        @"frameAgeMs": @(self.ageSeconds * 1000.0),
        @"windowMs": @(self.windowMS),
        @"dropPct": @(self.dropPercent),
        @"queue": @(self.queueDepth),
        @"decodeErrors": @(self.errorDelta),
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
@property(nonatomic, assign) NSUInteger lastRendererFrames;
@property(nonatomic, assign) NSUInteger lastRendererBackpressure;
@property(nonatomic, assign) NSUInteger lastRendererRecoveries;
@property(nonatomic, assign) NSUInteger lastRendererFailures;
@property(nonatomic, assign) BOOL memoryWarning;
@property(nonatomic, assign) CFTimeInterval lastOverlayAt;
@property(nonatomic, assign) NSUInteger lastOverlayAUs;
@property(nonatomic, assign) NSUInteger lastOverlayGaps;
@property(nonatomic, assign) NSUInteger lastOverlayErrors;
@property(nonatomic, assign) NSUInteger lastOverlayDrops;
@property(nonatomic, assign) NSUInteger lastOverlayRendererFrames;
@property(nonatomic, assign) NSUInteger lastOverlayRendererBackpressure;
@property(nonatomic, assign) NSUInteger lastOverlayRendererRecoveries;
@property(nonatomic, assign) NSUInteger lastOverlayRendererFailures;
@property(nonatomic, assign) NSUInteger lastOverlayAudioUnderruns;
@property(nonatomic, assign) NSUInteger lastOverlayAudioRestarts;
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
    self.lastRendererFrames = self.pipeline.rendererFrames;
    self.lastRendererBackpressure = self.pipeline.rendererBackpressureEvents;
    self.lastRendererRecoveries = self.pipeline.rendererRecoveries;
    self.lastRendererFailures = self.pipeline.rendererFailures;
    self.recentGapMS = 0.0;
    self.memoryWarning = NO;
    self.lastOverlayAt = 0.0;
    self.lastOverlayAUs = self.pipeline.videoAUs;
    self.lastOverlayGaps = self.pipeline.sequenceGaps;
    self.lastOverlayErrors = self.pipeline.decodeErrors;
    self.lastOverlayDrops = self.pipeline.droppedAUs;
    self.lastOverlayRendererFrames = self.pipeline.rendererFrames;
    self.lastOverlayRendererBackpressure = self.pipeline.rendererBackpressureEvents;
    self.lastOverlayRendererRecoveries = self.pipeline.rendererRecoveries;
    self.lastOverlayRendererFailures = self.pipeline.rendererFailures;
    self.lastOverlayAudioUnderruns = self.pipeline.audioUnderruns;
    self.lastOverlayAudioRestarts = self.pipeline.audioRestartCount;
}

- (void)resetVideoWindow {
    // VideoToolbox and the media counters restart for every encoder
    // generation. Rebase on the first watchdog tick after that asynchronous
    // reset, then wait for one complete two-second generation-local window.
    // Comparing new counters with the previous generation made a healthy
    // 60 FPS restart look like a decoder-throughput collapse.
    self.lastReportAt = 0.0;
    self.recentGapMS = 0.0;
    [self.streamView consumeRecentMaximumPresentationGapMS];
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
        self.lastPresentedFrames = self.streamView.presentedFrames;
        self.lastVideoAUs = self.pipeline.videoAUs;
        self.lastDecodedFrames = self.pipeline.decodedFrames;
        self.lastDecodeErrors = self.pipeline.decodeErrors;
        self.lastVideoDrops = self.pipeline.droppedAUs;
        self.lastRendererFrames = self.pipeline.rendererFrames;
        self.lastRendererBackpressure = self.pipeline.rendererBackpressureEvents;
        self.lastRendererRecoveries = self.pipeline.rendererRecoveries;
        self.lastRendererFailures = self.pipeline.rendererFailures;
        [self.streamView consumeRecentMaximumPresentationGapMS];
        return nil;
    }
    if (now - self.lastReportAt < 2.0) return nil;

    double dt = MAX(0.001, now - self.lastReportAt);
    NSUInteger presented = self.streamView.presentedFrames;
    NSUInteger aus = self.pipeline.videoAUs;
    NSUInteger decoded = self.pipeline.decodedFrames;
    NSUInteger errors = self.pipeline.decodeErrors;
    NSUInteger drops = self.pipeline.droppedAUs;
    NSUInteger rendererFrames = self.pipeline.rendererFrames;
    NSUInteger rendererBackpressure = self.pipeline.rendererBackpressureEvents;
    NSUInteger rendererRecoveries = self.pipeline.rendererRecoveries;
    NSUInteger rendererFailures = self.pipeline.rendererFailures;
    NSUInteger auDelta = aus >= self.lastVideoAUs ? aus - self.lastVideoAUs : 0;
    NSUInteger rendererDelta = rendererFrames >= self.lastRendererFrames ?
        rendererFrames - self.lastRendererFrames : rendererFrames;

    RBDiagnosticsReport *report = [[RBDiagnosticsReport alloc] init];
    report.presentedFPS = presented >= self.lastPresentedFrames ?
        (presented - self.lastPresentedFrames) / dt : 0.0;
    report.imageFPS = self.streamView.uniquePresentationFPS;
    report.AURate = auDelta / dt;
    report.decodeRate = decoded >= self.lastDecodedFrames ?
        (decoded - self.lastDecodedFrames) / dt : 0.0;
    report.rendererMode = self.pipeline.rendererMode;
    report.rendererRate = rendererDelta / dt;
    report.rendererMS = self.pipeline.averageRendererMS;
    report.rendererBackpressure = rendererBackpressure >= self.lastRendererBackpressure ?
        rendererBackpressure - self.lastRendererBackpressure : rendererBackpressure;
    report.rendererRecoveries = rendererRecoveries >= self.lastRendererRecoveries ?
        rendererRecoveries - self.lastRendererRecoveries : rendererRecoveries;
    report.rendererFailures = rendererFailures >= self.lastRendererFailures ?
        rendererFailures - self.lastRendererFailures : rendererFailures;
    if ([report.rendererMode isEqualToString:@"system"]) {
        // AVSampleBufferDisplayLayer does not expose presentation callbacks.
        // Its accepted-frame counter is the exact observable throughput; do
        // not substitute the deliberately coalesced UIKit metadata handoff.
        report.presentedFPS = report.rendererRate;
        report.imageFPS = report.rendererRate;
        report.decodeRate = report.rendererRate;
    }
    report.dropDelta = drops >= self.lastVideoDrops ? drops - self.lastVideoDrops : drops;
    report.errorDelta = errors >= self.lastDecodeErrors ? errors - self.lastDecodeErrors : errors;
    report.recentGapMS = [self.streamView consumeRecentMaximumPresentationGapMS];
    report.dropPercent = auDelta > 0 ? 100.0 * report.dropDelta / (double)auDelta : 0.0;
    report.callbackMS = self.pipeline.averageCallbackMS;
    report.ageSeconds = age;
    report.windowMS = dt * 1000.0;
    report.queueDepth = self.pipeline.queuedAUs;
    report.memoryWarning = self.memoryWarning;

    self.recentGapMS = report.recentGapMS;
    self.memoryWarning = NO;
    self.lastReportAt = now;
    self.lastPresentedFrames = presented;
    self.lastVideoAUs = aus;
    self.lastDecodedFrames = decoded;
    self.lastDecodeErrors = errors;
    self.lastVideoDrops = drops;
    self.lastRendererFrames = rendererFrames;
    self.lastRendererBackpressure = rendererBackpressure;
    self.lastRendererRecoveries = rendererRecoveries;
    self.lastRendererFailures = rendererFailures;
    return report;
}

static NSUInteger RBMetricDelta(NSUInteger current, NSUInteger previous) {
    return current >= previous ? current - previous : current;
}

- (RBDiagnosticsSnapshot *)overlaySnapshotForServer:(NSString *)server
                                            version:(NSString *)version
                                      compatibility:(NSString *)compatibilityVersion
                                             stream:(NSString *)streamState
                                              state:(NSString *)state
                                            latency:(double)latency
                                                age:(double)age {
    CFTimeInterval now = CACurrentMediaTime();
    NSUInteger aus = self.pipeline.videoAUs;
    NSUInteger gaps = self.pipeline.sequenceGaps;
    NSUInteger errors = self.pipeline.decodeErrors;
    NSUInteger drops = self.pipeline.droppedAUs;
    NSUInteger rendererFrames = self.pipeline.rendererFrames;
    NSUInteger rendererBackpressure = self.pipeline.rendererBackpressureEvents;
    NSUInteger rendererRecoveries = self.pipeline.rendererRecoveries;
    NSUInteger rendererFailures = self.pipeline.rendererFailures;
    NSUInteger underruns = self.pipeline.audioUnderruns;
    NSUInteger restarts = self.pipeline.audioRestartCount;
    double dt = self.lastOverlayAt > 0.0 ? MAX(0.001, now - self.lastOverlayAt) : 0.0;
    NSUInteger auDelta = RBMetricDelta(aus, self.lastOverlayAUs);
    NSUInteger gapDelta = RBMetricDelta(gaps, self.lastOverlayGaps);
    NSUInteger errorDelta = RBMetricDelta(errors, self.lastOverlayErrors);
    NSUInteger dropDelta = RBMetricDelta(drops, self.lastOverlayDrops);
    NSUInteger rendererFrameDelta = RBMetricDelta(rendererFrames, self.lastOverlayRendererFrames);
    NSUInteger rendererBackpressureDelta = RBMetricDelta(rendererBackpressure,
        self.lastOverlayRendererBackpressure);
    NSUInteger rendererRecoveryDelta = RBMetricDelta(rendererRecoveries,
        self.lastOverlayRendererRecoveries);
    NSUInteger rendererFailureDelta = RBMetricDelta(rendererFailures,
        self.lastOverlayRendererFailures);
    NSUInteger underrunDelta = RBMetricDelta(underruns, self.lastOverlayAudioUnderruns);
    NSUInteger restartDelta = RBMetricDelta(restarts, self.lastOverlayAudioRestarts);

    RBDiagnosticsSnapshot *snapshot = [[RBDiagnosticsSnapshot alloc] init];
    snapshot.server = server ?: @"Surf";
    snapshot.version = version ?: @"";
    snapshot.compatibilityVersion = compatibilityVersion ?: @"";
    snapshot.streamState = streamState ?: @"";
    snapshot.state = state ?: @"idle";
    snapshot.rendererMode = self.pipeline.rendererMode;
    snapshot.rendererMS = self.pipeline.averageRendererMS;
    snapshot.rendererBackpressure = rendererBackpressure;
    snapshot.rendererRecoveries = rendererRecoveries;
    snapshot.rendererFailures = rendererFailures;
    snapshot.imageFPS = [snapshot.rendererMode isEqualToString:@"system"] && dt > 0.0 ?
        rendererFrameDelta / dt : self.streamView.uniquePresentationFPS;
    snapshot.AURate = dt > 0.0 ? auDelta / dt : 0.0;
    snapshot.latencyMS = latency;
    snapshot.RTTMS = self.clockSync.lastRTTMS;
    snapshot.ageMS = age * 1000.0;
    snapshot.maxGapMS = self.recentGapMS;
    snapshot.queuedAUs = self.pipeline.queuedAUs;
    snapshot.sequenceGaps = gaps;
    snapshot.overwrittenFrames = self.streamView.overwrittenVideoFrames;
    snapshot.submitMS = self.pipeline.averageSubmitMS;
    snapshot.callbackMS = self.pipeline.averageCallbackMS;
    snapshot.handoffMS = self.pipeline.averageHandoffMS;
    snapshot.decodeErrors = errors;
    snapshot.droppedAUs = drops;
    snapshot.audioQueuedBuffers = self.pipeline.audioQueuedBuffers;
    snapshot.audioDroppedPCM = self.pipeline.audioDroppedPCM;
    snapshot.audioUnderruns = underruns;
    snapshot.audioRestarts = restarts;

    if (![snapshot.state isEqualToString:@"open"]) {
        snapshot.health = RBDiagnosticsHealthOffline;
        snapshot.healthLabel = @"Offline";
        snapshot.healthReason = [snapshot.state isEqualToString:@"connecting"] ?
            @"Connecting to the server" : @"No active server connection";
    } else if (rendererFailureDelta > 0 || rendererRecoveryDelta > 0 ||
               errorDelta > 0 || dropDelta >= 3 || snapshot.queuedAUs >= 3 ||
               snapshot.RTTMS >= 250.0 || snapshot.latencyMS >= 350.0 ||
               (![snapshot.rendererMode isEqualToString:@"system"] && snapshot.callbackMS >= 25.0) ||
               underrunDelta >= 2) {
        snapshot.health = RBDiagnosticsHealthUnstable;
        snapshot.healthLabel = @"Unstable";
        if (rendererFailureDelta > 0) snapshot.healthReason = @"The video renderer failed";
        else if (rendererRecoveryDelta > 0) snapshot.healthReason = @"The video renderer restarted its stream";
        else if (errorDelta > 0) snapshot.healthReason = @"The decoder reported a new error";
        else if (snapshot.queuedAUs >= 3) snapshot.healthReason = @"Video is backing up";
        else if (snapshot.RTTMS >= 250.0) snapshot.healthReason = @"Network round trip is high";
        else if (snapshot.latencyMS >= 350.0) snapshot.healthReason = @"Input response is high";
        else if (snapshot.callbackMS >= 25.0) snapshot.healthReason = @"Video decode is taking too long";
        else if (underrunDelta >= 2) snapshot.healthReason = @"Audio is repeatedly underrunning";
        else snapshot.healthReason = @"Video frames were dropped";
    } else if (rendererBackpressureDelta > 0 || dropDelta > 0 || gapDelta > 0 ||
               underrunDelta > 0 || restartDelta > 0 ||
               snapshot.queuedAUs >= 2 || snapshot.RTTMS >= 120.0 ||
               snapshot.latencyMS >= 200.0 ||
               (![snapshot.rendererMode isEqualToString:@"system"] && snapshot.callbackMS >= 12.0) ||
               snapshot.maxGapMS >= 150.0) {
        snapshot.health = RBDiagnosticsHealthDelayed;
        snapshot.healthLabel = @"Delayed";
        if (snapshot.RTTMS >= 120.0) snapshot.healthReason = @"Network response is slower than usual";
        else if (snapshot.latencyMS >= 200.0) snapshot.healthReason = @"Input is reaching the screen slowly";
        else if (rendererBackpressureDelta > 0) snapshot.healthReason = @"The video renderer briefly applied backpressure";
        else if (snapshot.queuedAUs >= 2) snapshot.healthReason = @"Video has a short queue";
        else if (snapshot.callbackMS >= 12.0) snapshot.healthReason = @"Decode time is elevated";
        else if (underrunDelta > 0 || restartDelta > 0) snapshot.healthReason = @"Audio briefly recovered";
        else snapshot.healthReason = @"The stream had a recent interruption";
    } else {
        snapshot.health = RBDiagnosticsHealthSmooth;
        snapshot.healthLabel = @"Smooth";
        snapshot.healthReason = @"Connection and media pipelines look healthy";
    }

    self.lastOverlayAt = now;
    self.lastOverlayAUs = aus;
    self.lastOverlayGaps = gaps;
    self.lastOverlayErrors = errors;
    self.lastOverlayDrops = drops;
    self.lastOverlayRendererFrames = rendererFrames;
    self.lastOverlayRendererBackpressure = rendererBackpressure;
    self.lastOverlayRendererRecoveries = rendererRecoveries;
    self.lastOverlayRendererFailures = rendererFailures;
    self.lastOverlayAudioUnderruns = underruns;
    self.lastOverlayAudioRestarts = restarts;
    return snapshot;
}

@end
