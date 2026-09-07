#import <UIKit/UIKit.h>

@interface RBRootViewController : UIViewController

// Navigate the remote browser (URL scheme, pasteboard banner, reader links).
- (void)openURLString:(NSString *)url;
- (void)openPairingURL:(NSURL *)url;
// Offer to open a URL sitting on the pasteboard (called on app activation).
- (void)checkPasteboard;
- (void)syncNativeLog;
// Quiesce media immediately in the background, then disconnect after the
// short return-to-app grace period. A later activation reconnects as needed.
- (void)applicationDidBecomeActive;
- (void)applicationDidEnterBackground;
@end
