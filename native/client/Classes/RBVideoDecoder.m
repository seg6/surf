#import "RBVideoDecoder.h"
#import "RBH264SampleBuilder.h"
#import "RBLog.h"
#import "RBProtocol.h"
#import "RBVTPrivate.h"

#import <QuartzCore/QuartzCore.h>

#include <dlfcn.h>
#include <libkern/OSAtomic.h>

typedef struct {
    CFTimeInterval submittedAt;
    unsigned long long interactionID;
    unsigned int auSequence;
    unsigned int sourceSequence;
    unsigned int encoderGeneration;
    unsigned long long sourceReceiveNS;
    unsigned long long encodeCompleteNS;
    unsigned long long socketWriteNS;
    unsigned int decoderGeneration;
} RBFrameTiming;

// ---- runtime symbol resolution ---------------------------------------------

static RBVTDecompressionSessionCreateFn rbVTCreate;
static RBVTDecompressionSessionDecodeFrameFn rbVTDecode;
static RBVTDecompressionSessionInvalidateFn rbVTInvalidate;

static BOOL RBResolveVT(void) {
    static dispatch_once_t once;
    static BOOL ok = NO;
    dispatch_once(&once, ^{
        const char *paths[] = {
            "/System/Library/PrivateFrameworks/VideoToolbox.framework/VideoToolbox",
            "/System/Library/Frameworks/VideoToolbox.framework/VideoToolbox",
        };
        void *handle = NULL;
        for (unsigned i = 0; i < 2 && !handle; i++) handle = dlopen(paths[i], RTLD_NOW);
        if (!handle) {
            RBLogEvent(@"decoder", @"error", @{@"framework": @"VideoToolbox", @"error": [NSString stringWithUTF8String:dlerror() ?: ""]}, @"Framework loading failed");
            return;
        }
        rbVTCreate = (RBVTDecompressionSessionCreateFn)dlsym(handle, "VTDecompressionSessionCreate");
        rbVTDecode = (RBVTDecompressionSessionDecodeFrameFn)dlsym(handle, "VTDecompressionSessionDecodeFrame");
        rbVTInvalidate = (RBVTDecompressionSessionInvalidateFn)dlsym(handle, "VTDecompressionSessionInvalidate");
        ok = rbVTCreate && rbVTDecode && rbVTInvalidate;
        RBLogEvent(@"decoder", ok ? @"info" : @"error", @{@"framework": @"VideoToolbox", @"resolved": @(ok)}, @"Framework symbols checked");
    });
    return ok;
}

// ---- decoder ----------------------------------------------------------------

// Legacy iOS 6/7 only: once more than three AUs are outstanding, latency is
// already outside the useful live window. Discard the complete dependency
// chain and resume at one requested IDR; never suppress arbitrary P outputs.
static const int kRBMaxQueuedAUs = 3;
// This many resyncs inside 30s means the lane is hurting more than helping.
static const int kRBMaxResyncs = 3;

@interface RBVideoDecoder () {
    VTDecompressionSessionRef _session;
    CVPixelBufferRef _pendingOutput;
    RBFrameMetadata *_pendingOutputMetadata;
    CFTimeInterval _pendingOutputCallbackAt;
    BOOL _outputHandoffScheduled;
    int32_t _inFlight;
    volatile int32_t _resyncPending;
    unsigned int _generation;
}
@property(nonatomic, strong) dispatch_queue_t queue;
@property(nonatomic, strong) RBH264SampleBuilder *sampleBuilder;
@property(nonatomic, assign) BOOL waitingForIDR;
@property(nonatomic, assign) int32_t queued;
@property(nonatomic, assign) NSUInteger decodedFrames;
@property(nonatomic, assign) NSUInteger decodeErrors;
@property(nonatomic, assign) NSUInteger submittedAUs;
@property(nonatomic, assign) NSUInteger droppedAUs;
@property(nonatomic, assign) NSUInteger callbackFrames;
@property(nonatomic, assign) double lastSubmitMS;
@property(nonatomic, assign) double averageSubmitMS;
@property(nonatomic, assign) double lastCallbackMS;
@property(nonatomic, assign) double averageCallbackMS;
@property(nonatomic, assign) double lastHandoffMS;
@property(nonatomic, assign) double averageHandoffMS;
@property(nonatomic, assign) int resyncs;
@property(nonatomic, assign) CFTimeInterval resyncWindowStart;
@property(nonatomic, assign) BOOL failed;
- (BOOL)completeFrameForGeneration:(unsigned int)generation;
- (void)enqueueOutputPixelBuffer:(CVPixelBufferRef)pixelBuffer
                        metadata:(RBFrameMetadata *)metadata
                      callbackAt:(CFTimeInterval)callbackAt;
