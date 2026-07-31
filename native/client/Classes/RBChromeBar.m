#import "RBChromeBar.h"
#import "RBTheme.h"

@interface RBChromeBar ()
@property(nonatomic, strong) UIButton *backButton;
@property(nonatomic, strong) UIButton *fwdButton;
@property(nonatomic, strong, readwrite) UIButton *moreButton;
@property(nonatomic, strong, readwrite) UIButton *shareButton;
@property(nonatomic, strong, readwrite) UIButton *libraryButton;
@property(nonatomic, strong, readwrite) RBOmnibox *omnibox;
@property(nonatomic, strong) UILabel *titleLabel;
@end

@implementation RBChromeBar

@synthesize moreButton = _moreButton;
@synthesize shareButton = _shareButton;
@synthesize libraryButton = _libraryButton;
@synthesize omnibox = _omnibox;
@synthesize phoneLayout = _phoneLayout;
@synthesize pageTitle = _pageTitle;

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backButton = [RBTheme barButtonWithIcon:RBIconBack target:self action:@selector(backTapped:)];
        self.fwdButton = [RBTheme barButtonWithIcon:RBIconForward target:self action:@selector(fwdTapped:)];
        self.shareButton = [RBTheme barButtonWithIcon:RBIconShare target:self action:@selector(shareTapped:)];
        self.libraryButton = [RBTheme barButtonWithIcon:RBIconBook target:self action:@selector(libraryTapped:)];
        self.moreButton = [RBTheme barButtonWithIcon:RBIconMore target:self action:@selector(moreTapped:)];
        self.backButton.enabled = NO;
        self.fwdButton.enabled = NO;
        self.backButton.accessibilityLabel = @"Back";
        self.fwdButton.accessibilityLabel = @"Forward";
        self.shareButton.accessibilityLabel = @"Share";
        self.libraryButton.accessibilityLabel = @"Bookmarks";
        self.moreButton.accessibilityLabel = @"More";
        [self addSubview:self.backButton];
        [self addSubview:self.fwdButton];
        [self addSubview:self.shareButton];
        [self addSubview:self.libraryButton];
        [self addSubview:self.moreButton];

        self.titleLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.titleLabel.backgroundColor = [UIColor clearColor];
        self.titleLabel.textAlignment = NSTextAlignmentCenter;
        self.titleLabel.lineBreakMode = NSLineBreakByTruncatingTail;
        self.titleLabel.font = [RBTheme fontOfSize:14.0 bold:YES];
        self.titleLabel.textColor = [RBTheme primaryTextColor];
        self.titleLabel.shadowColor = [RBTheme usesClassicAppearance]
            ? [UIColor colorWithWhite:1.0 alpha:0.72] : nil;
        self.titleLabel.shadowOffset = CGSizeMake(0.0, 1.0);
        [self addSubview:self.titleLabel];

        self.omnibox = [[RBOmnibox alloc] initWithFrame:CGRectZero];
        [self addSubview:self.omnibox];
    }
    return self;
}

- (void)setPhoneLayout:(BOOL)phoneLayout {
    if (_phoneLayout == phoneLayout) return;
    _phoneLayout = phoneLayout;
    self.backButton.hidden = phoneLayout;
    self.fwdButton.hidden = phoneLayout;
    self.shareButton.hidden = phoneLayout;
    self.libraryButton.hidden = phoneLayout;
    self.moreButton.hidden = phoneLayout;
    // Library already has a first-class toolbar button on iPad. Keeping a
    // second bookmark control inside the field makes the unified omnibox look
    // off-centre and steals space from security and address text.
    self.omnibox.showsBookmarkButton = NO;
    [self setNeedsLayout];
}

- (void)setPageTitle:(NSString *)pageTitle {
    if (_pageTitle == pageTitle || [_pageTitle isEqualToString:pageTitle]) return;
    _pageTitle = [pageTitle copy];
    self.titleLabel.text = [_pageTitle length] ? _pageTitle : @"Surf";
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat w = self.bounds.size.width;
    CGFloat h = self.bounds.size.height;
    if (self.phoneLayout) {
        self.titleLabel.hidden = YES;
        self.titleLabel.frame = CGRectZero;
        CGFloat fieldH = 32.0;
        CGFloat y = floorf((h - fieldH) / 2.0);
        self.omnibox.frame = CGRectMake(8.0, y, MAX(80.0, w - 16.0), fieldH);
        return;
    }

    self.titleLabel.hidden = YES;
    CGFloat buttonW = 40.0;
    CGFloat fieldH = 32.0;
    CGFloat y = floorf((h - fieldH) / 2.0);
    self.backButton.frame = CGRectMake(2.0, 0.0, buttonW, h);
    self.fwdButton.frame = CGRectMake(2.0 + buttonW, 0.0, buttonW, h);
    self.moreButton.frame = CGRectMake(w - buttonW - 2.0, 0.0, buttonW, h);
    self.libraryButton.frame = CGRectMake(w - buttonW * 2.0 - 2.0, 0.0, buttonW, h);
    self.shareButton.frame = CGRectMake(w - buttonW * 3.0 - 2.0, 0.0, buttonW, h);
    CGFloat left = 2.0 + buttonW * 2.0 + 6.0;
    CGFloat right = CGRectGetMinX(self.shareButton.frame) - 6.0;
    self.omnibox.frame = CGRectMake(left, y, MAX(120.0, right - left), fieldH);
}

- (void)setCanGoBack:(BOOL)back forward:(BOOL)forward {
    self.backButton.enabled = back;
    self.fwdButton.enabled = forward;
}

- (void)backTapped:(id)sender { [self.delegate chromeBack:self]; }
- (void)fwdTapped:(id)sender { [self.delegate chromeForward:self]; }
- (void)shareTapped:(id)sender { [self.delegate chrome:self shareFromButton:self.shareButton]; }
- (void)libraryTapped:(id)sender { [self.delegate chrome:self libraryFromButton:self.libraryButton]; }
- (void)moreTapped:(id)sender { [self.delegate chrome:self moreFromButton:self.moreButton]; }

@end
