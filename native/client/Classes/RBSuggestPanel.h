#import <UIKit/UIKit.h>

@class RBSuggestPanel;

@protocol RBSuggestPanelDelegate <NSObject>
- (void)suggestPanel:(RBSuggestPanel *)panel pickedURL:(NSString *)url;
@end

// Dropdown of history/bookmark matches shown under the omnibox while typing.
@interface RBSuggestPanel : UIView
@property(nonatomic, assign) id<RBSuggestPanelDelegate> delegate;

// items: array of {url, title} from the server's suggest reply.
- (void)showItems:(NSArray *)items;
- (void)hide;
- (CGFloat)desiredHeight;
@end
