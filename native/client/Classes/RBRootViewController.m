#import "RBRootViewController.h"
#import "RBAudioPlayer.h"
#import "RBChromeBar.h"
#import "RBConfig.h"
#import "RBFindBar.h"
#import "RBLibraryController.h"
#import "RBListPopover.h"
#import "RBReaderController.h"
#import "RBLog.h"
#import "RBOmnibox.h"
#import "RBProtocol.h"
#import "RBSession.h"
#import "RBSettingsController.h"
#import "RBStreamView.h"
#import "RBSuggestPanel.h"
#import "RBTabStrip.h"
#import "RBTheme.h"
#import "RBVideoDecoder.h"

#import <ImageIO/ImageIO.h>
#import <MessageUI/MessageUI.h>
#import <QuartzCore/QuartzCore.h>

#include <math.h>
#include <stdlib.h>

static UIImage *RBDecodeJPEG(NSData *data);

static const CGFloat kRBTopBarHeight = 56.0;
static const CGFloat kRBTabStripHeight = 34.0;
static const CGFloat kRBFindBarHeight = 44.0;

@interface RBRootViewController () <UITextFieldDelegate, RBSessionDelegate, RBChromeBarDelegate,
                                    RBOmniboxDelegate, RBTabStripDelegate, RBSuggestPanelDelegate,
                                    RBFindBarDelegate, RBSettingsDelegate, RBVideoDecoderDelegate,
                                    UIDocumentInteractionControllerDelegate, UIPopoverControllerDelegate,
                                    UIAlertViewDelegate, UIActionSheetDelegate,
                                    UIImagePickerControllerDelegate, UINavigationControllerDelegate,
                                    MFMailComposeViewControllerDelegate>
// Views
@property(nonatomic, strong) RBStreamView *streamView;
@property(nonatomic, strong) RBChromeBar *chromeBar;
@property(nonatomic, strong) RBTabStrip *tabStrip;
@property(nonatomic, strong) RBFindBar *findBar;
@property(nonatomic, strong) RBSuggestPanel *suggestPanel;
@property(nonatomic, strong) UIButton *restoreButton;
@property(nonatomic, strong) UILabel *toastLabel;
@property(nonatomic, strong) UILabel *connectionPill;
@property(nonatomic, strong) UILabel *debugLabel;
@property(nonatomic, strong) UITextField *hiddenInput;
// Controllers
@property(nonatomic, strong) RBSession *session;
@property(nonatomic, strong) RBSettingsController *settingsController;
@property(nonatomic, strong) UIPopoverController *popover;
@property(nonatomic, strong) UIDocumentInteractionController *docController;
// Connect flow
@property(nonatomic, copy) NSString *pendingServerURL;
@property(nonatomic, copy) NSString *pendingPassword;
// Frame pipeline
@property(nonatomic, strong) RBFrame *pendingFrame;
@property(nonatomic, assign) BOOL decodeBusy;
@property(nonatomic, assign) NSUInteger framesReceived;
@property(nonatomic, assign) NSUInteger framesDisplayed;
@property(nonatomic, assign) CFTimeInterval lastFrameAt;
@property(nonatomic, assign) CFTimeInterval lastPokeAt;
@property(nonatomic, assign) CFTimeInterval lastPerfLogAt;
@property(nonatomic, assign) NSUInteger perfLastFramesDisplayed;
@property(nonatomic, assign) NSUInteger perfLastFramesReceived;
@property(nonatomic, assign) NSUInteger perfLastVideoAUs;
@property(nonatomic, assign) NSUInteger perfLastDecodedFrames;
@property(nonatomic, assign) NSUInteger perfLastDecodeErrors;
@property(nonatomic, assign) NSUInteger perfLastVideoSubmitted;
@property(nonatomic, assign) NSUInteger perfLastVideoCallbacks;
@property(nonatomic, assign) NSUInteger perfLastVideoDrops;
@property(nonatomic, assign) double lastDecodeMS;
@property(nonatomic, assign) double averageDecodeMS;
// Page state
@property(nonatomic, assign) BOOL loading;
@property(nonatomic, assign) BOOL fullscreen;
@property(nonatomic, assign) BOOL findVisible;
@property(nonatomic, assign) BOOL debugVisible;
@property(nonatomic, copy) NSString *debugSummary;
@property(nonatomic, strong) NSArray *lastTabs;
// Copy menu
@property(nonatomic, copy) NSString *pendingCopyText;
@property(nonatomic, assign) CGPoint copyMenuPoint;
// Gestures
@property(nonatomic, assign) CGPoint panAnchor;
@property(nonatomic, assign) CGPoint lastPanPoint;
@property(nonatomic, assign) CGPoint inertiaAnchor;
@property(nonatomic, assign) CGPoint inertiaVelocity;
@property(nonatomic, strong) NSTimer *inertiaTimer;
@property(nonatomic, assign) CGPoint longPressStart;
@property(nonatomic, assign) BOOL longPressMoved;
// Video lane
@property(nonatomic, strong) RBVideoDecoder *videoDecoder;
@property(nonatomic, strong) RBAudioPlayer *audioPlayer;
@property(nonatomic, assign) BOOL videoActive;   // server confirmed video-config ok
@property(nonatomic, assign) BOOL videoRequested; // we sent video:on this connection
@property(nonatomic, assign) NSUInteger videoAUs;
// Keyboard avoidance (editable rect, viewport fractions)
@property(nonatomic, assign) CGRect editableRect;
@property(nonatomic, assign) BOOL editableHasRect;
@property(nonatomic, assign) BOOL keyboardVisible;
@property(nonatomic, assign) CGFloat keyboardTop;
@property(nonatomic, assign) CGFloat keyboardShift;
// JS dialogs (M2.1)
@property(nonatomic, strong) UIAlertView *dialogAlert;
@property(nonatomic, copy) NSString *dialogKind;
@property(nonatomic, assign) BOOL dialogSuppressReply;
// Uploads (M2.2)
@property(nonatomic, assign) BOOL chooserPending;
@property(nonatomic, strong) UIPopoverController *uploadPopover;
// Link context menu (M2.4)
@property(nonatomic, strong) NSDictionary *lastLinkInfo;
@property(nonatomic, strong) UIActionSheet *linkSheet;
@property(nonatomic, strong) NSArray *linkSheetActions;
// Error card (M2.5)
@property(nonatomic, strong) UIView *errorCard;
@property(nonatomic, strong) UILabel *errorCardLabel;
// Library (chrome rethink) / reader (M1.5)
@property(nonatomic, strong) RBLibraryController *libraryController;
@property(nonatomic, assign) BOOL readerPending;
@property(nonatomic, assign) BOOL readerResumeVideo;
// Pasteboard banner (M4.2)
@property(nonatomic, strong) UIAlertView *pasteboardAlert;
@property(nonatomic, copy) NSString *pasteboardURL;
// Latency echo (M1.1)
@property(nonatomic, assign) NSInteger latSeq;
@property(nonatomic, assign) CFTimeInterval latSentAt;
@property(nonatomic, assign) double lastRTTMS;
// Wheel coalescing (M1.3)
@property(nonatomic, assign) CGFloat wheelAccumX;
@property(nonatomic, assign) CGFloat wheelAccumY;
@property(nonatomic, assign) CFTimeInterval lastWheelSentAt;
// Edge swipes (M2.6): 0 none, -1 left edge (back), 1 right edge (forward)
@property(nonatomic, assign) int edgeSwipe;
@property(nonatomic, assign) CGPoint edgeStart;
@end

@implementation RBRootViewController

// ---------------------------------------------------------------- lifecycle

- (void)viewDidLoad {
    [super viewDidLoad];
    self.view.backgroundColor = [UIColor blackColor];
    [[NSNotificationCenter defaultCenter] addObserver:self selector:@selector(keyboardWillShow:)
                                                 name:UIKeyboardWillShowNotification object:nil];
    [[NSNotificationCenter defaultCenter] addObserver:self selector:@selector(keyboardWillHide:)
                                                 name:UIKeyboardWillHideNotification object:nil];
    [[NSNotificationCenter defaultCenter] addObserver:self selector:@selector(readerNavigate:)
                                                 name:@"RBReaderNavigate" object:nil];

    self.streamView = [[RBStreamView alloc] initWithFrame:CGRectZero];
    [self.view addSubview:self.streamView];
    self.audioPlayer = [[RBAudioPlayer alloc] init];

    UITapGestureRecognizer *tripleTap = [[UITapGestureRecognizer alloc] initWithTarget:self action:@selector(toggleDebug:)];
    tripleTap.numberOfTapsRequired = 3;
    [self.streamView addGestureRecognizer:tripleTap];

    UITapGestureRecognizer *tap = [[UITapGestureRecognizer alloc] initWithTarget:self action:@selector(tapped:)];
    [tap requireGestureRecognizerToFail:tripleTap];
    [self.streamView addGestureRecognizer:tap];

    UIPanGestureRecognizer *pan = [[UIPanGestureRecognizer alloc] initWithTarget:self action:@selector(panned:)];
    pan.maximumNumberOfTouches = 1;
    [self.streamView addGestureRecognizer:pan];

    UILongPressGestureRecognizer *longPress = [[UILongPressGestureRecognizer alloc] initWithTarget:self action:@selector(longPressed:)];
    longPress.minimumPressDuration = 0.55;
    [self.streamView addGestureRecognizer:longPress];

    self.chromeBar = [[RBChromeBar alloc] initWithFrame:CGRectZero];
    self.chromeBar.delegate = self;
    self.chromeBar.omnibox.delegate = self;
    [self.view addSubview:self.chromeBar];

    self.tabStrip = [[RBTabStrip alloc] initWithFrame:CGRectZero];
    self.tabStrip.delegate = self;
    [self.view addSubview:self.tabStrip];

    self.findBar = [[RBFindBar alloc] initWithFrame:CGRectZero];
    self.findBar.delegate = self;
    self.findBar.hidden = YES;
    [self.view addSubview:self.findBar];

    self.suggestPanel = [[RBSuggestPanel alloc] initWithFrame:CGRectZero];
    self.suggestPanel.delegate = self;
    [self.view addSubview:self.suggestPanel];

    self.restoreButton = [UIButton buttonWithType:UIButtonTypeCustom];
    self.restoreButton.backgroundColor = [UIColor colorWithWhite:0.0 alpha:0.42];
    self.restoreButton.layer.cornerRadius = 8.0;
    [self.restoreButton setImage:[RBTheme icon:RBIconShrink size:20.0 color:[UIColor colorWithWhite:1.0 alpha:0.9]]
                        forState:UIControlStateNormal];
    [self.restoreButton addTarget:self action:@selector(toggleFullscreen) forControlEvents:UIControlEventTouchUpInside];
    self.restoreButton.hidden = YES;
    [self.view addSubview:self.restoreButton];

    self.hiddenInput = [[UITextField alloc] initWithFrame:CGRectMake(-100.0, -100.0, 20.0, 20.0)];
    self.hiddenInput.delegate = self;
    self.hiddenInput.autocorrectionType = UITextAutocorrectionTypeNo;
    self.hiddenInput.autocapitalizationType = UITextAutocapitalizationTypeNone;
    self.hiddenInput.returnKeyType = UIReturnKeyGo;
    self.hiddenInput.text = @" ";
    [self.view addSubview:self.hiddenInput];

    self.toastLabel = [[UILabel alloc] initWithFrame:CGRectZero];
    self.toastLabel.backgroundColor = [UIColor colorWithWhite:0.10 alpha:0.86];
    self.toastLabel.textColor = [UIColor colorWithWhite:0.97 alpha:1.0];
    self.toastLabel.textAlignment = NSTextAlignmentCenter;
    self.toastLabel.font = [RBTheme fontOfSize:14.0 bold:NO];
    self.toastLabel.layer.cornerRadius = 14.0;
    self.toastLabel.layer.masksToBounds = YES;
    self.toastLabel.alpha = 0.0;
    [self.view addSubview:self.toastLabel];

    self.connectionPill = [[UILabel alloc] initWithFrame:CGRectZero];
    self.connectionPill.backgroundColor = [UIColor colorWithWhite:0.10 alpha:0.80];
    self.connectionPill.textColor = [UIColor colorWithWhite:0.95 alpha:1.0];
    self.connectionPill.textAlignment = NSTextAlignmentCenter;
    self.connectionPill.font = [RBTheme fontOfSize:12.0 bold:NO];
    self.connectionPill.layer.cornerRadius = 11.0;
    self.connectionPill.layer.masksToBounds = YES;
    self.connectionPill.hidden = YES;
    [self.view addSubview:self.connectionPill];

    self.debugLabel = [[UILabel alloc] initWithFrame:CGRectZero];
    self.debugLabel.backgroundColor = [UIColor colorWithWhite:0.0 alpha:0.72];
    self.debugLabel.textColor = [UIColor colorWithWhite:0.95 alpha:1.0];
    self.debugLabel.numberOfLines = 0;
    self.debugLabel.font = [UIFont fontWithName:@"Courier" size:10.0] ?: [UIFont systemFontOfSize:10.0];
    self.debugLabel.layer.cornerRadius = 6.0;
    self.debugLabel.layer.masksToBounds = YES;
    self.debugLabel.hidden = YES;
    self.debugLabel.userInteractionEnabled = NO;
    [self.view addSubview:self.debugLabel];

    [NSTimer scheduledTimerWithTimeInterval:1.0 target:self selector:@selector(watchdogTick:) userInfo:nil repeats:YES];

    RBLog(@"root view loaded, native %@", RBNativeVersion);
}

