#import "RBTheme.h"
#import "RBConfig.h"

#import <QuartzCore/QuartzCore.h>

@interface RBGradientBar ()
@property(nonatomic, strong) UIColor *lineColor;
@property(nonatomic, assign) BOOL hairlineAtTop;
@end

@implementation RBGradientBar

+ (Class)layerClass { return [CAGradientLayer class]; }

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.opaque = YES;
        [self setTopColor:[RBTheme barTopColor]
              bottomColor:[RBTheme barBottomColor]
                lineColor:[RBTheme barLineColor]];
    }
    return self;
}

- (void)setTopColor:(UIColor *)top bottomColor:(UIColor *)bottom lineColor:(UIColor *)line {
    CAGradientLayer *layer = (CAGradientLayer *)self.layer;
    layer.colors = @[(id)[top CGColor], (id)[bottom CGColor]];
    layer.startPoint = CGPointMake(0.5, 0.0);
    layer.endPoint = CGPointMake(0.5, 1.0);
    self.lineColor = line;
    [self setNeedsLayout];
}

- (void)setHairlineAtTop:(BOOL)top {
    if (_hairlineAtTop == top) return;
    _hairlineAtTop = top;
    [self setNeedsLayout];
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
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    hairline.backgroundColor = [self.lineColor CGColor];
    CGFloat hairlineY = self.hairlineAtTop ? 0.0 : self.bounds.size.height - 1.0;
    hairline.frame = CGRectMake(0.0, hairlineY,
                                self.bounds.size.width, 1.0);
    [CATransaction commit];
}

@end

@interface RBDarkPopoverBackgroundView : UIPopoverBackgroundView {
    CGFloat _rbArrowOffset;
    UIPopoverArrowDirection _rbArrowDirection;
}
@end

@implementation RBDarkPopoverBackgroundView

+ (UIEdgeInsets)contentViewInsets { return UIEdgeInsetsMake(10.0, 10.0, 10.0, 10.0); }
+ (CGFloat)arrowBase { return 28.0; }
+ (CGFloat)arrowHeight { return 14.0; }
+ (BOOL)wantsDefaultContentAppearance { return NO; }

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [UIColor clearColor];
        self.opaque = NO;
    }
    return self;
}

- (void)setArrowOffset:(CGFloat)arrowOffset {
    _rbArrowOffset = arrowOffset;
    [self setNeedsDisplay];
}

- (CGFloat)arrowOffset { return _rbArrowOffset; }

- (void)setArrowDirection:(UIPopoverArrowDirection)arrowDirection {
    _rbArrowDirection = arrowDirection;
    [self setNeedsDisplay];
}

- (UIPopoverArrowDirection)arrowDirection { return _rbArrowDirection; }

- (void)drawRect:(CGRect)rect {
    CGRect body = self.bounds;
    CGFloat arrowHeight = [[self class] arrowHeight];
    CGFloat halfBase = [[self class] arrowBase] / 2.0;
    if (self.arrowDirection == UIPopoverArrowDirectionUp) {
        body.origin.y += arrowHeight;
        body.size.height -= arrowHeight;
    } else if (self.arrowDirection == UIPopoverArrowDirectionDown) {
        body.size.height -= arrowHeight;
    } else if (self.arrowDirection == UIPopoverArrowDirectionLeft) {
        body.origin.x += arrowHeight;
        body.size.width -= arrowHeight;
    } else if (self.arrowDirection == UIPopoverArrowDirectionRight) {
        body.size.width -= arrowHeight;
    }

    UIColor *fill = [RBTheme pageBackgroundColor];
    UIColor *line = [RBTheme separatorColor];
    UIBezierPath *bodyPath = [UIBezierPath bezierPathWithRoundedRect:CGRectInset(body, 0.5, 0.5)
                                                        cornerRadius:9.0];
    [fill setFill];
    [bodyPath fill];
    [line setStroke];
    bodyPath.lineWidth = 1.0;
    [bodyPath stroke];

    UIBezierPath *arrow = [UIBezierPath bezierPath];
    CGFloat center = 0.0;
    if (self.arrowDirection == UIPopoverArrowDirectionUp ||
        self.arrowDirection == UIPopoverArrowDirectionDown) {
        center = CGRectGetMidX(self.bounds) + self.arrowOffset;
        center = MIN(CGRectGetMaxX(body) - 16.0, MAX(CGRectGetMinX(body) + 16.0, center));
        if (self.arrowDirection == UIPopoverArrowDirectionUp) {
            [arrow moveToPoint:CGPointMake(center, 0.5)];
            [arrow addLineToPoint:CGPointMake(center + halfBase, CGRectGetMinY(body) + 1.0)];
            [arrow addLineToPoint:CGPointMake(center - halfBase, CGRectGetMinY(body) + 1.0)];
        } else {
            [arrow moveToPoint:CGPointMake(center, CGRectGetMaxY(self.bounds) - 0.5)];
            [arrow addLineToPoint:CGPointMake(center - halfBase, CGRectGetMaxY(body) - 1.0)];
            [arrow addLineToPoint:CGPointMake(center + halfBase, CGRectGetMaxY(body) - 1.0)];
        }
    } else {
        center = CGRectGetMidY(self.bounds) + self.arrowOffset;
        center = MIN(CGRectGetMaxY(body) - 16.0, MAX(CGRectGetMinY(body) + 16.0, center));
        if (self.arrowDirection == UIPopoverArrowDirectionLeft) {
            [arrow moveToPoint:CGPointMake(0.5, center)];
            [arrow addLineToPoint:CGPointMake(CGRectGetMinX(body) + 1.0, center - halfBase)];
            [arrow addLineToPoint:CGPointMake(CGRectGetMinX(body) + 1.0, center + halfBase)];
        } else {
            [arrow moveToPoint:CGPointMake(CGRectGetMaxX(self.bounds) - 0.5, center)];
            [arrow addLineToPoint:CGPointMake(CGRectGetMaxX(body) - 1.0, center + halfBase)];
            [arrow addLineToPoint:CGPointMake(CGRectGetMaxX(body) - 1.0, center - halfBase)];
        }
    }
    [arrow closePath];
    [fill setFill];
    [arrow fill];
    [line setStroke];
    arrow.lineWidth = 1.0;
    [arrow stroke];
}

