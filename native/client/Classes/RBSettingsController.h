#import <UIKit/UIKit.h>

@class RBSettingsController;

@protocol RBSettingsDelegate <NSObject>
- (void)settings:(RBSettingsController *)settings connectToURL:(NSString *)url password:(NSString *)password;
- (void)settingsDismissed:(RBSettingsController *)settings;
@optional
// DATA section: what = history|cookies|cache. Only offered while connected.
- (void)settings:(RBSettingsController *)settings clearData:(NSString *)what;
@end

// App configuration as a real grouped settings screen (chrome rethink):
// SERVER (url/password/connect + saved servers + Bonjour discovery),
// DATA (clear), ABOUT (version).
// Present wrapped in a UINavigationController form sheet.
@interface RBSettingsController : UITableViewController
@property(nonatomic, assign) id<RBSettingsDelegate> delegate;
@property(nonatomic, assign) BOOL allowsCancel;
// Data actions only make sense with a live session.
@property(nonatomic, assign) BOOL connected;
- (id)initWithServerURL:(NSString *)serverURL password:(NSString *)password;
- (void)setStatusText:(NSString *)status isError:(BOOL)isError;
@end
