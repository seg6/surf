#import <Foundation/Foundation.h>

@class RBInteractionTracker;

@protocol RBInteractionTrackerDelegate <NSObject>
- (void)interactionTracker:(RBInteractionTracker *)tracker
                 didSendID:(unsigned long long)interactionID;
@end

@interface RBInteractionTracker : NSObject
@property(nonatomic, weak) id<RBInteractionTrackerDelegate> delegate;
@property(nonatomic, assign, readonly) double lastInteractionToPresentMS;
@property(nonatomic, assign, readonly) NSUInteger presentedInteractions;
- (NSDictionary *)decorateMessage:(NSDictionary *)message;
- (void)didPresentInteractionID:(unsigned long long)interactionID;
@end
