#import <Foundation/Foundation.h>
#import <CoreGraphics/CoreGraphics.h>

@class RBSession;
@class RBInteractionTracker;

typedef enum {
    RBSessionStateIdle,       // not started or given up
    RBSessionStateConnecting, // login / websocket handshake in flight
    RBSessionStateOpen,       // websocket up, frames flowing
    RBSessionStateRetrying    // lost the socket, reconnect scheduled
} RBSessionState;

@protocol RBSessionDelegate <NSObject>
- (void)session:(RBSession *)session status:(NSString *)status;
- (void)session:(RBSession *)session didChangeState:(RBSessionState)state;
// Device-key authentication and secure config fetch succeeded.
- (void)sessionDidAuthenticate:(RBSession *)session;
- (void)sessionNeedsServer:(RBSession *)session message:(NSString *)message;
- (void)session:(RBSession *)session requiresClientUpdate:(NSDictionary *)update;
- (void)sessionRequiresServerUpdate:(RBSession *)session serverVersion:(NSString *)version;
- (void)session:(RBSession *)session didReceiveFrameData:(NSData *)data;
- (void)session:(RBSession *)session didReceiveControlMessage:(NSDictionary *)message;
@end

@interface RBSession : NSObject
@property(nonatomic, strong, readonly) RBInteractionTracker *interactionTracker;
@property(nonatomic, weak) id<RBSessionDelegate> delegate;
@property(nonatomic, readonly) NSInteger viewWidth;
@property(nonatomic, readonly) NSInteger viewHeight;
@property(nonatomic, readonly) NSURL *baseURL;
@property(nonatomic, readonly) NSDictionary *server;
@property(nonatomic, readonly) RBSessionState state;
// Authentication can fail because the server no longer approves this device
// (for example after it was revoked). The saved server is still trustworthy;
// it only needs a fresh pairing approval.
@property(nonatomic, readonly) BOOL requiresPairing;
@property(nonatomic, readonly) NSDictionary *availableClientUpdate;

- (id)initWithServer:(NSDictionary *)server;
- (void)start;
// Stops reconnecting and closes the socket; used when switching servers.
- (void)shutdown;
// Authenticates with this saved server and revokes this device's own key.
- (BOOL)revokeThisDevice:(NSString **)error;
- (void)updateViewportWidth:(NSInteger)width height:(NSInteger)height;
- (void)updateViewportWidth:(NSInteger)width height:(NSInteger)height force:(BOOL)force;
- (void)sendMessage:(NSDictionary *)message;
// Mirrors the bounded native NDJSON log to the authenticated Surf server.
- (void)uploadNativeLogNow;
- (void)sendTouchPhase:(NSString *)phase
                points:(NSArray *)points
             timestamp:(unsigned long long)timestamp
               surface:(unsigned int)surface;
@end
