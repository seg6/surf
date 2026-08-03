#import "RBSecureHTTPClient.h"
#import "RBServerStore.h"
#import <CommonCrypto/CommonDigest.h>
#import <Security/SecureTransport.h>
#import <arpa/inet.h>
#import <errno.h>
#import <fcntl.h>
#import <netdb.h>
#import <netinet/in.h>
#import <sys/socket.h>
#import <sys/time.h>
#import <unistd.h>

static NSString *const RBTLSErrorDomain = @"SurfTLS";
static NSString *const RBDeviceSessionCookieName = @"surf_device";
static NSMutableDictionary *RBDeviceSessionCookies;

typedef struct {
    int fd;
    CFAbsoluteTime deadline;
} RBTLSConnection;

static BOOL RBTLSDeadlineExpired(RBTLSConnection *transport) {
    return transport->deadline > 0 && CFAbsoluteTimeGetCurrent() >= transport->deadline;
}

static NSError *RBTLSError(NSInteger code, NSString *message) {
    return [NSError errorWithDomain:RBTLSErrorDomain code:code
                           userInfo:@{NSLocalizedDescriptionKey: message ?: @"Secure connection failed"}];
}

static OSStatus RBTLSReadCallback(SSLConnectionRef connection, void *data, size_t *dataLength) {
    RBTLSConnection *transport = (RBTLSConnection *)connection;
    int fd = transport->fd;
    size_t requested = *dataLength;
    ssize_t count;
    do { count = recv(fd, data, requested, 0); } while (count < 0 && errno == EINTR);
    if (count > 0) {
        *dataLength = (size_t)count;
        return (size_t)count == requested ? noErr : errSSLWouldBlock;
    }
    *dataLength = 0;
    if (count == 0) return errSSLClosedGraceful;
    if (errno == EAGAIN || errno == EWOULDBLOCK) return RBTLSDeadlineExpired(transport) ? errSecIO : errSSLWouldBlock;
    return errSecIO;
}

static OSStatus RBTLSWriteCallback(SSLConnectionRef connection, const void *data, size_t *dataLength) {
    RBTLSConnection *transport = (RBTLSConnection *)connection;
    int fd = transport->fd;
    size_t requested = *dataLength;
    ssize_t count;
    do { count = send(fd, data, requested, 0); } while (count < 0 && errno == EINTR);
    if (count >= 0) {
        *dataLength = (size_t)count;
        return (size_t)count == requested ? noErr : errSSLWouldBlock;
    }
    *dataLength = 0;
    if (errno == EAGAIN || errno == EWOULDBLOCK) return RBTLSDeadlineExpired(transport) ? errSecIO : errSSLWouldBlock;
    return errSecIO;
}

static int RBConnectSocket(NSString *host, NSInteger port, NSTimeInterval timeout) {
    struct addrinfo hints;
    memset(&hints, 0, sizeof(hints));
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = SOCK_STREAM;
    NSString *portString = [NSString stringWithFormat:@"%ld", (long)port];
    struct addrinfo *addresses = NULL;
    if (getaddrinfo([host UTF8String], [portString UTF8String], &hints, &addresses) != 0) return -1;
    int fd = -1;
    for (struct addrinfo *address = addresses; address; address = address->ai_next) {
        fd = socket(address->ai_family, address->ai_socktype, address->ai_protocol);
        if (fd < 0) continue;
        struct timeval value;
        value.tv_sec = 1;
        value.tv_usec = 0;
        setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &value, sizeof(value));
        setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &value, sizeof(value));
#ifdef SO_NOSIGPIPE
        int one = 1;
        setsockopt(fd, SOL_SOCKET, SO_NOSIGPIPE, &one, sizeof(one));
