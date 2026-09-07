#import "RBTheme.h"
#import "RBConfig.h"
#import "RBDarkPopoverBackgroundView.h"

#import <QuartzCore/QuartzCore.h>
#import <objc/runtime.h>

// Compile with the iOS 8 SDK while opting into native appearance on iOS 13+.
// The selector is only called after a runtime availability check.
@protocol RBInterfaceStyleView <NSObject>
- (void)setOverrideUserInterfaceStyle:(NSInteger)style;
@end

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

@implementation RBTheme

+ (BOOL)isDarkMode {
    return [[NSUserDefaults standardUserDefaults] boolForKey:RBDefaultsDarkModeKey];
}

+ (BOOL)usesClassicAppearance {
    // UIKit's classic tint and control rendering differ from iOS 7 onward.
    return [[[UIDevice currentDevice] systemVersion] floatValue] < 7.0;
}

+ (UIColor *)deepTideColor {
    return [UIColor colorWithWhite:[self isDarkMode] ? 0.085 : 0.13 alpha:1.0];
}
+ (UIColor *)accentColor {
    return [self isDarkMode] ? [UIColor colorWithRed:0.039 green:0.518 blue:1.0 alpha:1.0]
                             : [UIColor colorWithRed:0.0 green:0.478 blue:1.0 alpha:1.0];
}
+ (UIColor *)seaGlassColor {
    return [self isDarkMode] ? [UIColor colorWithRed:0.19 green:0.82 blue:0.35 alpha:1.0]
                             : [UIColor colorWithRed:0.20 green:0.65 blue:0.30 alpha:1.0];
}
+ (UIColor *)foamColor {
    return [UIColor colorWithWhite:[self isDarkMode] ? 0.06 : 0.96 alpha:1.0];
}
+ (UIColor *)surfaceColor {
    return [UIColor colorWithWhite:[self isDarkMode] ? 0.12 : 1.0 alpha:1.0];
}
+ (UIColor *)panelColor {
    return [self isDarkMode] ? [self surfaceColor] : [self pageBackgroundColor];
}
+ (UIColor *)groupedCellColor {
    return [self isDarkMode] ? [UIColor colorWithWhite:0.18 alpha:1.0] : [self surfaceColor];
}
+ (UIColor *)mistColor {
    return [UIColor colorWithWhite:[self isDarkMode] ? 0.25 : 0.80 alpha:1.0];
}
+ (UIColor *)slateColor {
    return [UIColor colorWithWhite:[self isDarkMode] ? 0.67 : 0.45 alpha:1.0];
}

+ (UIColor *)barTopColor {
    return [UIColor colorWithWhite:[self isDarkMode] ? 0.13 : 1.0 alpha:1.0];
}
+ (UIColor *)barBottomColor {
    return [UIColor colorWithWhite:[self isDarkMode] ? 0.10 : 0.94 alpha:1.0];
}
+ (UIColor *)barLineColor {
    return [UIColor colorWithWhite:[self isDarkMode] ? 0.27 : 0.75 alpha:1.0];
}
+ (UIColor *)stripTopColor {
    return [UIColor colorWithWhite:[self isDarkMode] ? 0.10 : 0.92 alpha:1.0];
}
+ (UIColor *)stripBottomColor {
    return [UIColor colorWithWhite:[self isDarkMode] ? 0.08 : 0.88 alpha:1.0];
}
+ (UIColor *)iconColor {
    // Classic toolbars use subdued monochrome symbols, not iOS 7's vivid blue.
    // These icons sit on our light/dark browser surfaces; actual navigation-bar
    // items are rendered by UIKit and keep their system treatment.
    if ([self usesClassicAppearance]) {
        return [UIColor colorWithWhite:[self isDarkMode] ? 0.88 : 0.30 alpha:1.0];
    }
    return [self isDarkMode] ? [UIColor whiteColor] : [self accentColor];
}
+ (UIColor *)progressFillColor { return [self accentColor]; }
+ (UIColor *)pageBackgroundColor { return [self foamColor]; }
+ (UIColor *)primaryTextColor {
    return [self isDarkMode] ? [UIColor colorWithWhite:0.94 alpha:1.0] : [UIColor blackColor];
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
    [self applyInterfaceStyleToView:navigationBar];
    BOOL dark = [self isDarkMode];
    // Keep UIKit's titles/buttons. In dark mode match the surrounding panel;
    // in light mode restore this OS's unmodified native bar artwork.
    UIImage *background = dark ? [self solidImage:[self panelColor] cornerRadius:0] : nil;
    [navigationBar setBackgroundImage:background forBarMetrics:UIBarMetricsDefault];
    [navigationBar setBackgroundImage:background forBarMetrics:UIBarMetricsLandscapePhone];
    if ([navigationBar respondsToSelector:@selector(setShadowImage:)]) {
        navigationBar.shadowImage = dark ? [self solidImage:[self separatorColor] cornerRadius:0] : nil;
    }
    navigationBar.titleTextAttributes = nil;
    navigationBar.barStyle = dark ? UIBarStyleBlack : UIBarStyleDefault;
    navigationBar.translucent = !dark && ![self usesClassicAppearance];
    if ([self usesClassicAppearance]) {
        // iOS 6 tintColor paints the bar/buttons, not their foreground.
        navigationBar.tintColor = dark ? [self panelColor] : nil;
    } else {
        navigationBar.barTintColor = nil;
        navigationBar.tintColor = dark ? [UIColor whiteColor] : nil;
    }
}

