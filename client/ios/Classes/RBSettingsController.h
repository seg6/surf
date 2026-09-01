#import <UIKit/UIKit.h>

@class RBSettingsController;

@protocol RBSettingsDelegate <NSObject>
- (void)settingsWantsServers:(RBSettingsController *)settings;
- (void)settingsDismissed:(RBSettingsController *)settings;
@optional
- (void)settings:(RBSettingsController *)settings clearData:(NSString *)what;
- (void)settings:(RBSettingsController *)settings diagnosticsVisible:(BOOL)visible;
- (void)settings:(RBSettingsController *)settings preference:(NSString *)key enabled:(BOOL)enabled;
- (void)settingsWantsMediaControls:(RBSettingsController *)settings;
- (void)settingsWantsDiagnosticsInspector:(RBSettingsController *)settings;
- (void)settingsWantsClientUpdate:(RBSettingsController *)settings;
@end

@interface RBSettingsController : UITableViewController
@property(nonatomic, assign) id<RBSettingsDelegate> delegate;
@property(nonatomic, assign) BOOL connected;
@property(nonatomic, assign) BOOL diagnosticsVisible;
@property(nonatomic, strong) NSDictionary *availableClientUpdate;
- (id)initWithSelectedServerID:(NSString *)serverID;
- (void)reloadServers;
@end
