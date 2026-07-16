#import "RBStreamView.h"

#import <QuartzCore/QuartzCore.h>

@interface RBStreamView ()
@property(nonatomic, strong) UIImage *currentImage; // keeps the base CGImage alive (JPEG lane)
@property(nonatomic, strong) UIImage *overlayImage;
@property(nonatomic, strong) CALayer *overlayLayer;
@end

@implementation RBStreamView

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

- (void)layoutSubviews {
    [super layoutSubviews];
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    self.overlayLayer.frame = self.bounds;
    [CATransaction commit];
}

- (void)setVideoActive:(BOOL)videoActive {
    if (_videoActive == videoActive) return;
    _videoActive = videoActive;
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
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    self.currentImage = nil;
    self.layer.contents = (__bridge id)image; // layer retains; pixel buffer unlocks on release
    [CATransaction commit];
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
