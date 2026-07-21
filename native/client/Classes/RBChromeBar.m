#import "RBChromeBar.h"
#import "RBTheme.h"

@interface RBChromeBar ()
@property(nonatomic, strong) UIButton *backButton;
@property(nonatomic, strong) UIButton *fwdButton;
@property(nonatomic, strong) UIButton *keyboardButton;
@property(nonatomic, strong) UIButton *settingsButton;
@property(nonatomic, strong, readwrite) UIButton *actionButton;
@property(nonatomic, strong, readwrite) UIButton *libraryButton;
@property(nonatomic, strong, readwrite) RBOmnibox *omnibox;
@end

@implementation RBChromeBar

@synthesize actionButton = _actionButton;
@synthesize libraryButton = _libraryButton;
@synthesize omnibox = _omnibox;

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backButton = [RBTheme barButtonWithIcon:RBIconBack target:self action:@selector(backTapped:)];
        self.fwdButton = [RBTheme barButtonWithIcon:RBIconForward target:self action:@selector(fwdTapped:)];
        self.keyboardButton = [RBTheme barButtonWithIcon:RBIconKeyboard target:self action:@selector(keyboardTapped:)];
        self.actionButton = [RBTheme barButtonWithIcon:RBIconShare target:self action:@selector(actionsTapped:)];
        self.libraryButton = [RBTheme barButtonWithIcon:RBIconBook target:self action:@selector(libraryTapped:)];
        self.settingsButton = [RBTheme barButtonWithIcon:RBIconGear target:self action:@selector(settingsTapped:)];
        self.backButton.enabled = NO;
        self.fwdButton.enabled = NO;
        [self addSubview:self.backButton];
        [self addSubview:self.fwdButton];
        [self addSubview:self.keyboardButton];
        [self addSubview:self.actionButton];
        [self addSubview:self.libraryButton];
        [self addSubview:self.settingsButton];

        self.omnibox = [[RBOmnibox alloc] initWithFrame:CGRectZero];
        [self addSubview:self.omnibox];
    }
    return self;
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat w = self.bounds.size.width;
    CGFloat h = self.bounds.size.height;
    CGFloat buttonW = 44.0;
    CGFloat fieldH = 31.0;
    // floorf, not plain division: this device is non-retina (1x scale), so a
    // fractional origin (e.g. 56-tall bar, 31-tall field -> y=12.5) renders on
    // a blurry half-pixel boundary. The omnibox and everything inside it then
    // visibly fails to line up with the other bar buttons, which sit on whole
    // pixels (y=0..h).
    CGFloat y = floorf((h - fieldH) / 2.0);

    self.backButton.frame = CGRectMake(6.0, 0.0, buttonW, h);
    self.fwdButton.frame = CGRectMake(6.0 + buttonW, 0.0, buttonW, h);
    self.settingsButton.frame = CGRectMake(w - buttonW - 6.0, 0.0, buttonW, h);
    self.libraryButton.frame = CGRectMake(w - buttonW * 2.0 - 6.0, 0.0, buttonW, h);
    self.actionButton.frame = CGRectMake(w - buttonW * 3.0 - 6.0, 0.0, buttonW, h);
    self.keyboardButton.frame = CGRectMake(w - buttonW * 4.0 - 6.0, 0.0, buttonW, h);

    CGFloat left = 6.0 + buttonW * 2.0 + 10.0;
    CGFloat right = w - buttonW * 4.0 - 16.0;
    self.omnibox.frame = CGRectMake(left, y, MAX(120.0, right - left), fieldH);
}

- (void)setCanGoBack:(BOOL)back forward:(BOOL)forward {
    self.backButton.enabled = back;
    self.fwdButton.enabled = forward;
}

- (void)backTapped:(id)sender { [self.delegate chromeBack:self]; }
- (void)fwdTapped:(id)sender { [self.delegate chromeForward:self]; }
- (void)keyboardTapped:(id)sender { [self.delegate chromeKeyboard:self]; }
- (void)actionsTapped:(id)sender { [self.delegate chrome:self actionsFromButton:self.actionButton]; }
- (void)libraryTapped:(id)sender { [self.delegate chrome:self libraryFromButton:self.libraryButton]; }
- (void)settingsTapped:(id)sender { [self.delegate chromeSettings:self]; }

@end
