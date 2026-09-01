#import <Foundation/Foundation.h>

// A small iOS 6-compatible HTTPS client which validates Surf's exact leaf
// certificate fingerprint. It intentionally does not consult the system CA
// store after a server has been paired.
@interface RBSecureHTTPClient : NSObject

@property(nonatomic, copy, readonly) NSString *observedFingerprint;

- (id)initWithFingerprint:(NSString *)fingerprint allowUntrusted:(BOOL)allowUntrusted;
- (id)initWithFingerprint:(NSString *)fingerprint allowUntrusted:(BOOL)allowUntrusted systemTrust:(BOOL)systemTrust;
- (NSData *)sendRequest:(NSURLRequest *)request response:(NSHTTPURLResponse **)response error:(NSError **)error;

+ (NSString *)fingerprintForTrust:(SecTrustRef)trust;
+ (BOOL)endpoint:(NSString *)endpoint usesTunnelInServer:(NSDictionary *)server;
+ (RBSecureHTTPClient *)clientForServer:(NSDictionary *)server;
+ (RBSecureHTTPClient *)clientForEndpoint:(NSString *)endpoint fingerprint:(NSString *)fingerprint;

@end
