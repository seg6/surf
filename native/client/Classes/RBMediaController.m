#import "RBMediaController.h"
#import "RBTheme.h"

#import <QuartzCore/QuartzCore.h>

#include <math.h>

@interface RBMediaController ()
@property(nonatomic, strong) UILabel *titleLabel;
@property(nonatomic, strong) UILabel *timeLabel;
@property(nonatomic, strong) UILabel *emptyLabel;
@property(nonatomic, strong) UIButton *playButton;
@property(nonatomic, strong) UIButton *muteButton;
@property(nonatomic, strong) UISlider *volumeSlider;
@property(nonatomic, strong) NSTimer *refreshTimer;
@end

@implementation RBMediaController

- (id)init {
    self = [super init];
    if (self) {
        self.title = @"Page Media";
        self.contentSizeForViewInPopover = CGSizeMake(340.0, 186.0);
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    self.view.backgroundColor = [RBTheme pageBackgroundColor];

    self.titleLabel = [[UILabel alloc] initWithFrame:CGRectZero];
    self.titleLabel.backgroundColor = [UIColor clearColor];
    self.titleLabel.font = [RBTheme fontOfSize:15.0 bold:YES];
    self.titleLabel.textColor = [RBTheme primaryTextColor];
    self.titleLabel.lineBreakMode = NSLineBreakByTruncatingTail;
    [self.view addSubview:self.titleLabel];

    self.timeLabel = [[UILabel alloc] initWithFrame:CGRectZero];
    self.timeLabel.backgroundColor = [UIColor clearColor];
    self.timeLabel.font = [RBTheme fontOfSize:12.0 bold:NO];
    self.timeLabel.textColor = [RBTheme secondaryTextColor];
    [self.view addSubview:self.timeLabel];

    self.playButton = [self controlButton:@"Play" icon:RBIconMedia];
    [self.playButton addTarget:self action:@selector(playTapped:) forControlEvents:UIControlEventTouchUpInside];
    [self.view addSubview:self.playButton];

    self.muteButton = [self controlButton:@"Mute" icon:RBIconMute];
    [self.muteButton addTarget:self action:@selector(muteTapped:) forControlEvents:UIControlEventTouchUpInside];
    [self.view addSubview:self.muteButton];

    self.volumeSlider = [[UISlider alloc] initWithFrame:CGRectZero];
    self.volumeSlider.minimumValue = 0.0;
    self.volumeSlider.maximumValue = 1.0;
    self.volumeSlider.value = 1.0;
    [self.volumeSlider addTarget:self action:@selector(volumeCommitted:)
                forControlEvents:UIControlEventTouchUpInside | UIControlEventTouchUpOutside];
    [self.view addSubview:self.volumeSlider];

    self.emptyLabel = [[UILabel alloc] initWithFrame:CGRectZero];
    self.emptyLabel.backgroundColor = [UIColor clearColor];
    self.emptyLabel.text = @"No audio or video was found on this page.";
    self.emptyLabel.textAlignment = NSTextAlignmentCenter;
    self.emptyLabel.numberOfLines = 2;
    self.emptyLabel.font = [RBTheme fontOfSize:14.0 bold:NO];
    self.emptyLabel.textColor = [RBTheme secondaryTextColor];
    self.emptyLabel.hidden = YES;
    [self.view addSubview:self.emptyLabel];
}

- (UIButton *)controlButton:(NSString *)title icon:(RBIcon)icon {
    UIButton *button = [UIButton buttonWithType:UIButtonTypeCustom];
    [RBTheme styleSecondaryButton:button];
    [button setTitle:title forState:UIControlStateNormal];
    [button setImage:[RBTheme icon:icon size:17.0 color:[RBTheme accentColor]] forState:UIControlStateNormal];
    button.imageEdgeInsets = UIEdgeInsetsMake(0.0, -5.0, 0.0, 5.0);
    return button;
}

- (void)viewDidLayoutSubviews {
    [super viewDidLayoutSubviews];
    CGFloat w = self.view.bounds.size.width;
    self.titleLabel.frame = CGRectMake(18.0, 16.0, w - 36.0, 24.0);
    self.timeLabel.frame = CGRectMake(18.0, 40.0, w - 36.0, 18.0);
    self.playButton.frame = CGRectMake(18.0, 72.0, 138.0, 38.0);
    self.muteButton.frame = CGRectMake(w - 156.0, 72.0, 138.0, 38.0);
    self.volumeSlider.frame = CGRectMake(18.0, 126.0, w - 36.0, 32.0);
    self.emptyLabel.frame = CGRectMake(24.0, 48.0, w - 48.0, 72.0);
}

- (void)viewDidAppear:(BOOL)animated {
    [super viewDidAppear:animated];
    [self.delegate mediaControllerRequestsRefresh:self];
    self.refreshTimer = [NSTimer scheduledTimerWithTimeInterval:1.0 target:self
                                                       selector:@selector(refresh:)
                                                       userInfo:nil repeats:YES];
}

- (void)viewWillDisappear:(BOOL)animated {
    [super viewWillDisappear:animated];
    [self.refreshTimer invalidate];
    self.refreshTimer = nil;
}

static NSString *RBMediaTime(double seconds) {
    if (seconds < 0 || !isfinite(seconds)) seconds = 0;
    NSInteger total = (NSInteger)seconds;
    return [NSString stringWithFormat:@"%ld:%02ld", (long)(total / 60), (long)(total % 60)];
}

- (void)applyState:(NSDictionary *)state {
    if (![self isViewLoaded]) [self view];
    BOOL available = [[state objectForKey:@"available"] boolValue];
    self.emptyLabel.hidden = available;
    self.titleLabel.hidden = !available;
    self.timeLabel.hidden = !available;
    self.playButton.hidden = !available;
    self.muteButton.hidden = !available;
    self.volumeSlider.hidden = !available;
    if (!available) return;

    NSString *title = [state objectForKey:@"title"];
    self.titleLabel.text = [title length] ? title : @"Media on this page";
    double current = [[state objectForKey:@"currentTime"] doubleValue];
    double duration = [[state objectForKey:@"duration"] doubleValue];
    NSInteger count = MAX(1, [[state objectForKey:@"count"] integerValue]);
    self.timeLabel.text = duration > 0
        ? [NSString stringWithFormat:@"%@ of %@  ·  %ld item%@",
           RBMediaTime(current), RBMediaTime(duration), (long)count, count == 1 ? @"" : @"s"]
        : [NSString stringWithFormat:@"%ld media item%@", (long)count, count == 1 ? @"" : @"s"];
    [self.playButton setTitle:([[state objectForKey:@"paused"] boolValue] ? @"Play" : @"Pause")
                     forState:UIControlStateNormal];
    RBIcon playbackIcon = [[state objectForKey:@"paused"] boolValue] ? RBIconMedia : RBIconPause;
    [self.playButton setImage:[RBTheme icon:playbackIcon size:17.0 color:[RBTheme accentColor]]
                     forState:UIControlStateNormal];
    [self.muteButton setTitle:([[state objectForKey:@"muted"] boolValue] ? @"Unmute" : @"Mute")
                     forState:UIControlStateNormal];
    self.volumeSlider.value = MIN(1.0, MAX(0.0, [[state objectForKey:@"volume"] floatValue]));
}

- (void)playTapped:(id)sender {
    [self.delegate mediaControllerTogglePlayback:self];
}

- (void)muteTapped:(id)sender {
    [self.delegate mediaControllerToggleMute:self];
}

- (void)volumeCommitted:(UISlider *)slider {
    [self.delegate mediaController:self setVolume:slider.value];
}

- (void)refresh:(NSTimer *)timer {
    [self.delegate mediaControllerRequestsRefresh:self];
}

@end
