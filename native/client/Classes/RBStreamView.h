#import <UIKit/UIKit.h>
#import <CoreVideo/CoreVideo.h>

@class RBStreamView;
@class RBFrameMetadata;
@protocol RBStreamViewDelegate <NSObject>
- (void)streamView:(RBStreamView *)streamView didPresentMetadata:(RBFrameMetadata *)metadata;
@end

// Shows the latest decoded H.264 image on a display-link boundary.
@interface RBStreamView : UIView
@property(nonatomic, weak) id<RBStreamViewDelegate> presentationDelegate;
@property(nonatomic, assign) BOOL videoActive;
@property(nonatomic, readonly) NSUInteger presentedFrames;
@property(nonatomic, readonly) NSUInteger overwrittenVideoFrames;
@property(nonatomic, readonly) CFTimeInterval lastPresentationAt;
@property(nonatomic, readonly) double maximumPresentationGapMS;
@property(nonatomic, readonly) double motionPresentationFPS;
@property(nonatomic, readonly) unsigned int lastPresentedSourceSequence;

- (void)displayVideoPixelBuffer:(CVPixelBufferRef)pixelBuffer metadata:(RBFrameMetadata *)metadata;
- (void)beginMotionWindow;
- (void)continueMotionWindow;
- (void)endMotionWindow;
- (double)consumeRecentMaximumPresentationGapMS;
@end
