#import "RBVideoDecoder.h"
#import "RBLog.h"
#import "RBVTPrivate.h"

#import <QuartzCore/QuartzCore.h>

#include "rb_h264.h"

#include <dlfcn.h>
#include <libkern/OSAtomic.h>

typedef struct {
    CFTimeInterval submittedAt;
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
            RBLog(@"video: VideoToolbox dlopen failed: %s", dlerror());
            return;
        }
        rbVTCreate = (RBVTDecompressionSessionCreateFn)dlsym(handle, "VTDecompressionSessionCreate");
        rbVTDecode = (RBVTDecompressionSessionDecodeFrameFn)dlsym(handle, "VTDecompressionSessionDecodeFrame");
        rbVTInvalidate = (RBVTDecompressionSessionInvalidateFn)dlsym(handle, "VTDecompressionSessionInvalidate");
        ok = rbVTCreate && rbVTDecode && rbVTInvalidate;
        RBLog(@"video: VideoToolbox %@", ok ? @"resolved" : @"missing symbols");
    });
    return ok;
}

// ---- zero-copy CGImage over a CVPixelBuffer --------------------------------

static void RBReleasePixelBuffer(void *info, const void *data, size_t size) {
    CVPixelBufferRef pb = (CVPixelBufferRef)info;
    CVPixelBufferUnlockBaseAddress(pb, kCVPixelBufferLock_ReadOnly);
    CVPixelBufferRelease(pb);
}

// Wraps a BGRA pixel buffer without copying; the CGImage owns a lock+retain
// on the buffer, released when the image is destroyed.
static CGImageRef RBCreateImageFromPixelBuffer(CVPixelBufferRef pb) {
    if (CVPixelBufferLockBaseAddress(pb, kCVPixelBufferLock_ReadOnly) != kCVReturnSuccess) return NULL;
    CVPixelBufferRetain(pb);
    void *base = CVPixelBufferGetBaseAddress(pb);
    size_t bpr = CVPixelBufferGetBytesPerRow(pb);
    size_t w = CVPixelBufferGetWidth(pb);
    size_t h = CVPixelBufferGetHeight(pb);
    CGDataProviderRef provider = CGDataProviderCreateWithData(pb, base, bpr * h, RBReleasePixelBuffer);
    if (!provider) {
        CVPixelBufferUnlockBaseAddress(pb, kCVPixelBufferLock_ReadOnly);
        CVPixelBufferRelease(pb);
        return NULL;
    }
    CGColorSpaceRef space = CGColorSpaceCreateDeviceRGB();
    CGImageRef image = CGImageCreate(w, h, 8, 32, bpr, space,
                                     kCGBitmapByteOrder32Little | kCGImageAlphaNoneSkipFirst,
                                     provider, NULL, false, kCGRenderingIntentDefault);
    CGColorSpaceRelease(space);
    CGDataProviderRelease(provider); // image holds its own reference
    return image;
}

// ---- decoder ----------------------------------------------------------------

// Above this many queued-but-undecoded AUs we drop to the next IDR: the A5
// fell behind and P-frames only pile onto stale state.
static const int kRBMaxQueuedAUs = 8;
// This many resyncs inside 30s means the lane is hurting more than helping.
static const int kRBMaxResyncs = 3;

@interface RBVideoDecoder () {
    VTDecompressionSessionRef _session;
    CMVideoFormatDescriptionRef _format;
}
@property(nonatomic, strong) dispatch_queue_t queue;
@property(nonatomic, strong) NSData *currentSPS;
@property(nonatomic, strong) NSData *currentPPS;
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
@property(nonatomic, assign) double lastWrapMS;
@property(nonatomic, assign) double averageWrapMS;
@property(nonatomic, assign) int resyncs;
@property(nonatomic, assign) CFTimeInterval resyncWindowStart;
@property(nonatomic, assign) BOOL failed;
@end