- (void)viewDidAppear:(BOOL)animated {
    [super viewDidAppear:animated];
    if (self.session || self.settingsController) return;
    NSUserDefaults *defaults = [NSUserDefaults standardUserDefaults];
    NSString *url = [defaults stringForKey:RBDefaultsServerURLKey];
    NSString *password = [defaults stringForKey:RBDefaultsPasswordKey];
    if ([password isEqualToString:@"alpine"]) password = RBDefaultPassword;
    if ([url length] && [password length]) {
        [self connectToURL:url password:password];
    } else {
        [self presentSettingsAllowingCancel:NO message:nil];
    }
}

- (void)didReceiveMemoryWarning {
    [super didReceiveMemoryWarning];
    [self.tabStrip purgeIconCache];
    self.pendingFrame = nil;
}

// ------------------------------------------------------------------- layout

- (void)viewDidLayoutSubviews {
    [super viewDidLayoutSubviews];
    CGFloat w = self.view.bounds.size.width;
    CGFloat h = self.view.bounds.size.height;

    self.chromeBar.hidden = self.fullscreen;
    self.tabStrip.hidden = self.fullscreen;
    self.findBar.hidden = self.fullscreen || !self.findVisible;
    self.suggestPanel.hidden = self.fullscreen;
    self.restoreButton.hidden = !self.fullscreen;

    CGFloat contentTop = 0.0;
    if (!self.fullscreen) {
        self.chromeBar.frame = CGRectMake(0.0, 0.0, w, kRBTopBarHeight);
        self.tabStrip.frame = CGRectMake(0.0, kRBTopBarHeight, w, kRBTabStripHeight);
        contentTop = kRBTopBarHeight + kRBTabStripHeight;
        if (self.findVisible) {
            self.findBar.frame = CGRectMake(0.0, contentTop, w, kRBFindBarHeight);
            contentTop += kRBFindBarHeight;
        }
    } else {
        self.chromeBar.frame = CGRectZero;
        self.tabStrip.frame = CGRectZero;
        self.findBar.frame = CGRectZero;
    }

    CGFloat streamH = MAX(1.0, h - contentTop);
    self.streamView.bounds = CGRectMake(0.0, 0.0, w, streamH);
    self.streamView.center = CGPointMake(w / 2.0, contentTop + streamH / 2.0);

    CGRect omniboxFrame = [self.chromeBar convertRect:self.chromeBar.omnibox.frame toView:self.view];
    self.suggestPanel.frame = CGRectMake(omniboxFrame.origin.x, kRBTopBarHeight - 4.0,
                                         omniboxFrame.size.width, [self.suggestPanel desiredHeight]);

    self.restoreButton.frame = CGRectMake(w - 54.0, h - 54.0, 44.0, 44.0);
    self.toastLabel.frame = CGRectMake((w - 320.0) / 2.0, contentTop + 14.0, 320.0, 28.0);
    self.connectionPill.frame = CGRectMake(10.0, h - 34.0, 150.0, 22.0);
    CGFloat debugW = MIN(360.0, w - 20.0);
    self.debugLabel.frame = CGRectMake(10.0, contentTop + 8.0, debugW, 64.0);
    [self scheduleViewportUpdate];
}

- (BOOL)shouldAutorotateToInterfaceOrientation:(UIInterfaceOrientation)interfaceOrientation { return YES; }
- (NSUInteger)supportedInterfaceOrientations { return UIInterfaceOrientationMaskAll; }
- (BOOL)shouldAutorotate { return YES; }

- (void)willAnimateRotationToInterfaceOrientation:(UIInterfaceOrientation)toInterfaceOrientation duration:(NSTimeInterval)duration {
    [super willAnimateRotationToInterfaceOrientation:toInterfaceOrientation duration:duration];
    [self.view setNeedsLayout];
}

- (void)scheduleViewportUpdate {
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(sendCurrentViewportSize) object:nil];
    [self performSelector:@selector(sendCurrentViewportSize) withObject:nil afterDelay:0.08];
}

- (void)sendCurrentViewportSize {
    [self sendCurrentViewportSizeForced:NO];
}

- (void)sendCurrentViewportSizeForced:(BOOL)force {
    CGSize s = self.streamView.bounds.size;
    if (s.width < 10.0 || s.height < 10.0) return;
    [self.session updateViewportWidth:(NSInteger)(s.width + 0.5)
                                height:(NSInteger)(s.height + 0.5)
                                 force:force];
}

// ------------------------------------------------------------ connect flow

- (void)connectToURL:(NSString *)url password:(NSString *)password {
    NSURL *base = [NSURL URLWithString:url ?: @""];
    NSString *scheme = [[base scheme] lowercaseString];
    if (!base || ![base host] || (![scheme isEqualToString:@"http"] && ![scheme isEqualToString:@"https"])) {
        [self presentSettingsAllowingCancel:YES message:@"Enter a valid http:// or https:// server URL"];
        return;
    }
    self.pendingServerURL = url;
    self.pendingPassword = password;
    [self leaveVideoMode];
    RBSession *oldSession = self.session;
    oldSession.delegate = nil;
    [oldSession shutdown];
    self.session = [[RBSession alloc] initWithBaseURL:url];
    self.session.delegate = self;
    [self.session startWithPassword:password];
}

- (void)presentSettingsAllowingCancel:(BOOL)allowsCancel message:(NSString *)message {
    if (self.settingsController) {
        if (message) [self.settingsController setStatusText:message isError:YES];
        return;
    }
    NSUserDefaults *defaults = [NSUserDefaults standardUserDefaults];
    NSString *url = [defaults stringForKey:RBDefaultsServerURLKey] ?: RBDefaultServerURL;
    NSString *password = [defaults stringForKey:RBDefaultsPasswordKey] ?: RBDefaultPassword;
    if ([password isEqualToString:@"alpine"]) password = RBDefaultPassword;
    RBSettingsController *settings = [[RBSettingsController alloc] initWithServerURL:url password:password];
    settings.delegate = self;
    settings.allowsCancel = allowsCancel;
    settings.connected = self.session.state == RBSessionStateOpen;
    settings.diagnosticsVisible = self.debugVisible;
    self.settingsController = settings;
    UINavigationController *nav = [[UINavigationController alloc] initWithRootViewController:settings];
    nav.modalPresentationStyle = UIModalPresentationFormSheet;
    [self presentViewController:nav animated:YES completion:nil];
    if (message) [settings setStatusText:message isError:YES];
}

// ---- settings delegate (chrome rethink) ----------------------------------

- (void)settings:(RBSettingsController *)settings clearData:(NSString *)what {
    [self.session sendMessage:@{@"t": @"clear", @"what": what ?: @""}];
}

- (void)settingsStreamChanged:(RBSettingsController *)settings {
    [self sendCurrentStreamProfile];
    BOOL wantVideo = [[[NSUserDefaults standardUserDefaults] objectForKey:RBDefaultsVideoKey] boolValue] ||
                     [[NSUserDefaults standardUserDefaults] objectForKey:RBDefaultsVideoKey] == nil;
    if (self.videoActive || self.videoRequested) {
        [self.session sendMessage:@{@"t": @"video", @"on": @NO}];
        [self leaveVideoMode];
    }
    if (wantVideo) [self performSelector:@selector(maybeEnableVideo) withObject:nil afterDelay:0.2];
}

- (void)settings:(RBSettingsController *)settings setDiagnosticsVisible:(BOOL)visible {
    [self setDebugVisible:visible];
}

- (void)settings:(RBSettingsController *)settings connectToURL:(NSString *)url password:(NSString *)password {
    [self connectToURL:url password:password];
}

- (void)settingsDismissed:(RBSettingsController *)settings {
    // First-launch settings (no cancel) must stay up until a connection works.
    if (!settings.allowsCancel && self.session.state != RBSessionStateOpen) return;
    self.settingsController = nil;
    [self dismissViewControllerAnimated:YES completion:nil];
}

- (void)sessionDidAuthenticate:(RBSession *)session {
    if (session != self.session) return;
    NSUserDefaults *defaults = [NSUserDefaults standardUserDefaults];
    [defaults setObject:self.pendingServerURL forKey:RBDefaultsServerURLKey];
    [defaults setObject:self.pendingPassword forKey:RBDefaultsPasswordKey];
    [self saveServerURL:self.pendingServerURL];
    [defaults synchronize];
    if (self.settingsController) {
        [self.settingsController setStatusText:@"Connected" isError:NO];
        RBSettingsController *presented = self.settingsController;
        self.settingsController = nil;
        [presented.presentingViewController dismissViewControllerAnimated:YES completion:nil];
    }
}

