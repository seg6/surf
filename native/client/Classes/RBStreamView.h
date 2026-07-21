#import <UIKit/UIKit.h>

// Shows the remote page. Base = whichever lane is live (JPEG frames or
// decoded video); overlay = the sharp settle JPEG shown on top of video
// until the user next touches.
@interface RBStreamView : UIView
@property(nonatomic, assign) BOOL videoActive;

// JPEG lane / sharp frame. In video mode this goes to the overlay.
- (void)displayImage:(UIImage *)image width:(NSUInteger)width height:(NSUInteger)height;
// Video lane: decoded frame onto the base layer.
- (void)displayVideoImage:(CGImageRef)image;
- (void)hideSharpOverlay;
@end
