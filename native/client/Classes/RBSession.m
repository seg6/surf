#import "RBSession.h"
#import "RBConfig.h"
#import "RBLog.h"
#import "RBInteractionTracker.h"
#import "RBDeviceIdentity.h"
#import "RBSecureHTTPClient.h"
#import "RBSocket.h"

static NSString *RBURLEscape(NSString *s);

@interface RBSession () <RBSocketDelegate>
@property(nonatomic, copy) NSString *baseURLString;
@property(nonatomic, strong, readwrite) NSURL *baseURL;
@property(nonatomic, strong, readwrite) NSDictionary *server;
@property(nonatomic, assign, readwrite) RBSessionState state;
@property(nonatomic, strong) RBSocket *socket;
@property(nonatomic, copy) NSString *wsTicket;
@property(nonatomic, assign) NSInteger viewWidth;
@property(nonatomic, assign) NSInteger viewHeight;
@property(nonatomic, assign) BOOL socketOpen;
@property(nonatomic, assign) BOOL active;
@property(nonatomic, assign) NSTimeInterval reconnectDelay;
@property(nonatomic, assign) NSUInteger generation;
@property(nonatomic, strong, readwrite) RBInteractionTracker *interactionTracker;
@property(nonatomic, strong) NSDictionary *requiredClientUpdate;
@property(nonatomic, strong, readwrite) NSDictionary *availableClientUpdate;
@property(nonatomic, copy) NSString *requiredServerVersion;
@property(nonatomic, assign, readwrite) BOOL requiresPairing;
@property(nonatomic, assign) unsigned long long touchSequence;
@property(nonatomic, strong) NSData *lastUploadedLog;
@property(nonatomic, assign) BOOL logUploadRunning;
@property(nonatomic, assign) NSUInteger logUploadGeneration;
@end

@implementation RBSession

@synthesize viewWidth = _viewWidth;
@synthesize viewHeight = _viewHeight;

- (id)initWithServer:(NSDictionary *)server {
    self = [super init];
    if (self) {
        self.server = server;
        self.baseURLString = [server objectForKey:@"lastEndpoint"];
        self.baseURL = [NSURL URLWithString:self.baseURLString];
        self.viewWidth = 0;
        self.viewHeight = 0;
        self.state = RBSessionStateIdle;
        self.reconnectDelay = 1.0;
        self.interactionTracker = [[RBInteractionTracker alloc] init];
    }
    return self;
}

// Main thread only.
- (void)moveToState:(RBSessionState)state {
    if (state == _state) return;
    _state = state;
    [self.delegate session:self didChangeState:state];
}

- (void)start {
    if (!self.baseURL || ![self.baseURL host] || ![[[self.baseURL scheme] lowercaseString] isEqualToString:@"https"] || ![[self.server objectForKey:@"fingerprint"] length]) {
        [self.delegate sessionNeedsServer:self message:@"Choose a paired Surf server"];
        return;
    }
    NSUInteger generation = ++self.generation;
    self.socket.delegate = nil;
    [self.socket close];
    self.socket = nil;
    self.socketOpen = NO;
    self.active = YES;
    self.requiredClientUpdate = nil;
    self.availableClientUpdate = nil;
    self.requiredServerVersion = nil;
    self.requiresPairing = NO;
    [self moveToState:RBSessionStateConnecting];
    RBLogEvent(@"session", @"info",
               @{@"host": [self.baseURL host] ?: @"",
                 @"port": [self.baseURL port] ?: @443,
                 @"server_id": [self.server objectForKey:@"serverID"] ?: @""},
               @"Secure session starting");
    [self.delegate session:self status:@"authenticating device"];
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        NSString *error = nil;
        if (![self authenticateDevice:&error] || ![self fetchNativeConfig:&error]) {
            RBLogEvent(@"session", @"error", @{@"error": error ?: @""}, @"Secure session failed");
            dispatch_async(dispatch_get_main_queue(), ^{
                if (generation != self.generation) return;
                [self moveToState:RBSessionStateIdle];
                [self.delegate session:self status:error ?: @"secure authentication failed"];
                if (self.requiredClientUpdate) {
                    [self.delegate session:self requiresClientUpdate:self.requiredClientUpdate];
                } else if (self.requiredServerVersion) {
                    [self.delegate sessionRequiresServerUpdate:self serverVersion:self.requiredServerVersion];
                } else {
                    self.active = NO;
                    [self.delegate sessionNeedsServer:self message:error ?: @"Secure authentication failed"];
                }
            });
            return;
        }
        dispatch_async(dispatch_get_main_queue(), ^{
            if (generation != self.generation) return;
            [self.delegate sessionDidAuthenticate:self];
            [self connectSocket];
        });
    });
}

