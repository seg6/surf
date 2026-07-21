#import <Foundation/Foundation.h>

@class RBSocket;

@protocol RBSocketDelegate <NSObject>
- (void)socketDidOpen:(RBSocket *)socket;
- (void)socket:(RBSocket *)socket didCloseWithError:(NSString *)error;
- (void)socket:(RBSocket *)socket didReceiveText:(NSString *)text;
- (void)socket:(RBSocket *)socket didReceiveBinary:(NSData *)data;
@end

@interface RBSocket : NSObject
@property(nonatomic, weak) id<RBSocketDelegate> delegate;

- (id)initWithHost:(NSString *)host port:(NSInteger)port path:(NSString *)path secure:(BOOL)secure;
- (void)connect;
- (void)close;
- (void)sendJSON:(NSDictionary *)message;
@end
