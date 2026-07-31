#import <UIKit/UIKit.h>

@class RBTabStrip;

@protocol RBTabStripDelegate <NSObject>
- (void)tabStrip:(RBTabStrip *)strip selectTab:(NSInteger)tabID;
- (void)tabStrip:(RBTabStrip *)strip closeTab:(NSInteger)tabID;
- (void)tabStripNewTab:(RBTabStrip *)strip;
@end

// Classic iPad Safari tab row. Tabs fill the available width; excess tabs
// move into a >> overflow menu and the active tab is always kept visible.
@interface RBTabStrip : UIView <UIActionSheetDelegate>
@property(nonatomic, assign) id<RBTabStripDelegate> delegate;
- (void)setTabs:(NSArray *)tabs baseURL:(NSURL *)baseURL;
- (void)purgeIconCache; // compatibility no-op; thumbnails/favicons are absent
@end
