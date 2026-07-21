#import <UIKit/UIKit.h>

@class RBSettingsController;

@protocol RBSettingsDelegate <NSObject>
- (void)settings:(RBSettingsController *)settings connectToURL:(NSString *)url password:(NSString *)password;
- (void)settingsDismissed:(RBSettingsController *)settings;
@optional
// DATA section: what = history|cookies|cache. Only offered while connected.
- (void)settings:(RBSettingsController *)settings clearData:(NSString *)what;
// STREAM section changed (video toggle or profile): re-negotiate the lane.
- (void)settingsStreamChanged:(RBSettingsController *)settings;
// ABOUT section: diagnostics overlay switch.
- (void)settings:(RBSettingsController *)settings setDiagnosticsVisible:(BOOL)visible;
@end

// App configuration as a real grouped settings screen (chrome rethink):
// SERVER (url/password/connect + saved servers + Bonjour discovery),
// STREAM (video + profile), DATA (clear), ABOUT (version, diagnostics).
// Present wrapped in a UINavigationController form sheet.
@interface RBSettingsController : UITableViewController
@property(nonatomic, assign) id<RBSettingsDelegate> delegate;
@property(nonatomic, assign) BOOL allowsCancel;
// Data actions only make sense with a live session.
@property(nonatomic, assign) BOOL connected;
@property(nonatomic, assign) BOOL diagnosticsVisible;

- (id)initWithServerURL:(NSString *)serverURL password:(NSString *)password;
- (void)setStatusText:(NSString *)status isError:(BOOL)isError;
@end