- (void)saveServerURL:(NSString *)url {
    if (![url length]) return;
    NSUserDefaults *defaults = [NSUserDefaults standardUserDefaults];
    NSArray *old = [defaults arrayForKey:RBDefaultsServersKey] ?: @[];
    NSMutableArray *servers = [NSMutableArray arrayWithCapacity:MIN(8, [old count] + 1)];
    [servers addObject:@{@"title": [[NSURL URLWithString:url] host] ?: url, @"url": url}];
    for (NSDictionary *entry in old) {
        NSString *u = [entry objectForKey:@"url"];
        if (![u length] || [u isEqualToString:url]) continue;
        [servers addObject:entry];
        if ([servers count] >= 8) break;
    }
    [defaults setObject:servers forKey:RBDefaultsServersKey];
}

- (void)sessionNeedsPassword:(RBSession *)session message:(NSString *)message {
    if (session != self.session) return;
    [self presentSettingsAllowingCancel:NO message:message ?: @"Login failed"];
}

- (void)session:(RBSession *)session didChangeState:(RBSessionState)state {
    if (session != self.session) return;
    switch (state) {
        case RBSessionStateOpen:
            self.connectionPill.hidden = YES;
            [self.view setNeedsLayout];
            [self.view layoutIfNeeded];
            [self sendCurrentViewportSizeForced:YES];
            [self.session sendMessage:@{@"t": @"audio", @"on": @YES}];
            [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(maybeEnableVideo) object:nil];
            [self performSelector:@selector(maybeEnableVideo) withObject:nil afterDelay:0.12];
            break;
        case RBSessionStateConnecting:
            self.connectionPill.hidden = NO;
            self.connectionPill.text = @"Connecting…";
            [self leaveVideoMode];
            [self.audioPlayer stop];
            break;
        case RBSessionStateRetrying:
            self.connectionPill.hidden = NO;
            self.connectionPill.text = @"Reconnecting…";
            [self leaveVideoMode];
            break;
        case RBSessionStateIdle:
            self.connectionPill.hidden = NO;
            self.connectionPill.text = @"Disconnected";
            [self leaveVideoMode];
            [self.audioPlayer stop];
            break;
    }
}

// ------------------------------------------------------------- video lane

- (void)maybeEnableVideo {
    NSNumber *want = [[NSUserDefaults standardUserDefaults] objectForKey:RBDefaultsVideoKey];
    BOOL enabled = want == nil || [want boolValue];
    if (!enabled || ![RBVideoDecoder available]) return;
    [self sendCurrentStreamProfile];
    self.videoRequested = YES;
    [self.session sendMessage:@{@"t": @"video", @"on": @YES}];
}

- (NSString *)currentStreamProfile {
    NSString *p = [[NSUserDefaults standardUserDefaults] stringForKey:RBDefaultsStreamProfileKey];
    return [p length] ? p : @"balanced";
}

- (NSString *)titleForStreamProfile:(NSString *)profile {
    if ([profile isEqualToString:@"sharp"]) return @"Sharp 30";
    if ([profile isEqualToString:@"smooth"]) return @"Smooth 60";
    if ([profile isEqualToString:@"fast"]) return @"Fast 45";
    if ([profile isEqualToString:@"potato"]) return @"Low Data";
    if ([profile isEqualToString:@"max"]) return @"Max 60";
    return @"Balanced 30";
}

- (void)sendCurrentStreamProfile {
    [self.session sendMessage:@{@"t": @"stream", @"profile": [self currentStreamProfile]}];
}

// Local-only teardown (reconnects, server switches): the server side is
// cleaned up by its own disconnect path.
- (void)leaveVideoMode {
    if (!self.videoActive && !self.videoRequested) return;
    self.videoActive = NO;
    self.videoRequested = NO;
    self.streamView.videoActive = NO;
    [self.videoDecoder reset];
}

- (void)handleVideoConfig:(NSDictionary *)message {
    BOOL ok = [[message objectForKey:@"ok"] boolValue];
    if (!ok) {
        BOOL hadLane = self.videoActive;
        [self leaveVideoMode];
        if (hadLane) [self showToast:@"Video lane lost — using JPEG"];
        else if (self.videoRequested) [self showToast:@"Video unavailable — using JPEG"];
        return;
    }
    if (!self.videoDecoder) {
        self.videoDecoder = [[RBVideoDecoder alloc] init];
        self.videoDecoder.delegate = self;
    }
    self.videoDecoder.codedWidth = [[message objectForKey:@"w"] intValue] ?: 1024;
    self.videoDecoder.codedHeight = [[message objectForKey:@"h"] intValue] ?: 768;
    [self.videoDecoder reset];
    self.videoActive = YES;
    self.streamView.videoActive = YES;
    self.videoAUs = 0;
    RBLog(@"video: lane up %dx%d", self.videoDecoder.codedWidth, self.videoDecoder.codedHeight);
}

- (void)videoDecoder:(RBVideoDecoder *)decoder didDecodeImage:(CGImageRef)image {
    if (!self.videoActive) return;
    [self.streamView displayVideoImage:image];
    self.framesDisplayed++;
    self.lastFrameAt = CACurrentMediaTime();
}

- (void)videoDecoderDidFail:(RBVideoDecoder *)decoder {
    RBLog(@"video: decoder gave up, dropping to JPEG lane");
    [self.session sendMessage:@{@"t": @"video", @"on": @NO}];
    [self leaveVideoMode];
    [self showToast:@"Video decode failed — using JPEG"];
}

- (void)videoDecoderNeedsKeyframe:(RBVideoDecoder *)decoder {
    [self.session sendMessage:@{@"t": @"reqkeyframe"}];
}

- (void)session:(RBSession *)session status:(NSString *)status {
    RBLog(@"session status: %@", status);
}

// --------------------------------------------------------- incoming frames

- (void)session:(RBSession *)session didReceiveFrameData:(NSData *)data {
    NSString *error = nil;
    RBFrame *frame = [RBProtocol frameFromData:data error:&error];
    if (!frame) {
        RBLog(@"bad frame: %@", error);
        [self.session sendReady];
        return;
    }
    if (frame.type == 3) {
        // H.264 AU: not acked (flow control is IDR-drop based, server side).
        if (self.videoActive) {
            if (frame.width > 0 && frame.height > 0 &&
                (self.videoDecoder.codedWidth != frame.width || self.videoDecoder.codedHeight != frame.height)) {
                self.videoDecoder.codedWidth = frame.width;
                self.videoDecoder.codedHeight = frame.height;
                [self.videoDecoder reset];
                self.videoAUs = 0;
                RBLog(@"video: coded size changed to %dx%d", self.videoDecoder.codedWidth, self.videoDecoder.codedHeight);
            }
            self.videoAUs++;
            [self.videoDecoder feedAU:frame.payload idr:(frame.flags & 1) != 0];
        }
        return;
    }
    if (frame.type == 4) {
        [self.audioPlayer playPCM:frame.payload];
        return;
    }
    if (frame.type != 1) return;
    if (self.videoActive) {
        // Video mode is H.264 only. Any stray JPEG must be acked for the
        // server's type-1 window, but decoding it causes long A5 stalls.
        [self.session sendReady];
        return;
    }
    self.framesReceived++;
    if (self.pendingFrame) {
        self.pendingFrame = nil;
        [self.session sendReady];
    }
    self.pendingFrame = frame;
    [self startNextDecodeIfNeeded];
}

- (void)startNextDecodeIfNeeded {
    if (self.decodeBusy || !self.pendingFrame) return;
    RBFrame *frame = self.pendingFrame;
    self.pendingFrame = nil;
    self.decodeBusy = YES;
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        CFTimeInterval started = CACurrentMediaTime();
        UIImage *image = RBDecodeJPEG(frame.payload);
        double decodeMS = (CACurrentMediaTime() - started) * 1000.0;
        dispatch_async(dispatch_get_main_queue(), ^{
            if (image) {
                [CATransaction begin];
                [CATransaction setDisableActions:YES];
                [self.streamView displayImage:image width:frame.width height:frame.height];
                [CATransaction commit];
                self.framesDisplayed++;
                self.lastFrameAt = CACurrentMediaTime();
                self.lastDecodeMS = decodeMS;
                self.averageDecodeMS = self.averageDecodeMS <= 0.0 ? decodeMS : (self.averageDecodeMS * 0.85 + decodeMS * 0.15);
            } else {
                RBLog(@"jpeg decode failed seq=%u bytes=%u", frame.seq, [frame.payload length]);
            }
            [self.session sendReady];
            self.decodeBusy = NO;
            [self startNextDecodeIfNeeded];
        });
    });
}

static UIImage *RBDecodeJPEG(NSData *data) {
    CGImageSourceRef source = CGImageSourceCreateWithData((__bridge CFDataRef)data, NULL);
    if (!source) return nil;
    CGImageRef image = CGImageSourceCreateImageAtIndex(source, 0, NULL);
    CFRelease(source);
    if (!image) return nil;

    size_t width = CGImageGetWidth(image);
    size_t height = CGImageGetHeight(image);
    CGColorSpaceRef colorSpace = CGColorSpaceCreateDeviceRGB();
    CGContextRef ctx = CGBitmapContextCreate(NULL, width, height, 8, 0, colorSpace, kCGImageAlphaNoneSkipFirst | kCGBitmapByteOrder32Little);
    CGColorSpaceRelease(colorSpace);
    if (!ctx) {
        CGImageRelease(image);
        return nil;
    }
    CGContextDrawImage(ctx, CGRectMake(0, 0, width, height), image);
    CGImageRef decoded = CGBitmapContextCreateImage(ctx);
    CGContextRelease(ctx);
    CGImageRelease(image);
    if (!decoded) return nil;
    UIImage *out = [UIImage imageWithCGImage:decoded];
    CGImageRelease(decoded);
    return out;
}

// ------------------------------------------------------- control messages

