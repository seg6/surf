#import "RBSampleBufferRenderer.h"
#import "RBH264SampleBuilder.h"
#import "RBLog.h"
#import "RBProtocol.h"

#import <AVFoundation/AVFoundation.h>
#import <UIKit/UIKit.h>

enum {
    // AVSampleBufferDisplayLayer normally accepts a sample in 1–2 ms, but
    // iOS 8 occasionally holds enqueueSampleBuffer across a compositor burst.
    // Preserve every H.264 dependency for up to 300 ms at 60 FPS; the system
    // layer can then consume the burst in order and DisplayImmediately makes
    // composition converge on the newest decoded image. This is an emergency
    // fuse, not a target latency or an arbitrary-frame shedding policy.
    kRBSystemRendererPendingLimit = 18,
    // The iOS 6/7 core-only layer has no status or error property. If three
    // complete compressed cushions fill without one successful enqueue, a
    // flush is not repairing the remote FigVideoQueue; replace the layer.
    kRBSystemRendererCoreStallLimit = 3,
    kRBSystemRendererFailureLimit = 3,
};

typedef NS_ENUM(NSInteger, RBRendererIngressState) {
    RBRendererIngressRunning = 0,
    RBRendererIngressAwaitingIDR,
    RBRendererIngressIDRQueued,
};

@interface RBCompressedAU : NSObject
@property(nonatomic, strong) NSData *data;
@property(nonatomic, assign) BOOL idr;
@property(nonatomic, strong) RBFrameMetadata *metadata;
@property(nonatomic, assign) NSUInteger generation;
@end
@implementation RBCompressedAU
@end

@interface RBSampleBufferRenderer () {
    NSMutableArray *_pending;
    BOOL _pumpScheduled;
    BOOL _waitingForLayerReady;
    BOOL _needsFlush;
    BOOL _recoveryRequestOutstanding;
    BOOL _active;
    BOOL _failed;
    RBRendererIngressState _ingressState;
    NSUInteger _generation;
    RBFrameMetadata *_pendingAcceptedMetadata;
    BOOL _metadataHandoffScheduled;
    NSUInteger _consecutiveBackpressureRecoveries;
    BOOL _layerFailureScheduled;
}
@property(nonatomic, strong) dispatch_queue_t queue;
@property(nonatomic, strong) RBH264SampleBuilder *sampleBuilder;
@property(nonatomic, strong, readwrite) AVSampleBufferDisplayLayer *displayLayer;
@property(nonatomic, assign, readwrite) NSUInteger acceptedFrames;
@property(nonatomic, assign, readwrite) NSUInteger droppedAUs;
@property(nonatomic, assign, readwrite) NSUInteger backpressureEvents;
@property(nonatomic, assign, readwrite) NSUInteger recoveries;
@property(nonatomic, assign, readwrite) NSUInteger failures;
@property(nonatomic, assign, readwrite) double averageEnqueueMS;
@property(nonatomic, assign, readwrite) CFTimeInterval lastAcceptedAt;
@property(nonatomic, assign) CFTimeInterval failureWindowStart;
@property(nonatomic, assign) NSUInteger failureWindowCount;
@end

@implementation RBSampleBufferRenderer

+ (BOOL)available {
    Class layerClass = NSClassFromString(@"AVSampleBufferDisplayLayer");
    if (!layerClass || ![layerClass isSubclassOfClass:[CALayer class]]) return NO;
    SEL required[] = {
        @selector(enqueueSampleBuffer:),
        @selector(flush),
        @selector(flushAndRemoveImage),
        @selector(isReadyForMoreMediaData),
        @selector(requestMediaDataWhenReadyOnQueue:usingBlock:),
        @selector(stopRequestingMediaData),
        @selector(setVideoGravity:),
    };
    for (NSUInteger i = 0; i < sizeof(required) / sizeof(required[0]); i++) {
        if (![layerClass instancesRespondToSelector:required[i]]) return NO;
    }
    return YES;
}

static NSString *RBDisplayLayerFailureNotification(void) {
    // This data symbol is weak-imported by the iOS 6-targeted slice. Check its
    // address before reading it: the private iOS 6/7 class has the complete
    // render queue but does not export the later notification constant.
    if (&AVSampleBufferDisplayLayerFailedToDecodeNotification == NULL) return nil;
    return AVSampleBufferDisplayLayerFailedToDecodeNotification;
}

