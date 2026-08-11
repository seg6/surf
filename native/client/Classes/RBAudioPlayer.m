#import "RBAudioPlayer.h"
#import "RBLog.h"

#import <AudioToolbox/AudioToolbox.h>
#import <AVFoundation/AVFoundation.h>

static const int RBAudioBufferCount = 5;
static const int RBAudioStartChunks = 3; // 60ms
static const int RBAudioMaxPendingChunks = 5; // 100ms hard ceiling
static const NSUInteger RBAudioPCMSignalLogChunks = 250; // five seconds at 20ms/chunk

@interface RBAudioPlayer () {
    AudioQueueBufferRef _buffers[RBAudioBufferCount];
    BOOL _bufferFree[RBAudioBufferCount];
    unsigned int _expectedSequence;
    int _queuedBuffers;
    NSUInteger _chunkBytes;
    NSUInteger _signalChunks;
    NSUInteger _signalSilentChunks;
    NSUInteger _signalSamples;
    NSUInteger _signalNonzeroSamples;
    unsigned long long _signalAbsoluteSum;
    NSUInteger _signalPeak;
}
@property(nonatomic, assign) AudioQueueRef queue;
@property(nonatomic, assign) int sampleRate;
@property(nonatomic, assign) int channels;
@property(nonatomic, assign) BOOL started;
@property(nonatomic, assign) BOOL audioSessionActive;
@property(nonatomic, strong) NSMutableArray *pendingChunks;
@property(nonatomic, strong) NSLock *lock;
@property(nonatomic, assign, readwrite) NSUInteger droppedPCM;
@property(nonatomic, assign, readwrite) NSUInteger underruns;
@property(nonatomic, assign, readwrite) NSUInteger restartCount;
@end

@implementation RBAudioPlayer

static void RBAudioQueueCallback(void *userData, AudioQueueRef queue, AudioQueueBufferRef buffer) {
    RBAudioPlayer *player = (__bridge RBAudioPlayer *)userData;
    [player refillBuffer:buffer];
}

- (id)init {
    self = [super init];
    if (self) {
        self.lock = [[NSLock alloc] init];
        self.pendingChunks = [NSMutableArray array];
    }
    return self;
}

- (BOOL)activateAudioSession {
    AVAudioSession *session = [AVAudioSession sharedInstance];
    NSError *error = nil;
    if (![session setCategory:AVAudioSessionCategoryPlayback error:&error]) {
        RBLogEvent(@"audio", @"error", @{ @"operation": @"set_session_category",
                   @"category": AVAudioSessionCategoryPlayback,
                   @"error_code": @([error code]), @"error": [error localizedDescription] ?: @"" },
                   @"Audio playback session category failed");
        return NO;
    }
    error = nil;
    if (![session setActive:YES error:&error]) {
        RBLogEvent(@"audio", @"error", @{ @"operation": @"activate_session",
                   @"category": AVAudioSessionCategoryPlayback,
                   @"error_code": @([error code]), @"error": [error localizedDescription] ?: @"" },
                   @"Audio playback session activation failed");
        return NO;
    }
    self.audioSessionActive = YES;
    RBLogEvent(@"audio", @"info", @{ @"category": AVAudioSessionCategoryPlayback,
               @"active": @YES }, @"Audio playback session activated");
    return YES;
}

- (void)deactivateAudioSession {
    if (!self.audioSessionActive) return;
    self.audioSessionActive = NO;
    NSError *error = nil;
    BOOL ok = [[AVAudioSession sharedInstance]
               setActive:NO
               withOptions:AVAudioSessionSetActiveOptionNotifyOthersOnDeactivation
               error:&error];
    if (!ok) {
        RBLogEvent(@"audio", @"warn", @{ @"operation": @"deactivate_session",
                   @"error_code": @([error code]), @"error": [error localizedDescription] ?: @"" },
                   @"Audio playback session deactivation failed");
    } else {
        RBLogEvent(@"audio", @"info", @{ @"active": @NO }, @"Audio playback session deactivated");
    }
}

