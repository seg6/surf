#import <UIKit/UIKit.h>

@interface RBRootViewController : UIViewController

// Navigate the remote browser (URL scheme, pasteboard banner, reader links).
- (void)openURLString:(NSString *)url;
// Offer to open a URL sitting on the pasteboard (called on app activation).
- (void)checkPasteboard;
@end
