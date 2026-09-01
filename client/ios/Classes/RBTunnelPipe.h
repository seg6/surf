#import <Foundation/Foundation.h>

// Presents a Cloudflare WebSocket tunnel as one end of a local byte stream.
// The caller runs the ordinary pinned Surf TLS protocol over the returned fd.
@interface RBTunnelPipe : NSObject
- (id)initWithHost:(NSString *)host port:(NSInteger)port;
- (int)open:(NSString **)error;
- (void)close;
@end
