#import <UIKit/UIKit.h>

@class RBMediaController;

@protocol RBMediaControllerDelegate <NSObject>
- (void)mediaControllerTogglePlayback:(RBMediaController *)controller;
- (void)mediaControllerToggleMute:(RBMediaController *)controller;
- (void)mediaController:(RBMediaController *)controller setVolume:(CGFloat)volume;
- (void)mediaControllerRequestsRefresh:(RBMediaController *)controller;
@end

// Compact, stateful controls for media in the active Chromium tab.
@interface RBMediaController : UIViewController
@property(nonatomic, assign) id<RBMediaControllerDelegate> delegate;
- (void)applyState:(NSDictionary *)state;
@end
