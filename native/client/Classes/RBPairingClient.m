#import "RBPairingClient.h"
#import "RBConfig.h"
#import "RBDeviceIdentity.h"
#import "RBLog.h"
#import "RBSecureHTTPClient.h"
#import "RBServerStore.h"
#import <UIKit/UIKit.h>

@implementation RBPairingClient

+ (NSDictionary *)JSONRequest:(NSMutableURLRequest *)request fingerprint:(NSString *)fingerprint allowUntrusted:(BOOL)allowUntrusted systemTrust:(BOOL)systemTrust observed:(NSString **)observed error:(NSError **)error {
    RBSecureHTTPClient *client = [[RBSecureHTTPClient alloc] initWithFingerprint:fingerprint allowUntrusted:allowUntrusted systemTrust:systemTrust];
    NSHTTPURLResponse *response = nil;
    NSData *data = [client sendRequest:request response:&response error:error];
    if (observed) *observed = client.observedFingerprint;
    if (!data) return nil;
    if ([response statusCode] < 200 || [response statusCode] >= 300) {
        NSString *message = nil;
        id failure = [data length] ? [NSJSONSerialization JSONObjectWithData:data options:0 error:nil] : nil;
        if ([failure isKindOfClass:[NSDictionary class]]) {
            message = [failure objectForKey:@"message"] ?: [failure objectForKey:@"error"];
        }
        if (![message length]) message = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
        if (error) *error = [NSError errorWithDomain:@"SurfPairing" code:[response statusCode] userInfo:@{NSLocalizedDescriptionKey: [message length] ? message : @"Pairing request failed"}];
        return nil;
    }
    if (![data length]) return @{};
    id json = [NSJSONSerialization JSONObjectWithData:data options:0 error:error];
    return [json isKindOfClass:[NSDictionary class]] ? json : nil;
}

+ (NSMutableURLRequest *)requestForEndpoint:(NSString *)endpoint path:(NSString *)path method:(NSString *)method body:(NSDictionary *)body {
    NSURL *baseURL = [NSURL URLWithString:endpoint];
    NSURL *url = [NSURL URLWithString:path relativeToURL:baseURL];
    NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:url cachePolicy:NSURLRequestReloadIgnoringLocalCacheData timeoutInterval:20.0];
    [request setHTTPMethod:method];
    if (body) {
        [request setHTTPBody:[NSJSONSerialization dataWithJSONObject:body options:0 error:nil]];
        [request setValue:@"application/json" forHTTPHeaderField:@"Content-Type"];
    }
    return request;
}

+ (NSDictionary *)inspectEndpoint:(NSString *)endpoint error:(NSError **)error {
    endpoint = [RBServerStore normalizeEndpoint:endpoint];
    if (!endpoint) {
        if (error) *error = [NSError errorWithDomain:@"SurfPairing" code:1 userInfo:@{NSLocalizedDescriptionKey: @"Enter a valid Surf server address"}];
        return nil;
    }
    NSString *observed = nil;
    RBLogEvent(@"verification", @"info", @{@"phase": @"system_probe", @"endpoint": endpoint},
               @"Checking public endpoint");
    NSError *systemError = nil;
    NSDictionary *server = [self JSONRequest:[self requestForEndpoint:endpoint path:@"/api/v1/server" method:@"GET" body:nil]
                                   fingerprint:nil allowUntrusted:NO systemTrust:YES observed:nil error:&systemError];
    BOOL tunneled = [[[server objectForKey:@"transport"] lowercaseString] isEqualToString:@"tunnel"];
    if (!tunneled) {
        RBLogEvent(@"verification", systemError ? @"warn" : @"info",
                   @{@"phase": @"pinned_probe", @"endpoint": endpoint,
                     @"system_error": [systemError localizedDescription] ?: @""},
                   @"Falling back to direct identity probe");
        server = [self JSONRequest:[self requestForEndpoint:endpoint path:@"/api/v1/server" method:@"GET" body:nil]
                             fingerprint:nil allowUntrusted:YES systemTrust:NO observed:&observed error:error];
    }
    NSString *advertised = [[server objectForKey:@"fingerprint"] lowercaseString];
    BOOL identityMatches = tunneled ? [[server objectForKey:@"serverID"] isEqualToString:advertised] :
        ([observed length] && [advertised isEqualToString:observed] && [[server objectForKey:@"serverID"] isEqualToString:observed]);
    if (!server || !identityMatches || ![[server objectForKey:@"api"] isEqualToString:@"v1"]) {
        RBLogEvent(@"verification", @"error",
                   @{@"phase": @"complete", @"endpoint": endpoint,
                     @"error": error && *error ? [*error localizedDescription] : @"identity mismatch"},
                   @"Address verification failed");
        if (error && !*error) *error = [NSError errorWithDomain:@"SurfPairing" code:2 userInfo:@{NSLocalizedDescriptionKey: @"The server identity did not match its certificate"}];
        return nil;
    }
    NSMutableDictionary *result = [server mutableCopy];
    [result setObject:endpoint forKey:@"endpoint"];
    RBLogEvent(@"verification", @"info",
               @{@"phase": @"complete", @"endpoint": endpoint,
                 @"transport": tunneled ? @"tunnel" : @"direct",
                 @"server_id": [server objectForKey:@"serverID"] ?: @""},
               @"Address verified");
    return result;
}

