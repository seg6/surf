#import <Foundation/Foundation.h>

@interface RBAudioPlayer : NSObject
@property(nonatomic, assign, readonly) NSUInteger droppedPCM;
@property(nonatomic, assign, readonly) NSUInteger underruns;
@property(nonatomic, assign, readonly) NSUInteger restartCount;
@property(nonatomic, assign, readonly) int queuedBuffers;
- (void)configureSampleRate:(int)sampleRate channels:(int)channels;
- (void)playPCM:(NSData *)pcm sequence:(unsigned int)sequence;
- (void)stop;
@end
