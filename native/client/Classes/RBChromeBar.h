#import <UIKit/UIKit.h>

#import "RBOmnibox.h"
#import "RBTheme.h"

@class RBChromeBar;

@protocol RBChromeBarDelegate <NSObject>
- (void)chromeBack:(RBChromeBar *)bar;
- (void)chromeForward:(RBChromeBar *)bar;
- (void)chrome:(RBChromeBar *)bar shareFromButton:(UIButton *)button;
- (void)chrome:(RBChromeBar *)bar moreFromButton:(UIButton *)button;
- (void)chrome:(RBChromeBar *)bar libraryFromButton:(UIButton *)button;
@end

// Device-specific browser chrome. Phones show the unified omnibox while iPads
// use one desktop-style rail: navigation and address on the left, tabs in the
// middle, and browser actions on the right. Phone page titles live in Tabs,
// where they remain readable without crowding the address field.
@interface RBChromeBar : RBGradientBar
@property(nonatomic, assign) id<RBChromeBarDelegate> delegate;
@property(nonatomic, readonly) RBOmnibox *omnibox;
@property(nonatomic, readonly) UIButton *moreButton;
@property(nonatomic, readonly) UIButton *shareButton;
@property(nonatomic, readonly) UIButton *libraryButton;
// RBRootViewController installs the persistent iPad tab strip here. Keeping
// the host inside the chrome makes address, tabs, and actions respond as one
// surface during rotation without coupling this view to tab data.
@property(nonatomic, readonly) UIView *tabHostView;
@property(nonatomic, assign, getter=isPhoneLayout) BOOL phoneLayout;
@property(nonatomic, assign, getter=isBottomPositioned) BOOL bottomPositioned;
@property(nonatomic, copy) NSString *pageTitle;

- (void)setCanGoBack:(BOOL)back forward:(BOOL)forward;
- (void)setOmniboxExpanded:(BOOL)expanded animated:(BOOL)animated;
- (void)applyAppearance;
@end
