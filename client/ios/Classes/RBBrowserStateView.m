#import "RBBrowserStateView.h"
#import "RBTheme.h"

#import <QuartzCore/QuartzCore.h>

@interface RBBrowserStateView ()
@property(nonatomic, strong) UIImageView *markView;
@property(nonatomic, strong) UIActivityIndicatorView *spinner;
@property(nonatomic, strong) UILabel *titleLabel;
@property(nonatomic, strong) UILabel *detailLabel;
@property(nonatomic, strong) UIButton *primaryButton;
@property(nonatomic, strong) UIButton *secondaryButton;
@end

@implementation RBBrowserStateView

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [RBTheme pageBackgroundColor];
        self.hidden = YES;
        self.markView = [[UIImageView alloc] initWithImage:[UIImage imageNamed:@"brand-mark.png"]];
        self.markView.contentMode = UIViewContentModeScaleAspectFit;
        [self addSubview:self.markView];
        self.spinner = [[UIActivityIndicatorView alloc] initWithActivityIndicatorStyle:UIActivityIndicatorViewStyleGray];
        [self addSubview:self.spinner];
        self.titleLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.titleLabel.backgroundColor = [UIColor clearColor];
        self.titleLabel.textAlignment = NSTextAlignmentCenter;
        self.titleLabel.font = [RBTheme displayFontOfSize:21.0];
        self.titleLabel.textColor = [RBTheme primaryTextColor];
        [self addSubview:self.titleLabel];
        self.detailLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.detailLabel.backgroundColor = [UIColor clearColor];
        self.detailLabel.textAlignment = NSTextAlignmentCenter;
        self.detailLabel.numberOfLines = 0;
        self.detailLabel.lineBreakMode = NSLineBreakByTruncatingTail;
        self.detailLabel.font = [RBTheme fontOfSize:14.0 bold:NO];
        self.detailLabel.textColor = [RBTheme secondaryTextColor];
        [self addSubview:self.detailLabel];
        self.primaryButton = [self button];
        [self.primaryButton addTarget:self action:@selector(primary:) forControlEvents:UIControlEventTouchUpInside];
        [self addSubview:self.primaryButton];
        self.secondaryButton = [self button];
        [RBTheme styleSecondaryButton:self.secondaryButton];
        [self.secondaryButton addTarget:self action:@selector(secondary:) forControlEvents:UIControlEventTouchUpInside];
        [self addSubview:self.secondaryButton];
    }
    return self;
}

- (UIButton *)button {
    UIButton *button = [UIButton buttonWithType:UIButtonTypeCustom];
    [RBTheme stylePrimaryButton:button];
    return button;
}