// VT output callback (VT's thread): wrap + hand the frame to the main thread.
static void RBDecodeCallback(void *refcon, void *frameRefcon, OSStatus status,
                             VTDecodeInfoFlags flags, CVImageBufferRef imageBuffer,
                             CMTime pts, CMTime duration) {
    RBFrameTiming *timing = (RBFrameTiming *)frameRefcon;
    CFTimeInterval now = CACurrentMediaTime();
    double callbackMS = timing ? (now - timing->submittedAt) * 1000.0 : 0.0;
    if (timing) free(timing);
    if (status != noErr || !imageBuffer) return;
    RBVideoDecoder *decoder = (__bridge RBVideoDecoder *)refcon;
    CFTimeInterval wrapStart = CACurrentMediaTime();
    CGImageRef image = RBCreateImageFromPixelBuffer((CVPixelBufferRef)imageBuffer);
    if (!image) return;
    double wrapMS = (CACurrentMediaTime() - wrapStart) * 1000.0;
    decoder.callbackFrames++;
    decoder.lastCallbackMS = callbackMS;
    decoder.averageCallbackMS = decoder.averageCallbackMS <= 0.0 ? callbackMS : decoder.averageCallbackMS * 0.85 + callbackMS * 0.15;
    decoder.lastWrapMS = wrapMS;
    decoder.averageWrapMS = decoder.averageWrapMS <= 0.0 ? wrapMS : decoder.averageWrapMS * 0.85 + wrapMS * 0.15;
    dispatch_async(dispatch_get_main_queue(), ^{
        [decoder.delegate videoDecoder:decoder didDecodeImage:image];
        CGImageRelease(image);
    });
}

@implementation RBVideoDecoder

- (int)queuedAUs {
    return _queued;
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
    [self teardownSession];
}

- (void)feedAU:(NSData *)au idr:(BOOL)idr {
    if (self.failed || ![RBVideoDecoder available]) return;
    // Latest-wins is illegal for P-frames; when the queue backs up we drop
    // whole GOPs instead: skip until the next IDR drains through.
    if (OSAtomicIncrement32(&_queued) > kRBMaxQueuedAUs) {
        OSAtomicDecrement32(&_queued);
        if (!idr) {
            self.droppedAUs++;
            dispatch_async(self.queue, ^{ self.waitingForIDR = YES; });
            return;
        }
    }
    dispatch_async(self.queue, ^{
        OSAtomicDecrement32(&_queued);
        [self decodeAU:au idr:idr];
    });
}

- (void)reset {
    dispatch_async(self.queue, ^{
        [self teardownSession];
        self.currentSPS = nil;
        self.currentPPS = nil;
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
        self.lastWrapMS = 0.0;
        self.averageWrapMS = 0.0;
    });
}

// ---- decode queue only below here ----

