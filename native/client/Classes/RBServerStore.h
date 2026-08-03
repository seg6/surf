#import <Foundation/Foundation.h>

extern NSString *const RBServersV1DefaultsKey;
extern NSString *const RBLastServerIDDefaultsKey;

// Saved server dictionaries contain serverID, fingerprint, name, endpoints,
// lastEndpoint, lastConnected and keyTag. Private keys remain in Keychain.
@interface RBServerStore : NSObject

+ (void)performBreakingMigrationIfNeeded;
+ (NSArray *)servers;
+ (NSDictionary *)serverWithID:(NSString *)serverID;
+ (NSDictionary *)lastSelectedServer;
+ (void)saveServer:(NSDictionary *)server select:(BOOL)select;
+ (void)forgetServerID:(NSString *)serverID;
+ (void)renameServerID:(NSString *)serverID name:(NSString *)name;
+ (BOOL)addVerifiedEndpoint:(NSString *)endpoint toServerID:(NSString *)serverID;
+ (BOOL)addVerifiedEndpoint:(NSString *)endpoint transport:(NSString *)transport toServerID:(NSString *)serverID;
+ (NSString *)normalizeEndpoint:(NSString *)value;

@end