- (void)recordPCMSignal:(NSData *)pcm {
    const unsigned char *bytes = [pcm bytes];
    NSUInteger samples = [pcm length] / 2;
    NSUInteger nonzero = 0;
    unsigned long long absoluteSum = 0;
    NSUInteger peak = 0;
    for (NSUInteger i = 0; i < samples; i++) {
        unsigned short raw = (unsigned short)(bytes[i * 2] | ((unsigned short)bytes[i * 2 + 1] << 8));
        short sample = (short)raw;
        long magnitude = sample;
        if (magnitude < 0) magnitude = -magnitude;
        NSUInteger absolute = (NSUInteger)magnitude;
        if (absolute != 0) nonzero++;
        absoluteSum += absolute;
        if (absolute > peak) peak = absolute;
    }

    [self.lock lock];
    _signalChunks++;
    if (nonzero == 0) _signalSilentChunks++;
    _signalSamples += samples;
    _signalNonzeroSamples += nonzero;
    _signalAbsoluteSum += absoluteSum;
    if (peak > _signalPeak) _signalPeak = peak;
    BOOL shouldLog = _signalChunks >= RBAudioPCMSignalLogChunks;
    NSDictionary *fields = nil;
    if (shouldLog) {
        double nonzeroPercent = _signalSamples ? 100.0 * (double)_signalNonzeroSamples / (double)_signalSamples : 0.0;
        double meanAbsolute = _signalSamples ? (double)_signalAbsoluteSum / (double)_signalSamples : 0.0;
        fields = @{ @"pcm_state": _signalNonzeroSamples ? @"signal" : @"silent",
                    @"pcm_chunks": @(_signalChunks), @"pcm_silent_chunks": @(_signalSilentChunks),
                    @"pcm_samples": @(_signalSamples), @"pcm_nonzero_sample_percent": @(nonzeroPercent),
                    @"pcm_mean_absolute": @(meanAbsolute), @"pcm_peak": @(_signalPeak) };
        _signalChunks = 0;
        _signalSilentChunks = 0;
        _signalSamples = 0;
        _signalNonzeroSamples = 0;
        _signalAbsoluteSum = 0;
        _signalPeak = 0;
    }
    [self.lock unlock];
    if (fields) RBLogEvent(@"audio", @"info", fields, @"Received PCM signal sample");
}

- (void)configureSampleRate:(int)sampleRate channels:(int)channels {
    if (sampleRate <= 0) sampleRate = 16000;
    if (channels <= 0) channels = 1;
    if (self.queue && self.sampleRate == sampleRate && self.channels == channels) return;
    [self stop];
    self.sampleRate = sampleRate;
    self.channels = channels;
    _chunkBytes = (NSUInteger)(sampleRate * channels * 2 / 50);

    if (![self activateAudioSession]) return;

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
        RBLogEvent(@"audio", @"error", @{@"operation": @"create_queue", @"status": @(st)}, @"Audio queue creation failed");
        self.queue = NULL;
        [self deactivateAudioSession];
        return;
    }
    for (int i = 0; i < RBAudioBufferCount; i++) {
        st = AudioQueueAllocateBuffer(self.queue, 4096, &_buffers[i]);
        _bufferFree[i] = st == noErr;
        if (st != noErr) RBLogEvent(@"audio", @"error", @{@"operation": @"allocate_buffer", @"buffer_index": @(i), @"status": @(st)}, @"Audio buffer allocation failed");
    }
    RBLogEvent(@"audio", @"info", @{@"sample_rate": @(sampleRate), @"channels": @(channels)}, @"Audio queue configured");
}

- (BOOL)fillBufferLocked:(AudioQueueBufferRef)buffer allowSilence:(BOOL)allowSilence {
    NSData *chunk = nil;
    if ([self.pendingChunks count]) {
        chunk = [self.pendingChunks objectAtIndex:0];
        [self.pendingChunks removeObjectAtIndex:0];
    }
    if (!chunk && !allowSilence) return NO;
    UInt32 n = (UInt32)MIN(chunk ? [chunk length] : _chunkBytes, buffer->mAudioDataBytesCapacity);
    if (chunk) memcpy(buffer->mAudioData, [chunk bytes], n);
    else {
        memset(buffer->mAudioData, 0, n);
        self.underruns++;
    }
    buffer->mAudioDataByteSize = n;
    return YES;
}

