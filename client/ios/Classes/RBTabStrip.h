#import <UIKit/UIKit.h>

@class RBTabStrip;

@protocol RBTabStripDelegate <NSObject>
- (void)tabStrip:(RBTabStrip *)strip selectTab:(NSInteger)tabID;
- (void)tabStrip:(RBTabStrip *)strip closeTab:(NSInteger)tabID;
- (void)tabStripNewTab:(RBTabStrip *)strip;
@end

// Compact iPad tab rail hosted beside the omnibox. Tabs form one horizontally
// scrolling sequence, and the active tab is always brought fully into view.
@interface RBTabStrip : UIView
@property(nonatomic, assign) id<RBTabStripDelegate> delegate;
- (void)setTabs:(NSArray *)tabs baseURL:(NSURL *)baseURL fingerprint:(NSString *)fingerprint;
- (void)purgeIconCache;
- (void)applyAppearance;
@end
