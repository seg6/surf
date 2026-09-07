#import "RBClockSync.h"
#import <QuartzCore/QuartzCore.h>

@interface RBClockSync ()
@property(nonatomic, assign) CFTimeInterval sentAt;
@property(nonatomic, assign) CFTimeInterval lastProbeAt;
@property(nonatomic, strong) NSMutableArray *roundTrips;
@property(nonatomic, assign, readwrite) double lastRTTMS;
@end

@implementation RBClockSync

- (id)init {
    self = [super init];
    if (self) self.roundTrips = [NSMutableArray arrayWithCapacity:8];
    return self;
}

- (void)reset {
    self.sentAt = 0.0;
    self.lastProbeAt = 0.0;
    self.lastRTTMS = 0.0;
    [self.roundTrips removeAllObjects];
}

- (NSDictionary *)probeIfIdle {
    CFTimeInterval now = CACurrentMediaTime();
    if (self.sentAt > 0.0 && now - self.sentAt <= 10.0) return nil;
    if (self.sentAt > 0.0) self.sentAt = 0.0;
    // Connection setup competes with the first IDR, decoder creation, and
    // AudioQueue startup. Take a short burst of samples so one noisy startup
    // exchange cannot be displayed as the path RTT for the next 30 seconds.
    double interval = [self.roundTrips count] < 4 ? 1.0 : 30.0;
    if (self.lastProbeAt > 0.0 && now - self.lastProbeAt < interval) return nil;
    self.lastProbeAt = now;
    self.sentAt = now;
    return @{@"t": @"clock", @"c0": [NSNumber numberWithUnsignedLongLong:(unsigned long long)(now * 1e9)]};
}

- (BOOL)consumeControlMessage:(NSDictionary *)message {
    if (![[message objectForKey:@"t"] isEqualToString:@"clock"]) return NO;
    if (self.sentAt <= 0.0) return YES;
    unsigned long long c0 = [[message objectForKey:@"c0"] unsignedLongLongValue];
    unsigned long long s1 = [[message objectForKey:@"s1"] unsignedLongLongValue];
    unsigned long long s2 = [[message objectForKey:@"s2"] unsignedLongLongValue];
    unsigned long long c3 = (unsigned long long)(CACurrentMediaTime() * 1e9);
    unsigned long long elapsed = c3 >= c0 ? c3 - c0 : 0;
    unsigned long long server = s2 >= s1 ? s2 - s1 : 0;
    double rtt = elapsed >= server ? (double)(elapsed - server) / 1e6 : 0.0;
    [self.roundTrips addObject:[NSNumber numberWithDouble:rtt]];
    if ([self.roundTrips count] > 8) [self.roundTrips removeObjectAtIndex:0];
    double best = rtt;
    for (NSNumber *sample in self.roundTrips) best = MIN(best, [sample doubleValue]);
    // Do not publish the first handshake-era sample. The next watchdog tick
    // supplies a second sample and the lowest-RTT estimator converges quickly.
    if ([self.roundTrips count] >= 2) self.lastRTTMS = best;
    self.sentAt = 0.0;
    return YES;
}

@end