- (void)refillBuffer:(AudioQueueBufferRef)buffer {
    [self.lock lock];
    if (_queuedBuffers > 0) _queuedBuffers--;
    AudioQueueRef queue = self.queue;
    BOOL requeue = queue && self.started && [self fillBufferLocked:buffer allowSilence:YES];
    if (requeue) {
        _queuedBuffers++;
    } else {
        for (int i = 0; i < RBAudioBufferCount; i++) {
            if (_buffers[i] == buffer) _bufferFree[i] = YES;
        }
    }
    [self.lock unlock];
    if (requeue && AudioQueueEnqueueBuffer(queue, buffer, 0, NULL) != noErr) {
        [self.lock lock];
        if (_queuedBuffers > 0) _queuedBuffers--;
        self.started = NO;
        [self.lock unlock];
    }
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
    [self.pendingChunks removeAllObjects];
    [self.lock unlock];
}

- (void)playPCM:(NSData *)pcm sequence:(unsigned int)sequence {
    if (![pcm length]) return;
    [self recordPCMSignal:pcm];
    if (!self.queue) return;
    if (_expectedSequence != 0 && sequence != _expectedSequence) {
        if (sequence < _expectedSequence) {
            self.droppedPCM++;
            return;
        }
        // A single missing network chunk is a short silence, not a reason to
        // tear down a healthy AudioQueue. Larger discontinuities discard only
        // the pending jitter window; the hardware queue keeps running.
        [self.lock lock];
        if (sequence - _expectedSequence == 1 && _chunkBytes) {
            [self.pendingChunks addObject:[NSMutableData dataWithLength:_chunkBytes]];
        } else {
            [self.pendingChunks removeAllObjects];
        }
        [self.lock unlock];
    }
    _expectedSequence = sequence + 1;
    [self.lock lock];
    while ([self.pendingChunks count] >= RBAudioMaxPendingChunks) {
        [self.pendingChunks removeObjectAtIndex:0];
        self.droppedPCM++;
    }
    [self.pendingChunks addObject:pcm];
    BOOL shouldStart = !self.started && [self.pendingChunks count] >= RBAudioStartChunks;
    NSMutableArray *startBuffers = [NSMutableArray array];
    if (shouldStart) {
        for (int i = 0; i < RBAudioStartChunks; i++) {
            if (!_bufferFree[i] || ![self fillBufferLocked:_buffers[i] allowSilence:NO]) break;
            _bufferFree[i] = NO;
            _queuedBuffers++;
            [startBuffers addObject:[NSValue valueWithPointer:_buffers[i]]];
        }
        shouldStart = [startBuffers count] == RBAudioStartChunks;
        self.started = shouldStart;
    }
    AudioQueueRef queue = self.queue;
    [self.lock unlock];
    if (shouldStart) {
        OSStatus st = noErr;
        for (NSValue *value in startBuffers) {
            st = AudioQueueEnqueueBuffer(queue, [value pointerValue], 0, NULL);
            if (st != noErr) break;
        }
        if (st == noErr) st = AudioQueueStart(queue, NULL);
        if (st == noErr) {
            self.restartCount++;
        } else {
            RBLogEvent(@"audio", @"error", @{@"operation": @"start", @"status": @(st)}, @"Audio queue start failed");
            [self resetForFreshAudio];
        }
    }
}

- (int)queuedBuffers {
    [self.lock lock];
    int queued = _queuedBuffers + (int)[self.pendingChunks count];
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
    [self.pendingChunks removeAllObjects];
    for (int i = 0; i < RBAudioBufferCount; i++) {
        _buffers[i] = NULL;
        _bufferFree[i] = NO;
    }
    [self.lock unlock];
    if (q) {
        AudioQueueStop(q, true);
        AudioQueueDispose(q, true);
        RBLogEvent(@"audio", @"info", @{@"state": @"stopped"}, @"Audio queue stopped");
    }
    [self deactivateAudioSession];
}

- (void)dealloc {
    [self stop];
}

@end
