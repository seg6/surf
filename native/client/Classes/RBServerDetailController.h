#import <UIKit/UIKit.h>

@class RBServerDetailController;

@protocol RBServerDetailControllerDelegate <NSObject>
- (void)serverDetailController:(RBServerDetailController *)controller connectToServer:(NSDictionary *)server;
- (void)serverDetailController:(RBServerDetailController *)controller
                 verifyAddress:(NSString *)endpoint
                     forServer:(NSDictionary *)server;
- (void)serverDetailController:(RBServerDetailController *)controller pairAgainServer:(NSDictionary *)server;
- (void)serverDetailController:(RBServerDetailController *)controller forgetServer:(NSDictionary *)server;
- (void)serverDetailControllerDidChangeServer:(RBServerDetailController *)controller;
@end

@interface RBServerDetailController : UITableViewController
@property(nonatomic, assign) id<RBServerDetailControllerDelegate> delegate;
@property(nonatomic, assign) BOOL connected;
@property(nonatomic, assign) BOOL requiresPairing;
@property(nonatomic, copy, readonly) NSString *serverID;
- (id)initWithServer:(NSDictionary *)server;
- (void)reloadServer;
- (void)setStatusText:(NSString *)status isError:(BOOL)isError;
@end
