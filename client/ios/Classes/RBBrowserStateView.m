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
    CGFloat boxW = MIN(480.0, w - 60.0);
    CGFloat y = MAX(28.0, floorf(h * 0.15));
    self.markView.frame = CGRectMake((w - 58.0) / 2.0, y, 58.0, 58.0);
    y += 70.0;
    self.spinner.frame = CGRectMake((w - 24.0) / 2.0, y, 24.0, 24.0);
    if (self.spinner.isAnimating) y += 38.0;
    self.titleLabel.frame = CGRectMake((w - boxW) / 2.0, y, boxW, 30.0);
    self.detailLabel.frame = CGRectMake((w - boxW) / 2.0, y + 40.0, boxW, 56.0);
    self.primaryButton.frame = CGRectMake((w - 190.0) / 2.0, y + 112.0, 190.0, 42.0);
    self.secondaryButton.frame = CGRectMake((w - 190.0) / 2.0, y + 160.0, 190.0, 38.0);
}

- (void)primary:(id)sender { [self.delegate browserStateViewPrimaryAction:self]; }
- (void)secondary:(id)sender { [self.delegate browserStateViewSecondaryAction:self]; }

@end
