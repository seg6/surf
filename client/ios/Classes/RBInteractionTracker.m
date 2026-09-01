#import "RBInteractionTracker.h"
#import "RBLog.h"
#import <QuartzCore/QuartzCore.h>

@interface RBInteractionTracker ()
@property(nonatomic, assign) unsigned long long nextID;
@property(nonatomic, strong) NSMutableDictionary *sentAt;
@property(nonatomic, assign, readwrite) double lastInteractionToPresentMS;
@property(nonatomic, assign, readwrite) NSUInteger presentedInteractions;
@end

@implementation RBInteractionTracker

- (id)init {
    self = [super init];
    if (self) self.sentAt = [NSMutableDictionary dictionary];
    return self;
}

- (BOOL)isRenderMessage:(NSString *)type {
    static NSSet *types;
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        types = [NSSet setWithObjects:@"touch", @"key", @"paste", @"compose",
                 @"nav", @"reload", @"back", @"fwd", nil];
    });
    return [types containsObject:type];
}

- (NSDictionary *)decorateMessage:(NSDictionary *)message {
    if (![self isRenderMessage:[message objectForKey:@"t"]]) return message;
    NSMutableDictionary *out = [message mutableCopy];
    unsigned long long iid = ++self.nextID;
    CFTimeInterval now = CACurrentMediaTime();
    [out setObject:[NSNumber numberWithUnsignedLongLong:iid] forKey:@"iid"];
    [out setObject:[NSNumber numberWithUnsignedLongLong:(unsigned long long)(now * 1e9)] forKey:@"clientNs"];
    @synchronized (self) {
        [self.sentAt setObject:[NSNumber numberWithDouble:now]
                        forKey:[NSNumber numberWithUnsignedLongLong:iid]];
        if ([self.sentAt count] > 256) {
            NSArray *keys = [[self.sentAt allKeys] sortedArrayUsingSelector:@selector(compare:)];
            [self.sentAt removeObjectForKey:[keys objectAtIndex:0]];
        }
    }
    [self.delegate interactionTracker:self didSendID:iid];
    return out;
}

- (void)didPresentInteractionID:(unsigned long long)interactionID {
    if (!interactionID) return;
    NSNumber *key = [NSNumber numberWithUnsignedLongLong:interactionID];
    NSNumber *sent;
    @synchronized (self) {
        sent = [self.sentAt objectForKey:key];
        if (sent) {
            NSArray *keys = [[self.sentAt allKeys] copy];
            for (NSNumber *candidate in keys) {
                if ([candidate unsignedLongLongValue] <= interactionID) {
                    [self.sentAt removeObjectForKey:candidate];
                }
            }
        }
    }
    if (!sent) return;
    self.lastInteractionToPresentMS = (CACurrentMediaTime() - [sent doubleValue]) * 1000.0;
    self.presentedInteractions++;
    if (self.presentedInteractions % 30 == 0 || self.lastInteractionToPresentMS >= 100.0) {
        RBLogEvent(@"interaction", self.lastInteractionToPresentMS >= 100.0 ? @"warn" : @"info",
                   @{@"interaction_id": @(interactionID), @"latency_ms": @(self.lastInteractionToPresentMS),
                     @"pending": @([self.sentAt count])}, @"Interaction presented");
    }
}

@end
