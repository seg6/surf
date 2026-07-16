#import <UIKit/UIKit.h>

@class RBSettingsController;

@protocol RBSettingsDelegate <NSObject>
- (void)settings:(RBSettingsController *)settings connectToURL:(NSString *)url password:(NSString *)password;
- (void)settingsDismissed:(RBSettingsController *)settings;
@end

// Server + login form sheet. Presented on first launch (not dismissible until
// connected) and from the menu afterwards.
@interface RBSettingsController : UIViewController
@property(nonatomic, assign) id<RBSettingsDelegate> delegate;
@property(nonatomic, assign) BOOL allowsCancel;

- (id)initWithServerURL:(NSString *)serverURL password:(NSString *)password;
- (void)setStatusText:(NSString *)status isError:(BOOL)isError;
@end
