#import "RBSession.h"
#import "RBConfig.h"
#import "RBLog.h"
#import "RBSocket.h"

static NSString *RBURLEscape(NSString *s);

@interface RBSession () <RBSocketDelegate>
@property(nonatomic, copy) NSString *baseURLString;
@property(nonatomic, strong, readwrite) NSURL *baseURL;
@property(nonatomic, assign, readwrite) RBSessionState state;
@property(nonatomic, strong) RBSocket *socket;
@property(nonatomic, copy) NSString *token;
@property(nonatomic, assign) NSInteger viewWidth;
@property(nonatomic, assign) NSInteger viewHeight;
@property(nonatomic, assign) BOOL socketOpen;
@property(nonatomic, copy) NSString *lastPassword;
@end

@implementation RBSession

@synthesize viewWidth = _viewWidth;
@synthesize viewHeight = _viewHeight;

- (id)initWithBaseURL:(NSString *)baseURL {
    self = [super init];
    if (self) {
        self.baseURLString = baseURL;
        self.baseURL = [NSURL URLWithString:baseURL];
        self.viewWidth = 0;
        self.viewHeight = 0;
        self.state = RBSessionStateIdle;
    }
    return self;
}

// Main thread only.
- (void)moveToState:(RBSessionState)state {
    if (state == _state) return;
    _state = state;
    [self.delegate session:self didChangeState:state];
}

- (void)startWithPassword:(NSString *)password {
    self.lastPassword = password;
    [self moveToState:RBSessionStateConnecting];
    [self.delegate session:self status:@"logging in"];
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        NSString *error = nil;
        if (![self loginWithPassword:password error:&error] || ![self fetchNativeConfig:&error]) {
            RBLog(@"session start failed: %@", error);
            dispatch_async(dispatch_get_main_queue(), ^{
                self.lastPassword = nil;
                [self moveToState:RBSessionStateIdle];
                [self.delegate session:self status:error ?: @"login failed"];
                [self.delegate sessionNeedsPassword:self message:error ?: @"Login failed"];
            });
            return;
        }
        dispatch_async(dispatch_get_main_queue(), ^{
            [self.delegate sessionDidAuthenticate:self];
            [self connectSocket];
        });
    });
}

- (void)shutdown {
    self.lastPassword = nil;
    [NSObject cancelPreviousPerformRequestsWithTarget:self];
    [self.socket close];
    self.socket = nil;
    self.socketOpen = NO;
    [self moveToState:RBSessionStateIdle];
}

- (BOOL)loginWithPassword:(NSString *)password error:(NSString **)error {
    NSURL *url = [NSURL URLWithString:@"/login" relativeToURL:self.baseURL];
    NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:url cachePolicy:NSURLRequestReloadIgnoringLocalCacheData timeoutInterval:20.0];
    [request setHTTPMethod:@"POST"];
    NSString *body = [NSString stringWithFormat:@"password=%@", RBURLEscape(password ?: @"")];
    [request setHTTPBody:[body dataUsingEncoding:NSUTF8StringEncoding]];
    [request setValue:@"application/x-www-form-urlencoded" forHTTPHeaderField:@"Content-Type"];
    NSHTTPURLResponse *response = nil;
    NSData *data = [NSURLConnection sendSynchronousRequest:request returningResponse:&response error:nil];
    if (!data || [response statusCode] >= 400) {
        if (error) *error = @"login failed — check the server address and password";
        return NO;
    }
    return YES;
}

- (BOOL)fetchNativeConfig:(NSString **)error {
    NSURL *url = [NSURL URLWithString:@"/native-config" relativeToURL:self.baseURL];
    NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:url cachePolicy:NSURLRequestReloadIgnoringLocalCacheData timeoutInterval:20.0];
    NSHTTPURLResponse *response = nil;
    NSData *data = [NSURLConnection sendSynchronousRequest:request returningResponse:&response error:nil];
    if (!data || [response statusCode] != 200) {
        if (error) *error = @"native-config failed — wrong password or old server?";
        return NO;
    }
    NSDictionary *json = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
    if (![json isKindOfClass:[NSDictionary class]]) {
        if (error) *error = @"native-config was not JSON";
        return NO;
    }
    self.token = [json objectForKey:@"token"];
    NSInteger serverWidth = [[json objectForKey:@"vw"] integerValue] ?: 1024;
    NSInteger serverHeight = [[json objectForKey:@"vh"] integerValue] ?: 768;
    if (self.viewWidth <= 0 || self.viewHeight <= 0) {
        self.viewWidth = serverWidth;
        self.viewHeight = serverHeight;
    }
    NSString *nv = [json objectForKey:@"nv"];
    if (!self.token || ![nv isEqualToString:RBNativeVersion]) {
        if (error) *error = [NSString stringWithFormat:@"version mismatch app=%@ server=%@", RBNativeVersion, nv ?: @"?"];
        return NO;
    }
    RBLog(@"native config ok vw=%d vh=%d nv=%@", (int)self.viewWidth, (int)self.viewHeight, nv);
    return YES;
}

