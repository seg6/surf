#import <UIKit/UIKit.h>

#import "RBOmnibox.h"
#import "RBTheme.h"

@class RBChromeBar;

@protocol RBChromeBarDelegate <NSObject>
- (void)chromeBack:(RBChromeBar *)bar;
- (void)chromeForward:(RBChromeBar *)bar;
// Page actions (more icon): Reader, Find, Paste, sharing, Fullscreen.
- (void)chrome:(RBChromeBar *)bar actionsFromButton:(UIButton *)button;
// Library (book icon): History | Bookmarks | Downloads.
- (void)chrome:(RBChromeBar *)bar libraryFromButton:(UIButton *)button;
// Settings (gear icon): opens settings directly — there is no menu.
- (void)chromeSettings:(RBChromeBar *)bar;
@end

// The Safari-style top bar: back/forward, unified omnibox, then page actions,
// library, and settings.
@interface RBChromeBar : RBGradientBar
@property(nonatomic, assign) id<RBChromeBarDelegate> delegate;
@property(nonatomic, readonly) RBOmnibox *omnibox;
@property(nonatomic, readonly) UIButton *actionButton;
@property(nonatomic, readonly) UIButton *libraryButton;

- (void)setCanGoBack:(BOOL)back forward:(BOOL)forward;
@end
