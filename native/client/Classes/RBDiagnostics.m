#import "RBDiagnostics.h"
#import <QuartzCore/QuartzCore.h>

static const NSUInteger RBTraceCapacity = 4096;

@interface RBDiagnostics ()
@property(nonatomic, assign) CFTimeInterval latencySentAt;
@property(nonatomic, assign) CFTimeInterval lastProbeAt;
@property(nonatomic, strong) NSMutableArray *clockRTTs;
@property(nonatomic, assign) long long clockOffsetNS;
@property(nonatomic, assign, readwrite) double lastRTTMS;
@property(nonatomic, assign, readwrite, getter=isCapturing) BOOL capturing;
@property(nonatomic, strong) NSMutableArray *trace;
@property(nonatomic, assign, readwrite) NSUInteger droppedTraceEvents;
@end

@implementation RBDiagnostics

- (id)init {
    self = [super init];
    if (self) {
        self.trace = [NSMutableArray arrayWithCapacity:RBTraceCapacity];
        self.clockRTTs = [NSMutableArray arrayWithCapacity:8];
    }
    return self;
}

- (NSDictionary *)latencyProbeIfIdle {
    CFTimeInterval now = CACurrentMediaTime();
    // A disconnected socket can eat a probe. Do not let one missing reply
    // suppress clock synchronization for the rest of the session.
    if (self.latencySentAt > 0.0 && now - self.latencySentAt <= 10.0) return nil;
    if (self.latencySentAt > 0.0) self.latencySentAt = 0.0;
    CFTimeInterval interval = self.capturing ? 5.0 : 30.0;
    if (self.lastProbeAt > 0.0 && now - self.lastProbeAt < interval) return nil;
    self.lastProbeAt = now;
    self.latencySentAt = now;
    unsigned long long ns = (unsigned long long)(self.latencySentAt * 1e9);
    return @{@"t": @"clock", @"c0": [NSNumber numberWithUnsignedLongLong:ns]};
}

- (BOOL)consumeControlMessage:(NSDictionary *)message {
    if (![[message objectForKey:@"t"] isEqualToString:@"clock"]) return NO;
    if (self.latencySentAt > 0.0) {
        unsigned long long c0 = [[message objectForKey:@"c0"] unsignedLongLongValue];
        unsigned long long s1 = [[message objectForKey:@"s1"] unsignedLongLongValue];
        unsigned long long s2 = [[message objectForKey:@"s2"] unsignedLongLongValue];
        unsigned long long c3 = (unsigned long long)(CACurrentMediaTime() * 1e9);
        unsigned long long elapsed = c3 >= c0 ? c3 - c0 : 0;
        unsigned long long server = s2 >= s1 ? s2 - s1 : 0;
        double rtt = elapsed >= server ? (double)(elapsed - server) / 1e6 : 0.0;
        long long offset = ((long long)s1 - (long long)c0 + (long long)s2 - (long long)c3) / 2;
        [self.clockRTTs addObject:[NSNumber numberWithDouble:rtt]];
        if ([self.clockRTTs count] > 8) [self.clockRTTs removeObjectAtIndex:0];
        double best = rtt;
        for (NSNumber *sample in self.clockRTTs) best = MIN(best, [sample doubleValue]);
        if (rtt <= best) self.clockOffsetNS = offset;
        self.lastRTTMS = best;
        self.latencySentAt = 0.0;
    }
    return YES;
}

- (void)startCapture {
    @synchronized (self) {
        [self.trace removeAllObjects];
        self.droppedTraceEvents = 0;
        self.capturing = YES;
    }
    [self traceName:@"capture_start" values:nil];
}

- (NSArray *)stopCapture {
    [self traceName:@"capture_stop" values:nil];
    @synchronized (self) {
        self.capturing = NO;
        return [self.trace copy];
    }
}

- (void)traceName:(NSString *)name values:(NSDictionary *)values {
    if (!self.capturing || !name) return;
    long long clientNS = (long long)(CACurrentMediaTime() * 1e9);
    long long backendNS = clientNS + self.clockOffsetNS;
    NSDictionary *event = @{@"name": name, @"cat": @"native", @"ph": @"i",
                            @"ts": [NSNumber numberWithLongLong:MAX(0, backendNS / 1000)],
                            @"pid": @2, @"tid": @"ipad", @"args": values ?: @{}};
    @synchronized (self) {
        if ([self.trace count] == RBTraceCapacity) {
            [self.trace removeObjectAtIndex:0];
            self.droppedTraceEvents++;
        }
        [self.trace addObject:event];
    }
}

@end