- (void)session:(RBSession *)session didReceiveControlMessage:(NSDictionary *)message {
    NSString *t = [message objectForKey:@"t"];
    if ([t isEqualToString:@"url"]) {
        NSString *url = [message objectForKey:@"url"];
        if (url) [self.chromeBar.omnibox setURLText:url];
        [self.chromeBar.omnibox setStarred:[[message objectForKey:@"starred"] boolValue]];
        [self.chromeBar.omnibox setSecurityState:[message objectForKey:@"security"]];
        [self hideErrorCard];
    } else if ([t isEqualToString:@"histstate"]) {
        [self.chromeBar setCanGoBack:[[message objectForKey:@"back"] boolValue]
                             forward:[[message objectForKey:@"fwd"] boolValue]];
    } else if ([t isEqualToString:@"loading"]) {
        self.loading = [[message objectForKey:@"on"] boolValue];
        [self.chromeBar.omnibox setLoading:self.loading];
        if (self.loading) [self hideErrorCard];
    } else if ([t isEqualToString:@"editable"]) {
        if ([[message objectForKey:@"on"] boolValue]) {
            [self configureKeyboardForKind:[message objectForKey:@"kind"] rect:[message objectForKey:@"rect"]];
            [self showKeyboard];
        } else {
            self.editableHasRect = NO;
            if ([self.hiddenInput isFirstResponder]) [self.hiddenInput resignFirstResponder];
            [self updateKeyboardAvoidance];
        }
    } else if ([t isEqualToString:@"video-config"]) {
        [self handleVideoConfig:message];
    } else if ([t isEqualToString:@"audio-config"]) {
        if ([[message objectForKey:@"ok"] boolValue]) {
            [self.audioPlayer configureSampleRate:[[message objectForKey:@"rate"] intValue]
                                       channels:[[message objectForKey:@"channels"] intValue]];
            RBLog(@"audio: lane up");
        } else {
            [self.audioPlayer stop];
            RBLog(@"audio: lane down");
        }
    } else if ([t isEqualToString:@"copytext"]) {
        NSString *text = [message objectForKey:@"text"] ?: @"";
        if ([text length]) [self showCopyMenuForText:text];
        else [self showToast:@"No text selected"];
    } else if ([t isEqualToString:@"found"]) {
        [self.findBar setFound:[[message objectForKey:@"on"] boolValue]];
    } else if ([t isEqualToString:@"download"]) {
        [self showToast:[NSString stringWithFormat:@"Downloaded %@", [message objectForKey:@"name"] ?: @""]];
    } else if ([t isEqualToString:@"downloads"]) {
        [self.libraryController setDownloads:[message objectForKey:@"items"]];
    } else if ([t isEqualToString:@"tabs"]) {
        id tabs = [message objectForKey:@"tabs"];
        self.lastTabs = [tabs isKindOfClass:[NSArray class]] ? tabs : nil;
        [self.tabStrip setTabs:self.lastTabs baseURL:self.session.baseURL];
    } else if ([t isEqualToString:@"hist"]) {
        [self.libraryController setBookmarks:[message objectForKey:@"bookmarks"]];
    } else if ([t isEqualToString:@"starred"]) {
        [self.chromeBar.omnibox setStarred:[[message objectForKey:@"on"] boolValue]];
    } else if ([t isEqualToString:@"suggest"]) {
        if (self.chromeBar.omnibox.editing) {
            [self.suggestPanel showItems:[message objectForKey:@"items"]];
            [self.view setNeedsLayout];
        }
    } else if ([t isEqualToString:@"toast"]) {
        [self showToast:[message objectForKey:@"text"] ?: @"OK"];
    } else if ([t isEqualToString:@"dialog"]) {
        [self showDialogWithKind:[message objectForKey:@"kind"]
                            text:[message objectForKey:@"text"]
                      defaultText:[message objectForKey:@"def"]];
    } else if ([t isEqualToString:@"dialogdone"]) {
        [self dismissDialogSilently];
    } else if ([t isEqualToString:@"filechooser"]) {
        [self presentUploadPicker];
    } else if ([t isEqualToString:@"dlprogress"]) {
        NSString *name = [message objectForKey:@"name"] ?: @"download";
        int pct = [[message objectForKey:@"pct"] intValue];
        if (self.libraryController) {
            // Progress belongs in the Library rows, not toast spam.
            [self.libraryController updateDownloadProgress:name pct:pct];
        } else {
            [self showToast:(pct >= 0 ? [NSString stringWithFormat:@"%@ — %d%%", name, pct]
                                      : [NSString stringWithFormat:@"%@…", name])];
        }
    } else if ([t isEqualToString:@"linkinfo"]) {
        self.lastLinkInfo = message;
    } else if ([t isEqualToString:@"security"]) {
        [self.chromeBar.omnibox setSecurityState:[message objectForKey:@"state"]];
    } else if ([t isEqualToString:@"pageerror"]) {
        [self showErrorCardForURL:[message objectForKey:@"url"]];
    } else if ([t isEqualToString:@"reader"]) {
        [self handleReaderReply:message];
    } else if ([t isEqualToString:@"history"]) {
        [self.libraryController consumeHistoryReply:message];
    } else if ([t isEqualToString:@"lat"]) {
        if (self.latSentAt > 0.0) {
            self.lastRTTMS = (CACurrentMediaTime() - self.latSentAt) * 1000.0;
            self.latSentAt = 0.0;
        }
    }
}

// ----------------------------------------------------------------- gestures

- (CGPoint)fractionForPoint:(CGPoint)p {
    CGSize s = self.streamView.bounds.size;
    CGFloat x = MIN(1.0, MAX(0.0, p.x / MAX(1.0, s.width)));
    CGFloat y = MIN(1.0, MAX(0.0, p.y / MAX(1.0, s.height)));
    return CGPointMake(x, y);
}

- (void)tapped:(UITapGestureRecognizer *)tap {
    if (tap.state != UIGestureRecognizerStateEnded) return;
    if (self.chromeBar.omnibox.editing) {
        [self.chromeBar.omnibox dismissKeyboard];
        [self.suggestPanel hide];
        return;
    }
    [self hidePageKeyboard];
    [self stopInertia];
    [self hideCopyMenu];
    [self.streamView hideSharpOverlay];
    CGPoint p = [tap locationInView:self.streamView];
    [self showTapRippleAt:p];
    CGPoint f = [self fractionForPoint:p];
    [self.session sendClickX:f.x y:f.y];
}

// Tap ripple (M1.3): instant local acknowledgment — perceived latency is the
// one kind we can fix for free.
- (void)showTapRippleAt:(CGPoint)p {
    UIView *ripple = [[UIView alloc] initWithFrame:CGRectMake(p.x - 14.0, p.y - 14.0, 28.0, 28.0)];
    ripple.backgroundColor = [UIColor colorWithWhite:0.5 alpha:0.35];
    ripple.layer.cornerRadius = 14.0;
    ripple.userInteractionEnabled = NO;
    [self.streamView addSubview:ripple];
    [UIView animateWithDuration:0.35 animations:^{
        ripple.transform = CGAffineTransformMakeScale(1.7, 1.7);
        ripple.alpha = 0.0;
    } completion:^(BOOL finished) {
        [ripple removeFromSuperview];
    }];
}

- (void)panned:(UIPanGestureRecognizer *)pan {
    CGPoint p = [pan locationInView:self.streamView];
    if (pan.state == UIGestureRecognizerStateBegan) {
        [self hidePageKeyboard];
        [self stopInertia];
        [self.streamView hideSharpOverlay];
        // Edge swipes (M2.6): a pan born on a screen edge is history nav.
        CGFloat w = self.streamView.bounds.size.width;
        self.edgeSwipe = 0;
        if (p.x < 24.0) self.edgeSwipe = -1;
        else if (p.x > w - 24.0) self.edgeSwipe = 1;
        self.edgeStart = p;
        self.panAnchor = [self fractionForPoint:p];
        self.inertiaAnchor = self.panAnchor;
        self.lastPanPoint = p;
        self.wheelAccumX = 0.0;
        self.wheelAccumY = 0.0;
        self.lastWheelSentAt = 0.0;
        return;
    }
    if (pan.state == UIGestureRecognizerStateChanged) {
        if (self.edgeSwipe != 0) return; // candidate history swipe: no scrolling
        CGSize s = self.streamView.bounds.size;
        CGFloat dx = -(p.x - self.lastPanPoint.x) / MAX(1.0, s.width);
        CGFloat dy = -(p.y - self.lastPanPoint.y) / MAX(1.0, s.height);
        self.lastPanPoint = p;
        // Coalesce at ~30Hz (M1.3): halves the message rate of a flick with
        // no perceptible feel change; the remainder flushes on gesture end.
        self.wheelAccumX += dx;
        self.wheelAccumY += dy;
        CFTimeInterval now = CACurrentMediaTime();
        if (now - self.lastWheelSentAt >= 1.0 / 30.0) {
            [self flushWheelAccum];
            self.lastWheelSentAt = now;
        }
        CGPoint v = [pan velocityInView:self.streamView];
        self.inertiaVelocity = CGPointMake(-v.x, -v.y);
        return;
    }
    if (pan.state == UIGestureRecognizerStateEnded || pan.state == UIGestureRecognizerStateCancelled) {
        if (self.edgeSwipe != 0) {
            CGFloat travel = p.x - self.edgeStart.x;
            if (pan.state == UIGestureRecognizerStateEnded) {
                if (self.edgeSwipe == -1 && travel > 70.0) [self.session sendMessage:@{@"t": @"back"}];
                else if (self.edgeSwipe == 1 && travel < -70.0) [self.session sendMessage:@{@"t": @"fwd"}];
            }
            self.edgeSwipe = 0;
            return;
        }
        [self flushWheelAccum];
        if (pan.state == UIGestureRecognizerStateEnded) [self startInertiaIfNeeded];
    }
}

- (void)flushWheelAccum {
    if (fabs(self.wheelAccumX) < 0.0001 && fabs(self.wheelAccumY) < 0.0001) return;
    [self.session sendWheelX:self.panAnchor.x y:self.panAnchor.y dx:self.wheelAccumX dy:self.wheelAccumY];
    self.wheelAccumX = 0.0;
    self.wheelAccumY = 0.0;
}

- (void)startInertiaIfNeeded {
    CGFloat speed = sqrtf(self.inertiaVelocity.x * self.inertiaVelocity.x + self.inertiaVelocity.y * self.inertiaVelocity.y);
    if (speed < 220.0) return;
    [self stopInertia];
    self.inertiaTimer = [NSTimer scheduledTimerWithTimeInterval:1.0 / 60.0 target:self selector:@selector(inertiaTick:) userInfo:nil repeats:YES];
}

- (void)stopInertia {
    [self.inertiaTimer invalidate];
    self.inertiaTimer = nil;
}

- (void)inertiaTick:(NSTimer *)timer {
    CGSize s = self.streamView.bounds.size;
    CGFloat dt = 1.0 / 60.0;
    CGFloat dx = self.inertiaVelocity.x * dt / MAX(1.0, s.width);
    CGFloat dy = self.inertiaVelocity.y * dt / MAX(1.0, s.height);
    [self.session sendWheelX:self.inertiaAnchor.x y:self.inertiaAnchor.y dx:dx dy:dy];
    self.inertiaVelocity = CGPointMake(self.inertiaVelocity.x * 0.94, self.inertiaVelocity.y * 0.94);
    CGFloat speed = sqrtf(self.inertiaVelocity.x * self.inertiaVelocity.x + self.inertiaVelocity.y * self.inertiaVelocity.y);
    if (speed < 45.0) [self stopInertia];
}

- (void)longPressed:(UILongPressGestureRecognizer *)longPress {
    CGPoint p = [longPress locationInView:self.streamView];
    CGPoint f = [self fractionForPoint:p];
    if (longPress.state == UIGestureRecognizerStateBegan) {
        [self stopInertia];
        [self.streamView hideSharpOverlay];
        self.longPressStart = p;
        self.longPressMoved = NO;
        // Ask what's under the finger (M2.4); the answer usually lands well
        // before the finger lifts and decides menu vs. text selection.
        self.lastLinkInfo = nil;
        [self.session sendMessage:@{@"t": @"hit", @"x": [NSNumber numberWithFloat:f.x], @"y": [NSNumber numberWithFloat:f.y]}];
        [self.session sendMessage:@{@"t": @"lpdown", @"x": [NSNumber numberWithFloat:f.x], @"y": [NSNumber numberWithFloat:f.y]}];
        return;
    }
    if (longPress.state == UIGestureRecognizerStateChanged) {
        if (fabsf(p.x - self.longPressStart.x) > 8.0 || fabsf(p.y - self.longPressStart.y) > 8.0) self.longPressMoved = YES;
        if (self.longPressMoved) [self.session sendMessage:@{@"t": @"lpmove", @"x": [NSNumber numberWithFloat:f.x], @"y": [NSNumber numberWithFloat:f.y]}];
        return;
    }
    if (longPress.state == UIGestureRecognizerStateEnded || longPress.state == UIGestureRecognizerStateCancelled || longPress.state == UIGestureRecognizerStateFailed) {
        // A plain press on a link/image becomes a context menu, not a word
        // selection — matching what fingers expect from a browser.
        NSString *href = [self.lastLinkInfo objectForKey:@"href"];
        NSString *img = [self.lastLinkInfo objectForKey:@"img"];
        BOOL linky = !self.longPressMoved && ([href length] || [img length]);
        [self.session sendMessage:@{@"t": @"lpup", @"x": [NSNumber numberWithFloat:f.x], @"y": [NSNumber numberWithFloat:f.y],
                                    @"sel": [NSNumber numberWithBool:(!self.longPressMoved && !linky)]}];
        if (linky && longPress.state == UIGestureRecognizerStateEnded) [self presentLinkSheet];
    }
}

