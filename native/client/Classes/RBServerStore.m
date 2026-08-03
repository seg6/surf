#import "RBServerStore.h"
#import "RBConfig.h"
#import "RBDeviceIdentity.h"

NSString *const RBServersV1DefaultsKey = @"RBPairedServersV1";
NSString *const RBLastServerIDDefaultsKey = @"RBLastServerIDV1";
static NSString *const RBMigrationDefaultsKey = @"RBSecureServersMigrationV1";

@implementation RBServerStore

+ (void)performBreakingMigrationIfNeeded {
    NSUserDefaults *defaults = [NSUserDefaults standardUserDefaults];
    if ([defaults boolForKey:RBMigrationDefaultsKey]) return;
    [defaults removeObjectForKey:RBLegacyDefaultsServerURLKey];
    [defaults removeObjectForKey:RBLegacyDefaultsPasswordKey];
    [defaults removeObjectForKey:RBLegacyDefaultsServersKey];
    [defaults removeObjectForKey:RBServersV1DefaultsKey];
    [defaults removeObjectForKey:RBLastServerIDDefaultsKey];
    [defaults setBool:YES forKey:RBMigrationDefaultsKey];
    [defaults synchronize];
}

+ (NSArray *)servers {
    id value = [[NSUserDefaults standardUserDefaults] objectForKey:RBServersV1DefaultsKey];
    return [value isKindOfClass:[NSArray class]] ? value : @[];
}

+ (NSDictionary *)serverWithID:(NSString *)serverID {
    for (NSDictionary *server in [self servers]) {
        if ([[server objectForKey:@"serverID"] isEqualToString:serverID]) return server;
    }
    return nil;
}

+ (NSDictionary *)lastSelectedServer {
    NSString *serverID = [[NSUserDefaults standardUserDefaults] stringForKey:RBLastServerIDDefaultsKey];
    return [self serverWithID:serverID];
}

+ (void)saveServer:(NSDictionary *)server select:(BOOL)select {
    NSString *serverID = [server objectForKey:@"serverID"];
    if (![serverID length]) return;
    NSMutableArray *servers = [[self servers] mutableCopy];
    NSUInteger existing = NSNotFound;
    for (NSUInteger i = 0; i < [servers count]; i++) {
        if ([[[servers objectAtIndex:i] objectForKey:@"serverID"] isEqualToString:serverID]) { existing = i; break; }
    }
    if (existing == NSNotFound) [servers addObject:server];
    else [servers replaceObjectAtIndex:existing withObject:server];
    NSUserDefaults *defaults = [NSUserDefaults standardUserDefaults];
    [defaults setObject:servers forKey:RBServersV1DefaultsKey];
    if (select) [defaults setObject:serverID forKey:RBLastServerIDDefaultsKey];
    [defaults synchronize];
}

+ (void)forgetServerID:(NSString *)serverID {
    NSMutableArray *servers = [[self servers] mutableCopy];
    NSIndexSet *indexes = [servers indexesOfObjectsPassingTest:^BOOL(NSDictionary *server, NSUInteger index, BOOL *stop) {
        return [[server objectForKey:@"serverID"] isEqualToString:serverID];
    }];
    [servers removeObjectsAtIndexes:indexes];
    NSUserDefaults *defaults = [NSUserDefaults standardUserDefaults];
    [defaults setObject:servers forKey:RBServersV1DefaultsKey];
    if ([[defaults stringForKey:RBLastServerIDDefaultsKey] isEqualToString:serverID]) [defaults removeObjectForKey:RBLastServerIDDefaultsKey];
    [defaults synchronize];
    [RBDeviceIdentity deleteKeyForServerID:serverID];
}

+ (void)renameServerID:(NSString *)serverID name:(NSString *)name {
    NSDictionary *existing = [self serverWithID:serverID];
    if (!existing || ![[name stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]] length]) return;
    NSMutableDictionary *updated = [existing mutableCopy];
    [updated setObject:[name stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]] forKey:@"name"];
    [self saveServer:updated select:NO];
}

+ (BOOL)addVerifiedEndpoint:(NSString *)endpoint toServerID:(NSString *)serverID {
    return [self addVerifiedEndpoint:endpoint transport:nil toServerID:serverID];
}

+ (BOOL)addVerifiedEndpoint:(NSString *)endpoint transport:(NSString *)transport toServerID:(NSString *)serverID {
    endpoint = [self normalizeEndpoint:endpoint];
    NSDictionary *existing = [self serverWithID:serverID];
    if (!existing || !endpoint) return NO;
    NSMutableDictionary *updated = [existing mutableCopy];
    id savedEndpoints = [existing objectForKey:@"endpoints"];
    NSMutableArray *endpoints = [([savedEndpoints isKindOfClass:[NSArray class]] ? savedEndpoints : @[]) mutableCopy];
    if (![endpoints containsObject:endpoint]) [endpoints addObject:endpoint];
    [updated setObject:endpoints forKey:@"endpoints"];
    [updated setObject:endpoint forKey:@"lastEndpoint"];
    NSMutableArray *tunnels = [([existing objectForKey:@"tunnelEndpoints"] ?: @[]) mutableCopy];
    if ([[transport lowercaseString] isEqualToString:@"tunnel"]) {
        if (![tunnels containsObject:endpoint]) [tunnels addObject:endpoint];
    } else {
        [tunnels removeObject:endpoint];
    }
    [updated setObject:tunnels forKey:@"tunnelEndpoints"];
    [self saveServer:updated select:NO];
    return YES;
}

+ (NSString *)normalizeEndpoint:(NSString *)value {
    value = [value stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
    if (![value length]) return nil;
    if ([value rangeOfString:@"://"].location == NSNotFound) value = [@"https://" stringByAppendingString:value];
    NSURL *url = [NSURL URLWithString:value];
    if (![[[url scheme] lowercaseString] isEqualToString:@"https"] || ![[url host] length]) return nil;
    NSString *authority = [url host];
    if ([authority rangeOfString:@":"].location != NSNotFound && ![authority hasPrefix:@"["]) authority = [NSString stringWithFormat:@"[%@]", authority];
    if ([url port]) authority = [authority stringByAppendingFormat:@":%@", [url port]];
    return [@"https://" stringByAppendingString:authority];
}

@end
