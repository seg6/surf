#import "RBSocket.h"
#import "RBLog.h"
#import "RBSecureHTTPClient.h"

#import <CFNetwork/CFSocketStream.h>
#import <CommonCrypto/CommonDigest.h>
#import <QuartzCore/QuartzCore.h>
#import <Security/SecureTransport.h>
#import <arpa/inet.h>
#import <errno.h>
#import <fcntl.h>
#import <netdb.h>
#import <netinet/in.h>
#import <netinet/tcp.h>
#import <sys/socket.h>
#import <sys/time.h>
#import <unistd.h>

static const int RBSocketTimeoutSeconds = 8;

static NSString *RBBase64Encode(NSData *data) {
    static const char table[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    const unsigned char *bytes = (const unsigned char *)[data bytes];
    NSUInteger len = [data length];
    NSMutableString *out = [NSMutableString stringWithCapacity:((len + 2) / 3) * 4];
    for (NSUInteger i = 0; i < len; i += 3) {
        unsigned int v = (unsigned int)bytes[i] << 16;
        BOOL has2 = i + 1 < len;
        BOOL has3 = i + 2 < len;
        if (has2) v |= (unsigned int)bytes[i + 1] << 8;
        if (has3) v |= (unsigned int)bytes[i + 2];
        [out appendFormat:@"%c%c%c%c", table[(v >> 18) & 63], table[(v >> 12) & 63], has2 ? table[(v >> 6) & 63] : '=', has3 ? table[v & 63] : '='];
    }
    return out;
}

static BOOL RBWriteAll(int fd, const void *buf, NSUInteger len) {
    const unsigned char *p = (const unsigned char *)buf;
    while (len > 0) {
        ssize_t n = write(fd, p, len);
        if (n < 0) {
            if (errno == EINTR) continue;
            return NO;
        }
        if (n == 0) return NO;
        p += n;
        len -= (NSUInteger)n;
    }
    return YES;
}

static void RBSetSocketOptions(int fd) {
    struct timeval tv;
    tv.tv_sec = RBSocketTimeoutSeconds;
    tv.tv_usec = 0;
    setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));
    setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &tv, sizeof(tv));
#ifdef SO_NOSIGPIPE
    int one = 1;
    setsockopt(fd, SOL_SOCKET, SO_NOSIGPIPE, &one, sizeof(one));
#endif
    // Every message on this socket is either a small control frame or one
    // binary frame we want on the wire immediately (H.264 AU/PCM) — Nagle's
    // coalescing delay (up to ~40ms) only hurts here, never helps.
    int noDelay = 1;
    setsockopt(fd, IPPROTO_TCP, TCP_NODELAY, &noDelay, sizeof(noDelay));
}

static BOOL RBConnectWithTimeout(int fd, const struct sockaddr *addr, socklen_t len) {
    int flags = fcntl(fd, F_GETFL, 0);
    if (flags < 0) return connect(fd, addr, len) == 0;
    if (fcntl(fd, F_SETFL, flags | O_NONBLOCK) < 0) return connect(fd, addr, len) == 0;

    int rc = connect(fd, addr, len);
    if (rc == 0) {
        fcntl(fd, F_SETFL, flags);
        return YES;
    }
    if (errno != EINPROGRESS) {
        fcntl(fd, F_SETFL, flags);
        return NO;
    }

    fd_set wfds;
    FD_ZERO(&wfds);
    FD_SET(fd, &wfds);
    struct timeval tv;
    tv.tv_sec = RBSocketTimeoutSeconds;
    tv.tv_usec = 0;
    rc = select(fd + 1, NULL, &wfds, NULL, &tv);
    if (rc <= 0) {
        fcntl(fd, F_SETFL, flags);
        return NO;
    }
    int err = 0;
    socklen_t errLen = sizeof(err);
    if (getsockopt(fd, SOL_SOCKET, SO_ERROR, &err, &errLen) != 0 || err != 0) {
        fcntl(fd, F_SETFL, flags);
        return NO;
    }
    fcntl(fd, F_SETFL, flags);
    return YES;
}