@end

@implementation RBTheme

+ (BOOL)isDarkMode {
    return [[NSUserDefaults standardUserDefaults] boolForKey:RBDefaultsDarkModeKey];
}

+ (BOOL)usesClassicAppearance {
    // Retained as an API-availability predicate for older call sites. Surf's
    // visual identity is intentionally the same on every supported OS.
    return [[[UIDevice currentDevice] systemVersion] floatValue] < 7.0;
}

+ (UIColor *)deepTideColor {
    return [self isDarkMode] ? [UIColor colorWithRed:0.082 green:0.086 blue:0.098 alpha:1.0]
                             : [UIColor colorWithRed:0.063 green:0.165 blue:0.227 alpha:1.0];
}
+ (UIColor *)accentColor {
    return [self isDarkMode] ? [UIColor colorWithRed:0.337 green:0.635 blue:0.808 alpha:1.0]
                             : [UIColor colorWithRed:0.078 green:0.451 blue:0.722 alpha:1.0];
}
+ (UIColor *)seaGlassColor {
    return [self isDarkMode] ? [UIColor colorWithRed:0.412 green:0.714 blue:0.651 alpha:1.0]
                             : [UIColor colorWithRed:0.208 green:0.663 blue:0.722 alpha:1.0];
}
+ (UIColor *)foamColor {
    return [self isDarkMode] ? [UIColor colorWithRed:0.059 green:0.063 blue:0.071 alpha:1.0]
                             : [UIColor colorWithRed:0.969 green:0.980 blue:0.988 alpha:1.0];
}
+ (UIColor *)surfaceColor {
    return [self isDarkMode] ? [UIColor colorWithRed:0.106 green:0.114 blue:0.129 alpha:1.0]
                             : [UIColor whiteColor];
}
+ (UIColor *)mistColor {
    return [self isDarkMode] ? [UIColor colorWithRed:0.208 green:0.220 blue:0.247 alpha:1.0]
                             : [UIColor colorWithRed:0.863 green:0.910 blue:0.933 alpha:1.0];
}
+ (UIColor *)slateColor {
    return [self isDarkMode] ? [UIColor colorWithRed:0.631 green:0.647 blue:0.686 alpha:1.0]
                             : [UIColor colorWithRed:0.365 green:0.447 blue:0.502 alpha:1.0];
}

