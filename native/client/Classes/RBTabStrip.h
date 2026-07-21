#import <UIKit/UIKit.h>

@class RBTabStrip;

@protocol RBTabStripDelegate <NSObject>
- (void)tabStrip:(RBTabStrip *)strip selectTab:(NSInteger)tabID;
- (void)tabStrip:(RBTabStrip *)strip closeTab:(NSInteger)tabID;
- (void)tabStripNewTab:(RBTabStrip *)strip;
@end

// Safari-style tab bar: one cell per remote tab (favicon, title, close),
// plus a [+] button pinned at the right.
@interface RBTabStrip : UIView
@property(nonatomic, assign) id<RBTabStripDelegate> delegate;

// tabs: array of {id, title, url, active, icon?} from the server broadcast.
// baseURL resolves relative favicon paths like /tabicon/3?v=abc.
- (void)setTabs:(NSArray *)tabs baseURL:(NSURL *)baseURL;
- (void)purgeIconCache;
@end
