#import "RBTheme.h"

#import <QuartzCore/QuartzCore.h>

#include <math.h>

@interface RBGradientBar ()
@property(nonatomic, strong) UIColor *lineColor;
@end

@implementation RBGradientBar

+ (Class)layerClass {
    return [CAGradientLayer class];
}

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.opaque = YES;
        [self setTopColor:[RBTheme barTopColor] bottomColor:[RBTheme barBottomColor] lineColor:[RBTheme barLineColor]];
    }
    return self;
}

- (void)setTopColor:(UIColor *)top bottomColor:(UIColor *)bottom lineColor:(UIColor *)line {
    CAGradientLayer *layer = (CAGradientLayer *)self.layer;
    layer.colors = @[(id)[top CGColor], (id)[bottom CGColor]];
    self.lineColor = line;
    [self setNeedsDisplay];
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CALayer *hairline = nil;
    for (CALayer *sub in self.layer.sublayers) {
        if ([[sub valueForKey:@"rbHairline"] boolValue]) { hairline = sub; break; }
    }
    if (!hairline) {
        hairline = [CALayer layer];
        [hairline setValue:@YES forKey:@"rbHairline"];
        [self.layer addSublayer:hairline];
    }
    hairline.backgroundColor = [self.lineColor CGColor];
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    hairline.frame = CGRectMake(0.0, self.bounds.size.height - 1.0, self.bounds.size.width, 1.0);
    [CATransaction commit];
}

@end

@implementation RBTheme

+ (UIColor *)barTopColor { return [UIColor colorWithRed:0.76 green:0.79 blue:0.83 alpha:1.0]; }
+ (UIColor *)barBottomColor { return [UIColor colorWithRed:0.58 green:0.62 blue:0.68 alpha:1.0]; }
+ (UIColor *)barLineColor { return [UIColor colorWithRed:0.29 green:0.31 blue:0.35 alpha:1.0]; }
+ (UIColor *)stripTopColor { return [UIColor colorWithRed:0.53 green:0.56 blue:0.61 alpha:1.0]; }
+ (UIColor *)stripBottomColor { return [UIColor colorWithRed:0.44 green:0.47 blue:0.52 alpha:1.0]; }
+ (UIColor *)iconColor { return [UIColor colorWithRed:0.24 green:0.27 blue:0.31 alpha:1.0]; }
+ (UIColor *)progressFillColor { return [UIColor colorWithRed:0.60 green:0.72 blue:0.86 alpha:0.55]; }

+ (UIFont *)fontOfSize:(CGFloat)size bold:(BOOL)bold {
    return bold ? [UIFont boldSystemFontOfSize:size] : [UIFont systemFontOfSize:size];
}

+ (UIButton *)barButtonWithIcon:(RBIcon)icon target:(id)target action:(SEL)action {
    UIButton *button = [UIButton buttonWithType:UIButtonTypeCustom];
    UIImage *normal = [self icon:icon size:22.0 color:[self iconColor]];
    UIImage *pressed = [self icon:icon size:22.0 color:[[self iconColor] colorWithAlphaComponent:0.4]];
    UIImage *disabled = [self icon:icon size:22.0 color:[[self iconColor] colorWithAlphaComponent:0.25]];
    [button setImage:normal forState:UIControlStateNormal];
    [button setImage:pressed forState:UIControlStateHighlighted];
    [button setImage:disabled forState:UIControlStateDisabled];
    [button addTarget:target action:action forControlEvents:UIControlEventTouchUpInside];
    return button;
}

