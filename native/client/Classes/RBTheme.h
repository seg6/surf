#import <UIKit/UIKit.h>

typedef enum {
    RBIconBack,
    RBIconForward,
    RBIconReload,
    RBIconStop,
    RBIconStar,
    RBIconStarFill,
    RBIconGear,
    RBIconPlus,
    RBIconClose,
    RBIconExpand,
    RBIconShrink,
    RBIconChevronUp,
    RBIconChevronDown,
    RBIconBook, // library (history/bookmarks/downloads)
    RBIconMore, // Surf-specific actions
    RBIconShare,
    RBIconTabs,
    RBIconSearch,
    RBIconLock,
    RBIconWarning,
    RBIconReader,
    RBIconMedia,
    RBIconSliders,
    RBIconHistory,
    RBIconDownload,
    RBIconServer,
    RBIconQR,
    RBIconMoon,
    RBIconGauge,
    RBIconPause,
    RBIconMute
} RBIcon;

// Gradient bar with a 1px dark bottom hairline; the iOS 6 toolbar look.
@interface RBGradientBar : UIView
- (void)setTopColor:(UIColor *)top bottomColor:(UIColor *)bottom lineColor:(UIColor *)line;
@end

@interface RBTheme : NSObject
+ (UIImage *)icon:(RBIcon)icon size:(CGFloat)size color:(UIColor *)color;
+ (BOOL)usesClassicAppearance;
// Etched toolbar button: icon with a subtle bottom highlight, dims when pressed.
+ (UIButton *)barButtonWithIcon:(RBIcon)icon target:(id)target action:(SEL)action;
+ (UIColor *)barTopColor;
+ (UIColor *)barBottomColor;
+ (UIColor *)barLineColor;
+ (UIColor *)stripTopColor;
+ (UIColor *)stripBottomColor;
+ (UIColor *)iconColor;
+ (UIColor *)progressFillColor;
+ (UIColor *)pageBackgroundColor;
+ (UIColor *)primaryTextColor;
+ (UIColor *)secondaryTextColor;
+ (UIColor *)separatorColor;
+ (UIColor *)accentColor;
+ (UIColor *)deepTideColor;
+ (UIColor *)seaGlassColor;
+ (UIColor *)foamColor;
+ (UIColor *)mistColor;
+ (UIColor *)slateColor;
+ (UIFont *)fontOfSize:(CGFloat)size bold:(BOOL)bold;
+ (UIFont *)displayFontOfSize:(CGFloat)size;
+ (UIFont *)monospacedFontOfSize:(CGFloat)size bold:(BOOL)bold;
+ (void)styleNavigationBar:(UINavigationBar *)navigationBar;
+ (void)styleTableView:(UITableView *)tableView;
+ (void)stylePrimaryButton:(UIButton *)button;
+ (void)styleSecondaryButton:(UIButton *)button;
@end
