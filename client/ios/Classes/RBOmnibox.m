#import "RBOmnibox.h"
#import "RBTheme.h"

#import <QuartzCore/QuartzCore.h>

@interface RBOmnibox () <UITextFieldDelegate>
@property(nonatomic, strong) UIView *fieldBackground;
@property(nonatomic, strong) CALayer *progressLayer;
@property(nonatomic, strong) UITextField *field;
@property(nonatomic, strong) UIButton *starButton;
@property(nonatomic, strong) UIButton *reloadButton;
@property(nonatomic, strong) UIImageView *securityView;
@property(nonatomic, assign) BOOL lockVisible;
@property(nonatomic, assign) BOOL loading;
@property(nonatomic, assign) BOOL progressVisible;
@property(nonatomic, copy) NSString *committedURL;
@property(nonatomic, copy) NSString *securityState;
@property(nonatomic, assign) BOOL starred;
- (void)applyPlaceholderAppearance;
@end

@implementation RBOmnibox

@synthesize showsBookmarkButton = _showsBookmarkButton;
@synthesize showsCompactURL = _showsCompactURL;

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.showsBookmarkButton = NO;
        self.backgroundColor = [UIColor clearColor];

        self.fieldBackground = [[UIView alloc] initWithFrame:CGRectZero];
        self.fieldBackground.backgroundColor = [RBTheme surfaceColor];
        self.fieldBackground.layer.cornerRadius = 10.0;
        self.fieldBackground.layer.borderWidth = 1.0;
        self.fieldBackground.layer.borderColor = [[RBTheme mistColor] CGColor];
        self.fieldBackground.layer.masksToBounds = YES;
        [self addSubview:self.fieldBackground];

        CAGradientLayer *progress = [CAGradientLayer layer];
        progress.colors = @[(id)[[RBTheme accentColor] CGColor], (id)[[RBTheme seaGlassColor] CGColor]];
        progress.startPoint = CGPointMake(0.0, 0.5);
        progress.endPoint = CGPointMake(1.0, 0.5);
        progress.cornerRadius = 1.5;
        self.progressLayer = progress;
        self.progressLayer.anchorPoint = CGPointMake(0.0, 0.0);
        self.progressLayer.opacity = 0.0;
        [self.fieldBackground.layer addSublayer:self.progressLayer];

        self.field = [[UITextField alloc] initWithFrame:CGRectZero];
        self.field.delegate = self;
        self.field.borderStyle = UITextBorderStyleNone;
        self.field.backgroundColor = [UIColor clearColor];
        self.field.font = [RBTheme fontOfSize:14.0 bold:NO];
        self.field.textColor = [RBTheme primaryTextColor];
        [self applyPlaceholderAppearance];
        self.field.autocorrectionType = UITextAutocorrectionTypeNo;
        self.field.autocapitalizationType = UITextAutocapitalizationTypeNone;
        // This is an omnibox, not a URL-only field: ordinary text is sent to
        // the backend as a search query, so the keyboard must include spaces.
        self.field.keyboardType = UIKeyboardTypeDefault;
        self.field.returnKeyType = UIReturnKeyGo;
        self.field.clearButtonMode = UITextFieldViewModeWhileEditing;
        self.field.contentVerticalAlignment = UIControlContentVerticalAlignmentCenter;
        [self.field addTarget:self action:@selector(fieldChanged:) forControlEvents:UIControlEventEditingChanged];
        [self.fieldBackground addSubview:self.field];

        self.starButton = [UIButton buttonWithType:UIButtonTypeCustom];
        [self styleStar:NO];
        [self.starButton addTarget:self action:@selector(starTapped:) forControlEvents:UIControlEventTouchUpInside];
        [self.fieldBackground addSubview:self.starButton];

        self.reloadButton = [UIButton buttonWithType:UIButtonTypeCustom];
        [self styleReload];
        [self.reloadButton addTarget:self action:@selector(reloadTapped:) forControlEvents:UIControlEventTouchUpInside];
        [self.fieldBackground addSubview:self.reloadButton];

        self.securityView = [[UIImageView alloc] initWithFrame:CGRectZero];
        self.securityView.backgroundColor = [UIColor clearColor];
        self.securityView.contentMode = UIViewContentModeCenter;
        self.securityView.hidden = YES;
        self.securityView.isAccessibilityElement = YES;
        [self.fieldBackground addSubview:self.securityView];
    }
    return self;
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat w = self.bounds.size.width;
    CGFloat h = self.bounds.size.height;
    self.fieldBackground.frame = CGRectMake(0.0, 0.0, w, h);
    CGFloat side = h;
    BOOL editing = [self.field isFirstResponder];
    // Safari-style: star and reload/stop step out of the way while editing —
    // the field's left/right insets below already assume the space is free
    // (10pt/4pt vs. a full button-width inset), so leaving these visible
    // just overlaps the typed text and the native clear-button ("x").
    self.starButton.hidden = editing || !self.showsBookmarkButton;
    self.reloadButton.hidden = editing;
    self.starButton.frame = CGRectMake(0.0, 0.0, side, h);
    self.reloadButton.frame = CGRectMake(w - side, 0.0, side, h);
    BOOL showLock = self.lockVisible && !editing;
    self.securityView.hidden = !showLock;
    self.securityView.frame = CGRectMake(self.showsBookmarkButton ? side - 2.0 : 7.0, 0.0, 18.0, h);
    CGFloat left = editing ? 10.0 : (self.showsBookmarkButton ? side : 10.0);
    if (showLock) left += self.showsBookmarkButton ? 14.0 : 16.0;
    CGFloat right = editing ? 4.0 : side;
    self.field.frame = CGRectMake(left, 0.0, MAX(40.0, w - left - right), h);
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    CGFloat progressH = 2.5;
    self.progressLayer.bounds = CGRectMake(0.0, 0.0,
        self.progressVisible ? self.progressLayer.bounds.size.width : 0.0, progressH);
    self.progressLayer.position = CGPointMake(0.0, h - progressH);
    [CATransaction commit];
}

