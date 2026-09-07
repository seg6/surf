#import "RBDarkPopoverBackgroundView.h"
#import "RBTheme.h"
#import <QuartzCore/QuartzCore.h>

@implementation RBDarkPopoverBackgroundView {
    CGFloat _arrowOffset;
    UIPopoverArrowDirection _arrowDirection;
}

+ (UIEdgeInsets)contentViewInsets { return UIEdgeInsetsMake(6, 6, 6, 6); }
+ (CGFloat)arrowBase { return 22.0; }
+ (CGFloat)arrowHeight { return 11.0; }
+ (BOOL)wantsDefaultContentAppearance { return NO; }

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.opaque = NO;
        self.backgroundColor = [UIColor clearColor];
        self.contentMode = UIViewContentModeRedraw;
        self.layer.shadowColor = [UIColor blackColor].CGColor;
        self.layer.shadowOpacity = 0.35;
        self.layer.shadowRadius = 8.0;
        self.layer.shadowOffset = CGSizeMake(0, 3);
    }
    return self;
}

- (CGFloat)arrowOffset { return _arrowOffset; }
- (UIPopoverArrowDirection)arrowDirection { return _arrowDirection; }
- (void)setArrowOffset:(CGFloat)value { _arrowOffset = value; [self setNeedsDisplay]; }
- (void)setArrowDirection:(UIPopoverArrowDirection)value { _arrowDirection = value; [self setNeedsDisplay]; }

- (void)drawRect:(CGRect)rect {
    CGRect body = CGRectInset(self.bounds, 0.5, 0.5);
    CGFloat height = [[self class] arrowHeight];
    CGFloat half = [[self class] arrowBase] / 2.0;
    switch (self.arrowDirection) {
        case UIPopoverArrowDirectionUp: body.origin.y += height; body.size.height -= height; break;
        case UIPopoverArrowDirectionDown: body.size.height -= height; break;
        case UIPopoverArrowDirectionLeft: body.origin.x += height; body.size.width -= height; break;
        case UIPopoverArrowDirectionRight: body.size.width -= height; break;
        default: break;
    }
    if (body.size.width <= 0 || body.size.height <= 0) return;
    CGFloat l = CGRectGetMinX(body), r = CGRectGetMaxX(body);
    CGFloat t = CGRectGetMinY(body), b = CGRectGetMaxY(body);
    CGFloat radius = MIN(6.0, MIN(body.size.width, body.size.height) / 2.0);
    CGFloat halfX = MIN(half, MAX(0.0, body.size.width / 2.0 - radius));
    CGFloat halfY = MIN(half, MAX(0.0, body.size.height / 2.0 - radius));
    CGFloat x = MIN(r - radius - halfX, MAX(l + radius + halfX,
                    CGRectGetMidX(body) + self.arrowOffset));
    CGFloat y = MIN(b - radius - halfY, MAX(t + radius + halfY,
                    CGRectGetMidY(body) + self.arrowOffset));

    // One closed outline: the arrow has neither a different fill nor a seam
    // across its base. UIKit still owns anchoring, sizing and dismissal.
    UIBezierPath *path = [UIBezierPath bezierPath];
    [path moveToPoint:CGPointMake(l + radius, t)];
    if (self.arrowDirection == UIPopoverArrowDirectionUp) {
        [path addLineToPoint:CGPointMake(x - halfX, t)];
        [path addLineToPoint:CGPointMake(x, t - height)];
        [path addLineToPoint:CGPointMake(x + halfX, t)];
    }
    [path addLineToPoint:CGPointMake(r - radius, t)];
    [path addQuadCurveToPoint:CGPointMake(r, t + radius) controlPoint:CGPointMake(r, t)];
    if (self.arrowDirection == UIPopoverArrowDirectionRight) {
        [path addLineToPoint:CGPointMake(r, y - halfY)];
        [path addLineToPoint:CGPointMake(r + height, y)];
        [path addLineToPoint:CGPointMake(r, y + halfY)];
    }
    [path addLineToPoint:CGPointMake(r, b - radius)];
    [path addQuadCurveToPoint:CGPointMake(r - radius, b) controlPoint:CGPointMake(r, b)];
    if (self.arrowDirection == UIPopoverArrowDirectionDown) {
        [path addLineToPoint:CGPointMake(x + halfX, b)];
        [path addLineToPoint:CGPointMake(x, b + height)];
        [path addLineToPoint:CGPointMake(x - halfX, b)];
    }
    [path addLineToPoint:CGPointMake(l + radius, b)];
    [path addQuadCurveToPoint:CGPointMake(l, b - radius) controlPoint:CGPointMake(l, b)];
    if (self.arrowDirection == UIPopoverArrowDirectionLeft) {
        [path addLineToPoint:CGPointMake(l, y + halfY)];
        [path addLineToPoint:CGPointMake(l - height, y)];
        [path addLineToPoint:CGPointMake(l, y - halfY)];
    }
    [path addLineToPoint:CGPointMake(l, t + radius)];
    [path addQuadCurveToPoint:CGPointMake(l + radius, t) controlPoint:CGPointMake(l, t)];
    [path closePath];
    [[RBTheme panelColor] setFill];
    [path fill];
    [[RBTheme separatorColor] setStroke];
    path.lineWidth = 1.0;
    [path stroke];
    self.layer.shadowPath = path.CGPath;
}
@end
