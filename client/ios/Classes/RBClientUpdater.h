#import <Foundation/Foundation.h>

@class RBClientUpdater;

@protocol RBClientUpdaterDelegate <NSObject>
- (void)clientUpdater:(RBClientUpdater *)updater progress:(double)progress;
- (void)clientUpdaterDidInstall:(RBClientUpdater *)updater;
- (void)clientUpdater:(RBClientUpdater *)updater failed:(NSString *)message;
@end

@interface RBClientUpdater : NSObject <NSURLConnectionDataDelegate>
@property(nonatomic, weak) id<RBClientUpdaterDelegate> delegate;
- (id)initWithBaseURL:(NSURL *)baseURL fingerprint:(NSString *)fingerprint update:(NSDictionary *)update;
- (void)start;
@end
