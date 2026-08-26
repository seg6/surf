#import "RBDiagnostics.h"
#import "RBClockSync.h"
#import "RBMediaPipeline.h"
#import "RBStreamView.h"

#import <QuartzCore/QuartzCore.h>

@interface RBDiagnosticsSnapshot ()
@property(nonatomic, readwrite, copy) NSString *server;
@property(nonatomic, readwrite, copy) NSString *version;
@property(nonatomic, readwrite, copy) NSString *protocolVersion;
@property(nonatomic, readwrite, copy) NSString *streamState;
@property(nonatomic, readwrite, copy) NSString *state;
@property(nonatomic, readwrite, assign) RBDiagnosticsHealth health;
@property(nonatomic, readwrite, copy) NSString *healthLabel;
@property(nonatomic, readwrite, copy) NSString *healthReason;
@property(nonatomic, readwrite, assign) double imageFPS;
@property(nonatomic, readwrite, assign) double AURate;
@property(nonatomic, readwrite, assign) double latencyMS;
@property(nonatomic, readwrite, assign) double RTTMS;
@property(nonatomic, readwrite, assign) double ageMS;
@property(nonatomic, readwrite, assign) double maxGapMS;
@property(nonatomic, readwrite, assign) int queuedAUs;
@property(nonatomic, readwrite, assign) unsigned long long sequenceGaps;
@property(nonatomic, readwrite, assign) unsigned long long overwrittenFrames;
@property(nonatomic, readwrite, assign) double submitMS;
@property(nonatomic, readwrite, assign) double callbackMS;
@property(nonatomic, readwrite, assign) double wrapMS;
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
@property(nonatomic, assign) CFTimeInterval lastOverlayAt;
@property(nonatomic, assign) NSUInteger lastOverlayAUs;
@property(nonatomic, assign) NSUInteger lastOverlayGaps;
@property(nonatomic, assign) NSUInteger lastOverlayErrors;
@property(nonatomic, assign) NSUInteger lastOverlayDrops;
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
    self.recentGapMS = 0.0;
    self.memoryWarning = NO;
    self.lastOverlayAt = 0.0;
    self.lastOverlayAUs = self.pipeline.videoAUs;
    self.lastOverlayGaps = self.pipeline.sequenceGaps;
    self.lastOverlayErrors = self.pipeline.decodeErrors;
    self.lastOverlayDrops = self.pipeline.droppedAUs;
    self.lastOverlayAudioUnderruns = self.pipeline.audioUnderruns;
    self.lastOverlayAudioRestarts = self.pipeline.audioRestartCount;
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

static NSUInteger RBMetricDelta(NSUInteger current, NSUInteger previous) {
    return current >= previous ? current - previous : current;
}

- (RBDiagnosticsSnapshot *)overlaySnapshotForServer:(NSString *)server
                                            version:(NSString *)version
                                           protocol:(NSString *)protocolVersion
                                             stream:(NSString *)streamState
                                              state:(NSString *)state
                                            latency:(double)latency
                                                age:(double)age {
    CFTimeInterval now = CACurrentMediaTime();
    NSUInteger aus = self.pipeline.videoAUs;
    NSUInteger gaps = self.pipeline.sequenceGaps;
    NSUInteger errors = self.pipeline.decodeErrors;
    NSUInteger drops = self.pipeline.droppedAUs;
    NSUInteger underruns = self.pipeline.audioUnderruns;
    NSUInteger restarts = self.pipeline.audioRestartCount;
    double dt = self.lastOverlayAt > 0.0 ? MAX(0.001, now - self.lastOverlayAt) : 0.0;
    NSUInteger auDelta = RBMetricDelta(aus, self.lastOverlayAUs);
    NSUInteger gapDelta = RBMetricDelta(gaps, self.lastOverlayGaps);
    NSUInteger errorDelta = RBMetricDelta(errors, self.lastOverlayErrors);
    NSUInteger dropDelta = RBMetricDelta(drops, self.lastOverlayDrops);
    NSUInteger underrunDelta = RBMetricDelta(underruns, self.lastOverlayAudioUnderruns);
    NSUInteger restartDelta = RBMetricDelta(restarts, self.lastOverlayAudioRestarts);

    RBDiagnosticsSnapshot *snapshot = [[RBDiagnosticsSnapshot alloc] init];
    snapshot.server = server ?: @"Surf";
    snapshot.version = version ?: @"";
    snapshot.protocolVersion = protocolVersion ?: @"";
    snapshot.streamState = streamState ?: @"";
    snapshot.state = state ?: @"idle";
    snapshot.imageFPS = self.streamView.uniquePresentationFPS;
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
    snapshot.wrapMS = self.pipeline.averageWrapMS;
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
    } else if (errorDelta > 0 || dropDelta >= 3 || snapshot.queuedAUs >= 3 ||
               snapshot.RTTMS >= 250.0 || snapshot.latencyMS >= 350.0 ||
               snapshot.callbackMS >= 25.0 || underrunDelta >= 2) {
        snapshot.health = RBDiagnosticsHealthUnstable;
        snapshot.healthLabel = @"Unstable";
        if (errorDelta > 0) snapshot.healthReason = @"The decoder reported a new error";
        else if (snapshot.queuedAUs >= 3) snapshot.healthReason = @"Video is backing up";
        else if (snapshot.RTTMS >= 250.0) snapshot.healthReason = @"Network round trip is high";
        else if (snapshot.latencyMS >= 350.0) snapshot.healthReason = @"Input response is high";
        else if (snapshot.callbackMS >= 25.0) snapshot.healthReason = @"Video decode is taking too long";
        else if (underrunDelta >= 2) snapshot.healthReason = @"Audio is repeatedly underrunning";
        else snapshot.healthReason = @"Video frames were dropped";
    } else if (dropDelta > 0 || gapDelta > 0 || underrunDelta > 0 || restartDelta > 0 ||
               snapshot.queuedAUs >= 2 || snapshot.RTTMS >= 120.0 ||
               snapshot.latencyMS >= 200.0 || snapshot.callbackMS >= 12.0 ||
               snapshot.maxGapMS >= 150.0) {
        snapshot.health = RBDiagnosticsHealthDelayed;
        snapshot.healthLabel = @"Delayed";
        if (snapshot.RTTMS >= 120.0) snapshot.healthReason = @"Network response is slower than usual";
        else if (snapshot.latencyMS >= 200.0) snapshot.healthReason = @"Input is reaching the screen slowly";
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
    self.lastOverlayAudioUnderruns = underruns;
    self.lastOverlayAudioRestarts = restarts;
    return snapshot;
}

@end
