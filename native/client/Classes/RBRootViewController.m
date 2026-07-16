#import "RBRootViewController.h"
#import "RBChromeBar.h"
#import "RBConfig.h"
#import "RBFindBar.h"
#import "RBListPopover.h"
#import "RBLog.h"
#import "RBOmnibox.h"
#import "RBProtocol.h"
#import "RBSession.h"
#import "RBSettingsController.h"
#import "RBStreamView.h"
#import "RBSuggestPanel.h"
#import "RBTabStrip.h"
#import "RBTheme.h"

#import <ImageIO/ImageIO.h>
#import <QuartzCore/QuartzCore.h>

#include <math.h>

static const CGFloat kRBTopBarHeight = 56.0;
static const CGFloat kRBTabStripHeight = 34.0;
static const CGFloat kRBFindBarHeight = 44.0;

static UIImage *RBDecodeJPEG(NSData *data);

@interface RBRootViewController () <UITextFieldDelegate, RBSessionDelegate, RBChromeBarDelegate,
                                    RBOmniboxDelegate, RBTabStripDelegate, RBSuggestPanelDelegate,
                                    RBFindBarDelegate, RBSettingsDelegate,
                                    UIDocumentInteractionControllerDelegate, UIPopoverControllerDelegate>
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
@property(nonatomic, assign) double lastDecodeMS;
@property(nonatomic, assign) double averageDecodeMS;
// Page state
@property(nonatomic, assign) BOOL loading;
@property(nonatomic, assign) BOOL fullscreen;
@property(nonatomic, assign) BOOL findVisible;
@property(nonatomic, assign) BOOL debugVisible;
@property(nonatomic, strong) NSArray *lastTabs;
// Popover routing: which async reply should open a popover when it arrives
@property(nonatomic, copy) NSString *pendingPopoverKind;
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
// Zoom
@property(nonatomic, assign) CGFloat serverZoom;
@property(nonatomic, assign) CGFloat pinchPreviewScale;
@property(nonatomic, assign) CGPoint pinchCentroid;
@property(nonatomic, assign) BOOL pinchActive;
@property(nonatomic, assign) BOOL zoomPreviewPending;
@end

@implementation RBRootViewController

// ---------------------------------------------------------------- lifecycle

- (void)viewDidLoad {
    [super viewDidLoad];
    self.view.backgroundColor = [UIColor blackColor];
    self.serverZoom = 1.0;

    self.streamView = [[RBStreamView alloc] initWithFrame:CGRectZero];
    [self.view addSubview:self.streamView];

    UITapGestureRecognizer *tripleTap = [[UITapGestureRecognizer alloc] initWithTarget:self action:@selector(toggleDebug:)];
    tripleTap.numberOfTapsRequired = 3;
    [self.streamView addGestureRecognizer:tripleTap];

    UITapGestureRecognizer *doubleTap = [[UITapGestureRecognizer alloc] initWithTarget:self action:@selector(doubleTapped:)];
    doubleTap.numberOfTapsRequired = 2;
    [doubleTap requireGestureRecognizerToFail:tripleTap];
    [self.streamView addGestureRecognizer:doubleTap];

    UITapGestureRecognizer *tap = [[UITapGestureRecognizer alloc] initWithTarget:self action:@selector(tapped:)];
    [tap requireGestureRecognizerToFail:doubleTap];
    [tap requireGestureRecognizerToFail:tripleTap];
    [self.streamView addGestureRecognizer:tap];

    UIPanGestureRecognizer *pan = [[UIPanGestureRecognizer alloc] initWithTarget:self action:@selector(panned:)];
    pan.maximumNumberOfTouches = 1; // two fingers are a pinch, never a scroll
    [self.streamView addGestureRecognizer:pan];

    UIPinchGestureRecognizer *pinch = [[UIPinchGestureRecognizer alloc] initWithTarget:self action:@selector(pinched:)];
    [self.streamView addGestureRecognizer:pinch];

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
    self.debugLabel.backgroundColor = [UIColor colorWithWhite:0.0 alpha:0.68];
    self.debugLabel.textColor = [UIColor colorWithRed:0.95 green:0.78 blue:0.32 alpha:1.0];
    self.debugLabel.numberOfLines = 0;
    self.debugLabel.font = [UIFont fontWithName:@"Courier" size:11.0] ?: [UIFont systemFontOfSize:11.0];
    self.debugLabel.hidden = YES;
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
    }

    // bounds+center instead of frame: the stream view may carry a pinch
    // preview transform, under which setFrame is undefined.
    CGFloat streamH = MAX(1.0, h - contentTop);
    self.streamView.bounds = CGRectMake(0.0, 0.0, w, streamH);
    self.streamView.center = CGPointMake(w / 2.0, contentTop + streamH / 2.0);

    CGRect omniboxFrame = [self.chromeBar convertRect:self.chromeBar.omnibox.frame toView:self.view];
    self.suggestPanel.frame = CGRectMake(omniboxFrame.origin.x, kRBTopBarHeight - 4.0,
                                         omniboxFrame.size.width, [self.suggestPanel desiredHeight]);

    self.restoreButton.frame = CGRectMake(w - 54.0, h - 54.0, 44.0, 44.0);
    self.toastLabel.frame = CGRectMake((w - 320.0) / 2.0, contentTop + 14.0, 320.0, 28.0);
    self.connectionPill.frame = CGRectMake(10.0, h - 34.0, 150.0, 22.0);
    self.debugLabel.frame = CGRectMake(10.0, contentTop + 8.0, MIN(420.0, w - 20.0), 96.0);
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
    CGSize s = self.streamView.bounds.size;
    [self.session updateViewportWidth:(NSInteger)(s.width + 0.5) height:(NSInteger)(s.height + 0.5)];
}