// ----------------------------------------------------------- copy menu

- (BOOL)canBecomeFirstResponder {
    return YES;
}

- (BOOL)canPerformAction:(SEL)action withSender:(id)sender {
    if (action == @selector(rbCopySelection:)) return [self.pendingCopyText length] > 0;
    return NO;
}

- (void)showCopyMenuForText:(NSString *)text {
    self.pendingCopyText = text;
    self.copyMenuPoint = self.longPressStart;
    [self becomeFirstResponder];
    UIMenuController *menu = [UIMenuController sharedMenuController];
    menu.menuItems = @[[[UIMenuItem alloc] initWithTitle:@"Copy" action:@selector(rbCopySelection:)]];
    CGRect target = CGRectMake(self.copyMenuPoint.x - 2.0, self.copyMenuPoint.y - 2.0, 4.0, 4.0);
    [menu setTargetRect:target inView:self.streamView];
    [menu setMenuVisible:YES animated:YES];
}

- (void)hideCopyMenu {
    if (![self.pendingCopyText length]) return;
    self.pendingCopyText = nil;
    [[UIMenuController sharedMenuController] setMenuVisible:NO animated:YES];
}

- (void)rbCopySelection:(id)sender {
    [UIPasteboard generalPasteboard].string = self.pendingCopyText ?: @"";
    self.pendingCopyText = nil;
    [self showToast:@"Copied"];
}

// ------------------------------------------------------------- chrome bar

- (void)chromeBack:(RBChromeBar *)bar { [self.session sendMessage:@{@"t": @"back"}]; }
- (void)chromeForward:(RBChromeBar *)bar { [self.session sendMessage:@{@"t": @"fwd"}]; }

- (void)chromeKeyboard:(RBChromeBar *)bar {
    if ([self.hiddenInput isFirstResponder]) [self.hiddenInput resignFirstResponder];
    else [self showKeyboard];
}

// Page actions (share button): everything scoped to the current page.
- (void)chrome:(RBChromeBar *)bar actionsFromButton:(UIButton *)button {
    NSArray *items = @[
        [RBListItem itemWithTitle:@"Reader" subtitle:@"read this page natively" payload:@"reader"],
        [RBListItem itemWithTitle:@"Find on Page" subtitle:nil payload:@"find"],
        [RBListItem itemWithTitle:@"Paste to Page" subtitle:nil payload:@"paste"],
        [RBListItem itemWithTitle:@"Copy Page URL" subtitle:nil payload:@"copyurl"],
        [RBListItem itemWithTitle:@"Mail Link" subtitle:nil payload:@"maillink"],
        [RBListItem itemWithTitle:@"Fullscreen" subtitle:nil payload:@"fullscreen"],
    ];
    RBListPopover *list = [[RBListPopover alloc] initWithSections:@[@{@"title": @"", @"items": items}]];
    __weak RBRootViewController *weakSelf = self;
    list.onSelect = ^(RBListItem *item) {
        [weakSelf dismissPopover];
        [weakSelf handlePageAction:item.payload];
    };
    [self presentListPopover:list fromButton:button];
}

- (void)handlePageAction:(NSString *)action {
    if ([action isEqualToString:@"reader"]) {
        self.readerPending = YES;
        [self.session sendMessage:@{@"t": @"reader"}];
        [self showToast:@"Preparing reader…"];
    } else if ([action isEqualToString:@"find"]) {
        self.findVisible = YES;
        [self.view setNeedsLayout];
        [self.view layoutIfNeeded];
        [self.findBar focusField];
    } else if ([action isEqualToString:@"paste"]) {
        [self pasteToPage];
    } else if ([action isEqualToString:@"copyurl"]) {
        NSString *url = [self.chromeBar.omnibox currentText];
        [UIPasteboard generalPasteboard].string = url ?: @"";
        [self showToast:@"URL copied"];
    } else if ([action isEqualToString:@"maillink"]) {
        [self mailCurrentPage];
    } else if ([action isEqualToString:@"fullscreen"]) {
        [self toggleFullscreen];
    }
}

- (void)mailCurrentPage {
    if (![MFMailComposeViewController canSendMail]) {
        [self showToast:@"Mail is not set up on this iPad"];
        return;
    }
    MFMailComposeViewController *mail = [[MFMailComposeViewController alloc] init];
    mail.mailComposeDelegate = self;
    NSString *url = [self.chromeBar.omnibox currentText] ?: @"";
    NSString *title = url;
    for (NSDictionary *tab in self.lastTabs) {
        if ([tab isKindOfClass:[NSDictionary class]] && [[tab objectForKey:@"active"] boolValue]) {
            NSString *t = [tab objectForKey:@"title"];
            if ([t length]) title = t;
            break;
        }
    }
    [mail setSubject:title];
    [mail setMessageBody:url isHTML:NO];
    [self presentViewController:mail animated:YES completion:nil];
}

- (void)mailComposeController:(MFMailComposeViewController *)controller
          didFinishWithResult:(MFMailComposeResult)result error:(NSError *)error {
    [self dismissViewControllerAnimated:YES completion:nil];
}

// Library (book button): History | Bookmarks | Downloads in one surface.
- (void)chrome:(RBChromeBar *)bar libraryFromButton:(UIButton *)button {
    [self presentLibrary];
}

// Settings (gear): straight to settings — there is no menu.
- (void)chromeSettings:(RBChromeBar *)bar {
    [self presentSettingsAllowingCancel:YES message:nil];
}

- (void)pasteToPage {
    NSString *text = [UIPasteboard generalPasteboard].string;
    if (![text length]) {
        [self showToast:@"Clipboard is empty"];
        return;
    }
    [self.session sendMessage:@{@"t": @"paste", @"text": text}];
    [self showToast:@"Pasted to page"];
}

- (void)toggleFullscreen {
    self.fullscreen = !self.fullscreen;
    [self.view setNeedsLayout];
}

// ---------------------------------------------------------------- omnibox

- (void)omnibox:(RBOmnibox *)omnibox navigateTo:(NSString *)text {
    [self.suggestPanel hide];
    [self.session sendMessage:@{@"t": @"nav", @"url": text}];
}

- (void)omnibox:(RBOmnibox *)omnibox textChanged:(NSString *)text {
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(fireSuggest) object:nil];
    if (![text length]) {
        [self.suggestPanel hide];
        return;
    }
    [self performSelector:@selector(fireSuggest) withObject:nil afterDelay:0.25];
}

- (void)fireSuggest {
    NSString *text = [self.chromeBar.omnibox currentText];
    if (!self.chromeBar.omnibox.editing || ![text length]) return;
    [self.session sendMessage:@{@"t": @"suggest", @"q": text}];
}

- (void)omniboxEditingBegan:(RBOmnibox *)omnibox {}

- (void)omniboxEditingEnded:(RBOmnibox *)omnibox {
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(fireSuggest) object:nil];
    [self.suggestPanel hide];
}

- (void)omniboxStarTapped:(RBOmnibox *)omnibox {
    [self.session sendMessage:@{@"t": @"bookmark"}];
}

- (void)omniboxReloadOrStopTapped:(RBOmnibox *)omnibox {
    [self.session sendMessage:@{@"t": self.loading ? @"stop" : @"reload"}];
}

- (void)suggestPanel:(RBSuggestPanel *)panel pickedURL:(NSString *)url {
    [self.chromeBar.omnibox dismissKeyboard];
    [self.suggestPanel hide];
    [self.session sendMessage:@{@"t": @"nav", @"url": url}];
}

// -------------------------------------------------------------- tab strip

- (void)tabStrip:(RBTabStrip *)strip selectTab:(NSInteger)tabID {
    [self.session sendMessage:@{@"t": @"tab", @"action": @"select", @"id": [NSNumber numberWithInteger:tabID]}];
}

- (void)tabStrip:(RBTabStrip *)strip closeTab:(NSInteger)tabID {
    [self.session sendMessage:@{@"t": @"tab", @"action": @"close", @"id": [NSNumber numberWithInteger:tabID]}];
}

- (void)tabStripNewTab:(RBTabStrip *)strip {
    [self.session sendMessage:@{@"t": @"tab", @"action": @"new"}];
}

// --------------------------------------------------------------- find bar

- (void)findBar:(RBFindBar *)bar search:(NSString *)query direction:(NSInteger)direction {
    [self.session sendMessage:@{@"t": @"find", @"q": query, @"dir": [NSNumber numberWithInteger:direction]}];
}

- (void)findBarDone:(RBFindBar *)bar {
    self.findVisible = NO;
    [self.view setNeedsLayout];
}

// --------------------------------------------------------------- popovers

- (void)presentListPopover:(RBListPopover *)list fromButton:(UIButton *)button {
    [self dismissPopover];
    list.contentSizeForViewInPopover = [list preferredSize];
    UIPopoverController *popover = [[UIPopoverController alloc] initWithContentViewController:list];
    popover.delegate = self;
    self.popover = popover;
    CGRect anchor = [button convertRect:button.bounds toView:self.view];
    [popover presentPopoverFromRect:anchor inView:self.view
           permittedArrowDirections:UIPopoverArrowDirectionUp animated:YES];
}

- (void)dismissPopover {
    if (self.popover.popoverVisible) [self.popover dismissPopoverAnimated:NO];
    self.popover = nil;
}

- (void)popoverControllerDidDismissPopover:(UIPopoverController *)popoverController {
    if (popoverController == self.popover) self.popover = nil;
    if (popoverController == self.uploadPopover) {
        // Swiped away without picking: cancel the pending server chooser.
        self.uploadPopover = nil;
        if (self.chooserPending) {
            self.chooserPending = NO;
            [self postUploadData:nil filename:nil];
        }
    }
}

// ---------------------------------------------------------------- library

