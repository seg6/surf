#import "RBTunnelPipe.h"
#import "RBLog.h"
#import "RBSocket.h"

#import <sys/socket.h>
#import <unistd.h>

@interface RBTunnelPipe () <RBSocketDelegate>
@property(nonatomic, copy) NSString *host;
@property(nonatomic, assign) NSInteger port;
@property(nonatomic, strong) RBSocket *outer;
@property(nonatomic, strong) NSCondition *condition;
@property(nonatomic, assign) int bridgeFD;
@property(nonatomic, assign) BOOL opened;
@property(nonatomic, assign) BOOL finished;
@property(nonatomic, copy) NSString *failure;
@end

@implementation RBTunnelPipe

- (id)initWithHost:(NSString *)host port:(NSInteger)port {
    self = [super init];
    if (self) {
        self.host = host;
        self.port = port;
        self.bridgeFD = -1;
        self.condition = [[NSCondition alloc] init];
    }
    return self;
}

- (int)open:(NSString **)error {
    int pair[2] = {-1, -1};
    if (socketpair(AF_UNIX, SOCK_STREAM, 0, pair) != 0) {
        if (error) *error = @"could not create tunnel stream";
        return -1;
    }
    self.bridgeFD = pair[1];
    self.outer = [[RBSocket alloc] initWithHost:self.host port:self.port
                                           path:@"/api/v1/tunnel" secure:YES
                                    fingerprint:nil systemTrust:YES];
    self.outer.delegate = self;
    [self.outer connect];

    NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:12.0];
    [self.condition lock];
    while (!self.opened && !self.finished && [deadline timeIntervalSinceNow] > 0) {
        [self.condition waitUntilDate:deadline];
    }
    BOOL ready = self.opened;
    NSString *failure = self.failure;
    [self.condition unlock];
    if (!ready) {
        close(pair[0]);
        [self close];
        if (error) *error = failure ?: @"tunnel connection timed out";
        return -1;
    }

    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_HIGH, 0), ^{
        unsigned char buffer[16384];
        while (self.bridgeFD >= 0) {
            ssize_t count = read(self.bridgeFD, buffer, sizeof(buffer));
            if (count <= 0) break;
            [self.outer sendBinary:[NSData dataWithBytes:buffer length:(NSUInteger)count]];
        }
        [self close];
    });
    RBLogEvent(@"tunnel", @"info", @{@"host": self.host ?: @"", @"port": @(self.port),
               @"path": @"/api/v1/tunnel"}, @"Outer WebSocket tunnel opened");
    return pair[0];
}

- (void)close {
    @synchronized (self) {
        if (self.bridgeFD >= 0) {
            shutdown(self.bridgeFD, SHUT_RDWR);
            close(self.bridgeFD);
            self.bridgeFD = -1;
        }
    }
    self.outer.delegate = nil;
    [self.outer close];
    self.outer = nil;
}

- (void)socketDidOpen:(RBSocket *)socket {
    [self.condition lock];
    self.opened = YES;
    [self.condition broadcast];
    [self.condition unlock];
}

- (void)socket:(RBSocket *)socket didCloseWithError:(NSString *)error {
    [self.condition lock];
    self.finished = YES;
    self.failure = error;
    [self.condition broadcast];
    [self.condition unlock];
    if (self.bridgeFD >= 0) shutdown(self.bridgeFD, SHUT_RDWR);
}

- (void)socket:(RBSocket *)socket didReceiveText:(NSString *)text {
    [self close];
}

- (void)socket:(RBSocket *)socket didReceiveBinary:(NSData *)data {
    const unsigned char *bytes = [data bytes];
    NSUInteger remaining = [data length];
    while (remaining && self.bridgeFD >= 0) {
        ssize_t count = write(self.bridgeFD, bytes, remaining);
        if (count <= 0) { [self close]; return; }
        bytes += count;
        remaining -= (NSUInteger)count;
    }
}

@end
