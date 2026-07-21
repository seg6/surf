#import <UIKit/UIKit.h>

typedef enum {
    RBIconBack,
    RBIconForward,
    RBIconReload,
    RBIconStop,
    RBIconStar,
    RBIconStarFill,
    RBIconGear,
    RBIconKeyboard,
    RBIconPlus,
    RBIconClose,
    RBIconExpand,
    RBIconShrink,
    RBIconChevronUp,
    RBIconChevronDown,
    RBIconBook,  // library (history/bookmarks/downloads)
    RBIconShare  // page actions (square with up arrow)
} RBIcon;

// Gradient bar with a 1px dark bottom hairline; the iOS 6 toolbar look.
@interface RBGradientBar : UIView
- (void)setTopColor:(UIColor *)top bottomColor:(UIColor *)bottom lineColor:(UIColor *)line;
@end

@interface RBTheme : NSObject
+ (UIImage *)icon:(RBIcon)icon size:(CGFloat)size color:(UIColor *)color;
// Etched toolbar button: icon with a subtle bottom highlight, dims when pressed.
+ (UIButton *)barButtonWithIcon:(RBIcon)icon target:(id)target action:(SEL)action;
+ (UIColor *)barTopColor;
+ (UIColor *)barBottomColor;
+ (UIColor *)barLineColor;
+ (UIColor *)stripTopColor;
+ (UIColor *)stripBottomColor;
+ (UIColor *)iconColor;
+ (UIColor *)progressFillColor;
+ (UIFont *)fontOfSize:(CGFloat)size bold:(BOOL)bold;
@end