- (void)observeFailuresForLayer:(AVSampleBufferDisplayLayer *)layer {
    NSString *name = RBDisplayLayerFailureNotification();
    if (!layer || !name) return;
    [[NSNotificationCenter defaultCenter] addObserver:self
        selector:@selector(displayLayerFailedToDecode:) name:name object:layer];
}

- (void)stopObservingFailuresForLayer:(AVSampleBufferDisplayLayer *)layer {
    NSString *name = RBDisplayLayerFailureNotification();
    if (!layer || !name) return;
    [[NSNotificationCenter defaultCenter] removeObserver:self name:name object:layer];
}

- (AVSampleBufferDisplayLayer *)newDisplayLayer {
    Class layerClass = NSClassFromString(@"AVSampleBufferDisplayLayer");
    if (!layerClass) return nil;
    AVSampleBufferDisplayLayer *layer = [[layerClass alloc] init];
    layer.videoGravity = AVLayerVideoGravityResize;
    layer.opaque = YES;
    layer.backgroundColor = [[UIColor blackColor] CGColor];
    return layer;
}

- (id)init {
    self = [super init];
    if (self) {
        self.queue = dispatch_queue_create("surf.system-video-renderer", DISPATCH_QUEUE_SERIAL);
        _pending = [NSMutableArray array];
        _ingressState = RBRendererIngressAwaitingIDR;
        self.displayLayer = [self newDisplayLayer];
        if (!self.displayLayer) return nil;
        [self observeFailuresForLayer:self.displayLayer];
    }
    return self;
}

- (int)queuedAUs {
    @synchronized (self) {
        return (int)[_pending count];
    }
}

- (void)schedulePumpLocked {
    if (_pumpScheduled || !_active || _failed) return;
    _pumpScheduled = YES;
    dispatch_async(self.queue, ^{ [self pump]; });
}

- (void)configureWidth:(int)width height:(int)height {
    @synchronized (self) {
        _generation++;
        [_pending removeAllObjects];
        _ingressState = RBRendererIngressAwaitingIDR;
        _needsFlush = NO;
        _recoveryRequestOutstanding = NO;
        _active = YES;
        _failed = NO;
        _waitingForLayerReady = NO;
        _pumpScheduled = NO;
        _pendingAcceptedMetadata = nil;
        _metadataHandoffScheduled = NO;
        _consecutiveBackpressureRecoveries = 0;
        _layerFailureScheduled = NO;
    }
    dispatch_async(self.queue, ^{
        [self.displayLayer stopRequestingMediaData];
        [self.displayLayer flushAndRemoveImage];
        [self.sampleBuilder reset];
        self.sampleBuilder = [[RBH264SampleBuilder alloc]
            initWithWidth:MAX(2, width) height:MAX(2, height)];
        self.acceptedFrames = 0;
        self.droppedAUs = 0;
        self.backpressureEvents = 0;
        self.recoveries = 0;
        self.failures = 0;
        self.averageEnqueueMS = 0.0;
        self.lastAcceptedAt = 0.0;
        self.failureWindowStart = 0.0;
        self.failureWindowCount = 0;
        BOOL extendedFailureAPI = [self.displayLayer respondsToSelector:@selector(status)];
        RBLogEvent(@"renderer", @"info",
            @{@"mode": @"system", @"api": extendedFailureAPI ? @"extended" : @"core",
              @"coded_width": @(width), @"coded_height": @(height)},
            @"System compressed-video renderer ready");
    });
}

