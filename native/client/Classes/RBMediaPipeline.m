#import "RBMediaPipeline.h"
#import "RBAudioPlayer.h"
#import "RBLog.h"
#import "RBProtocol.h"
#import "RBVideoDecoder.h"

@interface RBMediaPipeline () <RBVideoDecoderDelegate>
@property(nonatomic, strong) RBVideoDecoder *decoder;
@property(nonatomic, strong) RBAudioPlayer *audio;
@property(nonatomic, assign, readwrite, getter=isVideoActive) BOOL videoActive;
@property(nonatomic, assign, readwrite) NSUInteger videoAUs;
@property(nonatomic, assign, readwrite) NSUInteger sequenceGaps;
@property(nonatomic, assign) unsigned int expectedAUSequence;
@end

@implementation RBMediaPipeline

+ (BOOL)videoAvailable { return [RBVideoDecoder available]; }

- (id)init {
    self = [super init];
    if (self) self.audio = [[RBAudioPlayer alloc] init];
    return self;
}

- (void)configureVideoWidth:(int)width height:(int)height {
    if (!self.decoder) {
        self.decoder = [[RBVideoDecoder alloc] init];
        self.decoder.delegate = self;
    }
    self.decoder.codedWidth = width > 0 ? width : 1024;
    self.decoder.codedHeight = height > 0 ? height : 768;
    [self.decoder reset];
    self.videoAUs = 0;
    self.expectedAUSequence = 0;
    self.videoActive = YES;
    RBLog(@"video: lane up %dx%d", self.decoder.codedWidth, self.decoder.codedHeight);
}

- (void)stopVideo {
    self.videoActive = NO;
    [self.decoder reset];
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
        RBLog(@"bad frame: %@", error);
        return;
    }
    if (frame.type == 3) {
        if (!self.videoActive) return;
        if (self.expectedAUSequence && frame.seq != self.expectedAUSequence) self.sequenceGaps++;
        self.expectedAUSequence = frame.seq + 1;
        if (frame.width > 0 && frame.height > 0 &&
            (self.decoder.codedWidth != frame.width || self.decoder.codedHeight != frame.height)) {
            self.decoder.codedWidth = frame.width;
            self.decoder.codedHeight = frame.height;
            [self.decoder reset];
            self.videoAUs = 0;
            RBLog(@"video: coded size changed to %dx%d", self.decoder.codedWidth, self.decoder.codedHeight);
        }
        self.videoAUs++;
        [self.decoder feedAU:frame.payload idr:(frame.flags & 1) != 0
                    metadata:[RBFrameMetadata metadataFromFrame:frame]];
    } else if (frame.type == 4) {
        [self.audio playPCM:frame.payload sequence:frame.seq];
    }
}

- (void)videoDecoder:(RBVideoDecoder *)decoder didDecodeImage:(CGImageRef)image
            metadata:(RBFrameMetadata *)metadata {
    if (self.videoActive) [self.delegate mediaPipeline:self didDecodeImage:image metadata:metadata];
}

- (void)videoDecoderDidFail:(RBVideoDecoder *)decoder {
    self.videoActive = NO;
    [self.delegate mediaPipelineDidFailVideo:self];
}

- (void)videoDecoderNeedsKeyframe:(RBVideoDecoder *)decoder {
    [self.delegate mediaPipelineNeedsKeyframe:self];
}

- (NSUInteger)decodedFrames { return self.decoder.decodedFrames; }
- (NSUInteger)decodeErrors { return self.decoder.decodeErrors; }
- (NSUInteger)submittedAUs { return self.decoder.submittedAUs; }
- (NSUInteger)callbackFrames { return self.decoder.callbackFrames; }
- (NSUInteger)droppedAUs { return self.decoder.droppedAUs; }
- (int)queuedAUs { return self.decoder.queuedAUs; }
- (double)averageSubmitMS { return self.decoder.averageSubmitMS; }
- (double)averageCallbackMS { return self.decoder.averageCallbackMS; }
- (double)averageWrapMS { return self.decoder.averageWrapMS; }
- (NSUInteger)audioDroppedPCM { return self.audio.droppedPCM; }
- (NSUInteger)audioUnderruns { return self.audio.underruns; }
- (NSUInteger)audioRestartCount { return self.audio.restartCount; }
- (int)audioQueuedBuffers { return self.audio.queuedBuffers; }

@end