// ------------------------------------------------------------ connect flow

- (void)connectToURL:(NSString *)url password:(NSString *)password {
    self.pendingServerURL = url;
    self.pendingPassword = password;
    [self.session shutdown];
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
    NSString *password = [defaults stringForKey:RBDefaultsPasswordKey] ?: @"";
    RBSettingsController *settings = [[RBSettingsController alloc] initWithServerURL:url password:password];
    settings.delegate = self;
    settings.allowsCancel = allowsCancel;
    self.settingsController = settings;
    [self presentViewController:settings animated:YES completion:nil];
    if (message) [settings setStatusText:message isError:YES];
}

- (void)settings:(RBSettingsController *)settings connectToURL:(NSString *)url password:(NSString *)password {
    [self connectToURL:url password:password];
}

- (void)settingsDismissed:(RBSettingsController *)settings {
    self.settingsController = nil;
    [self dismissViewControllerAnimated:YES completion:nil];
}

- (void)sessionDidAuthenticate:(RBSession *)session {
    if (session != self.session) return;
    NSUserDefaults *defaults = [NSUserDefaults standardUserDefaults];
    [defaults setObject:self.pendingServerURL forKey:RBDefaultsServerURLKey];
    [defaults setObject:self.pendingPassword forKey:RBDefaultsPasswordKey];
    [defaults synchronize];
    if (self.settingsController) {
        [self.settingsController setStatusText:@"Connected" isError:NO];
        RBSettingsController *presented = self.settingsController;
        self.settingsController = nil;
        [presented.presentingViewController dismissViewControllerAnimated:YES completion:nil];
    }
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
            break;
        case RBSessionStateConnecting:
            self.connectionPill.hidden = NO;
            self.connectionPill.text = @"Connecting…";
            break;
        case RBSessionStateRetrying:
            self.connectionPill.hidden = NO;
            self.connectionPill.text = @"Reconnecting…";
            break;
        case RBSessionStateIdle:
            self.connectionPill.hidden = NO;
            self.connectionPill.text = @"Disconnected";
            break;
    }
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
    if (frame.type != 1) return;
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
                if (self.zoomPreviewPending && !self.pinchActive) {
                    // Clear the pinch preview in the same transaction the
                    // (zoomed) frame lands — the web client's double-jump fix.
                    self.streamView.transform = CGAffineTransformIdentity;
                    self.zoomPreviewPending = NO;
                }
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
    } else if ([t isEqualToString:@"histstate"]) {
        [self.chromeBar setCanGoBack:[[message objectForKey:@"back"] boolValue]
                             forward:[[message objectForKey:@"fwd"] boolValue]];
    } else if ([t isEqualToString:@"loading"]) {
        self.loading = [[message objectForKey:@"on"] boolValue];
        [self.chromeBar.omnibox setLoading:self.loading];
    } else if ([t isEqualToString:@"editable"]) {
        if ([[message objectForKey:@"on"] boolValue]) [self showKeyboard];
        else if ([self.hiddenInput isFirstResponder]) [self.hiddenInput resignFirstResponder];
    } else if ([t isEqualToString:@"copytext"]) {
        NSString *text = [message objectForKey:@"text"] ?: @"";
        if ([text length]) [self showCopyMenuForText:text];
        else [self showToast:@"No text selected"];
    } else if ([t isEqualToString:@"found"]) {
        [self.findBar setFound:[[message objectForKey:@"on"] boolValue]];
    } else if ([t isEqualToString:@"download"]) {
        [self showToast:[NSString stringWithFormat:@"Downloaded %@", [message objectForKey:@"name"] ?: @""]];
    } else if ([t isEqualToString:@"downloads"]) {
        if ([self.pendingPopoverKind isEqualToString:@"downloads"]) {
            self.pendingPopoverKind = nil;
            [self presentDownloadsPopover:[message objectForKey:@"items"]];
        }
    } else if ([t isEqualToString:@"tabs"]) {
        id tabs = [message objectForKey:@"tabs"];
        self.lastTabs = [tabs isKindOfClass:[NSArray class]] ? tabs : nil;
        [self.tabStrip setTabs:self.lastTabs baseURL:self.session.baseURL];
    } else if ([t isEqualToString:@"hist"]) {
        if ([self.pendingPopoverKind isEqualToString:@"hist"]) {
            self.pendingPopoverKind = nil;
            [self presentHistoryPopover:message];
        }
    } else if ([t isEqualToString:@"starred"]) {
        [self.chromeBar.omnibox setStarred:[[message objectForKey:@"on"] boolValue]];
    } else if ([t isEqualToString:@"zoom"]) {
        double scale = [[message objectForKey:@"scale"] doubleValue];
        self.serverZoom = scale > 0.0 ? (CGFloat)scale : 1.0;
    } else if ([t isEqualToString:@"suggest"]) {
        if (self.chromeBar.omnibox.editing) {
            [self.suggestPanel showItems:[message objectForKey:@"items"]];
            [self.view setNeedsLayout];
        }
    } else if ([t isEqualToString:@"toast"]) {
        [self showToast:[message objectForKey:@"text"] ?: @"OK"];
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
    [self stopInertia];
    [self hideCopyMenu];
    CGPoint f = [self fractionForPoint:[tap locationInView:self.streamView]];
    [self.session sendClickX:f.x y:f.y];
}

