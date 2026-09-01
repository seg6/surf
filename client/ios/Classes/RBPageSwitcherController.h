#import <UIKit/UIKit.h>

@class RBPageSwitcherController;

@protocol RBPageSwitcherDelegate <NSObject>
- (void)pageSwitcher:(RBPageSwitcherController *)controller selectTab:(NSInteger)tabID;
- (void)pageSwitcher:(RBPageSwitcherController *)controller closeTab:(NSInteger)tabID;
- (void)pageSwitcherNewTab:(RBPageSwitcherController *)controller;
@end

// Phone tab overview: one horizontally paged preview per remote tab, with
// close, New Tab, and Done controls.
@interface RBPageSwitcherController : UIViewController <UIScrollViewDelegate>
@property(nonatomic, assign) id<RBPageSwitcherDelegate> delegate;

- (id)initWithTabs:(NSArray *)tabs thumbnails:(NSDictionary *)thumbnails baseURL:(NSURL *)baseURL fingerprint:(NSString *)fingerprint;
- (void)updateTabs:(NSArray *)tabs thumbnails:(NSDictionary *)thumbnails;
@end
