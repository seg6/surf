#import <UIKit/UIKit.h>
#import <CoreVideo/CoreVideo.h>

@class RBStreamView;
@class RBFrameMetadata;
@protocol RBStreamViewDelegate <NSObject>
- (void)streamView:(RBStreamView *)streamView didPresentMetadata:(RBFrameMetadata *)metadata;
- (void)streamView:(RBStreamView *)streamView touchesBegan:(NSSet *)touches withEvent:(UIEvent *)event;
- (void)streamView:(RBStreamView *)streamView touchesMoved:(NSSet *)touches withEvent:(UIEvent *)event;
- (void)streamView:(RBStreamView *)streamView touchesEnded:(NSSet *)touches withEvent:(UIEvent *)event;
- (void)streamView:(RBStreamView *)streamView touchesCancelled:(NSSet *)touches withEvent:(UIEvent *)event;
@end

// Shows the latest decoded H.264 image on a display-link boundary.
@interface RBStreamView : UIView
@property(nonatomic, weak) id<RBStreamViewDelegate> presentationDelegate;
@property(nonatomic, assign) BOOL videoActive;
@property(nonatomic, readonly) NSUInteger presentedFrames;
@property(nonatomic, readonly) NSUInteger overwrittenVideoFrames;
@property(nonatomic, readonly) CFTimeInterval lastPresentationAt;
// Unique source images presented during the trailing one-second window.
// Repeated recovery/keyframe AUs do not inflate this value.
@property(nonatomic, readonly) double uniquePresentationFPS;
@property(nonatomic, readonly) unsigned int lastPresentedSourceSequence;

- (void)displayVideoPixelBuffer:(CVPixelBufferRef)pixelBuffer metadata:(RBFrameMetadata *)metadata;
// Converts the retained current NV12 frame into a small CPU-side image. This
// is intentionally on-demand for the classic phone Pages switcher.
- (UIImage *)snapshotImageWithMaximumSize:(CGSize)maximumSize;
- (void)beginMotionWindow;
- (void)continueMotionWindow;
- (void)endMotionWindow;
- (double)consumeRecentMaximumPresentationGapMS;
@end