- (void)feedAU:(NSData *)au idr:(BOOL)idr metadata:(RBFrameMetadata *)metadata {
    if (![au length]) return;
    BOOL requestKeyframe = NO;
    BOOL enqueuePacket = YES;
    BOOL replaceStalledLayer = NO;
    NSUInteger stalledGeneration = 0;
    @synchronized (self) {
        if (!_active || _failed) return;
        if (_ingressState == RBRendererIngressAwaitingIDR && !idr) {
            self.droppedAUs++;
            return;
        }
        if (_ingressState == RBRendererIngressAwaitingIDR && idr) {
            _ingressState = RBRendererIngressIDRQueued;
            // A requested recovery boundary has arrived. If this IDR is later
            // discarded or rejected, that is a new episode and must be able
            // to request another one instead of waiting forever.
            _recoveryRequestOutstanding = NO;
        }

        if ([_pending count] >= kRBSystemRendererPendingLimit) {
            self.droppedAUs += [_pending count];
            [_pending removeAllObjects];
            _needsFlush = YES;
            self.recoveries++;
            _consecutiveBackpressureRecoveries++;
            if (_consecutiveBackpressureRecoveries >= kRBSystemRendererCoreStallLimit &&
                !_layerFailureScheduled) {
                _layerFailureScheduled = YES;
                replaceStalledLayer = YES;
                stalledGeneration = _generation;
            }
            _ingressState = RBRendererIngressAwaitingIDR;
            if (idr) {
                _ingressState = RBRendererIngressIDRQueued;
            } else if (!_recoveryRequestOutstanding) {
                _recoveryRequestOutstanding = YES;
                requestKeyframe = YES;
                self.droppedAUs++;
                enqueuePacket = NO;
            } else {
                self.droppedAUs++;
                enqueuePacket = NO;
            }
        }

        if (enqueuePacket) {
            RBCompressedAU *packet = [[RBCompressedAU alloc] init];
            packet.data = au;
            packet.idr = idr;
            packet.metadata = metadata;
            packet.generation = _generation;
            [_pending addObject:packet];
        }
        [self schedulePumpLocked];
    }
    if (requestKeyframe) {
        RBLogEvent(@"renderer", @"warn",
            @{@"mode": @"system", @"pending_limit": @(kRBSystemRendererPendingLimit)},
            @"System renderer exhausted its compressed burst cushion");
        dispatch_async(dispatch_get_main_queue(), ^{
            [self.delegate sampleBufferRendererNeedsKeyframe:self];
        });
    }
    if (replaceStalledLayer) {
        dispatch_async(self.queue, ^{
            BOOL stillCurrent = NO;
            @synchronized (self) {
                stillCurrent = _active && !_failed && _layerFailureScheduled &&
                    stalledGeneration == _generation;
            }
            if (stillCurrent) {
                RBLogEvent(@"renderer", @"error",
                    @{@"mode": @"system",
                      @"stalled_cushions": @(kRBSystemRendererCoreStallLimit)},
                    @"System renderer queue remained stalled after flush recovery");
                [self handleTerminalLayerFailure];
            }
        });
    }
}

- (void)waitForLayerReadiness {
    if (_waitingForLayerReady || !_active || _failed) return;
    _waitingForLayerReady = YES;
    self.backpressureEvents++;
    __weak RBSampleBufferRenderer *weakSelf = self;
    AVSampleBufferDisplayLayer *waitingLayer = self.displayLayer;
    [waitingLayer requestMediaDataWhenReadyOnQueue:self.queue usingBlock:^{
        RBSampleBufferRenderer *renderer = weakSelf;
        if (!renderer || renderer.displayLayer != waitingLayer ||
            !waitingLayer.readyForMoreMediaData) return;
        [waitingLayer stopRequestingMediaData];
        renderer->_waitingForLayerReady = NO;
        @synchronized (renderer) {
            [renderer schedulePumpLocked];
        }
    }];
}

- (RBCompressedAU *)nextPacketOrApplyFlush:(BOOL *)didFlush {
    RBCompressedAU *packet = nil;
    @synchronized (self) {
        if (_needsFlush) {
            _needsFlush = NO;
            if (didFlush) *didFlush = YES;
            return nil;
        }
        if (didFlush) *didFlush = NO;
        if ([_pending count]) {
            packet = [_pending objectAtIndex:0];
            [_pending removeObjectAtIndex:0];
        }
    }
    return packet;
}