- (void)setShowsBookmarkButton:(BOOL)showsBookmarkButton {
    if (_showsBookmarkButton == showsBookmarkButton) return;
    _showsBookmarkButton = showsBookmarkButton;
    [self setNeedsLayout];
}

- (void)setShowsCompactURL:(BOOL)showsCompactURL {
    if (_showsCompactURL == showsCompactURL) return;
    _showsCompactURL = showsCompactURL;
    if (![self.field isFirstResponder]) [self displayCommittedURL];
}

- (void)styleStar:(BOOL)starred {
    UIColor *color = starred ? [UIColor colorWithRed:0.85 green:0.66 blue:0.14 alpha:1.0]
                             : [RBTheme secondaryTextColor];
    UIImage *icon = [RBTheme icon:(starred ? RBIconStarFill : RBIconStar) size:17.0 color:color];
    [self.starButton setImage:icon forState:UIControlStateNormal];
}

- (void)styleReload {
    RBIcon which = self.loading ? RBIconStop : RBIconReload;
    UIImage *icon = [RBTheme icon:which size:15.0 color:[RBTheme secondaryTextColor]];
    [self.reloadButton setImage:icon forState:UIControlStateNormal];
}

- (void)applyPlaceholderAppearance {
    if ([RBTheme usesClassicAppearance]) {
        // iOS 6's UITextField keeps pointers into its attributed backing store
        // while it enters first-responder state. Replacing that store from the
        // editing delegate can leave UIKit with a dangling text-layout object.
        // Keep the classic path plain and style it through the field itself.
        self.field.attributedPlaceholder = nil;
        self.field.placeholder = @"Search or enter address";
        return;
    }
    self.field.attributedPlaceholder = [[NSAttributedString alloc]
        initWithString:@"Search or enter address"
            attributes:@{NSForegroundColorAttributeName: [RBTheme secondaryTextColor],
                         NSFontAttributeName: [RBTheme fontOfSize:14.0 bold:NO]}];
}

- (BOOL)editing {
    return [self.field isFirstResponder];
}

