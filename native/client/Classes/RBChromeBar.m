#import "RBChromeBar.h"
#import "RBTheme.h"

@interface RBChromeBar ()
@property(nonatomic, strong) UIButton *backButton;
@property(nonatomic, strong) UIButton *fwdButton;
@property(nonatomic, strong, readwrite) UIButton *moreButton;
@property(nonatomic, strong, readwrite) UIButton *shareButton;
@property(nonatomic, strong, readwrite) UIButton *libraryButton;
@property(nonatomic, strong, readwrite) RBOmnibox *omnibox;
@property(nonatomic, strong, readwrite) UIView *tabHostView;
@property(nonatomic, strong) UIView *workspaceDivider;
@property(nonatomic, strong) UILabel *titleLabel;
@property(nonatomic, assign) BOOL omniboxExpanded;
@end

@implementation RBChromeBar

@synthesize moreButton = _moreButton;
@synthesize shareButton = _shareButton;
@synthesize libraryButton = _libraryButton;
@synthesize omnibox = _omnibox;
@synthesize tabHostView = _tabHostView;
@synthesize phoneLayout = _phoneLayout;
@synthesize bottomPositioned = _bottomPositioned;
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
        self.libraryButton.accessibilityLabel = @"Library";
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
        self.titleLabel.font = [RBTheme displayFontOfSize:14.0];
        self.titleLabel.textColor = [RBTheme primaryTextColor];
        self.titleLabel.shadowColor = nil;
        [self addSubview:self.titleLabel];

        self.omnibox = [[RBOmnibox alloc] initWithFrame:CGRectZero];
        [self addSubview:self.omnibox];

        self.tabHostView = [[UIView alloc] initWithFrame:CGRectZero];
        self.tabHostView.backgroundColor = [UIColor clearColor];
        self.tabHostView.clipsToBounds = YES;
        [self addSubview:self.tabHostView];

        self.workspaceDivider = [[UIView alloc] initWithFrame:CGRectZero];
        self.workspaceDivider.backgroundColor = [[RBTheme barLineColor] colorWithAlphaComponent:0.72];
        self.workspaceDivider.userInteractionEnabled = NO;
        [self addSubview:self.workspaceDivider];
    }
    return self;
}

- (void)setPhoneLayout:(BOOL)phoneLayout {
    _phoneLayout = phoneLayout;
    [self setHairlineAtTop:phoneLayout ? NO : self.bottomPositioned];
    self.backButton.hidden = phoneLayout;
    self.fwdButton.hidden = phoneLayout;
    self.shareButton.hidden = phoneLayout;
    self.libraryButton.hidden = phoneLayout;
    self.moreButton.hidden = phoneLayout;
    self.tabHostView.hidden = phoneLayout;
    // The tab selector now has its own omnibox-style boundary, so a separate
    // workspace divider would duplicate the same visual job.
    self.workspaceDivider.hidden = YES;
    self.omnibox.showsCompactURL = !phoneLayout;
    if (phoneLayout) {
        self.omniboxExpanded = NO;
        self.tabHostView.alpha = 1.0;
        self.workspaceDivider.alpha = 1.0;
    } else {
        self.tabHostView.alpha = self.omniboxExpanded ? 0.0 : 1.0;
        self.workspaceDivider.alpha = self.omniboxExpanded ? 0.0 : 1.0;
    }
    self.tabHostView.userInteractionEnabled = !self.omniboxExpanded;
    // Library already has a first-class toolbar button on iPad. Keeping a
    // second bookmark control inside the field makes the unified omnibox look
    // off-centre and steals space from security and address text.
    self.omnibox.showsBookmarkButton = NO;
    [self setNeedsLayout];
}

