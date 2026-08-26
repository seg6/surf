#import "RBTheme.h"

#import <QuartzCore/QuartzCore.h>

@interface RBGradientBar ()
@property(nonatomic, strong) UIColor *lineColor;
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
    hairline.frame = CGRectMake(0.0, self.bounds.size.height - 1.0,
                                self.bounds.size.width, 1.0);
    [CATransaction commit];
}

@end

@implementation RBTheme

+ (BOOL)usesClassicAppearance {
    // Retained as an API-availability predicate for older call sites. Surf's
    // visual identity is intentionally the same on every supported OS.
    return [[[UIDevice currentDevice] systemVersion] floatValue] < 7.0;
}

+ (UIColor *)deepTideColor { return [UIColor colorWithRed:0.063 green:0.165 blue:0.227 alpha:1.0]; }
+ (UIColor *)accentColor { return [UIColor colorWithRed:0.078 green:0.451 blue:0.722 alpha:1.0]; }
+ (UIColor *)seaGlassColor { return [UIColor colorWithRed:0.208 green:0.663 blue:0.722 alpha:1.0]; }
+ (UIColor *)foamColor { return [UIColor colorWithRed:0.969 green:0.980 blue:0.988 alpha:1.0]; }
+ (UIColor *)mistColor { return [UIColor colorWithRed:0.863 green:0.910 blue:0.933 alpha:1.0]; }
+ (UIColor *)slateColor { return [UIColor colorWithRed:0.365 green:0.447 blue:0.502 alpha:1.0]; }

+ (UIColor *)barTopColor { return [UIColor colorWithRed:0.985 green:0.995 blue:1.0 alpha:1.0]; }
+ (UIColor *)barBottomColor { return [UIColor colorWithRed:0.945 green:0.972 blue:0.982 alpha:1.0]; }
+ (UIColor *)barLineColor { return [UIColor colorWithRed:0.76 green:0.84 blue:0.88 alpha:1.0]; }
+ (UIColor *)stripTopColor { return [UIColor colorWithRed:0.91 green:0.95 blue:0.97 alpha:1.0]; }
+ (UIColor *)stripBottomColor { return [UIColor colorWithRed:0.86 green:0.91 blue:0.94 alpha:1.0]; }
+ (UIColor *)iconColor { return [self accentColor]; }
+ (UIColor *)progressFillColor { return [self accentColor]; }
+ (UIColor *)pageBackgroundColor { return [self foamColor]; }
+ (UIColor *)primaryTextColor { return [self deepTideColor]; }
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

+ (void)styleTableView:(UITableView *)tableView {
    if (!tableView) return;
    tableView.backgroundView = nil;
    tableView.backgroundColor = [self foamColor];
    tableView.separatorColor = [self mistColor];
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
    button.backgroundColor = [UIColor whiteColor];
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
    [button setImage:[self icon:icon size:20.0 color:[self iconColor]] forState:UIControlStateNormal];
    [button setImage:[self icon:icon size:20.0 color:[self deepTideColor]] forState:UIControlStateHighlighted];
    [button setImage:[self icon:icon size:20.0 color:[[self slateColor] colorWithAlphaComponent:0.28]]
             forState:UIControlStateDisabled];
    [button setBackgroundImage:[self solidImage:[[self accentColor] colorWithAlphaComponent:0.10]
                                      cornerRadius:9.0]
                      forState:UIControlStateHighlighted];
    button.adjustsImageWhenHighlighted = NO;
    [button addTarget:target action:action forControlEvents:UIControlEventTouchUpInside];
    return button;
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