- (void)drainPendingOutput;
- (void)clearPendingOutput;
@end

// VT output callback (VT's thread): retain + hand the frame to the main thread.
static void RBDecodeCallback(void *refcon, void *frameRefcon, OSStatus status,
                             VTDecodeInfoFlags flags, CVImageBufferRef imageBuffer,
                             CMTime pts, CMTime duration) {
    RBFrameTiming *timing = (RBFrameTiming *)frameRefcon;
    RBVideoDecoder *decoder = (__bridge RBVideoDecoder *)refcon;
    BOOL currentGeneration = timing && [decoder completeFrameForGeneration:timing->decoderGeneration];
    CFTimeInterval now = CACurrentMediaTime();
    double callbackMS = timing ? (now - timing->submittedAt) * 1000.0 : 0.0;
    unsigned long long interactionID = timing ? timing->interactionID : 0;
    RBFrameMetadata *metadata = [[RBFrameMetadata alloc] init];
    metadata.interactionID = interactionID;
    if (timing) {
        metadata.auSequence = timing->auSequence;
        metadata.sourceSequence = timing->sourceSequence;
        metadata.encoderGeneration = timing->encoderGeneration;
        metadata.sourceReceiveNS = timing->sourceReceiveNS;
        metadata.encodeCompleteNS = timing->encodeCompleteNS;
        metadata.socketWriteNS = timing->socketWriteNS;
    }
    if (timing) free(timing);
    if (status != noErr || !imageBuffer || !currentGeneration) return;
    CFTimeInterval callbackAt = now;
    decoder.callbackFrames++;
    decoder.decodedFrames++;
    decoder.lastCallbackMS = callbackMS;
    decoder.averageCallbackMS = decoder.averageCallbackMS <= 0.0 ? callbackMS : decoder.averageCallbackMS * 0.85 + callbackMS * 0.15;
    [decoder enqueueOutputPixelBuffer:(CVPixelBufferRef)imageBuffer
                             metadata:metadata callbackAt:callbackAt];
}

@implementation RBVideoDecoder

- (BOOL)completeFrameForGeneration:(unsigned int)generation {
    if (generation != _generation) return NO;
    OSAtomicDecrement32(&_inFlight);
    return YES;
}

// VideoToolbox callbacks are not allowed to create an unbounded main-queue
// backlog. Keyboard and popover animations can briefly occupy UIKit; retaining
// every decoded IOSurface behind them exhausts the decoder's output pool and
// turns that short UI stall into a multi-second media stall. Keep only the
// newest completed surface and schedule at most one main-thread handoff.
- (void)enqueueOutputPixelBuffer:(CVPixelBufferRef)pixelBuffer
                        metadata:(RBFrameMetadata *)metadata
                      callbackAt:(CFTimeInterval)callbackAt {
    if (!pixelBuffer) return;
    CVPixelBufferRetain(pixelBuffer);
    BOOL schedule = NO;
    @synchronized (self) {
        if (_pendingOutput) {
            CVPixelBufferRelease(_pendingOutput);
            self.droppedAUs++;
        }
        _pendingOutput = pixelBuffer;
        _pendingOutputMetadata = metadata;
        _pendingOutputCallbackAt = callbackAt;
        if (!_outputHandoffScheduled) {
            _outputHandoffScheduled = YES;
            schedule = YES;
        }
    }
    if (schedule) {
        dispatch_async(dispatch_get_main_queue(), ^{ [self drainPendingOutput]; });
    }
}