static BOOL RBReadAll(int fd, void *buf, NSUInteger len) {
    unsigned char *p = (unsigned char *)buf;
    while (len > 0) {
        ssize_t n = read(fd, p, len);
        if (n < 0) {
            if (errno == EINTR) continue;
            return NO;
        }
        if (n == 0) return NO;
        p += n;
        len -= (NSUInteger)n;
    }
    return YES;
}

@interface RBSocket ()
@property(nonatomic, copy) NSString *host;
@property(nonatomic, copy) NSString *path;
@property(nonatomic, assign) NSInteger port;
@property(nonatomic, assign) BOOL secure;
@property(nonatomic, copy) NSString *expectedFingerprint;
@property(nonatomic, assign) int fd;
@property(nonatomic, assign) BOOL running;
@property(nonatomic, assign) BOOL connectStarted;
@property(nonatomic, assign) BOOL closeNotified;
@property(nonatomic, strong) NSLock *writeLock;
@property(nonatomic, strong) NSLock *tlsLock;
@property(nonatomic, strong) dispatch_queue_t writeQueue;
@property(nonatomic, assign) SSLContextRef tlsContext;
@property(nonatomic, assign) BOOL closing;
- (int)socketFileDescriptor;
@end

static OSStatus RBSocketTLSRead(SSLConnectionRef connection, void *data, size_t *dataLength) {
    RBSocket *socket = (__bridge RBSocket *)connection;
    int fd = [socket socketFileDescriptor];
    size_t requested = *dataLength;
    ssize_t count;
    do { count = recv(fd, data, requested, 0); } while (count < 0 && errno == EINTR);
    if (count > 0) {
        *dataLength = (size_t)count;
        return (size_t)count == requested ? noErr : errSSLWouldBlock;
    }
    *dataLength = 0;
    if (count == 0) return errSSLClosedGraceful;
    if (errno == EAGAIN || errno == EWOULDBLOCK) return errSSLWouldBlock;
    return errSecIO;
}

static OSStatus RBSocketTLSWrite(SSLConnectionRef connection, const void *data, size_t *dataLength) {
    RBSocket *socket = (__bridge RBSocket *)connection;
    int fd = [socket socketFileDescriptor];
    size_t requested = *dataLength;
    ssize_t count;
    do { count = send(fd, data, requested, 0); } while (count < 0 && errno == EINTR);
    if (count >= 0) {
        *dataLength = (size_t)count;
        return (size_t)count == requested ? noErr : errSSLWouldBlock;
    }
    *dataLength = 0;
    if (errno == EAGAIN || errno == EWOULDBLOCK) return errSSLWouldBlock;
    return errSecIO;
}

@implementation RBSocket

- (int)socketFileDescriptor { return self.fd; }

- (id)initWithHost:(NSString *)host port:(NSInteger)port path:(NSString *)path secure:(BOOL)secure fingerprint:(NSString *)fingerprint {
    self = [super init];
    if (self) {
        self.host = host;
        self.port = port;
        self.path = path;
        self.secure = secure;
        self.expectedFingerprint = [fingerprint lowercaseString];
        self.fd = -1;
        self.writeLock = [[NSLock alloc] init];
        self.tlsLock = [[NSLock alloc] init];
        self.writeQueue = dispatch_queue_create("surf.socket.write", DISPATCH_QUEUE_SERIAL);
    }
    return self;
}

- (void)connect {
    @synchronized (self) {
        if (self.connectStarted) return;
        self.connectStarted = YES;
        self.running = YES;
    }
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        NSString *error = nil;
        if (![self openAndHandshake:&error]) {
            [self notifyClose:error ?: @"connect failed"];
            return;
        }
        if (!self.running) {
            [self close];
            return;
        }
        dispatch_async(dispatch_get_main_queue(), ^{
            id<RBSocketDelegate> delegate = self.delegate;
            if (self.running && [delegate respondsToSelector:@selector(socketDidOpen:)]) [delegate socketDidOpen:self];
            else [self close];
        });
        [self readLoop];
    });
}

