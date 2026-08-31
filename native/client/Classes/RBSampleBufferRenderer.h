#import <Foundation/Foundation.h>
#import <QuartzCore/QuartzCore.h>

@class AVSampleBufferDisplayLayer;
@class RBFrameMetadata;
@class RBSampleBufferRenderer;

@protocol RBSampleBufferRendererDelegate <NSObject>
- (void)sampleBufferRenderer:(RBSampleBufferRenderer *)renderer
           didAcceptMetadata:(RBFrameMetadata *)metadata;
- (void)sampleBufferRenderer:(RBSampleBufferRenderer *)renderer
      didReplaceDisplayLayer:(CALayer *)displayLayer;
- (void)sampleBufferRendererNeedsKeyframe:(RBSampleBufferRenderer *)renderer;
- (void)sampleBufferRendererDidFail:(RBSampleBufferRenderer *)renderer;
@end

// Live H.264 renderer backed by AVSampleBufferDisplayLayer. iOS 6.1 already
// ships the core queue privately, while iOS 8 adds public failure reporting.
// Runtime capability checks select either form without an OS-version guess.
// Surf never owns the decoded IOSurfaces and UIKit never participates in the
// per-frame data path.
@interface RBSampleBufferRenderer : NSObject
@property(nonatomic, weak) id<RBSampleBufferRendererDelegate> delegate;
@property(nonatomic, strong, readonly) AVSampleBufferDisplayLayer *displayLayer;
@property(nonatomic, readonly) NSUInteger acceptedFrames;
@property(nonatomic, readonly) NSUInteger droppedAUs;
@property(nonatomic, readonly) NSUInteger backpressureEvents;
@property(nonatomic, readonly) NSUInteger recoveries;
@property(nonatomic, readonly) NSUInteger failures;
@property(nonatomic, readonly) int queuedAUs;
@property(nonatomic, readonly) double averageEnqueueMS;
@property(nonatomic, readonly) CFTimeInterval lastAcceptedAt;

+ (BOOL)available;
- (void)configureWidth:(int)width height:(int)height;
- (void)feedAU:(NSData *)au idr:(BOOL)idr metadata:(RBFrameMetadata *)metadata;
- (void)reset;
- (void)stop;
@end
