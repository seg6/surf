#import <UIKit/UIKit.h>

@class RBPairingController;

@protocol RBPairingControllerDelegate <NSObject>
- (void)pairingController:(RBPairingController *)controller didPairServer:(NSDictionary *)server;
- (void)pairingController:(RBPairingController *)controller
       foundKnownServer:(NSDictionary *)server
               endpoint:(NSString *)endpoint;
- (void)pairingControllerDidCancel:(RBPairingController *)controller;
@end

@interface RBPairingController : UIViewController
@property(nonatomic, assign) id<RBPairingControllerDelegate> delegate;
- (id)initWithEndpoint:(NSString *)endpoint
      expectedServerID:(NSString *)expectedServerID
     replacementServer:(NSDictionary *)replacementServer
                qrToken:(NSString *)qrToken;
@end