- (void)close {
    @synchronized (self) {
        if (self.closing) return;
        self.closing = YES;
        self.running = NO;
    }

    // Wake a blocked TLS operation before waiting for the context owner. Old
    // Secure Transport releases are not safe when SSLRead and SSLWrite enter
    // the same context concurrently, so both directions share tlsLock.
    if (self.fd >= 0) shutdown(self.fd, SHUT_RDWR);
    [self.tlsLock lock];
    if (self.tlsContext) {
        CFRelease(self.tlsContext);
        self.tlsContext = NULL;
    }
    if (self.fd >= 0) { close(self.fd); self.fd = -1; }
    [self.tlsLock unlock];
}

- (BOOL)openAndHandshake:(NSString **)error {
    if (self.secure) {
        if (![self openTLS:error]) return NO;
    } else if (![self openTCP:error]) {
        return NO;
    }

    unsigned char randomKey[16];
    for (NSUInteger i = 0; i < sizeof(randomKey); i++) randomKey[i] = (unsigned char)(arc4random() & 0xff);
    NSString *key = RBBase64Encode([NSData dataWithBytes:randomKey length:sizeof(randomKey)]);
    BOOL defaultPort = (!self.secure && self.port == 80) || (self.secure && self.port == 443);
    NSString *headerHost = [self.host rangeOfString:@":"].location == NSNotFound ? self.host : [NSString stringWithFormat:@"[%@]", self.host];
    NSString *hostHeader = defaultPort ? headerHost : [NSString stringWithFormat:@"%@:%d", headerHost, (int)self.port];
    NSString *request = [NSString stringWithFormat:
        @"GET %@ HTTP/1.1\r\nHost: %@\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %@\r\nSec-WebSocket-Version: 13\r\n\r\n",
        self.path, hostHeader, key];
    NSData *requestData = [request dataUsingEncoding:NSASCIIStringEncoding];
    [self.writeLock lock];
    BOOL wroteRequest = [self writeAll:[requestData bytes] length:[requestData length]];
    [self.writeLock unlock];
    if (!wroteRequest) {
        if (error) *error = @"write upgrade failed";
        return NO;
    }

    NSMutableData *header = [NSMutableData data];
    unsigned char c;
    while ([header length] < 16384) {
        if (![self readAll:&c length:1]) {
            if (error) *error = @"read upgrade failed";
            return NO;
        }
        [header appendBytes:&c length:1];
        if ([header length] >= 4) {
            const unsigned char *b = (const unsigned char *)[header bytes];
            NSUInteger n = [header length];
            if (b[n - 4] == '\r' && b[n - 3] == '\n' && b[n - 2] == '\r' && b[n - 1] == '\n') break;
        }
    }

    NSString *response = [[NSString alloc] initWithData:header encoding:NSASCIIStringEncoding];
    if ([response rangeOfString:@" 101 "].location == NSNotFound) {
        if (error) *error = [NSString stringWithFormat:@"upgrade rejected: %@", response ?: @"?"];
        return NO;
    }
    NSString *accept = [self header:@"Sec-WebSocket-Accept" inResponse:response];
    NSString *joined = [key stringByAppendingString:@"258EAFA5-E914-47DA-95CA-C5AB0DC85B11"];
    unsigned char digest[CC_SHA1_DIGEST_LENGTH];
    CC_SHA1([joined UTF8String], (CC_LONG)[joined lengthOfBytesUsingEncoding:NSUTF8StringEncoding], digest);
    NSString *want = RBBase64Encode([NSData dataWithBytes:digest length:sizeof(digest)]);
    if (!accept || ![accept isEqualToString:want]) {
        if (error) *error = @"bad websocket accept";
        return NO;
    }
    NSString *safePath = [[self.path componentsSeparatedByString:@"?"] objectAtIndex:0];
    RBLog(@"websocket open %@%@:%d%@", self.secure ? @"tls " : @"", self.host, (int)self.port, safePath);
    return YES;
}