- (void)presentLibrary {
    RBLibraryController *library = [[RBLibraryController alloc] init];
    __weak RBRootViewController *weakSelf = self;
    library.onRequestHistoryPage = ^(NSString *query, NSInteger offset) {
        [weakSelf.session sendMessage:@{@"t": @"history", @"q": query ?: @"",
                                        @"offset": [NSNumber numberWithInteger:offset]}];
    };
    library.onDeleteHistory = ^(NSDictionary *entry) {
        [weakSelf.session sendMessage:@{@"t": @"histdel",
                                        @"url": [entry objectForKey:@"url"] ?: @"",
                                        @"ts": [entry objectForKey:@"ts"] ?: [NSNumber numberWithInt:0]}];
    };
    library.onClearHistory = ^{
        [weakSelf.session sendMessage:@{@"t": @"clear", @"what": @"history"}];
    };
    library.onDeleteBookmark = ^(NSString *url) {
        [weakSelf.session sendMessage:@{@"t": @"bmdel", @"url": url ?: @""}];
    };
    library.onOpenDownload = ^(NSString *name) {
        [weakSelf openDownloadNamed:name];
    };
    library.onDeleteDownload = ^(NSString *name) {
        [weakSelf.session sendMessage:@{@"t": @"dldel", @"name": name ?: @""}];
    };
    library.onPick = ^(NSString *url) {
        [weakSelf dismissViewControllerAnimated:YES completion:nil];
        weakSelf.libraryController = nil;
        [weakSelf.session sendMessage:@{@"t": @"nav", @"url": url}];
    };
    library.onNeedsData = ^(NSString *kind) {
        if ([kind isEqualToString:@"bookmarks"]) [weakSelf.session sendMessage:@{@"t": @"hist"}];
        else if ([kind isEqualToString:@"downloads"]) [weakSelf.session sendMessage:@{@"t": @"downloads"}];
    };
    self.libraryController = library;
    UINavigationController *nav = [[UINavigationController alloc] initWithRootViewController:library];
    nav.modalPresentationStyle = UIModalPresentationFormSheet;
    [self presentViewController:nav animated:YES completion:nil];
    // Prefetch the other two tabs so switching is instant.
    [self.session sendMessage:@{@"t": @"hist"}];
    [self.session sendMessage:@{@"t": @"downloads"}];
}

// Fetch the file (auth cookie rides along automatically) into a stable,
// browsable folder (not NSTemporaryDirectory() — this is an unsandboxed
// jailbreak "System" app with no container of its own, and the OS's Open-In
// hand-off to a real sandboxed app needs a real path it can copy from, not
// scratch space that can vanish under memory pressure), then offer the
// system "Open in…" menu — the loop iOS 6 Safari never closed.
- (void)openDownloadNamed:(NSString *)name {
    if (![name length]) return;
    NSString *escaped = [name stringByAddingPercentEscapesUsingEncoding:NSUTF8StringEncoding];
    NSURL *url = [NSURL URLWithString:[@"/downloads/" stringByAppendingString:escaped] relativeToURL:self.session.baseURL];
    if (!url) return;
    [self showToast:[NSString stringWithFormat:@"Fetching %@…", name]];
    RBLog(@"download fetch GET %@", [url absoluteString]);
    NSURLRequest *request = [NSURLRequest requestWithURL:url cachePolicy:NSURLRequestReloadIgnoringLocalCacheData timeoutInterval:120.0];
    [NSURLConnection sendAsynchronousRequest:request queue:[NSOperationQueue mainQueue]
                           completionHandler:^(NSURLResponse *response, NSData *data, NSError *error) {
        NSInteger status = [response isKindOfClass:[NSHTTPURLResponse class]] ? [(NSHTTPURLResponse *)response statusCode] : 0;
        RBLog(@"download fetch result status=%d bytes=%d err=%@", (int)status, (int)[data length], [error localizedDescription] ?: @"");
        if (error || status >= 400 || ![data length]) {
            [self showToast:@"Download failed"];
            return;
        }
        NSString *dir = [RBLogDirectory stringByAppendingPathComponent:@"Downloads"];
        NSError *dirError = nil;
        if (![[NSFileManager defaultManager] createDirectoryAtPath:dir withIntermediateDirectories:YES attributes:nil error:&dirError]) {
            RBLog(@"download open: mkdir %@ failed: %@", dir, [dirError localizedDescription]);
        }
        NSString *path = [dir stringByAppendingPathComponent:name];
        if (![data writeToFile:path atomically:YES]) {
            RBLog(@"download fetch: writeToFile failed for %@", path);
            [self showToast:@"Could not save file"];
            return;
        }
        self.docController = [UIDocumentInteractionController interactionControllerWithURL:[NSURL fileURLWithPath:path]];
        self.docController.delegate = self;
        RBLog(@"download open: path=%@ UTI=%@ apps=%d", path, self.docController.UTI, (int)[self.docController.gestureRecognizers count]);
        // Present over the Library if it's up, else from the library button.
        UIView *host = self.presentedViewController ? self.presentedViewController.view : self.view;
        CGRect anchor = self.presentedViewController
            ? CGRectMake(host.bounds.size.width / 2.0 - 22.0, 40.0, 44.0, 44.0)
            : [self.chromeBar.libraryButton convertRect:self.chromeBar.libraryButton.bounds toView:self.view];
        BOOL presented = [self.docController presentOpenInMenuFromRect:anchor inView:host animated:YES];
        RBLog(@"download open: presentOpenInMenu -> %d", presented);
        if (!presented) {
            [self showToast:@"No app can open this file"];
        }
    }];
}

// These three are optional on the delegate and Apple only calls them if the
// OS actually attempts the hand-off — logging them tells us whether tapping
// an app in the "Open In" list ever reaches the OS at all, or dies earlier
// (e.g. in the popover itself).
- (void)documentInteractionController:(UIDocumentInteractionController *)controller willBeginSendingToApplication:(NSString *)application {
    RBLog(@"download open: willBeginSendingToApplication=%@", application ?: @"(nil)");
}

- (void)documentInteractionController:(UIDocumentInteractionController *)controller didEndSendingToApplication:(NSString *)application {
    RBLog(@"download open: didEndSendingToApplication=%@", application ?: @"(nil)");
}

- (void)documentInteractionControllerDidDismissOpenInMenu:(UIDocumentInteractionController *)controller {
    RBLog(@"download open: OpenInMenu dismissed");
}

// ----------------------------------------------------------- keyboard shim

- (void)showKeyboard {
    self.hiddenInput.text = @" ";
    [self.hiddenInput becomeFirstResponder];
    [self updateKeyboardAvoidance];
}

- (void)hidePageKeyboard {
    if (![self.hiddenInput isFirstResponder]) return;
    self.editableHasRect = NO;
    [self.hiddenInput resignFirstResponder];
    [self updateKeyboardAvoidance];
}

// configureKeyboardForKind maps the server's editable kind onto the shadow
// field: right keyboard layout, secure entry for passwords, and remembers
// the focused element's rect (viewport fractions) for keyboard avoidance.
- (void)configureKeyboardForKind:(NSString *)kind rect:(id)rectValue {
    UIKeyboardType type = UIKeyboardTypeDefault;
    BOOL secure = NO;
    if ([kind isEqualToString:@"password"]) secure = YES;
    else if ([kind isEqualToString:@"email"]) type = UIKeyboardTypeEmailAddress;
    else if ([kind isEqualToString:@"number"]) type = UIKeyboardTypeNumbersAndPunctuation;
    else if ([kind isEqualToString:@"url"]) type = UIKeyboardTypeURL;

    if (self.hiddenInput.keyboardType != type || self.hiddenInput.secureTextEntry != secure) {
        BOOL wasFirst = [self.hiddenInput isFirstResponder];
        if (wasFirst) [self.hiddenInput resignFirstResponder];
        self.hiddenInput.keyboardType = type;
        self.hiddenInput.secureTextEntry = secure;
        if (wasFirst) [self.hiddenInput becomeFirstResponder];
    }

    NSArray *rect = [rectValue isKindOfClass:[NSArray class]] ? rectValue : nil;
    if ([rect count] == 4) {
        self.editableRect = CGRectMake([[rect objectAtIndex:0] floatValue], [[rect objectAtIndex:1] floatValue],
                                       [[rect objectAtIndex:2] floatValue], [[rect objectAtIndex:3] floatValue]);
        self.editableHasRect = YES;
    } else {
        self.editableHasRect = NO;
    }
}

// ---- keyboard avoidance: slide the stream up when the keyboard would cover
// the focused field. Visual-only (a transform); input math is unaffected
// because gesture coordinates are taken in the stream view's own space.

- (void)keyboardWillShow:(NSNotification *)note {
    NSValue *frameValue = [[note userInfo] objectForKey:UIKeyboardFrameEndUserInfoKey];
    CGRect kf = [self.view convertRect:[frameValue CGRectValue] fromView:nil];
    self.keyboardTop = kf.origin.y;
    self.keyboardVisible = YES;
    [self updateKeyboardAvoidance];
}

- (void)keyboardWillHide:(NSNotification *)note {
    self.keyboardVisible = NO;
    [self updateKeyboardAvoidance];
}

- (void)updateKeyboardAvoidance {
    CGFloat shift = 0.0;
    if (self.keyboardVisible && self.editableHasRect && [self.hiddenInput isFirstResponder]) {
        CGSize s = self.streamView.bounds.size;
        // center is unaffected by the transform, so this is the unshifted top.
        CGFloat streamTop = self.streamView.center.y - s.height / 2.0;
        CGFloat fieldBottom = streamTop + (self.editableRect.origin.y + self.editableRect.size.height) * s.height;
        CGFloat limit = self.keyboardTop - 12.0;
        if (fieldBottom > limit) shift = MIN(fieldBottom - limit, s.height * 0.6);
    }
    if (shift == self.keyboardShift) return;
    self.keyboardShift = shift;
    [UIView animateWithDuration:0.22 animations:^{ [self applyStreamTransform]; }];
}

- (void)applyStreamTransform {
    self.streamView.transform = CGAffineTransformMakeTranslation(0.0, -self.keyboardShift);
}

- (BOOL)textFieldShouldReturn:(UITextField *)textField {
    if (textField == self.hiddenInput) {
        [self sendKeyName:@"Enter" keyCode:13];
        [self hidePageKeyboard];
        return NO;
    }
    return YES;
}

- (BOOL)textField:(UITextField *)textField shouldChangeCharactersInRange:(NSRange)range replacementString:(NSString *)string {
    if (textField != self.hiddenInput) return YES;
    if ([string length] == 0) {
        [self sendKeyName:@"Backspace" keyCode:8];
    } else if ([string length] > 1) {
        [self.session sendMessage:@{@"t": @"paste", @"text": string}];
    } else {
        [self.session sendMessage:@{@"t": @"key", @"text": string}];
    }
    self.hiddenInput.text = @" ";
    return NO;
}

- (void)sendKeyName:(NSString *)name keyCode:(NSInteger)keyCode {
    [self.session sendMessage:@{@"t": @"key", @"down": @YES, @"key": name, @"code": name, @"keyCode": [NSNumber numberWithInteger:keyCode]}];
    [self.session sendMessage:@{@"t": @"key", @"down": @NO, @"key": name, @"code": name, @"keyCode": [NSNumber numberWithInteger:keyCode]}];
}

// ---------------------------------------------------------- JS dialogs (M2.1)

