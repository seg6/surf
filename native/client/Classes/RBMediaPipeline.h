#import <Foundation/Foundation.h>
#import <CoreVideo/CoreVideo.h>
#import <CoreGraphics/CoreGraphics.h>

@class RBMediaPipeline;
@class RBFrameMetadata;

@protocol RBMediaPipelineDelegate <NSObject>
- (void)mediaPipeline:(RBMediaPipeline *)pipeline didDecodePixelBuffer:(CVPixelBufferRef)pixelBuffer
             metadata:(RBFrameMetadata *)metadata;
- (void)mediaPipelineDidFailVideo:(RBMediaPipeline *)pipeline;
- (void)mediaPipelineNeedsKeyframe:(RBMediaPipeline *)pipeline;
@end

// Owns all binary media state. Socket delivery may call consumeFrameData from
// its read thread; VideoToolbox and AudioQueue each retain their own serialized
// execution internally. UIKit remains entirely in the delegate.
@interface RBMediaPipeline : NSObject
@property(nonatomic, weak) id<RBMediaPipelineDelegate> delegate;
@property(nonatomic, assign, readonly, getter=isVideoActive) BOOL videoActive;
@property(nonatomic, assign, readonly) NSUInteger videoAUs;
@property(nonatomic, assign, readonly) NSUInteger decodedFrames;
@property(nonatomic, assign, readonly) NSUInteger decodeErrors;
@property(nonatomic, assign, readonly) NSUInteger submittedAUs;
@property(nonatomic, assign, readonly) NSUInteger callbackFrames;
@property(nonatomic, assign, readonly) NSUInteger droppedAUs;
@property(nonatomic, assign, readonly) NSUInteger sequenceGaps;
@property(nonatomic, assign, readonly) int queuedAUs;
@property(nonatomic, assign, readonly) double averageSubmitMS;
@property(nonatomic, assign, readonly) double averageCallbackMS;
@property(nonatomic, assign, readonly) double averageWrapMS;
@property(nonatomic, assign, readonly) NSUInteger audioDroppedPCM;
@property(nonatomic, assign, readonly) NSUInteger audioUnderruns;
@property(nonatomic, assign, readonly) NSUInteger audioRestartCount;
@property(nonatomic, assign, readonly) int audioQueuedBuffers;

+ (BOOL)videoAvailable;
- (void)configureVideoWidth:(int)width height:(int)height;
- (void)stopVideo;
- (void)configureAudioSampleRate:(int)sampleRate channels:(int)channels;
- (void)stopAudio;
- (void)stop;
- (void)consumeFrameData:(NSData *)data;
@end