// All icons are drawn into a size x size box with roughly 15% padding.
+ (UIImage *)icon:(RBIcon)icon size:(CGFloat)size color:(UIColor *)color {
    UIGraphicsBeginImageContextWithOptions(CGSizeMake(size, size), NO, 0.0);
    CGContextRef ctx = UIGraphicsGetCurrentContext();
    CGContextSetStrokeColorWithColor(ctx, [color CGColor]);
    CGContextSetFillColorWithColor(ctx, [color CGColor]);
    CGContextSetLineCap(ctx, kCGLineCapRound);
    CGContextSetLineJoin(ctx, kCGLineJoinRound);
    CGFloat s = size;
    CGFloat pad = s * 0.15;
    CGFloat mid = s / 2.0;
    CGFloat lw = MAX(2.0, s * 0.11);
    CGContextSetLineWidth(ctx, lw);

    switch (icon) {
        case RBIconBack: {
            CGContextMoveToPoint(ctx, s - pad * 1.6, pad);
            CGContextAddLineToPoint(ctx, pad * 1.4, mid);
            CGContextAddLineToPoint(ctx, s - pad * 1.6, s - pad);
            CGContextStrokePath(ctx);
            break;
        }
        case RBIconForward: {
            CGContextMoveToPoint(ctx, pad * 1.6, pad);
            CGContextAddLineToPoint(ctx, s - pad * 1.4, mid);
            CGContextAddLineToPoint(ctx, pad * 1.6, s - pad);
            CGContextStrokePath(ctx);
            break;
        }
        case RBIconChevronUp: {
            CGContextMoveToPoint(ctx, pad, s - pad * 1.8);
            CGContextAddLineToPoint(ctx, mid, pad * 1.4);
            CGContextAddLineToPoint(ctx, s - pad, s - pad * 1.8);
            CGContextStrokePath(ctx);
            break;
        }
        case RBIconChevronDown: {
            CGContextMoveToPoint(ctx, pad, pad * 1.8);
            CGContextAddLineToPoint(ctx, mid, s - pad * 1.4);
            CGContextAddLineToPoint(ctx, s - pad, pad * 1.8);
            CGContextStrokePath(ctx);
            break;
        }
        case RBIconReload: {
            CGFloat r = mid - pad;
            // Open circular arc with an arrowhead at the gap.
            CGContextAddArc(ctx, mid, mid, r, (CGFloat)(-M_PI * 0.35), (CGFloat)(M_PI * 1.25), 0);
            CGContextStrokePath(ctx);
            CGFloat ax = mid + r * (CGFloat)cos(-M_PI * 0.35);
            CGFloat ay = mid + r * (CGFloat)sin(-M_PI * 0.35);
            CGFloat ah = s * 0.24;
            CGContextMoveToPoint(ctx, ax - ah * 0.9, ay - ah * 0.55);
            CGContextAddLineToPoint(ctx, ax + ah * 0.45, ay - ah * 0.35);
            CGContextAddLineToPoint(ctx, ax - ah * 0.15, ay + ah * 0.75);
            CGContextClosePath(ctx);
            CGContextFillPath(ctx);
            break;
        }
        case RBIconStop:
        case RBIconClose: {
            CGFloat p = icon == RBIconClose ? pad * 1.4 : pad * 1.1;
            CGContextMoveToPoint(ctx, p, p);
            CGContextAddLineToPoint(ctx, s - p, s - p);
            CGContextMoveToPoint(ctx, s - p, p);
            CGContextAddLineToPoint(ctx, p, s - p);
            CGContextStrokePath(ctx);
            break;
        }
        case RBIconStar:
        case RBIconStarFill: {
            CGFloat rOuter = mid - pad * 0.7;
            CGFloat rInner = rOuter * 0.42;
            CGContextMoveToPoint(ctx, mid, mid - rOuter);
            for (int i = 1; i < 10; i++) {
                CGFloat r = (i % 2 == 0) ? rOuter : rInner;
                CGFloat a = (CGFloat)(-M_PI_2 + i * M_PI / 5.0);
                CGContextAddLineToPoint(ctx, mid + r * (CGFloat)cos(a), mid + r * (CGFloat)sin(a));
            }
            CGContextClosePath(ctx);
            if (icon == RBIconStarFill) CGContextFillPath(ctx);
            else {
                CGContextSetLineWidth(ctx, MAX(1.5, lw * 0.7));
                CGContextStrokePath(ctx);
            }
            break;
        }
        case RBIconGear: {
            CGFloat rOuter = mid - pad * 0.8;
            CGFloat rBody = rOuter * 0.72;
            CGFloat rHole = rOuter * 0.32;
            for (int i = 0; i < 8; i++) {
                CGFloat a = (CGFloat)(i * M_PI / 4.0);
                CGFloat toothW = rOuter * 0.42;
                CGContextSaveGState(ctx);
                CGContextTranslateCTM(ctx, mid, mid);
                CGContextRotateCTM(ctx, a);
                CGContextFillRect(ctx, CGRectMake(-toothW / 2.0, -rOuter, toothW, rOuter));
                CGContextRestoreGState(ctx);
            }
            CGContextFillEllipseInRect(ctx, CGRectMake(mid - rBody, mid - rBody, rBody * 2.0, rBody * 2.0));
            CGContextSetBlendMode(ctx, kCGBlendModeClear);
            CGContextFillEllipseInRect(ctx, CGRectMake(mid - rHole, mid - rHole, rHole * 2.0, rHole * 2.0));
            CGContextSetBlendMode(ctx, kCGBlendModeNormal);
            break;
        }
        case RBIconKeyboard: {
            CGFloat top = s * 0.26, bottom = s * 0.74;
            CGContextSetLineWidth(ctx, MAX(1.5, lw * 0.65));
            CGContextStrokeRect(ctx, CGRectMake(pad * 0.7, top, s - pad * 1.4, bottom - top));
            CGFloat kw = s * 0.075;
            for (int row = 0; row < 2; row++) {
                for (int col = 0; col < 4; col++) {
                    CGFloat kx = pad * 0.7 + s * 0.09 + col * s * 0.165 + (row ? s * 0.05 : 0.0);
                    CGFloat ky = top + s * 0.09 + row * s * 0.14;
                    CGContextFillRect(ctx, CGRectMake(kx, ky, kw, kw));
                }
            }
            CGContextFillRect(ctx, CGRectMake(s * 0.30, bottom - s * 0.15, s * 0.40, kw));
            break;
        }
        case RBIconPlus: {
            CGContextMoveToPoint(ctx, mid, pad);
            CGContextAddLineToPoint(ctx, mid, s - pad);
            CGContextMoveToPoint(ctx, pad, mid);
            CGContextAddLineToPoint(ctx, s - pad, mid);
            CGContextStrokePath(ctx);
            break;
        }
        case RBIconExpand: {
            // Two outward arrows, corner to corner.
            CGFloat a = s * 0.30;
            CGContextMoveToPoint(ctx, s - pad - a, pad);
            CGContextAddLineToPoint(ctx, s - pad, pad);
            CGContextAddLineToPoint(ctx, s - pad, pad + a);
            CGContextMoveToPoint(ctx, s - pad, pad);
            CGContextAddLineToPoint(ctx, mid + s * 0.04, mid - s * 0.04);
            CGContextMoveToPoint(ctx, pad, s - pad - a);
            CGContextAddLineToPoint(ctx, pad, s - pad);
            CGContextAddLineToPoint(ctx, pad + a, s - pad);
            CGContextMoveToPoint(ctx, pad, s - pad);
            CGContextAddLineToPoint(ctx, mid - s * 0.04, mid + s * 0.04);
            CGContextStrokePath(ctx);
            break;
        }
        case RBIconBook: {
            // Open book: spine at center, two gently sloped page panels.
            CGFloat top = s * 0.24, bottom = s * 0.78, inset = pad * 0.8;
            CGFloat lw2 = MAX(1.5, lw * 0.65);
            CGContextSetLineWidth(ctx, lw2);
            CGContextMoveToPoint(ctx, mid, top + s * 0.05);
            CGContextAddLineToPoint(ctx, mid, bottom);
            CGContextStrokePath(ctx);
            // left page
            CGContextMoveToPoint(ctx, mid, top + s * 0.05);
            CGContextAddQuadCurveToPoint(ctx, mid - s * 0.18, top - s * 0.02, inset, top + s * 0.06);
            CGContextAddLineToPoint(ctx, inset, bottom - s * 0.02);
            CGContextAddQuadCurveToPoint(ctx, mid - s * 0.18, bottom - s * 0.09, mid, bottom);
            CGContextClosePath(ctx);
            CGContextStrokePath(ctx);
            // right page
            CGContextMoveToPoint(ctx, mid, top + s * 0.05);
            CGContextAddQuadCurveToPoint(ctx, mid + s * 0.18, top - s * 0.02, s - inset, top + s * 0.06);
            CGContextAddLineToPoint(ctx, s - inset, bottom - s * 0.02);
            CGContextAddQuadCurveToPoint(ctx, mid + s * 0.18, bottom - s * 0.09, mid, bottom);
            CGContextClosePath(ctx);
            CGContextStrokePath(ctx);
            break;
        }
        case RBIconShare: {
            // Safari action glyph: box with an arrow rising out of the top.
            CGFloat lw2 = MAX(1.5, lw * 0.65);
            CGContextSetLineWidth(ctx, lw2);
            CGFloat boxTop = s * 0.40;
            CGContextMoveToPoint(ctx, mid - s * 0.14, boxTop);
            CGContextAddLineToPoint(ctx, pad, boxTop);
            CGContextAddLineToPoint(ctx, pad, s - pad * 0.9);
            CGContextAddLineToPoint(ctx, s - pad, s - pad * 0.9);
            CGContextAddLineToPoint(ctx, s - pad, boxTop);
            CGContextAddLineToPoint(ctx, mid + s * 0.14, boxTop);
            CGContextStrokePath(ctx);
            // arrow shaft + head
            CGContextMoveToPoint(ctx, mid, s * 0.62);
            CGContextAddLineToPoint(ctx, mid, pad * 0.7);
            CGContextStrokePath(ctx);
            CGContextMoveToPoint(ctx, mid - s * 0.13, pad * 0.7 + s * 0.13);
            CGContextAddLineToPoint(ctx, mid, pad * 0.7);
            CGContextAddLineToPoint(ctx, mid + s * 0.13, pad * 0.7 + s * 0.13);
            CGContextStrokePath(ctx);
            break;
        }
        case RBIconShrink: {
            CGFloat a = s * 0.30;
            CGContextMoveToPoint(ctx, mid + s * 0.06, mid - s * 0.06 - a);
            CGContextAddLineToPoint(ctx, mid + s * 0.06, mid - s * 0.06);
            CGContextAddLineToPoint(ctx, mid + s * 0.06 + a, mid - s * 0.06);
            CGContextMoveToPoint(ctx, mid + s * 0.06, mid - s * 0.06);
            CGContextAddLineToPoint(ctx, s - pad, pad);
            CGContextMoveToPoint(ctx, mid - s * 0.06 - a, mid + s * 0.06);
            CGContextAddLineToPoint(ctx, mid - s * 0.06, mid + s * 0.06);
            CGContextAddLineToPoint(ctx, mid - s * 0.06, mid + s * 0.06 + a);
            CGContextMoveToPoint(ctx, mid - s * 0.06, mid + s * 0.06);
            CGContextAddLineToPoint(ctx, pad, s - pad);
            CGContextStrokePath(ctx);
            break;
        }
    }

    UIImage *image = UIGraphicsGetImageFromCurrentImageContext();
    UIGraphicsEndImageContext();
    return image;
}

@end