- (void)showDialogWithKind:(NSString *)kind text:(NSString *)text defaultText:(NSString *)def {
    [self dismissDialogSilently];
    self.dialogKind = kind ?: @"alert";
    NSString *host = [self.session.baseURL host] ?: @"page";
    UIAlertView *alert;
    if ([self.dialogKind isEqualToString:@"prompt"]) {
        alert = [[UIAlertView alloc] initWithTitle:host message:text ?: @"" delegate:self
                                 cancelButtonTitle:@"Cancel" otherButtonTitles:@"OK", nil];
        alert.alertViewStyle = UIAlertViewStylePlainTextInput;
        [alert textFieldAtIndex:0].text = def ?: @"";
    } else if ([self.dialogKind isEqualToString:@"confirm"]) {
        alert = [[UIAlertView alloc] initWithTitle:host message:text ?: @"" delegate:self
                                 cancelButtonTitle:@"Cancel" otherButtonTitles:@"OK", nil];
    } else {
        alert = [[UIAlertView alloc] initWithTitle:host message:text ?: @"" delegate:self
                                 cancelButtonTitle:nil otherButtonTitles:@"OK", nil];
    }
    self.dialogAlert = alert;
    [alert show];
}

- (void)dismissDialogSilently {
    if (!self.dialogAlert) return;
    self.dialogSuppressReply = YES;
    [self.dialogAlert dismissWithClickedButtonIndex:self.dialogAlert.cancelButtonIndex animated:NO];
    self.dialogAlert = nil;
}

- (void)alertView:(UIAlertView *)alertView clickedButtonAtIndex:(NSInteger)buttonIndex {
    if (alertView == self.pasteboardAlert) {
        self.pasteboardAlert = nil;
        if (buttonIndex != alertView.cancelButtonIndex && [self.pasteboardURL length]) {
            [self openURLString:self.pasteboardURL];
        }
        return;
    }
    if (alertView != self.dialogAlert) return;
    self.dialogAlert = nil;
    if (self.dialogSuppressReply) {
        self.dialogSuppressReply = NO;
        return;
    }
    BOOL accept = alertView.cancelButtonIndex < 0 || buttonIndex != alertView.cancelButtonIndex;
    NSMutableDictionary *reply = [NSMutableDictionary dictionaryWithObjectsAndKeys:
                                  @"dialogreply", @"t", [NSNumber numberWithBool:accept], @"accept", nil];
    if (accept && [self.dialogKind isEqualToString:@"prompt"]) {
        NSString *text = [alertView textFieldAtIndex:0].text ?: @"";
        [reply setObject:text forKey:@"text"];
    }
    [self.session sendMessage:reply];
}

// ------------------------------------------------------------- uploads (M2.2)

- (void)presentUploadPicker {
    self.chooserPending = YES;
    UIImagePickerController *picker = [[UIImagePickerController alloc] init];
    picker.sourceType = UIImagePickerControllerSourceTypePhotoLibrary;
    picker.delegate = self;
    // iPad rule: the photo library picker must live in a popover.
    UIPopoverController *popover = [[UIPopoverController alloc] initWithContentViewController:picker];
    popover.delegate = self;
    self.uploadPopover = popover;
    CGRect anchor = [self.chromeBar.actionButton convertRect:self.chromeBar.actionButton.bounds toView:self.view];
    [popover presentPopoverFromRect:anchor inView:self.view
           permittedArrowDirections:UIPopoverArrowDirectionUp animated:YES];
    [self showToast:@"Pick a photo to upload"];
}

- (void)dismissUploadPopover {
    if (self.uploadPopover.popoverVisible) [self.uploadPopover dismissPopoverAnimated:YES];
    self.uploadPopover = nil;
}

- (void)imagePickerController:(UIImagePickerController *)picker
didFinishPickingMediaWithInfo:(NSDictionary *)info {
    [self dismissUploadPopover];
    if (!self.chooserPending) return;
    self.chooserPending = NO;
    UIImage *image = [info objectForKey:UIImagePickerControllerOriginalImage];
    NSData *jpeg = image ? UIImageJPEGRepresentation(image, 0.9) : nil;
    if (![jpeg length]) {
        [self postUploadData:nil filename:nil]; // cancel server-side
        return;
    }
    [self showToast:@"Uploading photo…"];
    [self postUploadData:jpeg filename:@"photo.jpg"];
}

- (void)imagePickerControllerDidCancel:(UIImagePickerController *)picker {
    [self dismissUploadPopover];
    if (!self.chooserPending) return;
    self.chooserPending = NO;
    [self postUploadData:nil filename:nil]; // clears the pending chooser
}

// postUploadData ships one file (or nothing = cancel) to POST /upload; the
// auth cookie from login rides along in the shared cookie jar.
- (void)postUploadData:(NSData *)data filename:(NSString *)filename {
    NSURL *url = [NSURL URLWithString:@"/upload" relativeToURL:self.session.baseURL];
    if (!url) return;
    NSString *boundary = [NSString stringWithFormat:@"rbsurf-%08x", arc4random()];
    NSMutableData *body = [NSMutableData data];
    if ([data length] && [filename length]) {
        [body appendData:[[NSString stringWithFormat:@"--%@\r\n", boundary] dataUsingEncoding:NSUTF8StringEncoding]];
        [body appendData:[[NSString stringWithFormat:
                           @"Content-Disposition: form-data; name=\"file\"; filename=\"%@\"\r\n"
                           @"Content-Type: application/octet-stream\r\n\r\n", filename]
                          dataUsingEncoding:NSUTF8StringEncoding]];
        [body appendData:data];
        [body appendData:[@"\r\n" dataUsingEncoding:NSUTF8StringEncoding]];
    }
    [body appendData:[[NSString stringWithFormat:@"--%@--\r\n", boundary] dataUsingEncoding:NSUTF8StringEncoding]];

    NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:url
                                                           cachePolicy:NSURLRequestReloadIgnoringLocalCacheData
                                                       timeoutInterval:180.0];
    request.HTTPMethod = @"POST";
    [request setValue:[NSString stringWithFormat:@"multipart/form-data; boundary=%@", boundary]
   forHTTPHeaderField:@"Content-Type"];
    request.HTTPBody = body;
    BOOL wasCancel = ![data length];
    [NSURLConnection sendAsynchronousRequest:request queue:[NSOperationQueue mainQueue]
                           completionHandler:^(NSURLResponse *response, NSData *rdata, NSError *error) {
        if (wasCancel) return;
        NSInteger status = [response isKindOfClass:[NSHTTPURLResponse class]] ? [(NSHTTPURLResponse *)response statusCode] : 0;
        if (error || status >= 400) {
            RBLog(@"upload failed status=%d err=%@", (int)status, error);
            [self showToast:@"Upload failed"];
        } else {
            [self showToast:@"Photo attached"];
        }
    }];
}

// ---------------------------------------------------- link context menu (M2.4)

- (void)presentLinkSheet {
    NSString *href = [self.lastLinkInfo objectForKey:@"href"];
    NSString *img = [self.lastLinkInfo objectForKey:@"img"];
    NSString *text = [self.lastLinkInfo objectForKey:@"text"];
    NSMutableArray *actions = [NSMutableArray array];
    UIActionSheet *sheet = [[UIActionSheet alloc] init];
    sheet.delegate = self;
    sheet.title = [text length] ? text : ([href length] ? href : img);
    if ([href length]) {
        [sheet addButtonWithTitle:@"Open"];
        [actions addObject:@"open"];
        [sheet addButtonWithTitle:@"Open in New Tab"];
        [actions addObject:@"newtab"];
        [sheet addButtonWithTitle:@"Copy Link"];
        [actions addObject:@"copy"];
    }
    if ([img length]) {
        [sheet addButtonWithTitle:@"Save Image"];
        [actions addObject:@"saveimg"];
    }
    sheet.cancelButtonIndex = [sheet addButtonWithTitle:@"Cancel"];
    [actions addObject:@"cancel"];
    self.linkSheetActions = actions;
    self.linkSheet = sheet;
    CGRect r = CGRectMake(self.longPressStart.x - 2.0, self.longPressStart.y - 2.0, 4.0, 4.0);
    [sheet showFromRect:r inView:self.streamView animated:YES];
}

- (void)actionSheet:(UIActionSheet *)actionSheet clickedButtonAtIndex:(NSInteger)buttonIndex {
    if (actionSheet != self.linkSheet) return;
    self.linkSheet = nil;
    if (buttonIndex < 0 || buttonIndex >= (NSInteger)[self.linkSheetActions count]) return;
    NSString *action = [self.linkSheetActions objectAtIndex:(NSUInteger)buttonIndex];
    NSString *href = [self.lastLinkInfo objectForKey:@"href"];
    NSString *img = [self.lastLinkInfo objectForKey:@"img"];
    if ([action isEqualToString:@"open"]) {
        [self.session sendMessage:@{@"t": @"nav", @"url": href ?: @""}];
    } else if ([action isEqualToString:@"newtab"]) {
        [self.session sendMessage:@{@"t": @"opennew", @"url": href ?: @""}];
    } else if ([action isEqualToString:@"copy"]) {
        [UIPasteboard generalPasteboard].string = href ?: @"";
        [self showToast:@"Link copied"];
    } else if ([action isEqualToString:@"saveimg"]) {
        [self saveImageFromURL:img];
    }
}

- (void)saveImageFromURL:(NSString *)urlString {
    NSURL *url = [NSURL URLWithString:urlString ?: @""];
    if (!url) return;
    [self showToast:@"Saving image…"];
    NSURLRequest *request = [NSURLRequest requestWithURL:url cachePolicy:NSURLRequestReloadIgnoringLocalCacheData timeoutInterval:60.0];
    [NSURLConnection sendAsynchronousRequest:request queue:[NSOperationQueue mainQueue]
                           completionHandler:^(NSURLResponse *response, NSData *data, NSError *error) {
        UIImage *image = [data length] ? [UIImage imageWithData:data] : nil;
        if (!image) {
            [self showToast:@"Could not load image"];
            return;
        }
        UIImageWriteToSavedPhotosAlbum(image, nil, NULL, NULL);
        [self showToast:@"Saved to Photos"];
    }];
}

// ---------------------------------------------------------- error card (M2.5)

- (void)showErrorCardForURL:(NSString *)url {
    if (!self.errorCard) {
        UIView *card = [[UIView alloc] initWithFrame:CGRectZero];
        card.backgroundColor = [UIColor colorWithWhite:0.13 alpha:0.96];
        card.layer.cornerRadius = 12.0;
        UILabel *label = [[UILabel alloc] initWithFrame:CGRectZero];
        label.backgroundColor = [UIColor clearColor];
        label.textColor = [UIColor colorWithWhite:0.95 alpha:1.0];
        label.font = [RBTheme fontOfSize:15.0 bold:NO];
        label.numberOfLines = 3;
        label.textAlignment = NSTextAlignmentCenter;
        [card addSubview:label];
        self.errorCardLabel = label;
        UIButton *retry = [UIButton buttonWithType:UIButtonTypeCustom];
        retry.tag = 1;
        retry.backgroundColor = [UIColor colorWithRed:0.28 green:0.42 blue:0.62 alpha:1.0];
        retry.layer.cornerRadius = 8.0;
        retry.titleLabel.font = [RBTheme fontOfSize:16.0 bold:YES];
        [retry setTitle:@"Try Again" forState:UIControlStateNormal];
        [retry addTarget:self action:@selector(errorRetryTapped:) forControlEvents:UIControlEventTouchUpInside];
        [card addSubview:retry];
        UIButton *dismiss = [UIButton buttonWithType:UIButtonTypeCustom];
        dismiss.tag = 2;
        dismiss.titleLabel.font = [RBTheme fontOfSize:14.0 bold:NO];
        [dismiss setTitle:@"Dismiss" forState:UIControlStateNormal];
        [dismiss setTitleColor:[UIColor colorWithWhite:0.7 alpha:1.0] forState:UIControlStateNormal];
        [dismiss addTarget:self action:@selector(hideErrorCard) forControlEvents:UIControlEventTouchUpInside];
        [card addSubview:dismiss];
        self.errorCard = card;
        [self.view addSubview:card];
    }
    self.errorCardLabel.text = [NSString stringWithFormat:@"Couldn't load\n%@", url ?: @"this page"];
    CGFloat w = 320.0;
    CGFloat x = (self.view.bounds.size.width - w) / 2.0;
    self.errorCard.frame = CGRectMake(x, self.view.bounds.size.height * 0.3, w, 170.0);
    self.errorCardLabel.frame = CGRectMake(16.0, 14.0, w - 32.0, 62.0);
    [self.errorCard viewWithTag:1].frame = CGRectMake(40.0, 86.0, w - 80.0, 40.0);
    [self.errorCard viewWithTag:2].frame = CGRectMake(40.0, 132.0, w - 80.0, 28.0);
    self.errorCard.hidden = NO;
    [self.view bringSubviewToFront:self.errorCard];
}