- (BOOL)openTCP:(NSString **)error {
    struct addrinfo hints;
    memset(&hints, 0, sizeof(hints));
    hints.ai_socktype = SOCK_STREAM;
    hints.ai_family = AF_UNSPEC;
    NSString *portString = [NSString stringWithFormat:@"%d", (int)self.port];
    struct addrinfo *res = NULL;
    int gai = getaddrinfo([self.host UTF8String], [portString UTF8String], &hints, &res);
    if (gai != 0) {
        if (error) *error = [NSString stringWithFormat:@"dns: %s", gai_strerror(gai)];
        return NO;
    }

    int fd = -1;
    for (struct addrinfo *ai = res; ai != NULL; ai = ai->ai_next) {
        fd = socket(ai->ai_family, ai->ai_socktype, ai->ai_protocol);
        if (fd < 0) continue;
        RBSetSocketOptions(fd);
        if (RBConnectWithTimeout(fd, ai->ai_addr, ai->ai_addrlen)) break;
        close(fd);
        fd = -1;
    }
    freeaddrinfo(res);
    if (fd < 0) {
        if (error) *error = @"tcp connect failed";
        return NO;
    }
    @synchronized (self) {
        // close may run while DNS/connect is in flight. Never publish a new
        // descriptor after the socket has already been cancelled.
        if (!self.running) {
            close(fd);
            if (error) *error = @"socket closed";
            return NO;
        }
        self.fd = fd;
    }
    return YES;
}

- (BOOL)openTLS:(NSString **)error {
    if (![self openTCP:error]) return NO;

    // Secure Transport on iOS 6 cannot tolerate another thread releasing the
    // context while SSLHandshake is using it. The same lock covers handshake,
    // all later reads/writes, and final teardown.
    [self.tlsLock lock];
    if (!self.running) {
        if (self.fd >= 0) { close(self.fd); self.fd = -1; }
        [self.tlsLock unlock];
        if (error) *error = @"socket closed";
        return NO;
    }
    SSLContextRef context = SSLCreateContext(kCFAllocatorDefault, kSSLClientSide, kSSLStreamType);
    if (!context) {
        close(self.fd); self.fd = -1;
        [self.tlsLock unlock];
        if (error) *error = @"tls context create failed";
        return NO;
    }
    self.tlsContext = context;
    OSStatus ioStatus = SSLSetIOFuncs(context, RBSocketTLSRead, RBSocketTLSWrite);
    OSStatus connectionStatus = SSLSetConnection(context, (__bridge SSLConnectionRef)self);
    OSStatus peerStatus = SSLSetPeerDomainName(context, [self.host UTF8String], strlen([self.host UTF8String]));
    OSStatus minStatus = SSLSetProtocolVersionMin(context, kTLSProtocol12);
    OSStatus maxStatus = SSLSetProtocolVersionMax(context, kTLSProtocol12);
    OSStatus authStatus = SSLSetSessionOption(context, kSSLSessionOptionBreakOnServerAuth, true);
    if (ioStatus != noErr || connectionStatus != noErr || peerStatus != noErr ||
        minStatus != noErr || maxStatus != noErr || authStatus != noErr) {
        if (error) *error = @"TLS 1.2 configuration failed";
        CFRelease(context); self.tlsContext = NULL;
        close(self.fd); self.fd = -1;
        [self.tlsLock unlock];
        return NO;
    }

    BOOL checkedIdentity = NO;
    for (;;) {
        OSStatus status = SSLHandshake(context);
        if (status == errSSLPeerAuthCompleted || status == noErr) {
            SecTrustRef trust = NULL;
            if (SSLCopyPeerTrust(context, &trust) == noErr && trust) {
                NSString *fingerprint = [RBSecureHTTPClient fingerprintForTrust:trust];
                CFRelease(trust);
                checkedIdentity = [self.expectedFingerprint length] && [fingerprint isEqualToString:self.expectedFingerprint];
            }
            if (!checkedIdentity) {
                if (error) *error = @"Server Identity Changed";
                break;
            }
            if (status == noErr) {
                [self.tlsLock unlock];
                return YES;
            }
            continue;
        }
        if (status == errSSLWouldBlock) continue;
        if (error) *error = [NSString stringWithFormat:@"TLS 1.2 handshake failed (%d)", (int)status];
        break;
    }
    CFRelease(context); self.tlsContext = NULL;
    close(self.fd); self.fd = -1;
    [self.tlsLock unlock];
    return NO;
}

