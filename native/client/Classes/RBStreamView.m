#import "RBStreamView.h"

#import <QuartzCore/QuartzCore.h>

@interface RBStreamView ()
@property(nonatomic, strong) UIImage *currentImage;
@end

@implementation RBStreamView

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [UIColor blackColor];
        self.opaque = YES;
        self.layer.contentsGravity = kCAGravityResize;
        self.multipleTouchEnabled = YES;
    }
    return self;
}

- (void)displayImage:(UIImage *)image width:(NSUInteger)width height:(NSUInteger)height {
    if (!image) return;
    self.currentImage = image;
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    self.layer.contents = (id)[image CGImage];
    [CATransaction commit];
}

@end