#endif
        int flags = fcntl(fd, F_GETFL, 0);
        if (flags >= 0) fcntl(fd, F_SETFL, flags | O_NONBLOCK);
        int connected = connect(fd, address->ai_addr, address->ai_addrlen);
        if (connected < 0 && errno == EINPROGRESS) {
            fd_set writable;
            FD_ZERO(&writable);
            FD_SET(fd, &writable);
            struct timeval connectTimeout;
            connectTimeout.tv_sec = MAX(1, (int)timeout);
            connectTimeout.tv_usec = 0;
            connected = select(fd + 1, NULL, &writable, NULL, &connectTimeout);
            if (connected > 0) {
                int socketError = 0;
                socklen_t socketErrorLength = sizeof(socketError);
                if (getsockopt(fd, SOL_SOCKET, SO_ERROR, &socketError, &socketErrorLength) < 0 || socketError) connected = -1;
            } else {
                connected = -1;
            }
        }
        if (connected == 0 || connected > 0) {
            if (flags >= 0) fcntl(fd, F_SETFL, flags);
            break;
        }
        close(fd);
        fd = -1;
    }
    freeaddrinfo(addresses);
    return fd;
}

static BOOL RBTLSWriteAll(SSLContextRef context, const void *bytes, NSUInteger length) {
    const unsigned char *cursor = bytes;
    while (length) {
        size_t written = 0;
        OSStatus status = SSLWrite(context, cursor, length, &written);
        cursor += written;
        length -= written;
        if (status == noErr) continue;
        if (status == errSSLWouldBlock) continue;
        return NO;
    }
    return YES;
}

static OSStatus RBTLSReadSome(SSLContextRef context, void *bytes, NSUInteger capacity, NSUInteger *received) {
    size_t count = 0;
    OSStatus status = SSLRead(context, bytes, capacity, &count);
    if (received) *received = count;
    return status;
}

static BOOL RBReadExact(SSLContextRef context, NSMutableData *target, NSUInteger length) {
    unsigned char buffer[16384];
    while (length) {
        NSUInteger count = 0;
        OSStatus status = RBTLSReadSome(context, buffer, MIN(sizeof(buffer), length), &count);
        if (count) { [target appendBytes:buffer length:count]; length -= count; }
        if (status == noErr || status == errSSLWouldBlock) continue;
        return length == 0;
    }
    return YES;
}

static NSData *RBReadLine(SSLContextRef context) {
    NSMutableData *line = [NSMutableData data];
    unsigned char byte = 0;
    while ([line length] < 16384) {
        NSUInteger count = 0;
        OSStatus status = RBTLSReadSome(context, &byte, 1, &count);
        if (count) {
            [line appendBytes:&byte length:1];
            NSUInteger length = [line length];
            const unsigned char *bytes = [line bytes];
            if (length >= 2 && bytes[length - 2] == '\r' && bytes[length - 1] == '\n') return line;
        }
        if (status == noErr || status == errSSLWouldBlock) continue;
        return nil;
    }
    return nil;
}