- (void)pump {
    for (;;) {
        @synchronized (self) {
            if (!_active || _failed) {
                _pumpScheduled = NO;
                return;
            }
        }

        BOOL didFlush = NO;
        RBCompressedAU *packet = [self nextPacketOrApplyFlush:&didFlush];
        if (didFlush) {
            [self.displayLayer flush];
            continue;
        }
        if (!packet) {
            @synchronized (self) {
                _pumpScheduled = NO;
                if ([_pending count]) [self schedulePumpLocked];
            }
            return;
        }
        if (packet.generation != _generation) continue;

        if (!self.displayLayer.readyForMoreMediaData) {
            @synchronized (self) {
                [_pending insertObject:packet atIndex:0];
                _pumpScheduled = NO;
            }
            [self waitForLayerReadiness];
            return;
        }

        BOOL formatChanged = NO;
        OSStatus status = noErr;
        CMSampleBufferRef sample = [self.sampleBuilder createSampleForAU:packet.data
            idr:packet.idr formatChanged:&formatChanged status:&status];
        if (!sample) {
            [self beginRecoveryForReason:@"sample-construction" status:status];
            continue;
        }
        if (formatChanged) {
            // A new format starts at the current IDR. Flush any images from the
            // previous format before submitting that dependency boundary.
            [self.displayLayer flush];
        }

        CFTimeInterval start = CACurrentMediaTime();
        [self.displayLayer enqueueSampleBuffer:sample];
        double enqueueMS = (CACurrentMediaTime() - start) * 1000.0;
        CFRelease(sample);

        self.acceptedFrames++;
        self.lastAcceptedAt = CACurrentMediaTime();
        self.averageEnqueueMS = self.averageEnqueueMS <= 0.0 ? enqueueMS :
            self.averageEnqueueMS * 0.85 + enqueueMS * 0.15;
        @synchronized (self) {
            _consecutiveBackpressureRecoveries = 0;
            _layerFailureScheduled = NO;
        }
        if (packet.idr) {
            @synchronized (self) {
                if (_ingressState == RBRendererIngressIDRQueued)
                    _ingressState = RBRendererIngressRunning;
                _recoveryRequestOutstanding = NO;
            }
        }
        [self enqueueMetadataHandoff:packet.metadata];

        if ([self.displayLayer respondsToSelector:@selector(status)] &&
            self.displayLayer.status == AVQueuedSampleBufferRenderingStatusFailed) {
            [self handleTerminalLayerFailure];
            return;
        }
    }
}

- (void)enqueueMetadataHandoff:(RBFrameMetadata *)metadata {
    BOOL schedule = NO;
    @synchronized (self) {
        _pendingAcceptedMetadata = metadata;
        if (!_metadataHandoffScheduled) {
            _metadataHandoffScheduled = YES;
            schedule = YES;
        }
    }
    if (!schedule) return;
    dispatch_async(dispatch_get_main_queue(), ^{ [self drainMetadataHandoff]; });
}

- (void)drainMetadataHandoff {
    RBFrameMetadata *metadata = nil;
    @synchronized (self) {
        metadata = _pendingAcceptedMetadata;
        _pendingAcceptedMetadata = nil;
        _metadataHandoffScheduled = NO;
    }
    if (metadata && _active && !_failed)
        [self.delegate sampleBufferRenderer:self didAcceptMetadata:metadata];
}

- (void)beginRecoveryForReason:(NSString *)reason status:(OSStatus)status {
    BOOL requestKeyframe = NO;
    @synchronized (self) {
        if (!_active || _failed) return;
        self.droppedAUs += [_pending count];
        [_pending removeAllObjects];
        _needsFlush = YES;
        _ingressState = RBRendererIngressAwaitingIDR;
        self.recoveries++;
        _recoveryRequestOutstanding = NO;
        if (!_recoveryRequestOutstanding) {
            _recoveryRequestOutstanding = YES;
            requestKeyframe = YES;
        }
    }
    RBLogEvent(@"renderer", @"warn",
        @{@"mode": @"system", @"reason": reason ?: @"unknown", @"status": @(status)},
        @"System renderer is waiting for a clean IDR");
    if (requestKeyframe) dispatch_async(dispatch_get_main_queue(), ^{
        [self.delegate sampleBufferRendererNeedsKeyframe:self];
    });
}

