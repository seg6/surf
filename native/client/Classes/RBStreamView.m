#import "RBStreamView.h"

#import <QuartzCore/QuartzCore.h>

@interface RBStreamView ()
@property(nonatomic, strong) UIImage *currentImage; // keeps the base CGImage alive (JPEG lane)
@property(nonatomic, strong) UIImage *overlayImage;
@property(nonatomic, strong) CALayer *overlayLayer;
@property(nonatomic, strong) CADisplayLink *videoDisplayLink;
@property(nonatomic, assign) CGImageRef pendingVideoImage;
@end

@implementation RBStreamView

- (void)startVideoDisplayLinkIfNeeded {
	if (self.videoDisplayLink || !self.videoActive || !self.window) return;
	self.videoDisplayLink = [CADisplayLink displayLinkWithTarget:self selector:@selector(displayVideoTick:)];
	self.videoDisplayLink.frameInterval = 2;
	[self.videoDisplayLink addToRunLoop:[NSRunLoop mainRunLoop] forMode:NSRunLoopCommonModes];
}

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [UIColor blackColor];
        self.opaque = YES;
        self.layer.contentsGravity = kCAGravityResize;
        self.multipleTouchEnabled = YES;

        self.overlayLayer = [CALayer layer];
        self.overlayLayer.contentsGravity = kCAGravityResize;
        self.overlayLayer.hidden = YES;
        [self.layer addSublayer:self.overlayLayer];
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
    self.overlayLayer.frame = self.bounds;
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
		// timestamps. Presenting each callback immediately made several
		// frames overwrite one another inside one run-loop/display refresh,
		// then left visible gaps. Keep only the newest decoded frame and
		// commit it on a 30Hz display-link boundary instead.
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
	}
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    self.overlayLayer.hidden = YES;
    self.overlayImage = nil;
    [CATransaction commit];
}

- (void)displayImage:(UIImage *)image width:(NSUInteger)width height:(NSUInteger)height {
    if (!image) return;
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    if (self.videoActive) {
        // Sharp settle frame: crisp text over the codec's smear, kept until
        // the next touch (not the next AU — static pages emit near-identical
        // P-frames that would instantly replace crisp with smear).
        self.overlayImage = image;
        self.overlayLayer.contents = (id)[image CGImage];
        self.overlayLayer.hidden = NO;
    } else {
        self.currentImage = image;
        self.layer.contents = (id)[image CGImage];
    }
    [CATransaction commit];
}

- (void)displayVideoImage:(CGImageRef)image {
    if (!image) return;
	CGImageRetain(image);
	if (_pendingVideoImage) CGImageRelease(_pendingVideoImage);
	_pendingVideoImage = image;
}

- (void)displayVideoTick:(CADisplayLink *)displayLink {
	if (!self.videoActive || !_pendingVideoImage) return;
	CGImageRef image = _pendingVideoImage;
	_pendingVideoImage = NULL;
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    self.currentImage = nil;
    self.layer.contents = (__bridge id)image; // layer retains; pixel buffer unlocks on release
    [CATransaction commit];
	CGImageRelease(image);
}

- (void)hideSharpOverlay {
    if (self.overlayLayer.hidden) return;
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    self.overlayLayer.hidden = YES;
    self.overlayImage = nil;
    [CATransaction commit];
}

@end
