#import <Foundation/Foundation.h>
#import <CoreMedia/CoreMedia.h>

// Serial-queue helper shared by Surf's video renderers. It owns the current
// H.264 parameter sets and turns one Annex-B access unit into a compressed
// CMSampleBuffer suitable for either VideoToolbox or AVSampleBufferDisplayLayer.
@interface RBH264SampleBuilder : NSObject
@property(nonatomic, assign) int codedWidth;
@property(nonatomic, assign) int codedHeight;
@property(nonatomic, readonly) CMVideoFormatDescriptionRef formatDescription;

- (id)initWithWidth:(int)width height:(int)height;
- (CMSampleBufferRef)createSampleForAU:(NSData *)au
                                   idr:(BOOL)idr
                         formatChanged:(BOOL *)formatChanged
                                status:(OSStatus *)status CF_RETURNS_RETAINED;
- (void)reset;
@end
