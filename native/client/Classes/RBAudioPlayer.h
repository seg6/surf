#import <Foundation/Foundation.h>

@interface RBAudioPlayer : NSObject
- (void)configureSampleRate:(int)sampleRate channels:(int)channels;
- (void)playPCM:(NSData *)pcm;
- (void)stop;
@end