- (void)doubleTapped:(UITapGestureRecognizer *)tap {
    if (tap.state != UIGestureRecognizerStateEnded) return;
    [self stopInertia];
    CGPoint f = [self fractionForPoint:[tap locationInView:self.streamView]];
    CGFloat target = self.serverZoom > 1.05 ? 1.0 : 2.0;
    [self.session sendMessage:@{@"t": @"zoom",
                                @"scale": [NSNumber numberWithFloat:target],
                                @"cx": [NSNumber numberWithFloat:f.x],
                                @"cy": [NSNumber numberWithFloat:f.y]}];
}

- (void)panned:(UIPanGestureRecognizer *)pan {
    CGPoint p = [pan locationInView:self.streamView];
    if (pan.state == UIGestureRecognizerStateBegan) {
        [self stopInertia];
        self.panAnchor = [self fractionForPoint:p];
        self.inertiaAnchor = self.panAnchor;
        self.lastPanPoint = p;
        return;
    }
    if (pan.state == UIGestureRecognizerStateChanged) {
        CGSize s = self.streamView.bounds.size;
        CGFloat dx = -(p.x - self.lastPanPoint.x) / MAX(1.0, s.width);
        CGFloat dy = -(p.y - self.lastPanPoint.y) / MAX(1.0, s.height);
        self.lastPanPoint = p;
        [self.session sendWheelX:self.panAnchor.x y:self.panAnchor.y dx:dx dy:dy];
        CGPoint v = [pan velocityInView:self.streamView];
        self.inertiaVelocity = CGPointMake(-v.x, -v.y);
        return;
    }
    if (pan.state == UIGestureRecognizerStateEnded) {
        [self startInertiaIfNeeded];
    }
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

// Pinch: pure local preview while the fingers are down (transform about the
// centroid), then one absolute zoom message on release. The preview stays up
// until the next frame lands (see startNextDecodeIfNeeded).
- (void)pinched:(UIPinchGestureRecognizer *)pinch {
    if (pinch.state == UIGestureRecognizerStateBegan) {
        [self stopInertia];
        self.pinchActive = YES;
        self.pinchCentroid = [pinch locationInView:self.streamView];
        self.pinchPreviewScale = 1.0;
        return;
    }
    if (pinch.state == UIGestureRecognizerStateChanged) {
        CGFloat s = pinch.scale;
        CGFloat total = self.serverZoom * s;
        if (total < 0.85) s = 0.85 / self.serverZoom;
        if (total > 3.4) s = 3.4 / self.serverZoom;
        self.pinchPreviewScale = s;
        CGSize size = self.streamView.bounds.size;
        CGFloat dx = self.pinchCentroid.x - size.width / 2.0;
        CGFloat dy = self.pinchCentroid.y - size.height / 2.0;
        CGAffineTransform t = CGAffineTransformMakeTranslation(dx, dy);
        t = CGAffineTransformScale(t, s, s);
        t = CGAffineTransformTranslate(t, -dx, -dy);
        self.streamView.transform = t;
        return;
    }
    if (pinch.state == UIGestureRecognizerStateEnded || pinch.state == UIGestureRecognizerStateCancelled) {
        self.pinchActive = NO;
        CGFloat target = self.serverZoom * self.pinchPreviewScale;
        if (target < 1.05) target = 1.0;
        if (target > 3.0) target = 3.0;
        CGPoint f = [self fractionForPoint:self.pinchCentroid];
        [self.session sendMessage:@{@"t": @"zoom",
                                    @"scale": [NSNumber numberWithFloat:target],
                                    @"cx": [NSNumber numberWithFloat:f.x],
                                    @"cy": [NSNumber numberWithFloat:f.y]}];
        self.zoomPreviewPending = YES;
    }
}

- (void)longPressed:(UILongPressGestureRecognizer *)longPress {
    CGPoint p = [longPress locationInView:self.streamView];
    CGPoint f = [self fractionForPoint:p];
    if (longPress.state == UIGestureRecognizerStateBegan) {
        [self stopInertia];
        self.longPressStart = p;
        self.longPressMoved = NO;
        [self.session sendMessage:@{@"t": @"lpdown", @"x": [NSNumber numberWithFloat:f.x], @"y": [NSNumber numberWithFloat:f.y]}];
        return;
    }
    if (longPress.state == UIGestureRecognizerStateChanged) {
        if (fabsf(p.x - self.longPressStart.x) > 8.0 || fabsf(p.y - self.longPressStart.y) > 8.0) self.longPressMoved = YES;
        if (self.longPressMoved) [self.session sendMessage:@{@"t": @"lpmove", @"x": [NSNumber numberWithFloat:f.x], @"y": [NSNumber numberWithFloat:f.y]}];
        return;
    }
    if (longPress.state == UIGestureRecognizerStateEnded || longPress.state == UIGestureRecognizerStateCancelled || longPress.state == UIGestureRecognizerStateFailed) {
        [self.session sendMessage:@{@"t": @"lpup", @"x": [NSNumber numberWithFloat:f.x], @"y": [NSNumber numberWithFloat:f.y], @"sel": [NSNumber numberWithBool:!self.longPressMoved]}];
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

- (void)chrome:(RBChromeBar *)bar menuFromButton:(UIButton *)button {
    NSArray *items = @[
        [RBListItem itemWithTitle:@"Find on Page" subtitle:nil payload:@"find"],
        [RBListItem itemWithTitle:@"History & Bookmarks" subtitle:nil payload:@"hist"],
        [RBListItem itemWithTitle:@"Downloads" subtitle:nil payload:@"downloads"],
        [RBListItem itemWithTitle:@"Paste to Page" subtitle:nil payload:@"paste"],
        [RBListItem itemWithTitle:@"Reset Zoom" subtitle:nil payload:@"zoomreset"],
        [RBListItem itemWithTitle:@"Fullscreen" subtitle:nil payload:@"fullscreen"],
        [RBListItem itemWithTitle:@"Settings" subtitle:nil payload:@"settings"],
        [RBListItem itemWithTitle:@"Debug Overlay" subtitle:nil payload:@"debug"],
    ];
    RBListPopover *list = [[RBListPopover alloc] initWithSections:@[@{@"title": @"", @"items": items}]];
    __weak RBRootViewController *weakSelf = self;
    list.onSelect = ^(RBListItem *item) {
        [weakSelf dismissPopover];
        [weakSelf handleMenuAction:item.payload];
    };
    [self presentListPopover:list fromButton:button];
}

- (void)handleMenuAction:(NSString *)action {
    if ([action isEqualToString:@"find"]) {
        self.findVisible = YES;
        [self.view setNeedsLayout];
        [self.view layoutIfNeeded];
        [self.findBar focusField];
    } else if ([action isEqualToString:@"hist"]) {
        self.pendingPopoverKind = @"hist";
        [self.session sendMessage:@{@"t": @"hist"}];
    } else if ([action isEqualToString:@"downloads"]) {
        self.pendingPopoverKind = @"downloads";
        [self.session sendMessage:@{@"t": @"downloads"}];
    } else if ([action isEqualToString:@"paste"]) {
        [self pasteToPage];
    } else if ([action isEqualToString:@"zoomreset"]) {
        [self.session sendMessage:@{@"t": @"zoom", @"scale": @1.0, @"cx": @0.5, @"cy": @0.5}];
    } else if ([action isEqualToString:@"fullscreen"]) {
        [self toggleFullscreen];
    } else if ([action isEqualToString:@"settings"]) {
        [self presentSettingsAllowingCancel:YES message:nil];
    } else if ([action isEqualToString:@"debug"]) {
        self.debugVisible = !self.debugVisible;
        self.debugLabel.hidden = !self.debugVisible;
    }
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
}

- (void)presentHistoryPopover:(NSDictionary *)message {
    NSMutableArray *sections = [NSMutableArray array];
    NSArray *bookmarks = [[message objectForKey:@"bookmarks"] isKindOfClass:[NSArray class]] ? [message objectForKey:@"bookmarks"] : nil;
    NSArray *hist = [[message objectForKey:@"hist"] isKindOfClass:[NSArray class]] ? [message objectForKey:@"hist"] : nil;
    if ([bookmarks count]) {
        NSMutableArray *items = [NSMutableArray array];
        for (NSDictionary *entry in bookmarks) {
            if (![entry isKindOfClass:[NSDictionary class]]) continue;
            NSString *url = [entry objectForKey:@"url"] ?: @"";
            NSString *title = [entry objectForKey:@"title"];
            [items addObject:[RBListItem itemWithTitle:([title length] ? title : url) subtitle:url payload:url]];
        }
        [sections addObject:@{@"title": @"Bookmarks", @"items": items}];
    }
    if ([hist count]) {
        NSMutableArray *items = [NSMutableArray array];
        for (NSDictionary *entry in hist) {
            if (![entry isKindOfClass:[NSDictionary class]]) continue;
            NSString *url = [entry objectForKey:@"url"] ?: @"";
            NSString *title = [entry objectForKey:@"title"];
            [items addObject:[RBListItem itemWithTitle:([title length] ? title : url) subtitle:url payload:url]];
        }
        [sections addObject:@{@"title": @"History", @"items": items}];
    }
    if (![sections count]) {
        [self showToast:@"No history yet"];
        return;
    }
    RBListPopover *list = [[RBListPopover alloc] initWithSections:sections];
    __weak RBRootViewController *weakSelf = self;
    list.onSelect = ^(RBListItem *item) {
        [weakSelf dismissPopover];
        if ([item.payload length]) [weakSelf.session sendMessage:@{@"t": @"nav", @"url": item.payload}];
    };
    [self presentListPopover:list fromButton:self.chromeBar.menuButton];
}

static NSString *RBFormatSize(long long bytes) {
    if (bytes >= 1024 * 1024) return [NSString stringWithFormat:@"%.1f MB", bytes / (1024.0 * 1024.0)];
    if (bytes >= 1024) return [NSString stringWithFormat:@"%.0f KB", bytes / 1024.0];
    return [NSString stringWithFormat:@"%lld B", bytes];
}

- (void)presentDownloadsPopover:(id)itemsValue {
    NSArray *raw = [itemsValue isKindOfClass:[NSArray class]] ? itemsValue : nil;
    if (![raw count]) {
        [self showToast:@"No downloads yet"];
        return;
    }
    NSMutableArray *items = [NSMutableArray array];
    for (NSDictionary *entry in raw) {
        if (![entry isKindOfClass:[NSDictionary class]]) continue;
        NSString *name = [entry objectForKey:@"name"] ?: @"download";
        long long size = [[entry objectForKey:@"size"] longLongValue];
        [items addObject:[RBListItem itemWithTitle:name subtitle:RBFormatSize(size) payload:name]];
    }
    RBListPopover *list = [[RBListPopover alloc] initWithSections:@[@{@"title": @"Downloads", @"items": items}]];
    __weak RBRootViewController *weakSelf = self;
    list.onSelect = ^(RBListItem *item) {
        [weakSelf dismissPopover];
        [weakSelf openDownloadNamed:item.payload];
    };
    [self presentListPopover:list fromButton:self.chromeBar.menuButton];
}

// Fetch the file (auth cookie rides along automatically) into tmp, then offer
// the system "Open in…" menu — the loop iOS 6 Safari never closed.
- (void)openDownloadNamed:(NSString *)name {
    if (![name length]) return;
    NSString *escaped = [name stringByAddingPercentEscapesUsingEncoding:NSUTF8StringEncoding];
    NSURL *url = [NSURL URLWithString:[@"/downloads/" stringByAppendingString:escaped] relativeToURL:self.session.baseURL];
    if (!url) return;
    [self showToast:[NSString stringWithFormat:@"Fetching %@…", name]];
    NSURLRequest *request = [NSURLRequest requestWithURL:url cachePolicy:NSURLRequestReloadIgnoringLocalCacheData timeoutInterval:120.0];
    [NSURLConnection sendAsynchronousRequest:request queue:[NSOperationQueue mainQueue]
                           completionHandler:^(NSURLResponse *response, NSData *data, NSError *error) {
        NSInteger status = [response isKindOfClass:[NSHTTPURLResponse class]] ? [(NSHTTPURLResponse *)response statusCode] : 0;
        if (error || status >= 400 || ![data length]) {
            [self showToast:@"Download failed"];
            return;
        }
        NSString *path = [NSTemporaryDirectory() stringByAppendingPathComponent:name];
        if (![data writeToFile:path atomically:YES]) {
            [self showToast:@"Could not save file"];
            return;
        }
        self.docController = [UIDocumentInteractionController interactionControllerWithURL:[NSURL fileURLWithPath:path]];
        self.docController.delegate = self;
        CGRect anchor = [self.chromeBar.menuButton convertRect:self.chromeBar.menuButton.bounds toView:self.view];
        if (![self.docController presentOpenInMenuFromRect:anchor inView:self.view animated:YES]) {
            [self showToast:@"No app can open this file"];
        }
    }];
}

// ----------------------------------------------------------- keyboard shim

- (void)showKeyboard {
    self.hiddenInput.text = @" ";
    [self.hiddenInput becomeFirstResponder];
}

- (BOOL)textFieldShouldReturn:(UITextField *)textField {
    if (textField == self.hiddenInput) {
        [self sendKeyName:@"Enter" keyCode:13];
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

- (void)toggleDebug:(UITapGestureRecognizer *)tap {
    if (tap.state != UIGestureRecognizerStateEnded) return;
    self.debugVisible = !self.debugVisible;
    self.debugLabel.hidden = !self.debugVisible;
}

- (void)watchdogTick:(NSTimer *)timer {
    double age = self.lastFrameAt > 0.0 ? CACurrentMediaTime() - self.lastFrameAt : 0.0;
    if (self.lastFrameAt > 0.0 && age > 1.5 && CACurrentMediaTime() - self.lastPokeAt > 1.0) {
        self.lastPokeAt = CACurrentMediaTime();
        [self.session sendMessage:@{@"t": @"poke"}];
    }
    if (!self.debugVisible) return;
    CGSize s = self.streamView.bounds.size;
    self.debugLabel.text = [NSString stringWithFormat:@"WRP %@\nrx:%u shown:%u pending:%@\ndecode: %.1f ms avg %.1f ms\nview: %.0fx%.0f zoom: %.2f age: %.1fs\n%@",
        RBNativeVersion,
        self.framesReceived,
        self.framesDisplayed,
        self.pendingFrame ? @"yes" : @"no",
        self.lastDecodeMS,
        self.averageDecodeMS,
        s.width,
        s.height,
        self.serverZoom,
        age,
        [self.session.baseURL absoluteString] ?: @"—"];
}

@end