+ (NSDictionary *)requestPairAtEndpoint:(NSString *)endpoint serverInfo:(NSDictionary *)serverInfo code:(NSString *)code qrToken:(NSString *)qrToken error:(NSError **)error {
    NSString *serverID = [serverInfo objectForKey:@"serverID"];
    NSString *publicKey = [RBDeviceIdentity ensurePublicKeyForServerID:serverID error:error];
    if (!publicKey) return nil;
    NSString *deviceName = [[UIDevice currentDevice] name] ?: @"iOS device";
    NSMutableDictionary *body = [NSMutableDictionary dictionaryWithObjectsAndKeys:deviceName, @"deviceName", publicKey, @"publicKey", nil];
    if ([code length]) [body setObject:code forKey:@"code"];
    if ([qrToken length]) [body setObject:qrToken forKey:@"qrToken"];
    NSDictionary *status = [self JSONRequest:[self requestForEndpoint:endpoint path:@"/api/v1/pairing/request" method:@"POST" body:body]
                                     fingerprint:serverID allowUntrusted:NO systemTrust:[[[serverInfo objectForKey:@"transport"] lowercaseString] isEqualToString:@"tunnel"] observed:nil error:error];
    if (!status) return nil;
    NSString *localPhrase = [RBDeviceIdentity pairingPhraseForServerID:serverID error:error];
    if (![localPhrase length] || ![localPhrase isEqualToString:[status objectForKey:@"phrase"]]) {
        if (error && !*error) *error = [NSError errorWithDomain:@"SurfPairing" code:10 userInfo:@{
            NSLocalizedDescriptionKey: @"The pairing response was not bound to this server identity"
        }];
        return nil;
    }
    NSMutableDictionary *pairing = [status mutableCopy];
    [pairing setObject:localPhrase forKey:@"phrase"];
    [pairing setObject:endpoint forKey:@"endpoint"];
    [pairing setObject:serverID forKey:@"serverID"];
    [pairing setObject:serverID forKey:@"fingerprint"];
    if ([[[serverInfo objectForKey:@"transport"] lowercaseString] isEqualToString:@"tunnel"]) [pairing setObject:@"tunnel" forKey:@"transport"];
    [pairing setObject:([serverInfo objectForKey:@"name"] ?: @"Surf") forKey:@"serverName"];
    return pairing;
}

+ (NSDictionary *)pairingRequest:(NSDictionary *)pairing path:(NSString *)path method:(NSString *)method error:(NSError **)error {
    NSString *endpoint = [pairing objectForKey:@"endpoint"];
    NSString *fingerprint = [pairing objectForKey:@"fingerprint"];
    NSDictionary *status = [self JSONRequest:[self requestForEndpoint:endpoint path:path method:method body:nil]
                                     fingerprint:fingerprint allowUntrusted:NO systemTrust:[[[pairing objectForKey:@"transport"] lowercaseString] isEqualToString:@"tunnel"] observed:nil error:error];
    if (!status) return nil;
    NSMutableDictionary *updated = [pairing mutableCopy];
    [updated addEntriesFromDictionary:status];
    return updated;
}

+ (NSDictionary *)confirmPairing:(NSDictionary *)pairing error:(NSError **)error {
    NSString *path = [@"/api/v1/pairing/confirm/" stringByAppendingString:[pairing objectForKey:@"id"] ?: @""];
    return [self pairingRequest:pairing path:path method:@"POST" error:error];
}

+ (NSDictionary *)statusForPairing:(NSDictionary *)pairing error:(NSError **)error {
    NSString *path = [@"/api/v1/pairing/status/" stringByAppendingString:[pairing objectForKey:@"id"] ?: @""];
    return [self pairingRequest:pairing path:path method:@"GET" error:error];
}

+ (NSDictionary *)cancelPairing:(NSDictionary *)pairing error:(NSError **)error {
    NSString *path = [@"/api/v1/pairing/cancel/" stringByAppendingString:[pairing objectForKey:@"id"] ?: @""];
    return [self pairingRequest:pairing path:path method:@"POST" error:error];
}

+ (NSDictionary *)acknowledgePairing:(NSDictionary *)pairing error:(NSError **)error {
    NSString *path = [@"/api/v1/pairing/ack/" stringByAppendingString:[pairing objectForKey:@"id"] ?: @""];
    return [self pairingRequest:pairing path:path method:@"POST" error:error];
}

+ (NSDictionary *)savedServerFromPairing:(NSDictionary *)pairing {
    NSString *endpoint = [pairing objectForKey:@"endpoint"], *serverID = [pairing objectForKey:@"serverID"];
    NSMutableDictionary *server = [@{ @"serverID": serverID, @"fingerprint": serverID,
              @"name": [pairing objectForKey:@"serverName"] ?: @"Surf",
              @"endpoints": endpoint ? @[endpoint] : @[], @"lastEndpoint": endpoint ?: @"",
              @"lastConnected": [NSDate date], @"keyTag": [RBDeviceIdentity keyTagForServerID:serverID] } mutableCopy];
    if ([[[pairing objectForKey:@"transport"] lowercaseString] isEqualToString:@"tunnel"] && endpoint) [server setObject:@[endpoint] forKey:@"tunnelEndpoints"];
    return server;
}

@end
