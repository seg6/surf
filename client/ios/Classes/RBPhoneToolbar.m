#import "RBPhoneToolbar.h"

#import <QuartzCore/QuartzCore.h>

@interface RBPhoneToolbar ()
@property(nonatomic, strong) UIButton *backButton;
@property(nonatomic, strong) UIButton *forwardButton;
@property(nonatomic, strong, readwrite) UIButton *shareButton;
@property(nonatomic, strong, readwrite) UIButton *pagesButton;
@property(nonatomic, strong, readwrite) UIButton *moreButton;
@property(nonatomic, strong) UILabel *tabCountLabel;
@end

@implementation RBPhoneToolbar

@synthesize shareButton = _shareButton;
@synthesize pagesButton = _pagesButton;
@synthesize moreButton = _moreButton;

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backButton = [RBTheme barButtonWithIcon:RBIconBack target:self action:@selector(backTapped:)];
        self.forwardButton = [RBTheme barButtonWithIcon:RBIconForward target:self action:@selector(forwardTapped:)];
        self.shareButton = [RBTheme barButtonWithIcon:RBIconShare target:self action:@selector(shareTapped:)];
        self.pagesButton = [RBTheme barButtonWithIcon:RBIconTabs target:self action:@selector(pagesTapped:)];
        self.moreButton = [RBTheme barButtonWithIcon:RBIconMore target:self action:@selector(moreTapped:)];
        self.backButton.enabled = NO;
        self.forwardButton.enabled = NO;
        NSArray *buttons = @[self.backButton, self.forwardButton, self.shareButton,
                             self.pagesButton, self.moreButton];
        NSArray *labels = @[@"Back", @"Forward", @"Share", @"Tabs", @"More"];
        for (NSUInteger i = 0; i < [buttons count]; i++) {
            UIButton *button = [buttons objectAtIndex:i];
            button.accessibilityLabel = [labels objectAtIndex:i];
            [self addSubview:button];
        }

        self.tabCountLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.tabCountLabel.backgroundColor = [RBTheme deepTideColor];
        self.tabCountLabel.textColor = [UIColor whiteColor];
        self.tabCountLabel.font = [RBTheme fontOfSize:9.0 bold:YES];
        self.tabCountLabel.textAlignment = NSTextAlignmentCenter;
        self.tabCountLabel.layer.cornerRadius = 7.5;
        self.tabCountLabel.layer.borderWidth = 1.0;
        self.tabCountLabel.layer.borderColor = [[RBTheme foamColor] CGColor];
        self.tabCountLabel.layer.masksToBounds = YES;
        self.tabCountLabel.userInteractionEnabled = NO;
        [self.pagesButton addSubview:self.tabCountLabel];
        [self setTabCount:0];
    }
    return self;
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat buttonW = self.bounds.size.width / 5.0;
    NSArray *buttons = @[self.backButton, self.forwardButton, self.shareButton,
                         self.pagesButton, self.moreButton];
    for (NSUInteger i = 0; i < [buttons count]; i++) {
        CGFloat x0 = floorf(buttonW * i);
        CGFloat x1 = floorf(buttonW * (i + 1));
        ((UIButton *)[buttons objectAtIndex:i]).frame = CGRectMake(x0, 0.0, x1 - x0, self.bounds.size.height);
    }
    self.tabCountLabel.frame = CGRectMake(floorf(self.pagesButton.bounds.size.width / 2.0 + 4.0),
                                           4.0, 20.0, 15.0);
}

- (void)setCanGoBack:(BOOL)back forward:(BOOL)forward {
    self.backButton.enabled = back;
    self.forwardButton.enabled = forward;
}

- (void)setTabCount:(NSUInteger)count {
    self.tabCountLabel.text = count > 99 ? @"99+" : [NSString stringWithFormat:@"%u", (unsigned int)count];
    self.tabCountLabel.hidden = count == 0;
    self.pagesButton.accessibilityValue = [NSString stringWithFormat:@"%u open", (unsigned int)count];
}

- (void)applyAppearance {
    [self setTopColor:[RBTheme barTopColor]
          bottomColor:[RBTheme barBottomColor]
            lineColor:[RBTheme barLineColor]];
    [RBTheme styleBarButton:self.backButton icon:RBIconBack];
    [RBTheme styleBarButton:self.forwardButton icon:RBIconForward];
    [RBTheme styleBarButton:self.shareButton icon:RBIconShare];
    [RBTheme styleBarButton:self.pagesButton icon:RBIconTabs];
    [RBTheme styleBarButton:self.moreButton icon:RBIconMore];
    self.tabCountLabel.backgroundColor = [RBTheme deepTideColor];
    self.tabCountLabel.layer.borderColor = [[RBTheme foamColor] CGColor];
}

- (void)backTapped:(id)sender { [self.delegate phoneToolbarBack:self]; }
- (void)forwardTapped:(id)sender { [self.delegate phoneToolbarForward:self]; }
- (void)shareTapped:(id)sender { [self.delegate phoneToolbar:self shareFromButton:self.shareButton]; }
- (void)pagesTapped:(id)sender { [self.delegate phoneToolbar:self pagesFromButton:self.pagesButton]; }
- (void)moreTapped:(id)sender { [self.delegate phoneToolbar:self moreFromButton:self.moreButton]; }

@end
