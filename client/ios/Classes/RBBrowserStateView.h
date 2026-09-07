#import <UIKit/UIKit.h>

typedef enum {
    RBBrowserStateHidden,
    RBBrowserStateConnecting,
    RBBrowserStateStartingVideo,
    RBBrowserStateReconnecting,
    RBBrowserStateDisconnected,
    RBBrowserStatePageError,
    RBBrowserStateVideoUnavailable
} RBBrowserState;

@class RBBrowserStateView;

@protocol RBBrowserStateViewDelegate <NSObject>
- (void)browserStateViewPrimaryAction:(RBBrowserStateView *)view;
- (void)browserStateViewSecondaryAction:(RBBrowserStateView *)view;
@end

@interface RBBrowserStateView : UIView
@property(nonatomic, assign) id<RBBrowserStateViewDelegate> delegate;
@property(nonatomic, assign) RBBrowserState state;
- (void)showState:(RBBrowserState)state detail:(NSString *)detail;
- (void)applyAppearance;
@end