- (void)shutdown {
    self.generation++;
    self.active = NO;
    [NSObject cancelPreviousPerformRequestsWithTarget:self];
    self.socket.delegate = nil;
    [self.socket close];
    self.socket = nil;
    self.socketOpen = NO;
    self.reconnectDelay = 1.0;
    self.logUploadRunning = NO;
    self.logUploadGeneration = self.generation;
    RBSetLogRecordHandler(nil);
    [self moveToState:RBSessionStateIdle];
}

- (void)uploadNativeLogNow {
    if (!self.active || self.logUploadRunning || !self.baseURL) return;
    self.logUploadRunning = YES;
    NSUInteger generation = self.generation;
    self.logUploadGeneration = generation;
    RBLogSnapshot(^(NSData *snapshot) {
        dispatch_async(dispatch_get_main_queue(), ^{
            if (generation != self.generation || !self.active) {
                if (self.logUploadGeneration == generation) self.logUploadRunning = NO;
                return;
            }
            if (self.lastUploadedLog && [self.lastUploadedLog isEqualToData:snapshot]) {
                if (self.logUploadGeneration == generation) self.logUploadRunning = NO;
                return;
            }
            dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
                NSURL *url = [NSURL URLWithString:@"/api/v1/client/logs" relativeToURL:self.baseURL];
                NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:url
                    cachePolicy:NSURLRequestReloadIgnoringLocalCacheData timeoutInterval:20.0];
                [request setHTTPMethod:@"PUT"];
                [request setHTTPBody:snapshot ?: [NSData data]];
                [request setValue:@"application/x-ndjson" forHTTPHeaderField:@"Content-Type"];
                NSHTTPURLResponse *response = nil;
                NSError *error = nil;
                NSData *result = [[RBSecureHTTPClient clientForServer:self.server]
                    sendRequest:request response:&response error:&error];
                BOOL uploaded = result != nil && [response statusCode] >= 200 && [response statusCode] < 300;
                dispatch_async(dispatch_get_main_queue(), ^{
                    if (generation == self.generation && uploaded) self.lastUploadedLog = snapshot;
                    if (self.logUploadGeneration == generation) self.logUploadRunning = NO;
                });
            });
        });
    });
}

- (BOOL)revokeThisDevice:(NSString **)error {
    if (![self authenticateDevice:error]) return NO;
    NSInteger status = 0;
    NSDictionary *result = [self sendJSONPath:@"/api/v1/auth/revoke" method:@"POST" body:nil status:&status error:error];
    if (!result || status != 204) return NO;
    RBLogEvent(@"authentication", @"info", @{@"server_id": [self.server objectForKey:@"serverID"] ?: @""}, @"Device authorization revoked");
    return YES;
}

