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

// Device-specific Safari top chrome. Phones show the unified omnibox while
// iPads show navigation and browser controls around it. Phone page titles live
// in Tabs, where they remain readable without crowding the address field.
@interface RBChromeBar : RBGradientBar
@property(nonatomic, assign) id<RBChromeBarDelegate> delegate;
@property(nonatomic, readonly) RBOmnibox *omnibox;
@property(nonatomic, readonly) UIButton *moreButton;
@property(nonatomic, readonly) UIButton *shareButton;
@property(nonatomic, readonly) UIButton *libraryButton;
@property(nonatomic, assign, getter=isPhoneLayout) BOOL phoneLayout;
@property(nonatomic, copy) NSString *pageTitle;

- (void)setCanGoBack:(BOOL)back forward:(BOOL)forward;
@end