+ (UIColor *)barTopColor {
    return [self isDarkMode] ? [UIColor colorWithRed:0.125 green:0.133 blue:0.149 alpha:1.0]
                             : [UIColor colorWithRed:0.985 green:0.995 blue:1.0 alpha:1.0];
}
+ (UIColor *)barBottomColor {
    return [self isDarkMode] ? [UIColor colorWithRed:0.098 green:0.102 blue:0.118 alpha:1.0]
                             : [UIColor colorWithRed:0.945 green:0.972 blue:0.982 alpha:1.0];
}
+ (UIColor *)barLineColor {
    return [self isDarkMode] ? [UIColor colorWithRed:0.235 green:0.247 blue:0.275 alpha:1.0]
                             : [UIColor colorWithRed:0.760 green:0.840 blue:0.880 alpha:1.0];
}
+ (UIColor *)stripTopColor {
    return [self isDarkMode] ? [UIColor colorWithRed:0.102 green:0.106 blue:0.122 alpha:1.0]
                             : [UIColor colorWithRed:0.910 green:0.950 blue:0.970 alpha:1.0];
}
+ (UIColor *)stripBottomColor {
    return [self isDarkMode] ? [UIColor colorWithRed:0.075 green:0.078 blue:0.090 alpha:1.0]
                             : [UIColor colorWithRed:0.860 green:0.910 blue:0.940 alpha:1.0];
}
+ (UIColor *)iconColor { return [self accentColor]; }
+ (UIColor *)progressFillColor { return [self accentColor]; }
+ (UIColor *)pageBackgroundColor { return [self foamColor]; }
+ (UIColor *)primaryTextColor {
    return [self isDarkMode] ? [UIColor colorWithRed:0.941 green:0.945 blue:0.957 alpha:1.0]
                             : [self deepTideColor];
}
+ (UIColor *)secondaryTextColor { return [self slateColor]; }
+ (UIColor *)separatorColor { return [self mistColor]; }

+ (UIFont *)fontOfSize:(CGFloat)size bold:(BOOL)bold {
    return bold ? [UIFont boldSystemFontOfSize:size] : [UIFont systemFontOfSize:size];
}

+ (UIFont *)displayFontOfSize:(CGFloat)size {
    // Keep display and body copy in the device's native UI family. Mixing
    // Avenir Next headings with Helvetica table controls was especially
    // conspicuous on iOS 7, where the lighter system typography dominates.
    return [UIFont boldSystemFontOfSize:size];
}

+ (UIFont *)monospacedFontOfSize:(CGFloat)size bold:(BOOL)bold {
    NSString *name = bold ? @"Menlo-Bold" : @"Menlo-Regular";
    NSString *fallback = bold ? @"Courier-Bold" : @"Courier";
    return [UIFont fontWithName:name size:size] ?: [UIFont fontWithName:fallback size:size] ?:
        [self fontOfSize:size bold:bold];
}

+ (UIImage *)solidImage:(UIColor *)color cornerRadius:(CGFloat)radius {
    CGSize size = CGSizeMake(radius * 2.0 + 2.0, radius * 2.0 + 2.0);
    UIGraphicsBeginImageContextWithOptions(size, NO, 0.0);
    UIBezierPath *path = [UIBezierPath bezierPathWithRoundedRect:(CGRect){CGPointZero, size}
                                                    cornerRadius:radius];
    [color setFill];
    [path fill];
    UIImage *image = UIGraphicsGetImageFromCurrentImageContext();
    UIGraphicsEndImageContext();
    return [image resizableImageWithCapInsets:UIEdgeInsetsMake(radius + 1.0, radius + 1.0,
                                                                radius + 1.0, radius + 1.0)];
}

+ (void)styleNavigationBar:(UINavigationBar *)navigationBar {
    if (!navigationBar) return;
    navigationBar.tintColor = [UIColor whiteColor];
    navigationBar.barStyle = UIBarStyleBlack;
    [navigationBar setBackgroundImage:[self solidImage:[self deepTideColor] cornerRadius:0.0]
                        forBarMetrics:UIBarMetricsDefault];
    if ([navigationBar respondsToSelector:@selector(setShadowImage:)]) {
        navigationBar.shadowImage = [self solidImage:[[self seaGlassColor] colorWithAlphaComponent:0.72]
                                          cornerRadius:0.0];
    }
    navigationBar.titleTextAttributes = @{
        UITextAttributeTextColor: [UIColor whiteColor],
        UITextAttributeTextShadowColor: [UIColor clearColor],
        UITextAttributeFont: [self displayFontOfSize:17.0]
    };
}

+ (void)stylePopoverController:(UIPopoverController *)popoverController {
    if (!popoverController || ![self isDarkMode]) return;
    if ([popoverController respondsToSelector:@selector(setBackgroundColor:)]) {
        [(id)popoverController setBackgroundColor:[self pageBackgroundColor]];
    } else {
        popoverController.popoverBackgroundViewClass = [RBDarkPopoverBackgroundView class];
    }
}

+ (void)styleTableView:(UITableView *)tableView {
    if (!tableView) return;
    tableView.backgroundView = nil;
    tableView.backgroundColor = [self foamColor];
    tableView.separatorColor = [self mistColor];
    tableView.indicatorStyle = [self isDarkMode] ? UIScrollViewIndicatorStyleWhite
                                                 : UIScrollViewIndicatorStyleDefault;
}