- (void)setURLText:(NSString *)url {
    self.committedURL = url;
    if (![self.field isFirstResponder]) [self displayCommittedURL];
}

- (void)displayCommittedURL {
    NSString *url = self.committedURL ?: @"";
    NSURL *parsed = [NSURL URLWithString:url];
    NSString *host = [parsed host];
    if (![host length] || ![url length]) {
        self.field.attributedText = nil;
        self.field.text = url;
        return;
    }
    if ([RBTheme usesClassicAppearance]) {
        NSString *shown = url;
        if (self.showsCompactURL) {
            shown = host;
            NSNumber *port = [parsed port];
            if (port) {
                BOOL IPv6 = [host rangeOfString:@":"].location != NSNotFound;
                shown = IPv6
                    ? [NSString stringWithFormat:@"[%@]:%@", host, port]
                    : [NSString stringWithFormat:@"%@:%@", host, port];
            }
        }
        self.field.attributedText = nil;
        self.field.font = [RBTheme fontOfSize:14.0 bold:self.showsCompactURL];
        self.field.textColor = [RBTheme primaryTextColor];
        self.field.text = shown;
        return;
    }
    if (self.showsCompactURL) {
        NSString *displayHost = host;
        NSNumber *port = [parsed port];
        if (port) {
            BOOL IPv6 = [host rangeOfString:@":"].location != NSNotFound;
            displayHost = IPv6
                ? [NSString stringWithFormat:@"[%@]:%@", host, port]
                : [NSString stringWithFormat:@"%@:%@", host, port];
        }
        self.field.attributedText = [[NSAttributedString alloc] initWithString:displayHost
            attributes:@{NSForegroundColorAttributeName: [RBTheme primaryTextColor],
                         NSFontAttributeName: [RBTheme fontOfSize:14.0 bold:YES]}];
        return;
    }
    NSMutableAttributedString *shown = [[NSMutableAttributedString alloc] initWithString:url
        attributes:@{NSForegroundColorAttributeName: [RBTheme secondaryTextColor],
                     NSFontAttributeName: [RBTheme fontOfSize:14.0 bold:NO]}];
    NSRange hostRange = [url rangeOfString:host];
    if (hostRange.location != NSNotFound) {
        [shown addAttributes:@{NSForegroundColorAttributeName: [RBTheme primaryTextColor],
                               NSFontAttributeName: [RBTheme fontOfSize:14.0 bold:YES]}
                       range:hostRange];
    }
    self.field.attributedText = shown;
}

- (NSString *)currentText {
    if ([self.field isFirstResponder]) return self.field.text ?: @"";
    return self.committedURL ?: self.field.text ?: @"";
}

- (void)setStarred:(BOOL)starred {
    _starred = starred;
    [self styleStar:starred];
}

- (void)setSecurityState:(NSString *)state {
    _securityState = [state copy];
    if ([state isEqualToString:@"secure"]) {
        self.securityView.image = [RBTheme icon:RBIconLock size:13.0 color:[RBTheme seaGlassColor]];
        self.securityView.accessibilityLabel = @"Secure connection";
        self.lockVisible = YES;
    } else if ([state isEqualToString:@"insecure"]) {
        self.securityView.image = [RBTheme icon:RBIconWarning size:13.0
                                               color:[UIColor colorWithRed:0.78 green:0.35 blue:0.18 alpha:1.0]];
        self.securityView.accessibilityLabel = @"Connection is not secure";
        self.lockVisible = YES;
    } else {
        self.lockVisible = NO;
    }
    [self setNeedsLayout];
}

- (void)applyAppearance {
    self.fieldBackground.backgroundColor = [RBTheme surfaceColor];
    self.fieldBackground.layer.borderColor = [[RBTheme mistColor] CGColor];
    self.field.textColor = [RBTheme primaryTextColor];
    [self applyPlaceholderAppearance];
    self.field.keyboardAppearance = [RBTheme isDarkMode] ? UIKeyboardAppearanceDark
                                                         : UIKeyboardAppearanceDefault;
    ((CAGradientLayer *)self.progressLayer).colors = @[(id)[[RBTheme accentColor] CGColor],
                                                       (id)[[RBTheme seaGlassColor] CGColor]];
    [self styleStar:self.starred];
    [self styleReload];
    [self setSecurityState:self.securityState];
    if (![self.field isFirstResponder]) [self displayCommittedURL];
}