- (void)displayLayerFailedToDecode:(NSNotification *)notification {
    if (notification.object != self.displayLayer) return;
    AVSampleBufferDisplayLayer *failedLayer = notification.object;
    dispatch_async(self.queue, ^{
        // A status check may already have replaced the layer before this
        // notification reaches our queue. Never tear down that fresh layer
        // for a delayed notification from its predecessor.
        if (failedLayer == self.displayLayer) [self handleTerminalLayerFailure];
    });
}

- (void)handleTerminalLayerFailure {
    @synchronized (self) {
        _layerFailureScheduled = NO;
        if (!_active || _failed) return;
    }
    CFTimeInterval now = CACurrentMediaTime();
    if (now - self.failureWindowStart > 30.0) {
        self.failureWindowStart = now;
        self.failureWindowCount = 0;
    }
    self.failureWindowCount++;
    self.failures++;
    NSError *error = nil;
    if ([self.displayLayer respondsToSelector:@selector(error)])
        error = self.displayLayer.error;
    RBLogEvent(@"renderer", @"error",
        @{@"mode": @"system", @"failure_count": @(self.failureWindowCount),
          @"error": [error localizedDescription] ?: @"display layer failed"},
        @"System display layer failed");
    if (self.failureWindowCount > kRBSystemRendererFailureLimit) {
        @synchronized (self) {
            _failed = YES;
            _active = NO;
            [_pending removeAllObjects];
            _pumpScheduled = NO;
        }
        dispatch_async(dispatch_get_main_queue(), ^{
            [self.delegate sampleBufferRendererDidFail:self];
        });
        return;
    }

    [self.displayLayer stopRequestingMediaData];
    __block AVSampleBufferDisplayLayer *replacement = nil;
    void (^replace)(void) = ^{
        replacement = [self newDisplayLayer];
        if (replacement) {
            [self observeFailuresForLayer:replacement];
            [self.delegate sampleBufferRenderer:self didReplaceDisplayLayer:replacement];
        }
    };
    if ([NSThread isMainThread]) replace();
    else dispatch_sync(dispatch_get_main_queue(), replace);
    if (!replacement) {
        @synchronized (self) { _failed = YES; _active = NO; }
        dispatch_async(dispatch_get_main_queue(), ^{
            [self.delegate sampleBufferRendererDidFail:self];
        });
        return;
    }
    [self stopObservingFailuresForLayer:self.displayLayer];
    self.displayLayer = replacement;
    @synchronized (self) {
        _consecutiveBackpressureRecoveries = 0;
        _waitingForLayerReady = NO;
        _pumpScheduled = NO;
    }
    [self.sampleBuilder reset];
    [self beginRecoveryForReason:@"display-layer-replacement" status:noErr];
}

- (void)reset {
    @synchronized (self) {
        if (!_active || _failed) return;
        _generation++;
        self.droppedAUs += [_pending count];
        [_pending removeAllObjects];
        _ingressState = RBRendererIngressAwaitingIDR;
        _needsFlush = NO;
        _recoveryRequestOutstanding = NO;
        _waitingForLayerReady = NO;
        _pumpScheduled = NO;
        _pendingAcceptedMetadata = nil;
        _metadataHandoffScheduled = NO;
        _consecutiveBackpressureRecoveries = 0;
        _layerFailureScheduled = NO;
    }
    dispatch_async(self.queue, ^{
        [self.displayLayer stopRequestingMediaData];
        [self.displayLayer flush];
        [self.sampleBuilder reset];
    });
}

- (void)stop {
    @synchronized (self) {
        _generation++;
        _active = NO;
        [_pending removeAllObjects];
        _ingressState = RBRendererIngressAwaitingIDR;
        _needsFlush = NO;
        _recoveryRequestOutstanding = NO;
        _waitingForLayerReady = NO;
        _pumpScheduled = NO;
        _pendingAcceptedMetadata = nil;
        _metadataHandoffScheduled = NO;
        _consecutiveBackpressureRecoveries = 0;
        _layerFailureScheduled = NO;
    }
    dispatch_async(self.queue, ^{
        [self.displayLayer stopRequestingMediaData];
        [self.displayLayer flushAndRemoveImage];
        [self.sampleBuilder reset];
    });
}

- (void)dealloc {
    [self stopObservingFailuresForLayer:self.displayLayer];
    [self.displayLayer stopRequestingMediaData];
}

@end
