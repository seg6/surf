#import <UIKit/UIKit.h>

#import "RBOmnibox.h"
#import "RBTheme.h"

@class RBChromeBar;

@protocol RBChromeBarDelegate <NSObject>
- (void)chromeBack:(RBChromeBar *)bar;
- (void)chromeForward:(RBChromeBar *)bar;
- (void)chromeKeyboard:(RBChromeBar *)bar;
- (void)chrome:(RBChromeBar *)bar menuFromButton:(UIButton *)button;
@end

// The Safari-style top bar: back/forward, unified omnibox, keyboard, menu.
@interface RBChromeBar : RBGradientBar
@property(nonatomic, assign) id<RBChromeBarDelegate> delegate;
@property(nonatomic, readonly) RBOmnibox *omnibox;
@property(nonatomic, readonly) UIButton *menuButton;

- (void)setCanGoBack:(BOOL)back forward:(BOOL)forward;
@end