- (void)teardownSession {
    if (_session) {
        rbVTInvalidate(_session);
        CFRelease(_session);
        _session = NULL;
    }
    if (_format) {
        CFRelease(_format);
        _format = NULL;
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
    if (self.resyncs > kRBMaxResyncs) {
        RBLog(@"video: %d resyncs in 30s, giving up on the lane", self.resyncs);
        self.failed = YES;
        [self teardownSession];
        dispatch_async(dispatch_get_main_queue(), ^{ [self.delegate videoDecoderDidFail:self]; });
        return;
    }
    // Ask the server for an early IDR rather than wait up to 2s for the next
    // scheduled one. Skipped on the give-up path above — the lane is about to
    // fall back to JPEG, so a restart would just be wasted server work.
    dispatch_async(dispatch_get_main_queue(), ^{ [self.delegate videoDecoderNeedsKeyframe:self]; });
}

- (BOOL)ensureSessionForAU:(const uint8_t *)bytes length:(size_t)len info:(rb_au_info *)info {
    if (!info->sps || !info->pps) return _session != NULL;
    NSData *sps = [NSData dataWithBytes:info->sps length:info->sps_len];
    NSData *pps = [NSData dataWithBytes:info->pps length:info->pps_len];
    if (_session && [sps isEqualToData:self.currentSPS] && [pps isEqualToData:self.currentPPS]) {
        return YES; // repeat-headers=1 resends identical sets every IDR
    }
    [self teardownSession];

    uint8_t avcc[512];
    size_t avccLen = rb_avcc_build(info->sps, info->sps_len, info->pps, info->pps_len, avcc, sizeof avcc);
    if (!avccLen) return NO;

    // Dimensions from the wire header would work too, but the caller passes
    // them per-AU; use what the stream itself declares via the server config.
    // (w/h only affect CGImage metadata; the decoder reads the SPS.)
    CFDataRef avccData = CFDataCreate(NULL, avcc, (CFIndex)avccLen);
    CFStringRef avcCKey = CFSTR("avcC");
    CFDictionaryRef atoms = CFDictionaryCreate(NULL, (const void **)&avcCKey, (const void **)&avccData, 1,
                                               &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFStringRef extKey = kCMFormatDescriptionExtension_SampleDescriptionExtensionAtoms;
    CFDictionaryRef extensions = CFDictionaryCreate(NULL, (const void **)&extKey, (const void **)&atoms, 1,
                                                    &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    OSStatus status = CMVideoFormatDescriptionCreate(NULL, kCMVideoCodecType_H264,
                                                     self.codedWidth, self.codedHeight, extensions, &_format);
    CFRelease(extensions);
    CFRelease(atoms);
    CFRelease(avccData);
    if (status != noErr || !_format) {
        RBLog(@"video: CMVideoFormatDescriptionCreate failed: %d", (int)status);
        return NO;
    }

    // BGRA out: the decoder does YUV→RGB for us and rendering stays trivial.
    int32_t bgra = kCVPixelFormatType_32BGRA;
    CFNumberRef pixfmt = CFNumberCreate(NULL, kCFNumberSInt32Type, &bgra);
    CFStringRef pixfmtKey = kCVPixelBufferPixelFormatTypeKey;
    CFDictionaryRef destAttrs = CFDictionaryCreate(NULL, (const void **)&pixfmtKey, (const void **)&pixfmt, 1,
                                                   &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFRelease(pixfmt);

    VTDecompressionOutputCallbackRecord record;
    record.decompressionOutputCallback = RBDecodeCallback;
    record.decompressionOutputRefCon = (__bridge void *)self;

    VTDecompressionSessionRef session = NULL;
    status = rbVTCreate(NULL, _format, NULL, destAttrs, &record, &session);
    CFRelease(destAttrs);
    if (status != noErr || !session) {
        RBLog(@"video: VTDecompressionSessionCreate failed: %d", (int)status);
        CFRelease(_format);
        _format = NULL;
        return NO;
    }
    _session = session;
    self.currentSPS = sps;
    self.currentPPS = pps;
    RBLog(@"video: VT session up (sps %u bytes, pps %u bytes)", (unsigned)info->sps_len, (unsigned)info->pps_len);
    return YES;
}

- (void)decodeAU:(NSData *)au idr:(BOOL)idr {
    if (self.failed) return;
    if (self.waitingForIDR && !idr) return;

    const uint8_t *bytes = (const uint8_t *)[au bytes];
    size_t len = [au length];
    rb_au_info info;
    if (rb_au_scan(bytes, len, &info) != 0 || !info.has_slice) return;

    if (![self ensureSessionForAU:bytes length:len info:&info]) {
        self.decodeErrors++;
        [self noteResync];
        return;
    }

    // Annex-B → AVCC in a malloc'd block whose ownership transfers to the
    // CMBlockBuffer (no copy).
    size_t cap = len + 64;
    uint8_t *avccBuf = malloc(cap);
    if (!avccBuf) return;
    size_t avccLen = rb_au_to_avcc(bytes, len, avccBuf, cap);
    if (!avccLen) {
        free(avccBuf);
        return;
    }

    CMBlockBufferRef block = NULL;
    OSStatus status = CMBlockBufferCreateWithMemoryBlock(NULL, avccBuf, avccLen, kCFAllocatorMalloc,
                                                         NULL, 0, avccLen, 0, &block);
    if (status != noErr) {
        free(avccBuf);
        self.decodeErrors++;
        return;
    }
    CMSampleBufferRef sample = NULL;
    size_t sampleSize = avccLen;
    status = CMSampleBufferCreate(NULL, block, true, NULL, NULL, _format,
                                  1, 0, NULL, 1, &sampleSize, &sample);
    CFRelease(block);
    if (status != noErr || !sample) {
        self.decodeErrors++;
        return;
    }

    VTDecodeInfoFlags flagsOut = 0;
    RBFrameTiming *timing = malloc(sizeof(RBFrameTiming));
    if (timing) timing->submittedAt = CACurrentMediaTime();
    CFTimeInterval submitStart = CACurrentMediaTime();
    status = rbVTDecode(_session, sample, 0 /* sync — fine at 15fps */, timing, &flagsOut);
    double submitMS = (CACurrentMediaTime() - submitStart) * 1000.0;
    CFRelease(sample);

    if (status != noErr && timing) free(timing);

    if (status == kVTInvalidSessionErr) {
        // Classic after app resume: session died under us. Rebuild at the
        // next IDR (repeat-headers guarantees fresh SPS/PPS there).
        RBLog(@"video: VT session invalidated, rebuilding at next IDR");
        [self teardownSession];
        self.currentSPS = nil;
        self.currentPPS = nil;
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
    self.submittedAUs++;
    self.lastSubmitMS = submitMS;
    self.averageSubmitMS = self.averageSubmitMS <= 0.0 ? submitMS : self.averageSubmitMS * 0.85 + submitMS * 0.15;
    self.decodedFrames++;
}

@end