static NSString *RBURLEscape(NSString *s) {
    CFStringRef escaped = CFURLCreateStringByAddingPercentEscapes(NULL, (CFStringRef)s, NULL, CFSTR(":/?#[]@!$&'()*+,;="), kCFStringEncodingUTF8);
    return CFBridgingRelease(escaped);
}

- (void)connectSocket {
    [self.socket close];
    [self moveToState:RBSessionStateConnecting];
    NSString *host = [self.baseURL host];
    NSInteger port = [[self.baseURL port] integerValue];
    if (port == 0) port = 80;
    NSString *path = [NSString stringWithFormat:@"/ws?k=%@&nv=%@", RBURLEscape(self.token ?: @""), RBURLEscape(RBNativeVersion)];
    self.socket = [[RBSocket alloc] initWithHost:host port:port path:path];
    self.socket.delegate = self;
    [self.delegate session:self status:@"connecting websocket"];
    [self.socket connect];
}

- (void)sendMessage:(NSDictionary *)message { [self.socket sendJSON:message]; }

- (void)sendReady { [self sendMessage:@{@"t": @"ready"}]; }

- (void)updateViewportWidth:(NSInteger)width height:(NSInteger)height {
    [self updateViewportWidth:width height:height force:NO];
}

- (void)updateViewportWidth:(NSInteger)width height:(NSInteger)height force:(BOOL)force {
    if (width <= 0 || height <= 0) return;
    if (!force && self.viewWidth == width && self.viewHeight == height) return;
    self.viewWidth = width;
    self.viewHeight = height;
    if (self.socketOpen) {
        RBLog(@"viewport update %dx%d%@", (int)width, (int)height, force ? @" forced" : @"");
        [self sendMessage:@{@"t": @"size", @"w": [NSNumber numberWithInteger:width], @"h": [NSNumber numberWithInteger:height]}];
    }
}

- (void)sendClickX:(CGFloat)x y:(CGFloat)y { [self sendMessage:@{@"t": @"click", @"x": [NSNumber numberWithFloat:x], @"y": [NSNumber numberWithFloat:y]}]; }
- (void)sendWheelX:(CGFloat)x y:(CGFloat)y dx:(CGFloat)dx dy:(CGFloat)dy {
    [self sendMessage:@{@"t": @"wheel", @"x": [NSNumber numberWithFloat:x], @"y": [NSNumber numberWithFloat:y], @"dx": [NSNumber numberWithFloat:dx], @"dy": [NSNumber numberWithFloat:dy]}];
}

- (void)socketDidOpen:(RBSocket *)socket {
    self.socketOpen = YES;
    [self sendMessage:@{@"t": @"size", @"w": [NSNumber numberWithInteger:self.viewWidth], @"h": [NSNumber numberWithInteger:self.viewHeight]}];
    [self moveToState:RBSessionStateOpen];
    [self.delegate session:self status:@"websocket open"];
}

- (void)socket:(RBSocket *)socket didCloseWithError:(NSString *)error {
    if (socket != self.socket) return; // stale socket from before a shutdown
    self.socketOpen = NO;
    [self.delegate session:self status:error ?: @"socket closed"];
    if (self.lastPassword) {
        [self moveToState:RBSessionStateRetrying];
        if ([error rangeOfString:@"upgrade rejected"].location != NSNotFound) {
            [self startWithPassword:self.lastPassword];
            return;
        }
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(1.5 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
            if (self.lastPassword) [self connectSocket];
        });
    } else {
        [self moveToState:RBSessionStateIdle];
    }
}

- (void)socket:(RBSocket *)socket didReceiveText:(NSString *)text {
    NSData *data = [text dataUsingEncoding:NSUTF8StringEncoding];
    NSDictionary *json = data ? [NSJSONSerialization JSONObjectWithData:data options:0 error:nil] : nil;
    if (![json isKindOfClass:[NSDictionary class]]) return;
    NSString *t = [json objectForKey:@"t"];
    if ([t isEqualToString:@"hello"]) {
        [self sendMessage:@{@"t": @"size", @"w": [NSNumber numberWithInteger:self.viewWidth], @"h": [NSNumber numberWithInteger:self.viewHeight]}];
    }
    [self.delegate session:self didReceiveControlMessage:json];
}

- (void)socket:(RBSocket *)socket didReceiveBinary:(NSData *)data {
    [self.delegate session:self didReceiveFrameData:data];
}

@end