- (void)dismissKeyboard {
    [self.field resignFirstResponder];
}

- (void)focus {
    [self.field becomeFirstResponder];
}

// The server only reports loading on/off, so the fill is Safari-style
// theatre: ease out toward 80% while loading, snap to 100% and fade on stop.
- (void)setLoading:(BOOL)loading {
    if (loading == _loading) return;
    _loading = loading;
    [self styleReload];
    CGFloat w = self.fieldBackground.bounds.size.width;
    CGFloat h = 2.5;
    if (loading) {
        self.progressVisible = YES;
        [CATransaction begin];
        [CATransaction setDisableActions:YES];
        self.progressLayer.opacity = 1.0;
        self.progressLayer.bounds = CGRectMake(0.0, 0.0, w * 0.08, h);
        [CATransaction commit];
        [CATransaction begin];
        [CATransaction setAnimationDuration:7.0];
        [CATransaction setAnimationTimingFunction:[CAMediaTimingFunction functionWithName:kCAMediaTimingFunctionEaseOut]];
        self.progressLayer.bounds = CGRectMake(0.0, 0.0, w * 0.8, h);
        [CATransaction commit];
    } else if (self.progressVisible) {
        [CATransaction begin];
        [CATransaction setAnimationDuration:0.22];
        self.progressLayer.bounds = CGRectMake(0.0, 0.0, w, h);
        [CATransaction commit];
        [self performSelector:@selector(fadeProgress) withObject:nil afterDelay:0.25];
    }
}

- (void)fadeProgress {
    if (self.loading) return;
    self.progressVisible = NO;
    [CATransaction begin];
    [CATransaction setAnimationDuration:0.3];
    self.progressLayer.opacity = 0.0;
    [CATransaction commit];
}

- (void)starTapped:(id)sender { [self.delegate omniboxStarTapped:self]; }
- (void)reloadTapped:(id)sender { [self.delegate omniboxReloadOrStopTapped:self]; }

- (void)fieldChanged:(id)sender {
    [self.delegate omnibox:self textChanged:self.field.text ?: @""];
}

- (BOOL)textFieldShouldBeginEditing:(UITextField *)textField {
    // Prepare editable text before UIKit starts its responder transition. In
    // particular, never replace attributedText from textFieldDidBeginEditing:
    // on iOS 6; doing so reproducibly crashes inside UIKit's touch dispatcher.
    textField.attributedText = nil;
    textField.font = [RBTheme fontOfSize:15.0 bold:NO];
    textField.textColor = [RBTheme primaryTextColor];
    textField.text = self.committedURL ?: @"";
    return YES;
}

- (void)textFieldDidBeginEditing:(UITextField *)textField {
    textField.font = [RBTheme fontOfSize:15.0 bold:NO];
    textField.textColor = [RBTheme primaryTextColor];
    self.fieldBackground.layer.borderColor = [[RBTheme accentColor] CGColor];
    self.fieldBackground.layer.borderWidth = 1.5;
    [self setNeedsLayout];
    [self.delegate omniboxEditingBegan:self];
    [textField performSelector:@selector(selectAll:) withObject:nil afterDelay:0.05];
}

- (void)textFieldDidEndEditing:(UITextField *)textField {
    self.fieldBackground.layer.borderColor = [[RBTheme mistColor] CGColor];
    self.fieldBackground.layer.borderWidth = 1.0;
    [self displayCommittedURL];
    [self setNeedsLayout];
    [self.delegate omniboxEditingEnded:self];
}

- (BOOL)textFieldShouldReturn:(UITextField *)textField {
    NSString *text = [textField.text stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
    [textField resignFirstResponder];
    if ([text length]) [self.delegate omnibox:self navigateTo:text];
    return NO;
}

@end
