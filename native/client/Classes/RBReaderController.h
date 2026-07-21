#import <UIKit/UIKit.h>

// Reader mode (M1.5): the server extracts the article, we render it in a
// local UIWebView — local scroll, local selection, real typography. The
// stream (and its battery cost) idles while this is open.
@interface RBReaderController : UIViewController

@property(nonatomic, copy) void (^onDismiss)(void);

- (id)initWithTitle:(NSString *)title html:(NSString *)html url:(NSString *)url;
@end
