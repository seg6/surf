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

- (NSDictionary *)probeIfIdle {
    CFTimeInterval now = CACurrentMediaTime();
    if (self.sentAt > 0.0 && now - self.sentAt <= 10.0) return nil;
    if (self.sentAt > 0.0) self.sentAt = 0.0;
    if (self.lastProbeAt > 0.0 && now - self.lastProbeAt < 30.0) return nil;
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
    self.lastRTTMS = best;
    self.sentAt = 0.0;
    return YES;
}

@end
