#import <UIKit/UIKit.h>

#import "RBTheme.h"

@class RBPhoneToolbar;

@protocol RBPhoneToolbarDelegate <NSObject>
- (void)phoneToolbarBack:(RBPhoneToolbar *)toolbar;
- (void)phoneToolbarForward:(RBPhoneToolbar *)toolbar;
- (void)phoneToolbar:(RBPhoneToolbar *)toolbar shareFromButton:(UIButton *)button;
- (void)phoneToolbar:(RBPhoneToolbar *)toolbar pagesFromButton:(UIButton *)button;
- (void)phoneToolbar:(RBPhoneToolbar *)toolbar moreFromButton:(UIButton *)button;
@end

@interface RBPhoneToolbar : RBGradientBar
@property(nonatomic, assign) id<RBPhoneToolbarDelegate> delegate;
@property(nonatomic, readonly) UIButton *shareButton;
@property(nonatomic, readonly) UIButton *pagesButton;
@property(nonatomic, readonly) UIButton *moreButton;

- (void)setCanGoBack:(BOOL)back forward:(BOOL)forward;
- (void)setTabCount:(NSUInteger)count;
- (void)applyAppearance;
@end
