#import "RBOmnibox.h"
#import "RBTheme.h"

#import <QuartzCore/QuartzCore.h>

@interface RBOmnibox () <UITextFieldDelegate>
@property(nonatomic, strong) UIView *fieldBackground;
@property(nonatomic, strong) CALayer *progressLayer;
@property(nonatomic, strong) UITextField *field;
@property(nonatomic, strong) UIButton *starButton;
@property(nonatomic, strong) UIButton *reloadButton;
@property(nonatomic, assign) BOOL loading;
@property(nonatomic, assign) BOOL progressVisible;
@property(nonatomic, copy) NSString *committedURL;
@end

@implementation RBOmnibox

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [UIColor clearColor];

        self.fieldBackground = [[UIView alloc] initWithFrame:CGRectZero];
        self.fieldBackground.backgroundColor = [UIColor whiteColor];
        self.fieldBackground.layer.cornerRadius = 7.0;
        self.fieldBackground.layer.borderWidth = 1.0;
        self.fieldBackground.layer.borderColor = [[UIColor colorWithRed:0.42 green:0.45 blue:0.50 alpha:1.0] CGColor];
        self.fieldBackground.layer.masksToBounds = YES;
        [self addSubview:self.fieldBackground];

        self.progressLayer = [CALayer layer];
        self.progressLayer.backgroundColor = [[RBTheme progressFillColor] CGColor];
        self.progressLayer.anchorPoint = CGPointMake(0.0, 0.0);
        self.progressLayer.opacity = 0.0;
        [self.fieldBackground.layer addSublayer:self.progressLayer];

        self.field = [[UITextField alloc] initWithFrame:CGRectZero];
        self.field.delegate = self;
        self.field.borderStyle = UITextBorderStyleNone;
        self.field.backgroundColor = [UIColor clearColor];
        self.field.font = [RBTheme fontOfSize:15.0 bold:NO];
        self.field.textColor = [UIColor colorWithWhite:0.15 alpha:1.0];
        self.field.placeholder = @"Search or enter address";
        self.field.autocorrectionType = UITextAutocorrectionTypeNo;
        self.field.autocapitalizationType = UITextAutocapitalizationTypeNone;
        self.field.keyboardType = UIKeyboardTypeURL;
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
    }
    return self;
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat w = self.bounds.size.width;
    CGFloat h = self.bounds.size.height;
    self.fieldBackground.frame = CGRectMake(0.0, 0.0, w, h);
    CGFloat side = h;
    self.starButton.frame = CGRectMake(0.0, 0.0, side, h);
    self.reloadButton.frame = CGRectMake(w - side, 0.0, side, h);
    BOOL editing = [self.field isFirstResponder];
    CGFloat left = editing ? 10.0 : side;
    CGFloat right = editing ? 4.0 : side;
    self.field.frame = CGRectMake(left, 0.0, MAX(40.0, w - left - right), h);
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    self.progressLayer.bounds = CGRectMake(0.0, 0.0, self.progressVisible ? self.progressLayer.bounds.size.width : 0.0, h);
    self.progressLayer.position = CGPointMake(0.0, 0.0);
    [CATransaction commit];
}

- (void)styleStar:(BOOL)starred {
    UIColor *color = starred ? [UIColor colorWithRed:0.85 green:0.66 blue:0.14 alpha:1.0]
                             : [UIColor colorWithWhite:0.55 alpha:1.0];
    UIImage *icon = [RBTheme icon:(starred ? RBIconStarFill : RBIconStar) size:17.0 color:color];
    [self.starButton setImage:icon forState:UIControlStateNormal];
}

- (void)styleReload {
    RBIcon which = self.loading ? RBIconStop : RBIconReload;
    UIImage *icon = [RBTheme icon:which size:15.0 color:[UIColor colorWithWhite:0.45 alpha:1.0]];
    [self.reloadButton setImage:icon forState:UIControlStateNormal];
}

- (BOOL)editing {
    return [self.field isFirstResponder];
}

- (void)setURLText:(NSString *)url {
    self.committedURL = url;
    if (![self.field isFirstResponder]) self.field.text = url;
}

- (NSString *)currentText {
    return self.field.text ?: @"";
}

- (void)setStarred:(BOOL)starred {
    [self styleStar:starred];
}

- (void)dismissKeyboard {
    [self.field resignFirstResponder];
}

// The server only reports loading on/off, so the fill is Safari-style
// theatre: ease out toward 80% while loading, snap to 100% and fade on stop.
- (void)setLoading:(BOOL)loading {
    if (loading == _loading) return;
    _loading = loading;
    [self styleReload];
    CGFloat w = self.fieldBackground.bounds.size.width;
    CGFloat h = self.fieldBackground.bounds.size.height;
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

- (void)textFieldDidBeginEditing:(UITextField *)textField {
    [self setNeedsLayout];
    [self.delegate omniboxEditingBegan:self];
    [textField performSelector:@selector(selectAll:) withObject:nil afterDelay:0.05];
}

- (void)textFieldDidEndEditing:(UITextField *)textField {
    textField.text = self.committedURL;
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
