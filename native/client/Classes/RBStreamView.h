#import <UIKit/UIKit.h>

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

- (void)displayVideoImage:(CGImageRef)image metadata:(RBFrameMetadata *)metadata;
@end