- (BOOL)readAll:(void *)buf length:(NSUInteger)len {
    if (!self.secure) return self.fd >= 0 && RBReadAll(self.fd, buf, len);
    unsigned char *p = (unsigned char *)buf;
    while (len > 0 && self.running) {
        size_t count = 0;
        [self.tlsLock lock];
        SSLContextRef context = self.running ? self.tlsContext : NULL;
        OSStatus status = context ? SSLRead(context, p, len, &count) : errSSLClosedAbort;
        [self.tlsLock unlock];
        p += count;
        len -= count;
        if (status == noErr || status == errSSLWouldBlock) continue;
        return NO;
    }
    return len == 0;
}

- (BOOL)writeAll:(const void *)buf length:(NSUInteger)len {
    if (!self.secure) return self.fd >= 0 && RBWriteAll(self.fd, buf, len);
    const unsigned char *p = (const unsigned char *)buf;
    while (len > 0 && self.running) {
        size_t count = 0;
        [self.tlsLock lock];
        SSLContextRef context = self.running ? self.tlsContext : NULL;
        OSStatus status = context ? SSLWrite(context, p, len, &count) : errSSLClosedAbort;
        [self.tlsLock unlock];
        p += count;
        len -= count;
        if (status == noErr || status == errSSLWouldBlock) continue;
        return NO;
    }
    return len == 0;
}

