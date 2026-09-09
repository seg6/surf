#import <UIKit/UIKit.h>

typedef enum {
    RBBrowserStateHidden,
    RBBrowserStateConnecting,
    RBBrowserStateStartingVideo,
    RBBrowserStateReconnecting,
    RBBrowserStateDisconnected,
    RBBrowserStatePageError,
    RBBrowserStateVideoUnavailable,
    RBBrowserStateSetup,
    RBBrowserStateSetupClosing,
    RBBrowserStateSetupForce
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