- (void)drainPendingOutput {
    CVPixelBufferRef pixelBuffer = NULL;
    RBFrameMetadata *metadata = nil;
    CFTimeInterval callbackAt = 0.0;
    @synchronized (self) {
        pixelBuffer = _pendingOutput;
        _pendingOutput = NULL;
        metadata = _pendingOutputMetadata;
        _pendingOutputMetadata = nil;
        callbackAt = _pendingOutputCallbackAt;
        _pendingOutputCallbackAt = 0.0;
        _outputHandoffScheduled = NO;
    }
    if (!pixelBuffer) return;
    double handoffMS = callbackAt > 0.0 ?
        (CACurrentMediaTime() - callbackAt) * 1000.0 : 0.0;
    self.lastHandoffMS = handoffMS;
    self.averageHandoffMS = self.averageHandoffMS <= 0.0 ? handoffMS :
        self.averageHandoffMS * 0.85 + handoffMS * 0.15;
    [self.delegate videoDecoder:self didDecodePixelBuffer:pixelBuffer metadata:metadata];
    CVPixelBufferRelease(pixelBuffer);
}

- (void)clearPendingOutput {
    @synchronized (self) {
        if (_pendingOutput) {
            CVPixelBufferRelease(_pendingOutput);
            _pendingOutput = NULL;
        }
        _pendingOutputMetadata = nil;
        _pendingOutputCallbackAt = 0.0;
        _outputHandoffScheduled = NO;
    }
}

- (int)queuedAUs {
    return _queued + _inFlight;
}

+ (BOOL)available {
    return RBResolveVT();
}

- (id)init {
    self = [super init];
    if (self) {
		self.queue = dispatch_queue_create("surf.videodecode", DISPATCH_QUEUE_SERIAL);
        self.waitingForIDR = YES;
    }
    return self;
}

- (void)dealloc {
    [self clearPendingOutput];
    [self teardownSession];
}

- (void)feedAU:(NSData *)au idr:(BOOL)idr metadata:(RBFrameMetadata *)metadata {
    if (self.failed || ![RBVideoDecoder available]) return;
    // Once one dependent frame has been shed, no later P-frame is decodable.
    // Reject it before it enters the serial queue and let exactly one IDR
    // request represent the whole recovery episode.
    int32_t resyncState = OSAtomicAdd32Barrier(0, &_resyncPending);
    if (!idr && resyncState == 1) {
        self.droppedAUs++;
        return;
    }
    // State 2 means the recovery IDR is already queued. P-frames arriving
    // after it are valid dependencies and must remain behind it in FIFO order.
    if (idr && resyncState == 1)
        OSAtomicCompareAndSwap32Barrier(1, 2, &_resyncPending);
    // Latest-wins is illegal for P-frames; when the queue backs up we drop
    // whole GOPs instead: skip until the next IDR drains through.
    int32_t depth = OSAtomicIncrement32Barrier(&_queued) +
        OSAtomicAdd32Barrier(0, &_inFlight);
    if (depth > kRBMaxQueuedAUs) {
        if (!idr) {
            OSAtomicDecrement32Barrier(&_queued);
            self.droppedAUs++;
            BOOL beganResync = OSAtomicCompareAndSwap32Barrier(0, 1, &_resyncPending);
            if (beganResync) {
                RBLogEvent(@"decoder", @"warn", @{ @"queue_depth": @(depth),
                    @"recovery": @"next_idr" }, @"Legacy decoder queue overflowed");
                dispatch_async(self.queue, ^{
                    self.waitingForIDR = YES;
                });
                dispatch_async(dispatch_get_main_queue(), ^{
                    [self.delegate videoDecoderNeedsKeyframe:self];
                });
            }
            return;
        }
        // Keep an overflowing IDR: it is the recovery boundary needed by all
        // later frames. Its queued count is consumed by the decode block
        // below; decrementing here as well used to drive the gauge negative.
    }
    dispatch_async(self.queue, ^{
        OSAtomicDecrement32(&_queued);
        [self decodeAU:au idr:idr metadata:metadata];
    });
}