- (NSString *)header:(NSString *)name inResponse:(NSString *)response {
    NSArray *lines = [response componentsSeparatedByString:@"\r\n"];
    NSString *prefix = [[name stringByAppendingString:@":"] lowercaseString];
    for (NSString *line in lines) {
        NSString *lower = [line lowercaseString];
        if ([lower hasPrefix:prefix]) {
            NSString *value = [line substringFromIndex:[prefix length]];
            return [value stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
        }
    }
    return nil;
}

- (void)readLoop {
    NSMutableData *fragment = nil;
    unsigned char fragmentOpcode = 0;
    while (self.running) {
      @autoreleasepool {
        unsigned char h[2];
        if (![self readAll:h length:2]) break;
        BOOL fin = (h[0] & 0x80) != 0;
        unsigned char opcode = h[0] & 0x0f;
        BOOL masked = (h[1] & 0x80) != 0;
        unsigned long long len = h[1] & 0x7f;
        if (len == 126) {
            unsigned char ext[2];
            if (![self readAll:ext length:2]) break;
            len = ((unsigned long long)ext[0] << 8) | ext[1];
        } else if (len == 127) {
            unsigned char ext[8];
            if (![self readAll:ext length:8]) break;
            len = 0;
            for (NSUInteger i = 0; i < 8; i++) len = (len << 8) | ext[i];
        }
        if (len > 8ULL * 1024ULL * 1024ULL) break;
        unsigned char mask[4] = {0, 0, 0, 0};
        if (masked && ![self readAll:mask length:4]) break;
        NSMutableData *payload = [NSMutableData dataWithLength:(NSUInteger)len];
        if (len > 0 && ![self readAll:[payload mutableBytes] length:(NSUInteger)len]) break;
        if (masked) {
            unsigned char *p = (unsigned char *)[payload mutableBytes];
            for (NSUInteger i = 0; i < (NSUInteger)len; i++) p[i] ^= mask[i & 3];
        }

        if (opcode == 0x8) {
            [self sendFrameOpcode:0x8 payload:payload];
            break;
        } else if (opcode == 0x9) {
            [self sendFrameOpcode:0xA payload:payload];
        } else if (opcode == 0x1 || opcode == 0x2 || opcode == 0x0) {
            if (opcode == 0x0) {
                if (!fragment) break;
                [fragment appendData:payload];
            } else if (fin) {
                [self deliverPayload:payload opcode:opcode];
                continue;
            } else {
                fragment = [payload mutableCopy];
                fragmentOpcode = opcode;
            }
            if (fin && fragment) {
                [self deliverPayload:fragment opcode:fragmentOpcode];
                fragment = nil;
                fragmentOpcode = 0;
            }
        }
      }
    }
    [self notifyClose:@"socket closed"];
}

- (void)deliverPayload:(NSData *)payload opcode:(unsigned char)opcode {
    if (opcode == 0x1) {
        NSString *text = [[NSString alloc] initWithData:payload encoding:NSUTF8StringEncoding];
        // Preserve WebSocket wire order across control and binary messages.
        // In particular, video-config must finish configuring VideoToolbox
        // before the immediately-following IDR reaches the media pipeline.
        // The socket read loop is never the main thread, so this cannot
        // self-deadlock. Binary payloads still bypass the main queue.
        dispatch_sync(dispatch_get_main_queue(), ^{
            id<RBSocketDelegate> delegate = self.delegate;
            if (self.running && [delegate respondsToSelector:@selector(socket:didReceiveText:)]) [delegate socket:self didReceiveText:text ?: @""];
        });
    } else if (opcode == 0x2) {
        // Media must never transit UIKit's main queue. At 30 video AUs plus
        // 50 audio packets per second, posting every packet there created a
        // persistent queue behind touch handling and display-link callbacks;
        // latency echoes routinely arrived 1s late even on a healthy LAN.
        // This method already runs on the socket's single read thread, while
        // RBVideoDecoder immediately hands AUs to its own serial queue and
        // RBAudioPlayer protects its AudioQueue state with a lock.
        id<RBSocketDelegate> delegate = self.delegate;
        if (self.running && [delegate respondsToSelector:@selector(socket:didReceiveBinary:)]) {
            [delegate socket:self didReceiveBinary:payload];
        }
    }
}

- (void)notifyClose:(NSString *)error {
    __block id<RBSocketDelegate> delegate = nil;
    @synchronized (self) {
        if (self.closeNotified) return;
        self.closeNotified = YES;
        delegate = self.delegate;
    }
    [self close];
    dispatch_async(dispatch_get_main_queue(), ^{
        if ([delegate respondsToSelector:@selector(socket:didCloseWithError:)]) [delegate socket:self didCloseWithError:error];
    });
}

- (void)sendJSON:(NSDictionary *)message {
    NSData *data = [NSJSONSerialization dataWithJSONObject:message options:0 error:nil];
    if (data) [self sendFrameOpcode:0x1 payload:data async:YES];
}

- (void)sendFrameOpcode:(unsigned char)opcode payload:(NSData *)payload {
	[self sendFrameOpcode:opcode payload:payload async:NO];
}

- (void)sendFrameOpcode:(unsigned char)opcode payload:(NSData *)payload async:(BOOL)async {
    if (!self.running) return;
    NSData *payloadCopy = [payload copy];
    if (async) {
        CFTimeInterval enqueuedAt = CACurrentMediaTime();
        dispatch_async(self.writeQueue, ^{
            double dwellMS = (CACurrentMediaTime() - enqueuedAt) * 1000.0;
            if (dwellMS >= 10.0) RBLog(@"socket control queue dwell %.1fms", dwellMS);
            [self sendFrameOpcode:opcode payload:payloadCopy async:NO];
        });
        return;
    }
    NSUInteger len = [payloadCopy length];
    NSMutableData *frame = [NSMutableData data];
    unsigned char b0 = 0x80 | opcode;
    [frame appendBytes:&b0 length:1];
    if (len < 126) {
        unsigned char b1 = 0x80 | (unsigned char)len;
        [frame appendBytes:&b1 length:1];
    } else if (len <= 65535) {
        unsigned char h[4] = {0x80 | 126, (unsigned char)((len >> 8) & 0xff), (unsigned char)(len & 0xff)};
        [frame appendBytes:h length:3];
    } else {
        unsigned char h[9];
        h[0] = 0x80 | 127;
        unsigned long long n = len;
        for (int i = 8; i >= 1; i--) { h[i] = (unsigned char)(n & 0xff); n >>= 8; }
        [frame appendBytes:h length:9];
    }
    unsigned char mask[4];
    for (NSUInteger i = 0; i < 4; i++) mask[i] = (unsigned char)(arc4random() & 0xff);
    [frame appendBytes:mask length:4];
    NSMutableData *masked = [payloadCopy mutableCopy];
    unsigned char *p = (unsigned char *)[masked mutableBytes];
    for (NSUInteger i = 0; i < len; i++) p[i] ^= mask[i & 3];
    [frame appendData:masked];

    [self.writeLock lock];
    BOOL ok = [self writeAll:[frame bytes] length:[frame length]];
    [self.writeLock unlock];
    if (!ok) [self close];
}

@end
