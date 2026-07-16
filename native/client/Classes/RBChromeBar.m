#import "RBChromeBar.h"
#import "RBTheme.h"

@interface RBChromeBar ()
@property(nonatomic, strong) UIButton *backButton;
@property(nonatomic, strong) UIButton *fwdButton;
@property(nonatomic, strong) UIButton *keyboardButton;
@property(nonatomic, strong, readwrite) UIButton *menuButton;
@property(nonatomic, strong, readwrite) RBOmnibox *omnibox;
@end

@implementation RBChromeBar

@synthesize menuButton = _menuButton;
@synthesize omnibox = _omnibox;

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backButton = [RBTheme barButtonWithIcon:RBIconBack target:self action:@selector(backTapped:)];
        self.fwdButton = [RBTheme barButtonWithIcon:RBIconForward target:self action:@selector(fwdTapped:)];
        self.keyboardButton = [RBTheme barButtonWithIcon:RBIconKeyboard target:self action:@selector(keyboardTapped:)];
        self.menuButton = [RBTheme barButtonWithIcon:RBIconGear target:self action:@selector(menuTapped:)];
        self.backButton.enabled = NO;
        self.fwdButton.enabled = NO;
        [self addSubview:self.backButton];
        [self addSubview:self.fwdButton];
        [self addSubview:self.keyboardButton];
        [self addSubview:self.menuButton];

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
    CGFloat y = (h - fieldH) / 2.0;

    self.backButton.frame = CGRectMake(6.0, 0.0, buttonW, h);
    self.fwdButton.frame = CGRectMake(6.0 + buttonW, 0.0, buttonW, h);
    self.menuButton.frame = CGRectMake(w - buttonW - 6.0, 0.0, buttonW, h);
    self.keyboardButton.frame = CGRectMake(w - buttonW * 2.0 - 6.0, 0.0, buttonW, h);

    CGFloat left = 6.0 + buttonW * 2.0 + 10.0;
    CGFloat right = w - buttonW * 2.0 - 16.0;
    self.omnibox.frame = CGRectMake(left, y, MAX(120.0, right - left), fieldH);
}

- (void)setCanGoBack:(BOOL)back forward:(BOOL)forward {
    self.backButton.enabled = back;
    self.fwdButton.enabled = forward;
}

- (void)backTapped:(id)sender { [self.delegate chromeBack:self]; }
- (void)fwdTapped:(id)sender { [self.delegate chromeForward:self]; }
- (void)keyboardTapped:(id)sender { [self.delegate chromeKeyboard:self]; }
- (void)menuTapped:(id)sender { [self.delegate chrome:self menuFromButton:self.menuButton]; }

@end
