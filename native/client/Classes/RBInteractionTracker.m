#import "RBInteractionTracker.h"
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
        types = [NSSet setWithObjects:@"click", @"wheel", @"lpdown", @"lpmove", @"lpup",
                 @"key", @"paste", @"nav", @"reload", @"back", @"fwd", @"zoom", nil];
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
        if (sent) [self.sentAt removeObjectForKey:key];
    }
    if (!sent) return;
    self.lastInteractionToPresentMS = (CACurrentMediaTime() - [sent doubleValue]) * 1000.0;
    self.presentedInteractions++;
}

@end
