#import "RBSocket.h"
#import "RBLog.h"

#import <CommonCrypto/CommonDigest.h>
#import <arpa/inet.h>
#import <errno.h>
#import <netdb.h>
#import <sys/socket.h>
#import <unistd.h>

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
@property(nonatomic, assign) int fd;
@property(nonatomic, assign) BOOL running;
@property(nonatomic, strong) NSLock *writeLock;
@end

@implementation RBSocket

- (id)initWithHost:(NSString *)host port:(NSInteger)port path:(NSString *)path {
    self = [super init];
    if (self) {
        self.host = host;
        self.port = port;
        self.path = path;
        self.fd = -1;
        self.writeLock = [[NSLock alloc] init];
    }
    return self;
}

- (void)connect {
    self.running = YES;
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        NSString *error = nil;
        if (![self openAndHandshake:&error]) {
            [self notifyClose:error ?: @"connect failed"];
            return;
        }
        dispatch_async(dispatch_get_main_queue(), ^{ [self.delegate socketDidOpen:self]; });
        [self readLoop];
    });
}

- (void)close {
    self.running = NO;
    if (self.fd >= 0) {
        shutdown(self.fd, SHUT_RDWR);
        close(self.fd);
        self.fd = -1;
    }
}

- (BOOL)openAndHandshake:(NSString **)error {
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
        if (connect(fd, ai->ai_addr, ai->ai_addrlen) == 0) break;
        close(fd);
        fd = -1;
    }
    freeaddrinfo(res);
    if (fd < 0) {
        if (error) *error = @"tcp connect failed";
        return NO;
    }
    self.fd = fd;

    unsigned char randomKey[16];
    for (NSUInteger i = 0; i < sizeof(randomKey); i++) randomKey[i] = (unsigned char)(arc4random() & 0xff);
    NSString *key = RBBase64Encode([NSData dataWithBytes:randomKey length:sizeof(randomKey)]);
    NSString *hostHeader = self.port == 80 ? self.host : [NSString stringWithFormat:@"%@:%d", self.host, (int)self.port];
    NSString *request = [NSString stringWithFormat:
        @"GET %@ HTTP/1.1\r\nHost: %@\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %@\r\nSec-WebSocket-Version: 13\r\n\r\n",
        self.path, hostHeader, key];
    NSData *requestData = [request dataUsingEncoding:NSASCIIStringEncoding];
    if (!RBWriteAll(fd, [requestData bytes], [requestData length])) {
        if (error) *error = @"write upgrade failed";
        return NO;
    }

    NSMutableData *header = [NSMutableData data];
    unsigned char c;
    while ([header length] < 16384) {
        if (!RBReadAll(fd, &c, 1)) {
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
    RBLog(@"websocket open %@:%d%@", self.host, (int)self.port, self.path);
    return YES;
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
        unsigned char h[2];
        if (!RBReadAll(self.fd, h, 2)) break;
        BOOL fin = (h[0] & 0x80) != 0;
        unsigned char opcode = h[0] & 0x0f;
        BOOL masked = (h[1] & 0x80) != 0;
        unsigned long long len = h[1] & 0x7f;
        if (len == 126) {
            unsigned char ext[2];
            if (!RBReadAll(self.fd, ext, 2)) break;
            len = ((unsigned long long)ext[0] << 8) | ext[1];
        } else if (len == 127) {
            unsigned char ext[8];
            if (!RBReadAll(self.fd, ext, 8)) break;
            len = 0;
            for (NSUInteger i = 0; i < 8; i++) len = (len << 8) | ext[i];
        }
        if (len > 8ULL * 1024ULL * 1024ULL) break;
        unsigned char mask[4] = {0, 0, 0, 0};
        if (masked && !RBReadAll(self.fd, mask, 4)) break;
        NSMutableData *payload = [NSMutableData dataWithLength:(NSUInteger)len];
        if (len > 0 && !RBReadAll(self.fd, [payload mutableBytes], (NSUInteger)len)) break;
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
    [self notifyClose:@"socket closed"];
}

- (void)deliverPayload:(NSData *)payload opcode:(unsigned char)opcode {
    if (opcode == 0x1) {
        NSString *text = [[NSString alloc] initWithData:payload encoding:NSUTF8StringEncoding];
        dispatch_async(dispatch_get_main_queue(), ^{ [self.delegate socket:self didReceiveText:text ?: @""]; });
    } else if (opcode == 0x2) {
        NSData *copy = [payload copy];
        dispatch_async(dispatch_get_main_queue(), ^{ [self.delegate socket:self didReceiveBinary:copy]; });
    }
}

- (void)notifyClose:(NSString *)error {
    [self close];
    dispatch_async(dispatch_get_main_queue(), ^{ [self.delegate socket:self didCloseWithError:error]; });
}

- (void)sendJSON:(NSDictionary *)message {
    NSData *data = [NSJSONSerialization dataWithJSONObject:message options:0 error:nil];
    if (data) [self sendFrameOpcode:0x1 payload:data];
}

- (void)sendFrameOpcode:(unsigned char)opcode payload:(NSData *)payload {
    if (self.fd < 0 || !self.running) return;
    NSUInteger len = [payload length];
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
    NSMutableData *masked = [payload mutableCopy];
    unsigned char *p = (unsigned char *)[masked mutableBytes];
    for (NSUInteger i = 0; i < len; i++) p[i] ^= mask[i & 3];
    [frame appendData:masked];

    [self.writeLock lock];
    BOOL ok = RBWriteAll(self.fd, [frame bytes], [frame length]);
    [self.writeLock unlock];
    if (!ok) [self close];
}

@end