- (void)setBottomPositioned:(BOOL)bottomPositioned {
    if (_bottomPositioned == bottomPositioned) return;
    _bottomPositioned = bottomPositioned;
    [self setHairlineAtTop:self.phoneLayout ? NO : bottomPositioned];
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
        CGFloat fieldH = 36.0;
        CGFloat y = floorf((h - fieldH) / 2.0);
        self.omnibox.frame = CGRectMake(10.0, y, MAX(80.0, w - 20.0), fieldH);
        self.tabHostView.frame = CGRectZero;
        self.workspaceDivider.frame = CGRectZero;
        return;
    }

    self.titleLabel.hidden = YES;
    CGFloat buttonW = 44.0;
    CGFloat fieldH = 36.0;
    CGFloat y = floorf((h - fieldH) / 2.0);
    self.backButton.frame = CGRectMake(0.0, 0.0, buttonW, h);
    self.fwdButton.frame = CGRectMake(buttonW, 0.0, buttonW, h);
    self.moreButton.frame = CGRectMake(w - buttonW, 0.0, buttonW, h);
    self.libraryButton.frame = CGRectMake(w - buttonW * 2.0, 0.0, buttonW, h);
    self.shareButton.frame = CGRectMake(w - buttonW * 3.0, 0.0, buttonW, h);
    CGFloat left = buttonW * 2.0 + 6.0;
    CGFloat actionsLeft = CGRectGetMinX(self.shareButton.frame);
    CGFloat desiredFieldWidth = MIN(240.0, MAX(180.0, floorf(w * 0.22)));
    CGFloat railGap = 9.0;
    CGFloat minimumTabWidth = 150.0;
    CGFloat maximumFieldWidth = MAX(140.0, actionsLeft - left - railGap - minimumTabWidth);
    CGFloat fieldWidth = self.omniboxExpanded
        ? MAX(140.0, actionsLeft - left - 6.0)
        : MIN(desiredFieldWidth, maximumFieldWidth);
    self.omnibox.frame = CGRectMake(left, y, fieldWidth, fieldH);

    self.workspaceDivider.frame = CGRectZero;
    CGFloat tabsLeft = CGRectGetMaxX(self.omnibox.frame) + railGap;
    CGFloat tabsRight = actionsLeft - 5.0;
    self.tabHostView.frame = CGRectMake(tabsLeft, y, MAX(0.0, tabsRight - tabsLeft), fieldH);
}

- (void)setOmniboxExpanded:(BOOL)expanded animated:(BOOL)animated {
    if (self.phoneLayout || self.omniboxExpanded == expanded) return;
    self.omniboxExpanded = expanded;
    self.tabHostView.userInteractionEnabled = !expanded;
    [self bringSubviewToFront:self.omnibox];
    void (^changes)(void) = ^{
        self.tabHostView.alpha = expanded ? 0.0 : 1.0;
        self.workspaceDivider.alpha = expanded ? 0.0 : 1.0;
        [self setNeedsLayout];
        [self layoutIfNeeded];
    };
    if (animated) {
        [UIView animateWithDuration:0.20
                              delay:0.0
                            options:UIViewAnimationOptionBeginFromCurrentState |
                                    UIViewAnimationOptionCurveEaseInOut
                         animations:changes completion:nil];
    } else {
        changes();
    }
}

- (void)setCanGoBack:(BOOL)back forward:(BOOL)forward {
    self.backButton.enabled = back;
    self.fwdButton.enabled = forward;
}

- (void)applyAppearance {
    [self setTopColor:[RBTheme barTopColor]
          bottomColor:[RBTheme barBottomColor]
            lineColor:[RBTheme barLineColor]];
    [RBTheme styleBarButton:self.backButton icon:RBIconBack];
    [RBTheme styleBarButton:self.fwdButton icon:RBIconForward];
    [RBTheme styleBarButton:self.shareButton icon:RBIconShare];
    [RBTheme styleBarButton:self.libraryButton icon:RBIconBook];
    [RBTheme styleBarButton:self.moreButton icon:RBIconMore];
    self.titleLabel.textColor = [RBTheme primaryTextColor];
    self.workspaceDivider.backgroundColor = [[RBTheme barLineColor] colorWithAlphaComponent:0.72];
    [self.omnibox applyAppearance];
}

- (void)backTapped:(id)sender { [self.delegate chromeBack:self]; }
- (void)fwdTapped:(id)sender { [self.delegate chromeForward:self]; }
- (void)shareTapped:(id)sender { [self.delegate chrome:self shareFromButton:self.shareButton]; }
- (void)libraryTapped:(id)sender { [self.delegate chrome:self libraryFromButton:self.libraryButton]; }
- (void)moreTapped:(id)sender { [self.delegate chrome:self moreFromButton:self.moreButton]; }

@end