- (NSDictionary *)sendJSONPath:(NSString *)path method:(NSString *)method body:(NSDictionary *)body status:(NSInteger *)status error:(NSString **)error {
    NSURL *url = [NSURL URLWithString:path relativeToURL:self.baseURL];
    NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:url cachePolicy:NSURLRequestReloadIgnoringLocalCacheData timeoutInterval:20.0];
    [request setHTTPMethod:method];
    if (body) {
        [request setHTTPBody:[NSJSONSerialization dataWithJSONObject:body options:0 error:nil]];
        [request setValue:@"application/json" forHTTPHeaderField:@"Content-Type"];
    }
    RBSecureHTTPClient *client = [RBSecureHTTPClient clientForServer:self.server];
    NSHTTPURLResponse *response = nil;
    NSError *requestError = nil;
    NSData *data = [client sendRequest:request response:&response error:&requestError];
    if (status) *status = [response statusCode];
    if (!data || [response statusCode] < 200 || [response statusCode] >= 300) {
        NSString *serverMessage = data ? [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding] : nil;
        RBLogEvent(@"api", @"error", @{@"host": [url host] ?: @"", @"path": [url path] ?: @"",
                   @"status": @([response statusCode]), @"error": [requestError localizedDescription] ?: @""}, @"Secure API request failed");
        if (error) *error = [requestError localizedDescription] ?: ([serverMessage length] ? serverMessage : @"Secure server request failed");
        return nil;
    }
    if (![data length]) return @{};
    id json = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
    if (![json isKindOfClass:[NSDictionary class]]) {
        if (error) *error = @"Surf returned invalid secure API data";
        return nil;
    }
    return json;
}

- (BOOL)authenticateDevice:(NSString **)error {
    NSString *serverID = [self.server objectForKey:@"serverID"];
    NSError *keyError = nil;
    NSString *deviceID = [RBDeviceIdentity deviceIDForServerID:serverID error:&keyError];
    if (!deviceID) {
        if (error) *error = [keyError localizedDescription];
        return NO;
    }
    NSInteger challengeStatus = 0;
    NSDictionary *challenge = [self sendJSONPath:@"/api/v1/auth/challenge" method:@"POST" body:@{@"deviceID": deviceID}
                                             status:&challengeStatus error:error];
    if (!challenge) {
        if (challengeStatus == 401) {
            self.requiresPairing = YES;
            if (error) *error = @"Pairing Required. This device is no longer approved by the server. Tap the saved server to pair again.";
        }
        return NO;
    }
    NSString *signature = [RBDeviceIdentity signAuthenticationForServerID:serverID deviceID:deviceID
                                                               challengeID:[challenge objectForKey:@"id"]
                                                                     nonce:[challenge objectForKey:@"nonce"] error:&keyError];
    if (!signature) {
        if (error) *error = [keyError localizedDescription];
        return NO;
    }
    NSInteger completeStatus = 0;
    NSDictionary *complete = [self sendJSONPath:@"/api/v1/auth/complete" method:@"POST"
                                           body:@{@"challengeID": [challenge objectForKey:@"id"] ?: @"", @"signature": signature}
                                         status:&completeStatus error:error];
    if (!complete) {
        if (completeStatus == 401) {
            self.requiresPairing = YES;
            if (error) *error = @"Pairing Required. This device is no longer approved by the server. Tap the saved server to pair again.";
        }
        return NO;
    }
    RBLogEvent(@"authentication", @"info", @{@"server_id": serverID ?: @"", @"device_id": deviceID ?: @""}, @"Device authenticated");
    return YES;
}