+ (void)applyInterfaceStyleToView:(UIView *)view {
    if ([view respondsToSelector:@selector(setOverrideUserInterfaceStyle:)]) {
        [(id<RBInterfaceStyleView>)view setOverrideUserInterfaceStyle:[self isDarkMode] ? 2 : 1];
    }
}

+ (void)styleKeyboard:(UIResponder<UITextInputTraits> *)input {
    if (!input || ![input respondsToSelector:@selector(setKeyboardAppearance:)]) return;
    UIKeyboardAppearance appearance = [self isDarkMode]
        ? ([self usesClassicAppearance] ? UIKeyboardAppearanceAlert : UIKeyboardAppearanceDark)
        : UIKeyboardAppearanceDefault;
    BOOL changed = input.keyboardAppearance != appearance;
    input.keyboardAppearance = appearance;
    // Keep the current text, selection and responder. Never dismiss/reopen the
    // keyboard just to recolor it. Legacy keyboards may ignore this preference.
    if (changed && [input isFirstResponder]) [input reloadInputViews];
}

+ (void)stylePopoverController:(UIPopoverController *)popoverController {
    if (!popoverController) return;
    BOOL dark = [self isDarkMode];
    // The native tint API only exists on iOS 7+. Use the documented background
    // view extension on iOS 6, only for dark Surf-owned popovers.
    popoverController.popoverBackgroundViewClass = dark && [self usesClassicAppearance]
        ? [RBDarkPopoverBackgroundView class] : nil;
    [self applyInterfaceStyleToView:popoverController.contentViewController.view];
    if ([popoverController respondsToSelector:@selector(setBackgroundColor:)]) {
        [(id)popoverController setBackgroundColor:dark ? [self panelColor] : nil];
    }
}

+ (void)styleTableView:(UITableView *)tableView {
    if (!tableView) return;
    [self applyInterfaceStyleToView:tableView];
    // A fresh UIKit table supplies this OS's grouped background (including
    // classic textures) and separators, without hardcoded modern substitutes.
    UITableView *defaults = [[UITableView alloc] initWithFrame:CGRectZero style:tableView.style];
    [self applyInterfaceStyleToView:defaults];
    tableView.backgroundView = [self isDarkMode] ? nil : defaults.backgroundView;
    tableView.backgroundColor = [self isDarkMode]
        ? (tableView.style == UITableViewStyleGrouped ? [self pageBackgroundColor] : [self panelColor])
        : defaults.backgroundColor;
    tableView.separatorColor = [self isDarkMode] ? [self mistColor] : defaults.separatorColor;
    tableView.indicatorStyle = [self isDarkMode] ? UIScrollViewIndicatorStyleWhite
                                                 : UIScrollViewIndicatorStyleDefault;
}

+ (void)styleTableSectionView:(UIView *)view {
    // Keep native header/footer layout, typography and casing. Only supply the
    // missing legacy dark foreground; UIKit itself handles light mode.
    if ([view isKindOfClass:[UITableViewHeaderFooterView class]]) {
        UITableViewHeaderFooterView *header = (UITableViewHeaderFooterView *)view;
        static char nativeSectionColorsKey;
        NSDictionary *native = objc_getAssociatedObject(view, &nativeSectionColorsKey);
        if (!native) {
            native = @{@"text":header.textLabel.textColor ?: [UIColor grayColor],
                       @"shadow":header.textLabel.shadowColor ?: [NSNull null]};
            objc_setAssociatedObject(view, &nativeSectionColorsKey, native, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
        }
        id shadow = [native objectForKey:@"shadow"];
        header.textLabel.textColor = [self isDarkMode] ? [self secondaryTextColor] : [native objectForKey:@"text"];
        header.textLabel.shadowColor = [self isDarkMode] ? [UIColor clearColor] :
            (shadow == [NSNull null] ? nil : shadow);
    }
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
    [button setBackgroundImage:[self solidImage:[[self iconColor] colorWithAlphaComponent:0.12]
                                      cornerRadius:9.0]
                      forState:UIControlStateHighlighted];
    button.adjustsImageWhenHighlighted = NO;
}

+ (unichar)codepointForIcon:(RBIcon)icon {
    // Codepoints from lucide-static 1.34.0. The source package and complete
    // ISC/MIT notices are retained in client/ios/Artwork.
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
