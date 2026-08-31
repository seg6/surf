#import <Foundation/Foundation.h>
#import <CoreGraphics/CoreGraphics.h>
#import <CoreVideo/CoreVideo.h>

@class RBVideoDecoder;
@class RBFrameMetadata;

@protocol RBVideoDecoderDelegate <NSObject>
// Main thread. The image wraps the decoder's pixel buffer zero-copy; display
// it (retain via layer.contents) and let go — releasing it unlocks the buffer.
- (void)videoDecoder:(RBVideoDecoder *)decoder didDecodePixelBuffer:(CVPixelBufferRef)pixelBuffer
            metadata:(RBFrameMetadata *)metadata;
// Main thread. The decode path is unrecoverable (too many resyncs); the
// caller should surface video unavailable and offer an explicit retry.
- (void)videoDecoderDidFail:(RBVideoDecoder *)decoder;
// Main thread. A real decode-error resync just happened (VT session loss,
// bad SPS/PPS) — not a client-side queue-congestion drop, which already
// waits on its own. The caller should ask the server for a recovery IDR
// ({"t":"reqkeyframe"}); healthy streams have no periodic IDR.
- (void)videoDecoderNeedsKeyframe:(RBVideoDecoder *)decoder;
@end

// H.264 decode via VideoToolbox, resolved with dlopen/dlsym at runtime
// (private framework on iOS 6). One serial decode queue; IDR-based resync on
// any error; hard fail after repeated resyncs.
@interface RBVideoDecoder : NSObject
@property(nonatomic, assign) id<RBVideoDecoderDelegate> delegate;
// Coded stream size from video-config; set before the first feedAU.
@property(nonatomic, assign) int codedWidth;
@property(nonatomic, assign) int codedHeight;
@property(nonatomic, readonly) NSUInteger decodedFrames;
@property(nonatomic, readonly) NSUInteger decodeErrors;
@property(nonatomic, readonly) NSUInteger submittedAUs;
@property(nonatomic, readonly) NSUInteger droppedAUs;
@property(nonatomic, readonly) NSUInteger callbackFrames;
@property(nonatomic, readonly) int queuedAUs;
@property(nonatomic, readonly) double lastSubmitMS;
@property(nonatomic, readonly) double averageSubmitMS;
@property(nonatomic, readonly) double lastCallbackMS;
@property(nonatomic, readonly) double averageCallbackMS;
@property(nonatomic, readonly) double lastHandoffMS;
@property(nonatomic, readonly) double averageHandoffMS;

// Whether VideoToolbox resolved — safe to call any time, caches its answer.
+ (BOOL)available;

- (void)feedAU:(NSData *)au idr:(BOOL)idr metadata:(RBFrameMetadata *)metadata;
// Drops all state (session, parameter sets, queued AUs); next feed must be an IDR.
- (void)reset;
@end