- (BOOL)fetchNativeConfig:(NSString **)error {
    NSString *path = [NSString stringWithFormat:@"/api/v1/config?av=%@&cv=%@&nv=%@",
                      RBURLEscape(RBAppVersion), RBURLEscape(RBCompatibilityVersion),
                      RBURLEscape(RBWireCompatibilityVersion())];
    NSDictionary *json = [self sendJSONPath:path method:@"GET" body:nil status:nil error:error];
    if (!json) return NO;
    self.wsTicket = [json objectForKey:@"ticket"];
    NSInteger serverWidth = [[json objectForKey:@"vw"] integerValue] ?: 1024;
    NSInteger serverHeight = [[json objectForKey:@"vh"] integerValue] ?: 768;
    if (self.viewWidth <= 0 || self.viewHeight <= 0) {
        self.viewWidth = serverWidth;
        self.viewHeight = serverHeight;
    }
    NSString *nv = [json objectForKey:@"nv"];
    NSInteger serverCompatibility = [[json objectForKey:@"compatibilityVersion"] integerValue];
    if (serverCompatibility <= 0 && [nv isEqualToString:@"20260831-1"]) serverCompatibility = 1;
    NSString *compatibility = [json objectForKey:@"compatibility"];
    if ([compatibility isEqualToString:@"client-update-required"]) {
        NSDictionary *update = [json objectForKey:@"clientUpdate"];
        if ([update isKindOfClass:[NSDictionary class]]) self.requiredClientUpdate = update;
        if (error) *error = @"This device needs a Surf update";
        return NO;
    }
    if ([compatibility isEqualToString:@"server-update-required"]) {
        self.requiredServerVersion = [json objectForKey:@"version"] ?: @"?";
        if (error) *error = @"This server must be updated";
        return NO;
    }
    if (!self.wsTicket || serverCompatibility != [RBCompatibilityVersion integerValue]) {
        if (error) *error = [NSString stringWithFormat:@"compatibility mismatch client=%@ server=%ld",
                             RBCompatibilityVersion, (long)serverCompatibility];
        return NO;
    }
    NSDictionary *availableUpdate = [json objectForKey:@"clientUpdate"];
    if ([availableUpdate isKindOfClass:[NSDictionary class]] &&
        ![[availableUpdate objectForKey:@"required"] boolValue]) {
        self.availableClientUpdate = availableUpdate;
    }
    RBLogEvent(@"session", @"info", @{@"viewport_width": @(self.viewWidth),
               @"viewport_height": @(self.viewHeight),
               @"compatibility": @(serverCompatibility),
               @"server_version": [json objectForKey:@"version"] ?: @""},
               @"Native configuration accepted");
    return YES;
}

static NSString *RBURLEscape(NSString *s) {
    CFStringRef escaped = CFURLCreateStringByAddingPercentEscapes(NULL, (CFStringRef)s, NULL, CFSTR(":/?#[]@!$&'()*+,;="), kCFStringEncodingUTF8);
    return CFBridgingRelease(escaped);
}

- (void)connectSocket {
    if (!self.active) return;
    self.socket.delegate = nil;
    [self.socket close];
    self.socket = nil;
    self.socketOpen = NO;
    [self moveToState:RBSessionStateConnecting];
    NSString *host = [self.baseURL host];
    NSInteger port = [[self.baseURL port] integerValue];
    if (port == 0) port = [[[self.baseURL scheme] lowercaseString] isEqualToString:@"https"] ? 443 : 80;
    NSString *path = [NSString stringWithFormat:@"/api/v1/ws?ticket=%@&cv=%@&nv=%@",
                      RBURLEscape(self.wsTicket ?: @""), RBURLEscape(RBCompatibilityVersion),
                      RBURLEscape(RBWireCompatibilityVersion())];
    BOOL tunneled = [RBSecureHTTPClient endpoint:self.baseURLString usesTunnelInServer:self.server];
    if (tunneled) {
        self.socket = [[RBSocket alloc] initWithHost:host port:port path:path secure:YES
                                        fingerprint:[self.server objectForKey:@"fingerprint"]
                                         tunnelHost:host tunnelPort:port];
    } else {
        self.socket = [[RBSocket alloc] initWithHost:host port:port path:path secure:YES fingerprint:[self.server objectForKey:@"fingerprint"]];
    }
    self.socket.delegate = self;
    [self.delegate session:self status:@"connecting websocket"];
    [self.socket connect];
}

- (void)sendMessage:(NSDictionary *)message {
    [self.socket sendJSON:[self.interactionTracker decorateMessage:message]];
}

- (void)updateViewportWidth:(NSInteger)width height:(NSInteger)height {
    [self updateViewportWidth:width height:height force:NO];
}

