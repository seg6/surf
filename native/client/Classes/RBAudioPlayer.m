#import "RBAudioPlayer.h"
#import "RBLog.h"

#import <AudioToolbox/AudioToolbox.h>

static const int RBAudioBufferCount = 8;

@interface RBAudioPlayer () {
    AudioQueueBufferRef _buffers[RBAudioBufferCount];
    BOOL _bufferFree[RBAudioBufferCount];
}
@property(nonatomic, assign) AudioQueueRef queue;
@property(nonatomic, assign) int sampleRate;
@property(nonatomic, assign) int channels;
@property(nonatomic, assign) BOOL started;
@property(nonatomic, strong) NSLock *lock;
@end

@implementation RBAudioPlayer

static void RBAudioQueueCallback(void *userData, AudioQueueRef queue, AudioQueueBufferRef buffer) {
    RBAudioPlayer *player = (__bridge RBAudioPlayer *)userData;
    [player markBufferFree:buffer];
}

- (id)init {
    self = [super init];
    if (self) {
        self.lock = [[NSLock alloc] init];
    }
    return self;
}

- (void)configureSampleRate:(int)sampleRate channels:(int)channels {
    if (sampleRate <= 0) sampleRate = 16000;
    if (channels <= 0) channels = 1;
    if (self.queue && self.sampleRate == sampleRate && self.channels == channels) return;
    [self stop];
    self.sampleRate = sampleRate;
    self.channels = channels;

    AudioStreamBasicDescription fmt;
    memset(&fmt, 0, sizeof(fmt));
    fmt.mSampleRate = sampleRate;
    fmt.mFormatID = kAudioFormatLinearPCM;
    fmt.mFormatFlags = kLinearPCMFormatFlagIsSignedInteger | kLinearPCMFormatFlagIsPacked;
    fmt.mBytesPerPacket = channels * 2;
    fmt.mFramesPerPacket = 1;
    fmt.mBytesPerFrame = channels * 2;
    fmt.mChannelsPerFrame = channels;
    fmt.mBitsPerChannel = 16;

    OSStatus st = AudioQueueNewOutput(&fmt, RBAudioQueueCallback, (__bridge void *)self, NULL, NULL, 0, &_queue);
    if (st != noErr || !self.queue) {
        RBLog(@"audio: AudioQueueNewOutput failed %ld", (long)st);
        self.queue = NULL;
        return;
    }
    for (int i = 0; i < RBAudioBufferCount; i++) {
        st = AudioQueueAllocateBuffer(self.queue, 4096, &_buffers[i]);
        _bufferFree[i] = st == noErr;
        if (st != noErr) RBLog(@"audio: allocate buffer failed %ld", (long)st);
    }
    RBLog(@"audio: queue configured %dHz ch=%d", sampleRate, channels);
}

- (void)markBufferFree:(AudioQueueBufferRef)buffer {
    [self.lock lock];
    for (int i = 0; i < RBAudioBufferCount; i++) {
        if (_buffers[i] == buffer) {
            _bufferFree[i] = YES;
            break;
        }
    }
    [self.lock unlock];
}

- (void)playPCM:(NSData *)pcm {
    if (!self.queue || ![pcm length]) return;
    [self.lock lock];
    int slot = -1;
    for (int i = 0; i < RBAudioBufferCount; i++) {
        if (_bufferFree[i]) {
            slot = i;
            _bufferFree[i] = NO;
            break;
        }
    }
    [self.lock unlock];
    if (slot < 0) return;

    AudioQueueBufferRef buffer = _buffers[slot];
    UInt32 n = (UInt32)MIN([pcm length], buffer->mAudioDataBytesCapacity);
    memcpy(buffer->mAudioData, [pcm bytes], n);
    buffer->mAudioDataByteSize = n;
    OSStatus st = AudioQueueEnqueueBuffer(self.queue, buffer, 0, NULL);
    if (st != noErr) {
        RBLog(@"audio: enqueue failed %ld", (long)st);
        [self markBufferFree:buffer];
        return;
    }
    if (!self.started) {
        st = AudioQueueStart(self.queue, NULL);
        if (st == noErr) self.started = YES;
        else RBLog(@"audio: start failed %ld", (long)st);
    }
}

- (void)stop {
    [self.lock lock];
    AudioQueueRef q = self.queue;
    self.queue = NULL;
    self.started = NO;
    for (int i = 0; i < RBAudioBufferCount; i++) {
        _buffers[i] = NULL;
        _bufferFree[i] = NO;
    }
    [self.lock unlock];
    if (q) {
        AudioQueueStop(q, true);
        AudioQueueDispose(q, true);
        RBLog(@"audio: queue stopped");
    }
}

- (void)dealloc {
    [self stop];
}

@end
