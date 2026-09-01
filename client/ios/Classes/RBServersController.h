#import <UIKit/UIKit.h>

@class RBServersController;

@protocol RBServersControllerDelegate <NSObject>
- (void)serversController:(RBServersController *)controller connectToServer:(NSDictionary *)server;
- (void)serversController:(RBServersController *)controller
              pairEndpoint:(NSString *)endpoint
          expectedServerID:(NSString *)expectedServerID
         replacementServer:(NSDictionary *)replacementServer
                    qrToken:(NSString *)qrToken;
- (void)serversController:(RBServersController *)controller
             verifyAddress:(NSString *)endpoint
                 forServer:(NSDictionary *)server;
- (void)serversController:(RBServersController *)controller forgetServer:(NSDictionary *)server;
- (void)serversControllerWantsQRScanner:(RBServersController *)controller;
- (void)serversControllerDidCancel:(RBServersController *)controller;
@end

@interface RBServersController : UITableViewController
@property(nonatomic, assign) id<RBServersControllerDelegate> delegate;
@property(nonatomic, assign) BOOL allowsCancel;
@property(nonatomic, assign) BOOL connected;
@property(nonatomic, copy) NSString *pairingRequiredServerID;
- (id)initWithSelectedServerID:(NSString *)serverID firstLaunch:(BOOL)firstLaunch;
- (void)reloadServers;
- (void)setStatusText:(NSString *)status isError:(BOOL)isError;
- (void)setConnectingServerID:(NSString *)serverID;
@end