- (void)updateViewportWidth:(NSInteger)width height:(NSInteger)height force:(BOOL)force {
    // The native stream view is laid out to even dimensions, but normalize
    // here too so reconnects and any future callers can never request a
    // 4:2:0-incompatible odd viewport and trigger a quality-losing rescale.
    width &= ~1;
    height &= ~1;
    if (width <= 0 || height <= 0) return;
    if (!force && self.viewWidth == width && self.viewHeight == height) return;
    self.viewWidth = width;
    self.viewHeight = height;
    if (self.socketOpen) {
        RBLogEvent(@"session", @"info", @{@"viewport_width": @(width), @"viewport_height": @(height), @"forced": @(force)}, @"Viewport updated");
        [self sendMessage:@{@"t": @"size", @"w": [NSNumber numberWithInteger:width], @"h": [NSNumber numberWithInteger:height]}];
    }
}

- (void)sendTouchPhase:(NSString *)phase
                points:(NSArray *)points
             timestamp:(unsigned long long)timestamp
               surface:(unsigned int)surface {
    if (!self.socketOpen || ![phase length] || !surface) return;
    NSDictionary *message = @{@"t": @"touch", @"phase": phase,
                              @"seq": [NSNumber numberWithUnsignedLongLong:++self.touchSequence],
                              @"surface": [NSNumber numberWithUnsignedInt:surface],
                              @"ts": [NSNumber numberWithUnsignedLongLong:timestamp],
                              @"points": points ?: @[]};
    NSDictionary *decorated = [self.interactionTracker decorateMessage:message];
    [self.socket sendTouchJSON:decorated coalescible:[phase isEqualToString:@"move"]];
}

- (void)socketDidOpen:(RBSocket *)socket {
    if (socket != self.socket) {
        socket.delegate = nil;
        [socket close];
        return;
    }
    self.socketOpen = YES;
    __weak RBSession *weakSelf = self;
    RBSetLogRecordHandler(^(NSDictionary *record) {
        dispatch_async(dispatch_get_main_queue(), ^{
            RBSession *session = weakSelf;
            if (!session || !session.socketOpen || !session.active) return;
            [session sendMessage:@{@"t": @"log-record", @"record": record ?: @{}}];
        });
    });
    self.reconnectDelay = 1.0;
    [self moveToState:RBSessionStateOpen];
    [self.delegate session:self status:@"websocket open"];
    [self uploadNativeLogNow];
}

- (void)socket:(RBSocket *)socket didCloseWithError:(NSString *)error {
    if (socket != self.socket) return; // stale socket from before a shutdown
    self.socketOpen = NO;
    [self.delegate session:self status:error ?: @"socket closed"];
    if (self.active) {
        [self moveToState:RBSessionStateRetrying];
        if ([error rangeOfString:@"upgrade rejected"].location != NSNotFound) {
            [self start];
            return;
        }
        NSTimeInterval delay = self.reconnectDelay;
        self.reconnectDelay = MIN(self.reconnectDelay * 1.7, 15.0);
        NSUInteger generation = self.generation;
        [self.delegate session:self status:[NSString stringWithFormat:@"reconnecting in %.1fs", delay]];
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(delay * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
            if (generation == self.generation && self.active && self.socket == socket) [self connectSocket];
        });
    } else {
        [self moveToState:RBSessionStateIdle];
    }
}

- (void)socket:(RBSocket *)socket didReceiveText:(NSString *)text {
    if (socket != self.socket) return;
    NSData *data = [text dataUsingEncoding:NSUTF8StringEncoding];
    NSDictionary *json = data ? [NSJSONSerialization JSONObjectWithData:data options:0 error:nil] : nil;
    if (![json isKindOfClass:[NSDictionary class]]) return;
    [self.delegate session:self didReceiveControlMessage:json];
}

- (void)socket:(RBSocket *)socket didReceiveBinary:(NSData *)data {
    if (socket != self.socket) return;
    [self.delegate session:self didReceiveFrameData:data];
}

@end
