#import <UIKit/UIKit.h>

// Public UIKit customization point for iOS 6, which has no popover color API.
// Light mode and iOS 7+ use UIKit's stock background view instead.
@interface RBDarkPopoverBackgroundView : UIPopoverBackgroundView
@end