- (void)showState:(RBBrowserState)state detail:(NSString *)detail {
    self.state = state;
    self.hidden = state == RBBrowserStateHidden;
    [self.spinner stopAnimating];
    self.primaryButton.hidden = NO;
    self.secondaryButton.hidden = NO;
    switch (state) {
        case RBBrowserStateSetup:
        case RBBrowserStateSetupForce:
            self.titleLabel.text = @"Browser setup";
            self.detailLabel.text = detail ?: @"Your browser is open on the computer. Browsing is paused.";
            [self.primaryButton setTitle:@"Resume here" forState:UIControlStateNormal];
            [self.secondaryButton setTitle:state == RBBrowserStateSetupForce ? @"Force close and resume…" : @"Choose Server" forState:UIControlStateNormal];
            break;
        case RBBrowserStateSetupClosing:
            self.titleLabel.text = @"Switching browser";
            self.detailLabel.text = detail ?: @"Waiting for the computer’s browser…";
            [self.spinner startAnimating];
            self.primaryButton.hidden = YES;
            [self.secondaryButton setTitle:@"Choose Server" forState:UIControlStateNormal];
            break;
        case RBBrowserStateConnecting:
            self.titleLabel.text = @"Connecting to Surf";
            self.detailLabel.text = detail ?: @"Starting your browser session…";
            [self.spinner startAnimating];
            self.primaryButton.hidden = YES;
            [self.secondaryButton setTitle:@"Choose Server" forState:UIControlStateNormal];
            break;
        case RBBrowserStateStartingVideo:
            self.titleLabel.text = @"Starting Video";
            self.detailLabel.text = detail ?: @"Preparing the live browser view…";
            [self.spinner startAnimating];
            self.primaryButton.hidden = YES;
            [self.secondaryButton setTitle:@"Choose Server" forState:UIControlStateNormal];
            break;
        case RBBrowserStateReconnecting:
            self.titleLabel.text = @"Reconnecting";
            self.detailLabel.text = detail ?: @"The page is safe. Surf is reconnecting to the server.";
            [self.spinner startAnimating];
            [self.primaryButton setTitle:@"Reconnect Now" forState:UIControlStateNormal];
            [self.secondaryButton setTitle:@"Choose Server" forState:UIControlStateNormal];
            break;
        case RBBrowserStateDisconnected:
            self.titleLabel.text = @"Surf Is Offline";
            self.detailLabel.text = detail ?: @"Check the server or choose another saved connection.";
            [self.primaryButton setTitle:@"Reconnect" forState:UIControlStateNormal];
            [self.secondaryButton setTitle:@"Choose Server" forState:UIControlStateNormal];
            break;
        case RBBrowserStatePageError:
            self.titleLabel.text = @"Page Couldn’t Load";
            self.detailLabel.text = detail ?: @"The server could not open this page.";
            [self.primaryButton setTitle:@"Try Again" forState:UIControlStateNormal];
            [self.secondaryButton setTitle:@"Go Back" forState:UIControlStateNormal];
            break;
        case RBBrowserStateVideoUnavailable:
            self.titleLabel.text = @"Video Unavailable";
            self.detailLabel.text = detail ?: @"The browser is still connected, but the video stream stopped.";
            [self.primaryButton setTitle:@"Retry Video" forState:UIControlStateNormal];
            [self.secondaryButton setTitle:@"Reconnect" forState:UIControlStateNormal];
            break;
        default:
            break;
    }
    [self setNeedsLayout];
}

- (void)applyAppearance {
    self.backgroundColor = [RBTheme pageBackgroundColor];
    self.titleLabel.textColor = [RBTheme primaryTextColor];
    self.detailLabel.textColor = [RBTheme secondaryTextColor];
    [RBTheme stylePrimaryButton:self.primaryButton];
    [RBTheme styleSecondaryButton:self.secondaryButton];
    self.spinner.activityIndicatorViewStyle = [RBTheme isDarkMode]
        ? UIActivityIndicatorViewStyleWhite : UIActivityIndicatorViewStyleGray;
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat w = self.bounds.size.width, h = self.bounds.size.height;
    CGFloat boxW = MIN(480.0, MAX(160.0, w - 40.0));
    BOOL compact = h < 340.0;
    CGFloat markH = compact ? 0.0 : 70.0;
    CGFloat spinnerH = self.spinner.isAnimating ? 30.0 : 0.0;
    CGFloat buttonsH = (self.primaryButton.hidden ? 0.0 : 50.0) + (self.secondaryButton.hidden ? 0.0 : 42.0);
    CGFloat detailH = MIN([self.detailLabel sizeThatFits:CGSizeMake(boxW, CGFLOAT_MAX)].height,
                         MAX(40.0, h - markH - spinnerH - buttonsH - 78.0));
    CGFloat totalH = markH + spinnerH + 40.0 + detailH + 16.0 + buttonsH;
    CGFloat y = MAX(12.0, floorf((h - totalH) / 2.0));
    self.markView.hidden = compact;
    self.markView.frame = CGRectMake((w - 58.0) / 2.0, y, 58.0, 58.0);
    y += markH;
    self.spinner.frame = CGRectMake((w - 24.0) / 2.0, y, 24.0, 24.0);
    y += spinnerH;
    self.titleLabel.frame = CGRectMake((w - boxW) / 2.0, y, boxW, 30.0);
    self.detailLabel.frame = CGRectMake((w - boxW) / 2.0, y + 40.0, boxW, detailH);
    y += 40.0 + detailH + 16.0;
    CGFloat buttonW = MIN(260.0, boxW);
    self.primaryButton.frame = CGRectMake((w - buttonW) / 2.0, y, buttonW, 42.0);
    if (!self.primaryButton.hidden) y += 50.0;
    self.secondaryButton.frame = CGRectMake((w - buttonW) / 2.0, y, buttonW, 38.0);
}

- (void)primary:(id)sender { [self.delegate browserStateViewPrimaryAction:self]; }
- (void)secondary:(id)sender { [self.delegate browserStateViewSecondaryAction:self]; }

@end
