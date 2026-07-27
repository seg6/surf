#import "RBAudioPlayer.h"
#import "RBLog.h"

#import <AudioToolbox/AudioToolbox.h>

static const int RBAudioBufferCount = 12;
// The original iPad's AudioQueue consumes Pulse-derived packets in small
// bursts rather than at a perfectly uniform cadence. Ten packets (200ms)
// survived the measured burst envelope without the repeated underruns and
// clicks seen at 100ms, while still imposing a hard latency ceiling.
static const int RBAudioMaxQueuedBuffers = 10;

@interface RBAudioPlayer () {
    AudioQueueBufferRef _buffers[RBAudioBufferCount];
    BOOL _bufferFree[RBAudioBufferCount];
    unsigned int _expectedSequence;
    int _queuedBuffers;
}
@property(nonatomic, assign) AudioQueueRef queue;
@property(nonatomic, assign) int sampleRate;
@property(nonatomic, assign) int channels;
@property(nonatomic, assign) BOOL started;
@property(nonatomic, strong) NSLock *lock;
@property(nonatomic, assign, readwrite) NSUInteger droppedPCM;
@property(nonatomic, assign, readwrite) NSUInteger underruns;
@property(nonatomic, assign, readwrite) NSUInteger restartCount;
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
            if (_queuedBuffers > 0) _queuedBuffers--;
            if (_queuedBuffers == 0 && self.started) {
                self.underruns++;
                self.started = NO;
            }
            break;
        }
    }
    [self.lock unlock];
}

- (void)resetForFreshAudio {
    AudioQueueRef q = self.queue;
    if (!q) return;
    AudioQueueReset(q);
    [self.lock lock];
    for (int i = 0; i < RBAudioBufferCount; i++) {
        _bufferFree[i] = _buffers[i] != NULL;
    }
    self.started = NO;
    _queuedBuffers = 0;
    [self.lock unlock];
}

- (void)playPCM:(NSData *)pcm sequence:(unsigned int)sequence {
    if (!self.queue || ![pcm length]) return;
    if (_expectedSequence != 0 && sequence != _expectedSequence) {
        RBLog(@"audio: sequence gap expected=%u got=%u; dropping queued audio", _expectedSequence, sequence);
        [self resetForFreshAudio];
    }
    _expectedSequence = sequence + 1;
    [self.lock lock];
    if (_queuedBuffers >= RBAudioMaxQueuedBuffers) {
        self.droppedPCM++;
        [self.lock unlock];
        return;
    }
    int slot = -1;
    for (int i = 0; i < RBAudioBufferCount; i++) {
        if (_bufferFree[i]) {
            slot = i;
            _bufferFree[i] = NO;
            break;
        }
    }
    [self.lock unlock];
    if (slot < 0) {
        // A transient startup overrun is less audible than repeatedly
        // resetting a running AudioQueue. Backend and sequence-gap handling
        // already bound long-lived lag.
        self.droppedPCM++;
        return;
    }

    AudioQueueBufferRef buffer = _buffers[slot];
    UInt32 n = (UInt32)MIN([pcm length], buffer->mAudioDataBytesCapacity);
    memcpy(buffer->mAudioData, [pcm bytes], n);
    buffer->mAudioDataByteSize = n;
    // Account for the buffer before enqueueing it. AudioQueue is allowed to
    // invoke the completion callback as soon as enqueue returns (and on a
    // running queue it can race this thread).
    [self.lock lock];
    _queuedBuffers++;
    int queued = _queuedBuffers;
    [self.lock unlock];
    OSStatus st = AudioQueueEnqueueBuffer(self.queue, buffer, 0, NULL);
    if (st != noErr) {
        RBLog(@"audio: enqueue failed %ld", (long)st);
        [self markBufferFree:buffer];
        return;
    }
    // A tiny jitter cushion prevents the old start-empty-underrun cycle that
    // produced clicks and growing A/V skew until reconnect. Three 20ms chunks
    // is enough for LAN scheduling jitter without becoming perceptible lag.
    if (!self.started && queued >= 3) {
        st = AudioQueueStart(self.queue, NULL);
        if (st == noErr) {
            self.started = YES;
            self.restartCount++;
        }
        else RBLog(@"audio: start failed %ld", (long)st);
    }
}

- (int)queuedBuffers {
    [self.lock lock];
    int queued = _queuedBuffers;
    [self.lock unlock];
    return queued;
}

- (void)stop {
    [self.lock lock];
    AudioQueueRef q = self.queue;
    self.queue = NULL;
    self.started = NO;
    _queuedBuffers = 0;
    _expectedSequence = 0;
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