- (void)reset {
    dispatch_async(self.queue, ^{
        self->_generation++;
        [self teardownSession];
        [self clearPendingOutput];
        self->_inFlight = 0;
        self->_resyncPending = 0;
        self.sampleBuilder = [[RBH264SampleBuilder alloc]
            initWithWidth:MAX(2, self.codedWidth) height:MAX(2, self.codedHeight)];
        self.waitingForIDR = YES;
        self.resyncs = 0;
        self.failed = NO;
        self.submittedAUs = 0;
        self.droppedAUs = 0;
        self.callbackFrames = 0;
        self.lastSubmitMS = 0.0;
        self.averageSubmitMS = 0.0;
        self.lastCallbackMS = 0.0;
        self.averageCallbackMS = 0.0;
        self.lastHandoffMS = 0.0;
        self.averageHandoffMS = 0.0;
    });
}

// ---- decode queue only below here ----

- (void)teardownSession {
    if (_session) {
        rbVTInvalidate(_session);
        CFRelease(_session);
        _session = NULL;
    }
}

- (void)noteResync {
    CFTimeInterval now = CACurrentMediaTime();
    if (now - self.resyncWindowStart > 30.0) {
        self.resyncWindowStart = now;
        self.resyncs = 0;
    }
    self.resyncs++;
    self.waitingForIDR = YES;
    OSAtomicCompareAndSwap32Barrier(0, 1, &_resyncPending);
    if (self.resyncs > kRBMaxResyncs) {
        RBLogEvent(@"decoder", @"error", @{@"resyncs": @(self.resyncs), @"window_seconds": @30}, @"Decoder resync budget exhausted");
        self.failed = YES;
        [self teardownSession];
        dispatch_async(dispatch_get_main_queue(), ^{ [self.delegate videoDecoderDidFail:self]; });
        return;
    }
    // Ask the server for a recovery IDR; healthy streams intentionally have
    // no periodic IDR. Skipped on the give-up path above because recovery
    // then requires the user's explicit retry.
    dispatch_async(dispatch_get_main_queue(), ^{ [self.delegate videoDecoderNeedsKeyframe:self]; });
}

