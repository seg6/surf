#import "RBMediaPipeline.h"
#import "RBAudioPlayer.h"
#import "RBLog.h"
#import "RBProtocol.h"
#import "RBSampleBufferRenderer.h"
#import "RBVideoDecoder.h"

@interface RBMediaPipeline () <RBVideoDecoderDelegate, RBSampleBufferRendererDelegate>
@property(nonatomic, strong) RBVideoDecoder *decoder;
@property(nonatomic, strong) RBSampleBufferRenderer *sampleRenderer;
@property(nonatomic, strong) RBAudioPlayer *audio;
@property(nonatomic, assign, readwrite, getter=isVideoActive) BOOL videoActive;
@property(nonatomic, assign, readwrite) NSUInteger videoAUs;
@property(nonatomic, assign, readwrite) NSUInteger sequenceGaps;
@property(nonatomic, assign) unsigned int expectedAUSequence;
@property(nonatomic, assign) int codedWidth;
@property(nonatomic, assign) int codedHeight;
@end

@implementation RBMediaPipeline

+ (BOOL)videoAvailable {
    return [RBSampleBufferRenderer available] || [RBVideoDecoder available];
}

- (id)init {
    self = [super init];
    if (self) {
        self.audio = [[RBAudioPlayer alloc] init];
        if ([RBSampleBufferRenderer available]) {
            self.sampleRenderer = [[RBSampleBufferRenderer alloc] init];
            self.sampleRenderer.delegate = self;
        }
    }
    return self;
}

- (BOOL)configureLegacyDecoder {
    if (![RBVideoDecoder available]) return NO;
    if (!self.decoder) {
        self.decoder = [[RBVideoDecoder alloc] init];
        self.decoder.delegate = self;
    }
    if (!self.decoder) return NO;
    self.decoder.codedWidth = self.codedWidth;
    self.decoder.codedHeight = self.codedHeight;
    [self.decoder reset];
    return YES;
}

- (void)configureVideoWidth:(int)width height:(int)height {
    self.codedWidth = width > 0 ? width : 1024;
    self.codedHeight = height > 0 ? height : 768;
    if (self.sampleRenderer) {
        [self.sampleRenderer configureWidth:self.codedWidth height:self.codedHeight];
    } else if (![self configureLegacyDecoder]) {
        self.videoActive = NO;
        RBLogEvent(@"media", @"error", @{@"lane": @"video"},
                   @"No usable video renderer is available");
        [self.delegate mediaPipelineDidFailVideo:self];
        return;
    }
    self.videoAUs = 0;
    self.expectedAUSequence = 0;
    self.videoActive = YES;
    RBLogEvent(@"media", @"info", @{@"lane": @"video", @"state": @"ready",
               @"coded_width": @(self.codedWidth), @"coded_height": @(self.codedHeight),
               @"renderer": self.rendererMode}, @"Video lane ready");
}

- (void)stopVideo {
    self.videoActive = NO;
    [self.sampleRenderer stop];
    [self.decoder reset];
}

- (void)recoverVideo {
    if (!self.videoActive) return;
    if (self.sampleRenderer) [self.sampleRenderer reset];
    else [self.decoder reset];
}

- (void)configureAudioSampleRate:(int)sampleRate channels:(int)channels {
    [self.audio configureSampleRate:sampleRate channels:channels];
}

- (void)stopAudio { [self.audio stop]; }
- (void)stop { [self stopVideo]; [self stopAudio]; }

- (void)consumeFrameData:(NSData *)data {
    NSString *error = nil;
    RBFrame *frame = [RBProtocol frameFromData:data error:&error];
    if (!frame) {
        RBLogEvent(@"protocol", @"error", @{@"error": error ?: @""}, @"Invalid media frame received");
        return;
    }
    if (frame.type == 3) {
        if (!self.videoActive) return;
        if (self.expectedAUSequence && frame.seq != self.expectedAUSequence) self.sequenceGaps++;
        self.expectedAUSequence = frame.seq + 1;
        if (frame.width > 0 && frame.height > 0 &&
            (self.codedWidth != frame.width || self.codedHeight != frame.height)) {
            self.codedWidth = frame.width;
            self.codedHeight = frame.height;
            if (self.sampleRenderer)
                [self.sampleRenderer configureWidth:frame.width height:frame.height];
            else {
                self.decoder.codedWidth = frame.width;
                self.decoder.codedHeight = frame.height;
                [self.decoder reset];
            }
            self.videoAUs = 0;
            RBLogEvent(@"media", @"info", @{@"coded_width": @(self.codedWidth),
                       @"coded_height": @(self.codedHeight), @"renderer": self.rendererMode},
                       @"Video coded size changed");
        }
        self.videoAUs++;
        RBFrameMetadata *metadata = [RBFrameMetadata metadataFromFrame:frame];
        if (self.sampleRenderer)
            [self.sampleRenderer feedAU:frame.payload idr:(frame.flags & 1) != 0 metadata:metadata];
        else
            [self.decoder feedAU:frame.payload idr:(frame.flags & 1) != 0 metadata:metadata];
    } else if (frame.type == 4) {
        [self.audio playPCM:frame.payload sequence:frame.seq];
    }
}

