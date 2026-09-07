#import <UIKit/UIKit.h>

@class RBNewTabView;

@protocol RBNewTabViewDelegate <NSObject>
- (void)newTabViewWantsOmnibox:(RBNewTabView *)view;
- (void)newTabView:(RBNewTabView *)view openURL:(NSString *)url;
- (void)newTabViewWantsLibrary:(RBNewTabView *)view;
@end

// Instant native surface shown for about:blank#surf-new. Chromium remains on a
// settled blank document until the user chooses a destination.
@interface RBNewTabView : UIView
@property(nonatomic, assign) id<RBNewTabViewDelegate> delegate;
- (void)setFavorites:(NSArray *)favorites;
- (void)applyAppearance;
@end