- (BOOL)ensureSessionForFormat:(CMVideoFormatDescriptionRef)format changed:(BOOL)changed {
    if (_session && !changed) return YES;
    if (!format) return NO;
    [self teardownSession];
    // Keep the hardware decoder's native bi-planar YUV surface. Converting
    // every frame to BGRA inside VideoToolbox consumed most of one A5 frame
    // budget and retained several 2.8MiB RGB buffers.
    int32_t nv12 = kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange;
    CFNumberRef pixfmt = CFNumberCreate(NULL, kCFNumberSInt32Type, &nv12);
    CFDictionaryRef ioSurfaceAttrs = CFDictionaryCreate(NULL, NULL, NULL, 0,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    const void *destKeys[] = {
        kCVPixelBufferPixelFormatTypeKey,
        kCVPixelBufferOpenGLESCompatibilityKey,
        kCVPixelBufferIOSurfacePropertiesKey,
    };
    const void *destValues[] = {pixfmt, kCFBooleanTrue, ioSurfaceAttrs};
    CFDictionaryRef destAttrs = CFDictionaryCreate(NULL, destKeys, destValues, 3,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFRelease(ioSurfaceAttrs);
    CFRelease(pixfmt);

    VTDecompressionOutputCallbackRecord record;
    record.decompressionOutputCallback = RBDecodeCallback;
    record.decompressionOutputRefCon = (__bridge void *)self;

    VTDecompressionSessionRef session = NULL;
    OSStatus status = rbVTCreate(NULL, format, NULL, destAttrs, &record, &session);
    CFRelease(destAttrs);
    if (status != noErr || !session) {
        RBLogEvent(@"decoder", @"error", @{@"operation": @"create_session", @"status": @(status)}, @"VideoToolbox session creation failed");
        return NO;
    }
    _session = session;
    RBLogEvent(@"decoder", @"info",
        @{@"renderer": @"legacy-gl", @"coded_width": @(self.codedWidth),
          @"coded_height": @(self.codedHeight)}, @"Legacy VideoToolbox session ready");
    return YES;
}

- (void)decodeAU:(NSData *)au idr:(BOOL)idr metadata:(RBFrameMetadata *)metadata {
    if (self.failed) return;
    if (self.waitingForIDR && !idr) return;

    if (!self.sampleBuilder) {
        self.sampleBuilder = [[RBH264SampleBuilder alloc]
            initWithWidth:MAX(2, self.codedWidth) height:MAX(2, self.codedHeight)];
    }
    BOOL formatChanged = NO;
    OSStatus status = noErr;
    CMSampleBufferRef sample = [self.sampleBuilder createSampleForAU:au idr:idr
        formatChanged:&formatChanged status:&status];
    if (!sample || ![self ensureSessionForFormat:self.sampleBuilder.formatDescription
                                          changed:formatChanged]) {
        if (sample) CFRelease(sample);
        self.decodeErrors++;
        [self noteResync];
        return;
    }

    VTDecodeInfoFlags flagsOut = 0;
    RBFrameTiming *timing = malloc(sizeof(RBFrameTiming));
    if (timing) {
        timing->submittedAt = CACurrentMediaTime();
        timing->interactionID = metadata.interactionID;
        timing->auSequence = metadata.auSequence;
        timing->sourceSequence = metadata.sourceSequence;
        timing->encoderGeneration = metadata.encoderGeneration;
        timing->sourceReceiveNS = metadata.sourceReceiveNS;
        timing->encodeCompleteNS = metadata.encodeCompleteNS;
        timing->socketWriteNS = metadata.socketWriteNS;
        timing->decoderGeneration = _generation;
    }
    if (!timing) {
        CFRelease(sample);
        self.decodeErrors++;
        return;
    }
    CFTimeInterval submitStart = CACurrentMediaTime();
    OSAtomicIncrement32(&_inFlight);
    status = rbVTDecode(_session, sample, kVTDecodeFrame_EnableAsynchronousDecompression,
                        timing, &flagsOut);
    double submitMS = (CACurrentMediaTime() - submitStart) * 1000.0;
    CFRelease(sample);

    if (status != noErr && timing) {
        OSAtomicDecrement32(&_inFlight);
        free(timing);
    }

    if (status == kVTInvalidSessionErr) {
        // Classic after app resume: session died under us. Rebuild at the
        // next IDR (repeat-headers guarantees fresh SPS/PPS there).
        RBLogEvent(@"decoder", @"warn", @{@"recovery": @"next_idr"}, @"VideoToolbox session invalidated");
        [self teardownSession];
        [self.sampleBuilder reset];
        self.decodeErrors++;
        [self noteResync];
        return;
    }
    if (status != noErr) {
        self.decodeErrors++;
        [self noteResync];
        return;
    }
    self.waitingForIDR = NO;
    if (idr) OSAtomicCompareAndSwap32Barrier(2, 0, &_resyncPending);
    self.submittedAUs++;
    self.lastSubmitMS = submitMS;
    self.averageSubmitMS = self.averageSubmitMS <= 0.0 ? submitMS : self.averageSubmitMS * 0.85 + submitMS * 0.15;
}

@end
