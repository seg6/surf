#import <Foundation/Foundation.h>
#import <CoreGraphics/CoreGraphics.h>

@class RBSession;

typedef enum {
    RBSessionStateIdle,       // not started or given up
    RBSessionStateConnecting, // login / websocket handshake in flight
    RBSessionStateOpen,       // websocket up, frames flowing
    RBSessionStateRetrying    // lost the socket, reconnect scheduled
} RBSessionState;

@protocol RBSessionDelegate <NSObject>
- (void)session:(RBSession *)session status:(NSString *)status;
- (void)session:(RBSession *)session didChangeState:(RBSessionState)state;
// Login + native-config succeeded: credentials are good, safe to persist.
- (void)sessionDidAuthenticate:(RBSession *)session;
- (void)sessionNeedsPassword:(RBSession *)session message:(NSString *)message;
- (void)session:(RBSession *)session didReceiveFrameData:(NSData *)data;
- (void)session:(RBSession *)session didReceiveControlMessage:(NSDictionary *)message;
@end

@interface RBSession : NSObject
@property(nonatomic, weak) id<RBSessionDelegate> delegate;
@property(nonatomic, readonly) NSInteger viewWidth;
@property(nonatomic, readonly) NSInteger viewHeight;
@property(nonatomic, readonly) NSURL *baseURL;
@property(nonatomic, readonly) RBSessionState state;

- (id)initWithBaseURL:(NSString *)baseURL;
- (void)startWithPassword:(NSString *)password;
// Stops reconnecting and closes the socket; used when switching servers.
- (void)shutdown;
- (void)updateViewportWidth:(NSInteger)width height:(NSInteger)height;
- (void)updateViewportWidth:(NSInteger)width height:(NSInteger)height force:(BOOL)force;
- (void)sendMessage:(NSDictionary *)message;
- (void)sendReady;
- (void)sendClickX:(CGFloat)x y:(CGFloat)y;
- (void)sendWheelX:(CGFloat)x y:(CGFloat)y dx:(CGFloat)dx dy:(CGFloat)dy;
@end
