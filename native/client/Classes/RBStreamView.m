#import "RBStreamView.h"
#import "RBProtocol.h"

#import <QuartzCore/QuartzCore.h>

@interface RBStreamView ()
@property(nonatomic, strong) CALayer *contentLayer;
@property(nonatomic, strong) CADisplayLink *videoDisplayLink;
@property(nonatomic, assign) CGImageRef pendingVideoImage;
@property(nonatomic, strong) RBFrameMetadata *pendingMetadata;
@property(nonatomic, assign) NSUInteger presentedFrames;
@property(nonatomic, assign) NSUInteger overwrittenVideoFrames;
@property(nonatomic, assign) CFTimeInterval lastPresentationAt;
@property(nonatomic, assign) double maximumPresentationGapMS;
@end

@implementation RBStreamView

- (void)startVideoDisplayLinkIfNeeded {
	if (self.videoDisplayLink || !self.videoActive || !self.window) return;
	self.videoDisplayLink = [CADisplayLink displayLinkWithTarget:self selector:@selector(displayVideoTick:)];
	// Poll every physical refresh, even though the stream is nominally 30fps.
	// A fixed every-second-tick schedule aliases badly with VideoToolbox's
	// ~16ms callback latency: a frame arriving just after its 30Hz tick waits
	// 33ms and is often overwritten by the following decode. A 60Hz poll
	// presents each completed frame on the next refresh without duplicating
	// work when no frame is pending.
	self.videoDisplayLink.frameInterval = 1;
	[self.videoDisplayLink addToRunLoop:[NSRunLoop mainRunLoop] forMode:NSRunLoopCommonModes];
}

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [UIColor blackColor];
        self.opaque = YES;
        self.multipleTouchEnabled = YES;

        self.contentLayer = [CALayer layer];
        self.contentLayer.contentsGravity = kCAGravityResize;
        [self.layer addSublayer:self.contentLayer];
    }
    return self;
}

- (void)dealloc {
	[self.videoDisplayLink invalidate];
	if (_pendingVideoImage) CGImageRelease(_pendingVideoImage);
}

- (void)layoutSubviews {
    [super layoutSubviews];
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    self.contentLayer.frame = self.bounds;
    [CATransaction commit];
}

- (void)didMoveToWindow {
	[super didMoveToWindow];
	if (self.window) {
		[self startVideoDisplayLinkIfNeeded];
	} else {
		// Break CADisplayLink's retain on its target even if the controller is
		// torn down without explicitly leaving video mode.
		[self.videoDisplayLink invalidate];
		self.videoDisplayLink = nil;
	}
}

- (void)setVideoActive:(BOOL)videoActive {
    if (_videoActive == videoActive) return;
    _videoActive = videoActive;
	if (videoActive) {
		// Decode/network completion is bursty and carries no presentation
		// timestamps. Keep only the newest decoded frame and commit it on the
		// next physical display refresh.
		[self startVideoDisplayLinkIfNeeded];
	} else {
		// CADisplayLink retains its target; invalidate and release it here
		// rather than leaving a view -> link -> view retain cycle.
		[self.videoDisplayLink invalidate];
		self.videoDisplayLink = nil;
		if (_pendingVideoImage) {
			CGImageRelease(_pendingVideoImage);
			_pendingVideoImage = NULL;
		}
        self.pendingMetadata = nil;
		self.lastPresentationAt = 0.0;
	}
}

- (void)displayVideoImage:(CGImageRef)image metadata:(RBFrameMetadata *)metadata {
    if (!image) return;
	CGImageRetain(image);
	if (_pendingVideoImage) {
		self.overwrittenVideoFrames++;
		CGImageRelease(_pendingVideoImage);
	}
	_pendingVideoImage = image;
    self.pendingMetadata = metadata;
}

- (void)displayVideoTick:(CADisplayLink *)displayLink {
	if (!self.videoActive || !_pendingVideoImage) return;
	CGImageRef image = _pendingVideoImage;
	_pendingVideoImage = NULL;
    RBFrameMetadata *metadata = self.pendingMetadata;
    self.pendingMetadata = nil;
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    self.contentLayer.contents = (__bridge id)image; // layer retains; pixel buffer unlocks on release
    [CATransaction commit];
	CFTimeInterval now = CACurrentMediaTime();
	if (self.lastPresentationAt > 0.0) {
		double gapMS = (now - self.lastPresentationAt) * 1000.0;
		if (gapMS > self.maximumPresentationGapMS) self.maximumPresentationGapMS = gapMS;
	}
	self.lastPresentationAt = now;
	self.presentedFrames++;
    [self.presentationDelegate streamView:self didPresentMetadata:metadata];
	CGImageRelease(image);
}

@end
