#import <UIKit/UIKit.h>

@class RBFindBar;

@protocol RBFindBarDelegate <NSObject>
- (void)findBar:(RBFindBar *)bar search:(NSString *)query direction:(NSInteger)direction;
- (void)findBarDone:(RBFindBar *)bar;
@end

// Find-on-page bar: query field, prev/next, result state, Done.
@interface RBFindBar : UIView
@property(nonatomic, assign) id<RBFindBarDelegate> delegate;

- (void)focusField;
- (void)setFound:(BOOL)found;
@end