static NSString *RBTrimHeaderValue(NSString *value) {
    return [value stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
}

@interface RBSecureHTTPClient ()
@property(nonatomic, copy) NSString *expectedFingerprint;
@property(nonatomic, assign) BOOL allowUntrusted;
@property(nonatomic, assign) BOOL systemTrust;
@property(nonatomic, copy, readwrite) NSString *observedFingerprint;
@end

@implementation RBSecureHTTPClient

+ (void)initialize {
    if (self != [RBSecureHTTPClient class]) return;
    RBDeviceSessionCookies = [[NSMutableDictionary alloc] init];

    // Builds before Surf 0.10 stored device sessions in the process-wide
    // cookie jar. Cookies do not include a TCP port, so that could disclose a
    // session to another server on the same host. Remove those legacy values;
    // pinned sessions below are memory-only and keyed by server identity.
    NSHTTPCookieStorage *storage = [NSHTTPCookieStorage sharedHTTPCookieStorage];
    for (NSHTTPCookie *cookie in [[storage cookies] copy]) {
        if ([[cookie name] isEqualToString:RBDeviceSessionCookieName]) [storage deleteCookie:cookie];
    }
}

- (id)initWithFingerprint:(NSString *)fingerprint allowUntrusted:(BOOL)allowUntrusted {
    return [self initWithFingerprint:fingerprint allowUntrusted:allowUntrusted systemTrust:NO];
}

- (id)initWithFingerprint:(NSString *)fingerprint allowUntrusted:(BOOL)allowUntrusted systemTrust:(BOOL)systemTrust {
    self = [super init];
    if (self) {
        self.expectedFingerprint = [[fingerprint lowercaseString] copy];
        self.allowUntrusted = allowUntrusted;
        self.systemTrust = systemTrust;
    }
    return self;
}

+ (NSString *)fingerprintForTrust:(SecTrustRef)trust {
    if (!trust || SecTrustGetCertificateCount(trust) < 1) return nil;
    SecCertificateRef certificate = SecTrustGetCertificateAtIndex(trust, 0);
    if (!certificate) return nil;
    NSData *data = CFBridgingRelease(SecCertificateCopyData(certificate));
    if (![data length]) return nil;
    unsigned char digest[CC_SHA256_DIGEST_LENGTH];
    CC_SHA256([data bytes], (CC_LONG)[data length], digest);
    NSMutableString *result = [NSMutableString stringWithCapacity:CC_SHA256_DIGEST_LENGTH * 2];
    for (NSUInteger i = 0; i < CC_SHA256_DIGEST_LENGTH; i++) [result appendFormat:@"%02x", digest[i]];
    return result;
}

+ (BOOL)endpoint:(NSString *)endpoint usesTunnelInServer:(NSDictionary *)server {
    id values = [server objectForKey:@"tunnelEndpoints"];
    return [endpoint length] && [values isKindOfClass:[NSArray class]] && [values containsObject:endpoint];
}

+ (RBSecureHTTPClient *)clientForServer:(NSDictionary *)server {
    NSString *endpoint = [server objectForKey:@"lastEndpoint"];
    return [[RBSecureHTTPClient alloc] initWithFingerprint:[server objectForKey:@"fingerprint"]
                                            allowUntrusted:NO
                                                systemTrust:[self endpoint:endpoint usesTunnelInServer:server]];
}

+ (RBSecureHTTPClient *)clientForEndpoint:(NSString *)endpoint fingerprint:(NSString *)fingerprint {
    for (NSDictionary *server in [RBServerStore servers]) {
        if (![[server objectForKey:@"fingerprint"] isEqualToString:fingerprint]) continue;
        return [[RBSecureHTTPClient alloc] initWithFingerprint:fingerprint allowUntrusted:NO
                                                   systemTrust:[self endpoint:endpoint usesTunnelInServer:server]];
    }
    return [[RBSecureHTTPClient alloc] initWithFingerprint:fingerprint allowUntrusted:NO];
}

- (NSData *)sendSystemTrustedRequest:(NSURLRequest *)request response:(NSHTTPURLResponse **)response error:(NSError **)error {
    NSMutableURLRequest *systemRequest = [request mutableCopy];
    [systemRequest setHTTPShouldHandleCookies:NO];
    if ([self.expectedFingerprint length] && ![[systemRequest allHTTPHeaderFields] objectForKey:@"Cookie"]) {
        NSString *cookie = nil;
        @synchronized ([RBSecureHTTPClient class]) {
            cookie = [RBDeviceSessionCookies objectForKey:self.expectedFingerprint];
        }
        if ([cookie length]) [systemRequest setValue:cookie forHTTPHeaderField:@"Cookie"];
    }
    NSURLResponse *rawResponse = nil;
    NSData *data = [NSURLConnection sendSynchronousRequest:systemRequest returningResponse:&rawResponse error:error];
    NSHTTPURLResponse *httpResponse = [rawResponse isKindOfClass:[NSHTTPURLResponse class]] ? (NSHTTPURLResponse *)rawResponse : nil;
    if (response) *response = httpResponse;
    if (data && [self.expectedFingerprint length]) {
        NSDictionary *headers = [httpResponse allHeaderFields];
        for (NSHTTPCookie *cookie in [NSHTTPCookie cookiesWithResponseHeaderFields:headers forURL:[request URL]]) {
            if (![[cookie name] isEqualToString:RBDeviceSessionCookieName]) continue;
            NSDate *expires = [cookie expiresDate];
            @synchronized ([RBSecureHTTPClient class]) {
                if (![[cookie value] length] || (expires && [expires timeIntervalSinceNow] <= 0)) {
                    [RBDeviceSessionCookies removeObjectForKey:self.expectedFingerprint];
                } else {
                    [RBDeviceSessionCookies setObject:[NSString stringWithFormat:@"%@=%@", [cookie name], [cookie value]]
                                                forKey:self.expectedFingerprint];
                }
            }
        }
    }
    return data;
}

- (BOOL)handshake:(SSLContextRef)context error:(NSError **)error {
    BOOL checkedIdentity = NO;
    for (;;) {
        OSStatus status = SSLHandshake(context);
        if (status == errSSLPeerAuthCompleted) {
            SecTrustRef trust = NULL;
            if (SSLCopyPeerTrust(context, &trust) != noErr || !trust) {
                if (error) *error = RBTLSError(4, @"Could not read the server identity");
                return NO;
            }
            self.observedFingerprint = [RBSecureHTTPClient fingerprintForTrust:trust];
            SecTrustResultType trustResult = kSecTrustResultInvalid;
            OSStatus trustStatus = self.systemTrust ? SecTrustEvaluate(trust, &trustResult) : noErr;
            CFRelease(trust);
            BOOL trustedBySystem = trustStatus == noErr &&
                (trustResult == kSecTrustResultProceed || trustResult == kSecTrustResultUnspecified);
            checkedIdentity = [self.observedFingerprint length] &&
                (self.allowUntrusted || trustedBySystem || [self.observedFingerprint isEqualToString:self.expectedFingerprint]);
            if (!checkedIdentity) {
                if (error) *error = RBTLSError(2, @"Server Identity Changed");
                return NO;
            }
            continue;
        }
        if (status == errSSLWouldBlock) continue;
        if (status != noErr) {
            if (error) *error = RBTLSError(status, [NSString stringWithFormat:@"TLS 1.2 handshake failed (%d)", (int)status]);
            return NO;
        }
        if (!checkedIdentity) {
            SecTrustRef trust = NULL;
            if (SSLCopyPeerTrust(context, &trust) == noErr && trust) {
                self.observedFingerprint = [RBSecureHTTPClient fingerprintForTrust:trust];
                SecTrustResultType trustResult = kSecTrustResultInvalid;
                OSStatus trustStatus = self.systemTrust ? SecTrustEvaluate(trust, &trustResult) : noErr;
                BOOL trustedBySystem = trustStatus == noErr &&
                    (trustResult == kSecTrustResultProceed || trustResult == kSecTrustResultUnspecified);
                CFRelease(trust);
                if (self.systemTrust && !trustedBySystem) self.observedFingerprint = nil;
            }
            checkedIdentity = [self.observedFingerprint length] &&
                (self.allowUntrusted || self.systemTrust || [self.observedFingerprint isEqualToString:self.expectedFingerprint]);
        }
        if (!checkedIdentity) {
            if (error) *error = RBTLSError(2, @"Server Identity Changed");
            return NO;
        }
        return YES;
    }
}

- (NSData *)readResponse:(SSLContextRef)context URL:(NSURL *)url response:(NSHTTPURLResponse **)response error:(NSError **)error {
    NSMutableData *headerData = [NSMutableData data];
    unsigned char byte = 0;
    while ([headerData length] < 65536) {
        NSUInteger count = 0;
        OSStatus status = RBTLSReadSome(context, &byte, 1, &count);
        if (count) {
            [headerData appendBytes:&byte length:1];
            NSUInteger length = [headerData length];
            const unsigned char *bytes = [headerData bytes];
            if (length >= 4 && bytes[length - 4] == '\r' && bytes[length - 3] == '\n' && bytes[length - 2] == '\r' && bytes[length - 1] == '\n') break;
        }
        if (status == noErr || status == errSSLWouldBlock) continue;
        if (error) *error = RBTLSError(status, @"The server closed the connection before sending a response");
        return nil;
    }
    NSString *headerText = [[NSString alloc] initWithData:headerData encoding:NSISOLatin1StringEncoding];
    NSArray *lines = [headerText componentsSeparatedByString:@"\r\n"];
    if (![lines count]) { if (error) *error = RBTLSError(5, @"Invalid HTTP response"); return nil; }
    NSArray *statusParts = [[lines objectAtIndex:0] componentsSeparatedByString:@" "];
    NSInteger statusCode = [statusParts count] > 1 ? [[statusParts objectAtIndex:1] integerValue] : 0;
    NSMutableDictionary *headers = [NSMutableDictionary dictionary];
    NSMutableDictionary *lowerHeaders = [NSMutableDictionary dictionary];
    for (NSUInteger i = 1; i < [lines count]; i++) {
        NSString *line = [lines objectAtIndex:i];
        NSRange colon = [line rangeOfString:@":"];
        if (colon.location == NSNotFound) continue;
        NSString *name = [line substringToIndex:colon.location];
        NSString *value = RBTrimHeaderValue([line substringFromIndex:colon.location + 1]);
        [headers setObject:value forKey:name];
        [lowerHeaders setObject:value forKey:[name lowercaseString]];
    }
    if (response) *response = [[NSHTTPURLResponse alloc] initWithURL:url statusCode:statusCode HTTPVersion:@"HTTP/1.1" headerFields:headers];
    if (!self.allowUntrusted && [self.expectedFingerprint length]) {
        for (NSHTTPCookie *cookie in [NSHTTPCookie cookiesWithResponseHeaderFields:headers forURL:url]) {
            if (![[cookie name] isEqualToString:RBDeviceSessionCookieName]) continue;
            NSDate *expires = [cookie expiresDate];
            @synchronized ([RBSecureHTTPClient class]) {
                if (![[cookie value] length] || (expires && [expires timeIntervalSinceNow] <= 0)) {
                    [RBDeviceSessionCookies removeObjectForKey:self.expectedFingerprint];
                } else {
                    [RBDeviceSessionCookies setObject:[NSString stringWithFormat:@"%@=%@", [cookie name], [cookie value]]
                                                forKey:self.expectedFingerprint];
                }
            }
        }
    }

    NSMutableData *body = [NSMutableData data];
    NSString *transfer = [[lowerHeaders objectForKey:@"transfer-encoding"] lowercaseString];
    if ([transfer length] && [transfer rangeOfString:@"chunked"].location != NSNotFound) {
        for (;;) {
            NSData *lineData = RBReadLine(context);
            if (!lineData) { if (error) *error = RBTLSError(6, @"Incomplete chunked response"); return nil; }
            NSString *line = [[NSString alloc] initWithData:lineData encoding:NSASCIIStringEncoding];
            NSString *sizeText = [[[line componentsSeparatedByString:@";"] objectAtIndex:0] stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
            unsigned long long size = 0;
            [[NSScanner scannerWithString:sizeText] scanHexLongLong:&size];
            if (size == 0) {
                while ([(lineData = RBReadLine(context)) length] > 2) {}
                break;
            }
            if (size > NSUIntegerMax || !RBReadExact(context, body, (NSUInteger)size)) { if (error) *error = RBTLSError(7, @"Incomplete response body"); return nil; }
            NSMutableData *ending = [NSMutableData data];
            if (!RBReadExact(context, ending, 2)) { if (error) *error = RBTLSError(7, @"Incomplete response body"); return nil; }
        }
    } else if ([lowerHeaders objectForKey:@"content-length"]) {
        unsigned long long size = [[lowerHeaders objectForKey:@"content-length"] longLongValue];
        if (size > NSUIntegerMax || !RBReadExact(context, body, (NSUInteger)size)) { if (error) *error = RBTLSError(7, @"Incomplete response body"); return nil; }
    } else {
        unsigned char buffer[16384];
        for (;;) {
            NSUInteger count = 0;
            OSStatus status = RBTLSReadSome(context, buffer, sizeof(buffer), &count);
            if (count) [body appendBytes:buffer length:count];
            if (status == noErr || status == errSSLWouldBlock) continue;
            if (status == errSSLClosedGraceful || status == errSSLClosedAbort) break;
            if (error) *error = RBTLSError(status, @"Could not read the secure response");
            return nil;
        }
    }
    return body;
}

- (NSData *)sendRequest:(NSURLRequest *)request response:(NSHTTPURLResponse **)response error:(NSError **)error {
    self.observedFingerprint = nil;
    NSURL *url = [request URL];
    if (![[[url scheme] lowercaseString] isEqualToString:@"https"] || ![[url host] length]) {
        if (error) *error = RBTLSError(1, @"Surf requires an HTTPS server address");
        return nil;
    }
    if (self.systemTrust) return [self sendSystemTrustedRequest:request response:response error:error];
    NSInteger port = [[url port] integerValue] ?: 443;
    int fd = RBConnectSocket([url host], port, MAX(1.0, [request timeoutInterval]));
    if (fd < 0) { if (error) *error = RBTLSError(3, @"Could not connect to the Surf server"); return nil; }

    SSLContextRef context = SSLCreateContext(kCFAllocatorDefault, kSSLClientSide, kSSLStreamType);
    if (!context) { close(fd); if (error) *error = RBTLSError(4, @"Could not create TLS context"); return nil; }
    RBTLSConnection transport = { fd, CFAbsoluteTimeGetCurrent() + MAX(1.0, [request timeoutInterval]) };
    OSStatus ioStatus = SSLSetIOFuncs(context, RBTLSReadCallback, RBTLSWriteCallback);
    OSStatus connectionStatus = SSLSetConnection(context, &transport);
    OSStatus peerStatus = SSLSetPeerDomainName(context, [[url host] UTF8String], strlen([[url host] UTF8String]));
    OSStatus minStatus = SSLSetProtocolVersionMin(context, kTLSProtocol12);
    OSStatus maxStatus = SSLSetProtocolVersionMax(context, kTLSProtocol12);
    OSStatus authStatus = SSLSetSessionOption(context, kSSLSessionOptionBreakOnServerAuth, true);
    if (ioStatus != noErr || connectionStatus != noErr || peerStatus != noErr ||
        minStatus != noErr || maxStatus != noErr || authStatus != noErr) {
        if (error) *error = RBTLSError(4, [NSString stringWithFormat:
            @"TLS 1.2 setup failed (io=%d connection=%d peer=%d min=%d max=%d auth=%d)",
            (int)ioStatus, (int)connectionStatus, (int)peerStatus,
            (int)minStatus, (int)maxStatus, (int)authStatus]);
        CFRelease(context); close(fd); return nil;
    }
    if (![self handshake:context error:error]) { CFRelease(context); close(fd); return nil; }

    NSString *path = [url path];
    if (![path length]) path = @"/";
    if ([[url query] length]) path = [path stringByAppendingFormat:@"?%@", [url query]];
    NSString *hostHeader = [url host];
    if ([hostHeader rangeOfString:@":"].location != NSNotFound) hostHeader = [NSString stringWithFormat:@"[%@]", hostHeader];
    if ([url port] && port != 443) hostHeader = [hostHeader stringByAppendingFormat:@":%ld", (long)port];
    NSString *method = [request HTTPMethod] ?: @"GET";
    NSMutableDictionary *headers = [NSMutableDictionary dictionaryWithDictionary:[request allHTTPHeaderFields] ?: @{}];
    if (![headers objectForKey:@"Cookie"] && !self.allowUntrusted && [self.expectedFingerprint length]) {
        NSString *cookie = nil;
        @synchronized ([RBSecureHTTPClient class]) {
            cookie = [RBDeviceSessionCookies objectForKey:self.expectedFingerprint];
        }
        if ([cookie length]) [headers setObject:cookie forKey:@"Cookie"];
    }
    NSData *body = [request HTTPBody] ?: [NSData data];
    [headers setObject:hostHeader forKey:@"Host"];
    [headers setObject:@"close" forKey:@"Connection"];
    if ([body length]) [headers setObject:[NSString stringWithFormat:@"%lu", (unsigned long)[body length]] forKey:@"Content-Length"];
    NSMutableString *head = [NSMutableString stringWithFormat:@"%@ %@ HTTP/1.1\r\n", method, path];
    for (NSString *name in headers) [head appendFormat:@"%@: %@\r\n", name, [headers objectForKey:name]];
    [head appendString:@"\r\n"];
    NSData *headData = [head dataUsingEncoding:NSUTF8StringEncoding];
    BOOL wrote = RBTLSWriteAll(context, [headData bytes], [headData length]) &&
                 (![body length] || RBTLSWriteAll(context, [body bytes], [body length]));
    if (!wrote) { if (error) *error = RBTLSError(8, @"Could not send the secure request"); CFRelease(context); close(fd); return nil; }
    NSData *result = [self readResponse:context URL:url response:response error:error];
    SSLClose(context);
    CFRelease(context);
    close(fd);
    return result;
}

@end