- (void)errorRetryTapped:(id)sender {
    [self hideErrorCard];
    [self.session sendMessage:@{@"t": @"reload"}];
}

- (void)hideErrorCard {
    self.errorCard.hidden = YES;
}

// ------------------------------------------------------------ reader (M1.5)

- (void)handleReaderReply:(NSDictionary *)message {
    if (!self.readerPending) return;
    self.readerPending = NO;
    if (![[message objectForKey:@"ok"] boolValue]) {
        [self showToast:@"No article found on this page"];
        return;
    }
    // Park the video lane while reading: local rendering needs no stream.
    self.readerResumeVideo = self.videoActive || self.videoRequested;
    if (self.readerResumeVideo) {
        [self.session sendMessage:@{@"t": @"video", @"on": @NO}];
        [self leaveVideoMode];
    }
    RBReaderController *reader = [[RBReaderController alloc]
                                  initWithTitle:[message objectForKey:@"title"]
                                           html:[message objectForKey:@"html"]
                                            url:[message objectForKey:@"url"]];
    __weak RBRootViewController *weakSelf = self;
    reader.onDismiss = ^{
        if (weakSelf.readerResumeVideo) {
            weakSelf.readerResumeVideo = NO;
            [weakSelf performSelector:@selector(maybeEnableVideo) withObject:nil afterDelay:0.2];
        }
    };
    [self presentViewController:reader animated:YES completion:nil];
}

- (void)readerNavigate:(NSNotification *)note {
    NSString *url = [note.object isKindOfClass:[NSString class]] ? note.object : nil;
    if ([url length]) [self.session sendMessage:@{@"t": @"nav", @"url": url}];
}

// ---------------------------------------- device integration (M4.1 / M4.2)

- (void)openURLString:(NSString *)url {
    if (![url length]) return;
    [self.session sendMessage:@{@"t": @"nav", @"url": url}];
}

- (void)checkPasteboard {
    NSString *text = [UIPasteboard generalPasteboard].string;
    if (![text hasPrefix:@"http://"] && ![text hasPrefix:@"https://"]) return;
    NSString *last = [[NSUserDefaults standardUserDefaults] stringForKey:RBDefaultsLastPasteboardKey];
    if ([text isEqualToString:last]) return;
    [[NSUserDefaults standardUserDefaults] setObject:text forKey:RBDefaultsLastPasteboardKey];
    [[NSUserDefaults standardUserDefaults] synchronize];
    if (self.session.state != RBSessionStateOpen) return;
    self.pasteboardURL = text;
    NSString *shown = [text length] > 96 ? [[text substringToIndex:96] stringByAppendingString:@"…"] : text;
    self.pasteboardAlert = [[UIAlertView alloc] initWithTitle:@"Open copied link?" message:shown
                                                     delegate:self cancelButtonTitle:@"Not Now"
                                            otherButtonTitles:@"Open", nil];
    [self.pasteboardAlert show];
}

// ------------------------------------------------------------------ toasts

- (void)showToast:(NSString *)text {
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(hideToast) object:nil];
    self.toastLabel.text = text ?: @"";
    [self.view bringSubviewToFront:self.toastLabel];
    [UIView animateWithDuration:0.15 animations:^{ self.toastLabel.alpha = 1.0; }];
    [self performSelector:@selector(hideToast) withObject:nil afterDelay:1.9];
}

- (void)hideToast {
    [UIView animateWithDuration:0.35 animations:^{ self.toastLabel.alpha = 0.0; }];
}

// --------------------------------------------------------- debug + watchdog

- (void)setDebugVisible:(BOOL)debugVisible {
    _debugVisible = debugVisible;
    self.debugLabel.hidden = !debugVisible;
    if (debugVisible) {
        [self refreshDebugOverlayWithAge:(self.lastFrameAt > 0.0 ? CACurrentMediaTime() - self.lastFrameAt : 0.0)];
        [self.view bringSubviewToFront:self.debugLabel];
    }
}

- (void)toggleDebug:(UITapGestureRecognizer *)tap {
    if (tap.state != UIGestureRecognizerStateEnded) return;
    [self setDebugVisible:!self.debugVisible];
}

- (void)refreshDebugOverlayWithAge:(double)age {
    if (!self.debugVisible) return;
    NSString *summary = self.debugSummary ?: @"warming up metrics";
    NSString *state = self.session.state == RBSessionStateOpen ? @"open" : (self.session.state == RBSessionStateConnecting ? @"connecting" : @"idle");
    NSString *pending = self.pendingFrame ? @"pending" : @"clear";
    self.debugLabel.text = [NSString stringWithFormat:@" %@ %@ %@\n %@\n view %.0fx%.0f age %.1fs rtt %.0fms %@",
                            RBNativeVersion,
                            state,
                            [self titleForStreamProfile:[self currentStreamProfile]],
                            summary,
                            self.streamView.bounds.size.width,
                            self.streamView.bounds.size.height,
                            age,
                            self.lastRTTMS,
                            pending];
}

- (void)watchdogTick:(NSTimer *)timer {
    double age = self.lastFrameAt > 0.0 ? CACurrentMediaTime() - self.lastFrameAt : 0.0;
    if (self.lastFrameAt > 0.0 && age > 1.5 && CACurrentMediaTime() - self.lastPokeAt > 1.0) {
        self.lastPokeAt = CACurrentMediaTime();
        [self.session sendMessage:@{@"t": @"poke"}];
    }
    // Latency echo (M1.1): only while the overlay is up; one in flight at a time.
    if (self.debugVisible && self.session.state == RBSessionStateOpen && self.latSentAt <= 0.0) {
        self.latSeq++;
        self.latSentAt = CACurrentMediaTime();
        [self.session sendMessage:@{@"t": @"lat", @"id": [NSNumber numberWithInteger:self.latSeq]}];
    }

    CFTimeInterval now = CACurrentMediaTime();
    if (self.lastPerfLogAt <= 0.0) self.lastPerfLogAt = now;
    if (now - self.lastPerfLogAt >= 5.0) {
        double dt = MAX(0.001, now - self.lastPerfLogAt);
        NSUInteger decoded = self.videoDecoder ? self.videoDecoder.decodedFrames : 0;
        NSUInteger errors = self.videoDecoder ? self.videoDecoder.decodeErrors : 0;
        NSUInteger submitted = self.videoDecoder ? self.videoDecoder.submittedAUs : 0;
        NSUInteger callbacks = self.videoDecoder ? self.videoDecoder.callbackFrames : 0;
        NSUInteger drops = self.videoDecoder ? self.videoDecoder.droppedAUs : 0;
        double fps = (self.framesDisplayed - self.perfLastFramesDisplayed) / dt;
        double rxps = (self.framesReceived - self.perfLastFramesReceived) / dt;
        double aups = (self.videoAUs - self.perfLastVideoAUs) / dt;
        double vtDone = (decoded - self.perfLastDecodedFrames) / dt;
        double vtSubmit = (submitted - self.perfLastVideoSubmitted) / dt;
        double vtCB = (callbacks - self.perfLastVideoCallbacks) / dt;
        NSUInteger dropDelta = drops - self.perfLastVideoDrops;
        NSUInteger errDelta = errors - self.perfLastDecodeErrors;
        int queued = self.videoDecoder ? self.videoDecoder.queuedAUs : 0;
        double vtCallMS = self.videoDecoder ? self.videoDecoder.averageSubmitMS : 0.0;
        double vtCBMS = self.videoDecoder ? self.videoDecoder.averageCallbackMS : 0.0;
        double wrapMS = self.videoDecoder ? self.videoDecoder.averageWrapMS : 0.0;
        self.debugSummary = [NSString stringWithFormat:@"%@ fps %.1f rx %.1f au %.1f vt %.1f/%.1f/%.1f q %d d+%u e+%u ms %.1f/%.1f/%.1f",
                             self.videoActive ? @"video" : @"jpeg",
                             fps,
                             rxps,
                             aups,
                             vtDone,
                             vtSubmit,
                             vtCB,
                             queued,
                             (unsigned)dropDelta,
                             (unsigned)errDelta,
                             vtCallMS,
                             vtCBMS,
                             wrapMS];
        RBLog(@"perf lane=%@ fps=%.1f rxps=%.1f aups=%.1f vt_done=%.1f/s vt_submit=%.1f/s vt_cb=%.1f/s q=%d drops+%u vt_call=%.2f/%.2fms vt_cb_ms=%.2f/%.2f wrap=%.2f/%.2fms jpeg=%.1f/%.1fms errs+%u age=%.2fs pending=%@ view=%.0fx%.0f",
              self.videoActive ? @"video" : @"jpeg",
              fps,
              rxps,
              aups,
              vtDone,
              vtSubmit,
              vtCB,
              queued,
              (unsigned)dropDelta,
              self.videoDecoder ? self.videoDecoder.lastSubmitMS : 0.0,
              self.videoDecoder ? self.videoDecoder.averageSubmitMS : 0.0,
              self.videoDecoder ? self.videoDecoder.lastCallbackMS : 0.0,
              self.videoDecoder ? self.videoDecoder.averageCallbackMS : 0.0,
              self.videoDecoder ? self.videoDecoder.lastWrapMS : 0.0,
              self.videoDecoder ? self.videoDecoder.averageWrapMS : 0.0,
              self.lastDecodeMS,
              self.averageDecodeMS,
              (unsigned)errDelta,
              age,
              self.pendingFrame ? @"yes" : @"no",
              self.streamView.bounds.size.width,
              self.streamView.bounds.size.height);
        self.lastPerfLogAt = now;
        self.perfLastFramesDisplayed = self.framesDisplayed;
        self.perfLastFramesReceived = self.framesReceived;
        self.perfLastVideoAUs = self.videoAUs;
        self.perfLastDecodedFrames = decoded;
        self.perfLastDecodeErrors = errors;
        self.perfLastVideoSubmitted = submitted;
        self.perfLastVideoCallbacks = callbacks;
        self.perfLastVideoDrops = drops;
    }
    [self refreshDebugOverlayWithAge:age];
}

@end
