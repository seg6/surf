#import <Foundation/Foundation.h>
#import <Security/Security.h>

@interface RBDeviceIdentity : NSObject

+ (NSString *)keyTagForServerID:(NSString *)serverID;
+ (NSString *)ensurePublicKeyForServerID:(NSString *)serverID error:(NSError **)error;
+ (NSString *)deviceIDForServerID:(NSString *)serverID error:(NSError **)error;
+ (NSString *)pairingPhraseForServerID:(NSString *)serverID error:(NSError **)error;
+ (NSString *)signAuthenticationForServerID:(NSString *)serverID
                                    deviceID:(NSString *)deviceID
                                 challengeID:(NSString *)challengeID
                                       nonce:(NSString *)nonce
                                       error:(NSError **)error;
+ (void)deleteKeyForServerID:(NSString *)serverID;

@end
