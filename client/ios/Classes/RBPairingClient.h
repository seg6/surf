#import <Foundation/Foundation.h>

@interface RBPairingClient : NSObject

+ (NSDictionary *)inspectEndpoint:(NSString *)endpoint error:(NSError **)error;
+ (NSDictionary *)requestPairAtEndpoint:(NSString *)endpoint
                              serverInfo:(NSDictionary *)serverInfo
                                    code:(NSString *)code
                                 qrToken:(NSString *)qrToken
                                   error:(NSError **)error;
+ (NSDictionary *)confirmPairing:(NSDictionary *)pairing error:(NSError **)error;
+ (NSDictionary *)statusForPairing:(NSDictionary *)pairing error:(NSError **)error;
+ (NSDictionary *)cancelPairing:(NSDictionary *)pairing error:(NSError **)error;
+ (NSDictionary *)acknowledgePairing:(NSDictionary *)pairing error:(NSError **)error;
+ (NSDictionary *)savedServerFromPairing:(NSDictionary *)pairing;

@end