- (void)videoDecoder:(RBVideoDecoder *)decoder didDecodePixelBuffer:(CVPixelBufferRef)pixelBuffer
            metadata:(RBFrameMetadata *)metadata {
    if (self.videoActive) [self.delegate mediaPipeline:self didDecodePixelBuffer:pixelBuffer metadata:metadata];
}

- (void)videoDecoderDidFail:(RBVideoDecoder *)decoder {
    self.videoActive = NO;
    [self.delegate mediaPipelineDidFailVideo:self];
}

- (void)videoDecoderNeedsKeyframe:(RBVideoDecoder *)decoder {
    [self.delegate mediaPipelineNeedsKeyframe:self];
}

- (void)sampleBufferRenderer:(RBSampleBufferRenderer *)renderer
           didAcceptMetadata:(RBFrameMetadata *)metadata {
    if (self.videoActive)
        [self.delegate mediaPipeline:self didAcceptSystemFrame:metadata];
}

- (void)sampleBufferRenderer:(RBSampleBufferRenderer *)renderer
      didReplaceDisplayLayer:(CALayer *)displayLayer {
    [self.delegate mediaPipeline:self didReplaceSystemDisplayLayer:displayLayer];
}

- (void)sampleBufferRendererNeedsKeyframe:(RBSampleBufferRenderer *)renderer {
    [self.delegate mediaPipelineNeedsKeyframe:self];
}

- (void)sampleBufferRendererDidFail:(RBSampleBufferRenderer *)renderer {
    if (renderer != self.sampleRenderer) return;
    [renderer stop];
    renderer.delegate = nil;
    self.sampleRenderer = nil;
    [self.delegate mediaPipeline:self didReplaceSystemDisplayLayer:nil];
    if (![self configureLegacyDecoder]) {
        self.videoActive = NO;
        [self.delegate mediaPipelineDidFailVideo:self];
        return;
    }
    RBLogEvent(@"media", @"warn", @{@"renderer": @"legacy-gl"},
               @"Fell back from the system renderer to VideoToolbox and OpenGL");
    [self.delegate mediaPipelineNeedsKeyframe:self];
}

- (NSString *)rendererMode { return self.sampleRenderer ? @"system" : @"legacy-gl"; }
- (CALayer *)systemDisplayLayer { return (CALayer *)self.sampleRenderer.displayLayer; }
- (NSUInteger)rendererFrames { return self.sampleRenderer ? self.sampleRenderer.acceptedFrames : self.decoder.decodedFrames; }
- (NSUInteger)rendererBackpressureEvents { return self.sampleRenderer.backpressureEvents; }
- (NSUInteger)rendererRecoveries { return self.sampleRenderer.recoveries; }
- (NSUInteger)rendererFailures { return self.sampleRenderer.failures; }
- (double)averageRendererMS { return self.sampleRenderer ? self.sampleRenderer.averageEnqueueMS : self.decoder.averageSubmitMS; }
- (NSUInteger)decodedFrames { return self.sampleRenderer ? self.sampleRenderer.acceptedFrames : self.decoder.decodedFrames; }
- (NSUInteger)decodeErrors { return self.sampleRenderer ? self.sampleRenderer.failures : self.decoder.decodeErrors; }
- (NSUInteger)submittedAUs { return self.sampleRenderer ? self.sampleRenderer.acceptedFrames : self.decoder.submittedAUs; }
- (NSUInteger)callbackFrames { return self.sampleRenderer ? self.sampleRenderer.acceptedFrames : self.decoder.callbackFrames; }
- (NSUInteger)droppedAUs { return self.sampleRenderer ? self.sampleRenderer.droppedAUs : self.decoder.droppedAUs; }
- (int)queuedAUs { return self.sampleRenderer ? self.sampleRenderer.queuedAUs : self.decoder.queuedAUs; }
- (double)averageSubmitMS { return self.sampleRenderer ? self.sampleRenderer.averageEnqueueMS : self.decoder.averageSubmitMS; }
- (double)averageCallbackMS { return self.sampleRenderer ? 0.0 : self.decoder.averageCallbackMS; }
- (double)averageHandoffMS { return self.sampleRenderer ? 0.0 : self.decoder.averageHandoffMS; }
- (NSUInteger)audioDroppedPCM { return self.audio.droppedPCM; }
- (NSUInteger)audioUnderruns { return self.audio.underruns; }
- (NSUInteger)audioRestartCount { return self.audio.restartCount; }
- (int)audioQueuedBuffers { return self.audio.queuedBuffers; }

@end