+ (void)stylePrimaryButton:(UIButton *)button {
    button.backgroundColor = [self accentColor];
    button.layer.cornerRadius = 9.0;
    button.layer.borderWidth = 0.0;
    [button setTitleColor:[UIColor whiteColor] forState:UIControlStateNormal];
    [button setTitleColor:[[UIColor whiteColor] colorWithAlphaComponent:0.70]
                  forState:UIControlStateHighlighted];
    button.titleLabel.font = [self displayFontOfSize:15.0];
}

+ (void)styleSecondaryButton:(UIButton *)button {
    button.backgroundColor = [self surfaceColor];
    button.layer.cornerRadius = 9.0;
    button.layer.borderWidth = 1.0;
    button.layer.borderColor = [[self mistColor] CGColor];
    [button setTitleColor:[self accentColor] forState:UIControlStateNormal];
    [button setTitleColor:[[self accentColor] colorWithAlphaComponent:0.55]
                  forState:UIControlStateHighlighted];
    button.titleLabel.font = [self displayFontOfSize:14.0];
}

+ (UIButton *)barButtonWithIcon:(RBIcon)icon target:(id)target action:(SEL)action {
    UIButton *button = [UIButton buttonWithType:UIButtonTypeCustom];
    [self styleBarButton:button icon:icon];
    [button addTarget:target action:action forControlEvents:UIControlEventTouchUpInside];
    return button;
}

+ (void)styleBarButton:(UIButton *)button icon:(RBIcon)icon {
    [button setImage:[self icon:icon size:20.0 color:[self iconColor]] forState:UIControlStateNormal];
    UIColor *highlight = [self isDarkMode] ? [self primaryTextColor] : [self deepTideColor];
    [button setImage:[self icon:icon size:20.0 color:highlight] forState:UIControlStateHighlighted];
    [button setImage:[self icon:icon size:20.0 color:[[self slateColor] colorWithAlphaComponent:0.42]]
             forState:UIControlStateDisabled];
    [button setBackgroundImage:[self solidImage:[[self accentColor] colorWithAlphaComponent:0.12]
                                      cornerRadius:9.0]
                      forState:UIControlStateHighlighted];
    button.adjustsImageWhenHighlighted = NO;
}

+ (unichar)codepointForIcon:(RBIcon)icon {
    // Codepoints from lucide-static 1.34.0. The source package and complete
    // ISC/MIT notices are retained in native/client/Artwork.
    switch (icon) {
        case RBIconBack: return 57454;
        case RBIconForward: return 57455;
        case RBIconChevronDown: return 57453;
        case RBIconChevronUp: return 57456;
        case RBIconReload: return 57669;
        case RBIconStop: return 57703;
        case RBIconClose: return 57778;
        case RBIconStar:
        case RBIconStarFill: return 57718;
        case RBIconGear: return 57684;
        case RBIconPlus: return 57661;
        case RBIconExpand: return 57618;
        case RBIconShrink: return 57626;
        case RBIconBook: return 57439;
        case RBIconMore: return 57526;
        case RBIconShare: return 57685;
        case RBIconTabs: return 57644;
        case RBIconSearch: return 57681;
        case RBIconLock: return 58673;
        case RBIconWarning: return 57747;
        case RBIconReader: return 58184;
        case RBIconMedia: return 57472;
        case RBIconSliders: return 58010;
        case RBIconHistory: return 57845;
        case RBIconDownload: return 57522;
        case RBIconServer: return 57629;
        case RBIconQR: return 57944;
        case RBIconMoon: return 57630;
        case RBIconGauge: return 57791;
        case RBIconPause: return 57646;
        case RBIconMute: return 57772;
    }
    return 57681;
}

+ (UIImage *)icon:(RBIcon)icon size:(CGFloat)size color:(UIColor *)color {
    UIFont *font = [UIFont fontWithName:@"lucide" size:size];
    if (!font) return nil;
    unichar codepoint = [self codepointForIcon:icon];
    NSString *glyph = [NSString stringWithCharacters:&codepoint length:1];
    CGSize glyphSize = [glyph sizeWithFont:font];
    UIGraphicsBeginImageContextWithOptions(CGSizeMake(size, size), NO, 0.0);
    [color set];
    CGPoint point = CGPointMake(floorf((size - glyphSize.width) / 2.0),
                                floorf((size - glyphSize.height) / 2.0));
    [glyph drawAtPoint:point withFont:font];
    UIImage *image = UIGraphicsGetImageFromCurrentImageContext();
    UIGraphicsEndImageContext();
    return image;
}

@end
