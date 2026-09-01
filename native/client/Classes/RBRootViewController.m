#import "RBRootViewController.h"
#import "RBActionActivity.h"
#import "RBActionMenuController.h"
#import "RBChromeBar.h"
#import "RBBrowserStateView.h"
#import "RBConfig.h"
#import "RBClientUpdater.h"
#import "RBDiagnostics.h"
#import "RBDiagnosticsOverlay.h"
#import "RBFindBar.h"
#import "RBLibraryController.h"
#import "RBListPopover.h"
#import "RBReaderController.h"
#import "RBLog.h"
#import "RBMediaPipeline.h"
#import "RBMediaController.h"
#import "RBNewTabView.h"
#import "RBInteractionTracker.h"
#import "RBOmnibox.h"
#import "RBProtocol.h"
#import "RBQRScannerController.h"
#import "RBPageSwitcherController.h"
#import "RBPairingController.h"
#import "RBPairingClient.h"
#import "RBPhoneToolbar.h"
#import "RBSecureHTTPClient.h"
#import "RBSelectController.h"
#import "RBServerStore.h"
#import "RBServersController.h"
#import "RBSession.h"
#import "RBSettingsController.h"
#import "RBStreamView.h"
#import "RBSuggestPanel.h"
#import "RBTabStrip.h"
#import "RBTheme.h"

#import <ImageIO/ImageIO.h>
#import <MessageUI/MessageUI.h>
#import <QuartzCore/QuartzCore.h>

#include <math.h>
#include <stdlib.h>


static const CGFloat kRBTopBarHeight = 50.0;
static const CGFloat kRBFindBarHeight = 40.0;
static const NSTimeInterval kRBBackgroundDisconnectDelay = 60.0;

static BOOL RBIsPad(void) {
    return UI_USER_INTERFACE_IDIOM() == UIUserInterfaceIdiomPad;
}

static BOOL RBValidClipboardText(id value) {
    if (![value isKindOfClass:[NSString class]]) return NO;
    NSString *text = value;
    for (NSUInteger index = 0; index < [text length]; index++) {
        if ([text characterAtIndex:index] == 0) return NO;
    }
    NSData *data = [text dataUsingEncoding:NSUTF8StringEncoding];
    return data != nil && [data length] <= 64 * 1024;
}

// iOS 6 form sheets keep the keyboard on screen even after their child view
// resigns first responder unless the navigation controller opts out.
@interface RBModalNavigationController : UINavigationController
@end

@implementation RBModalNavigationController
- (BOOL)disablesAutomaticKeyboardDismissal { return NO; }
- (void)viewWillAppear:(BOOL)animated {
    [super viewWillAppear:animated];
    [RBTheme styleNavigationBar:self.navigationBar];
    self.view.backgroundColor = [RBTheme pageBackgroundColor];
    // UIKit owns the form-sheet mask. Clipping the child navigation view too
    // exposes the system's white backing through both sets of rounded corners.
    self.view.layer.cornerRadius = 0.0;
    self.view.layer.masksToBounds = NO;
}
@end

// H.264 4:2:0 requires even coded dimensions. Keep the visible stream surface
// even as well so Chromium, VideoEncoder, and OpenGL all operate at the same
// pixel size instead of scaling an odd viewport down and back up.
static CGFloat RBEvenExtent(CGFloat value) {
    NSInteger whole = (NSInteger)floor(value);
    if (whole < 2) return 2.0;
    return (CGFloat)(whole & ~1);
}

@interface RBRootViewController () <UITextFieldDelegate, RBSessionDelegate, RBChromeBarDelegate,
                                    RBPhoneToolbarDelegate, RBPageSwitcherDelegate,
                                    RBOmniboxDelegate, RBTabStripDelegate, RBSuggestPanelDelegate,
                                    RBFindBarDelegate, RBSettingsDelegate, RBServersControllerDelegate,
                                    RBPairingControllerDelegate, RBMediaPipelineDelegate, RBStreamViewDelegate,
                                    RBNewTabViewDelegate, RBBrowserStateViewDelegate,
                                    RBMediaControllerDelegate,
                                    RBInteractionTrackerDelegate, RBClientUpdaterDelegate,
                                    UIDocumentInteractionControllerDelegate, UIPopoverControllerDelegate,
                                    UIAlertViewDelegate, RBSelectControllerDelegate,
                                    RBQRScannerDelegate,
                                    UIImagePickerControllerDelegate, UINavigationControllerDelegate,
                                    MFMailComposeViewControllerDelegate,
                                    RBDiagnosticsOverlayDelegate>
// Views
@property(nonatomic, strong) RBStreamView *streamView;
@property(nonatomic, strong) RBNewTabView *startPageView;
@property(nonatomic, strong) RBBrowserStateView *browserStateView;
@property(nonatomic, strong) RBChromeBar *chromeBar;
@property(nonatomic, strong) RBPhoneToolbar *phoneToolbar;
@property(nonatomic, strong) RBTabStrip *tabStrip;
@property(nonatomic, strong) RBFindBar *findBar;
@property(nonatomic, strong) RBSuggestPanel *suggestPanel;
@property(nonatomic, strong) UIButton *restoreButton;
@property(nonatomic, strong) UIButton *fullscreenBackButton;
@property(nonatomic, strong) UIButton *fullscreenForwardButton;
@property(nonatomic, strong) UILabel *toastLabel;
@property(nonatomic, strong) UILabel *connectionPill;
@property(nonatomic, strong) RBDiagnosticsOverlay *diagnosticsOverlay;
@property(nonatomic, strong) UITextField *hiddenInput;
@property(nonatomic, strong) UIBarButtonItem *pagePasteButton;
// Controllers
@property(nonatomic, strong) RBSession *session;
@property(nonatomic, strong) RBSettingsController *settingsController;
@property(nonatomic, strong) RBServersController *serversController;
@property(nonatomic, strong) RBPairingController *pairingController;
@property(nonatomic, strong) UINavigationController *modalNavigationController;
@property(nonatomic, strong) RBPageSwitcherController *pageSwitcherController;
@property(nonatomic, strong) UIPopoverController *popover;
@property(nonatomic, strong) UIViewController *compactPopoverController;
@property(nonatomic, strong) UIViewController *activityController;
@property(nonatomic, strong) RBActionMenuController *actionMenuController;
@property(nonatomic, copy) NSString *pendingActivityAction;
@property(nonatomic, strong) UIDocumentInteractionController *docController;
@property(nonatomic, strong) RBMediaController *pageMediaController;
@property(nonatomic, strong) RBSelectController *selectController;
@property(nonatomic, strong) UINavigationController *selectNavigationController;
@property(nonatomic, strong) UIPopoverController *selectPopover;
// Connect flow
@property(nonatomic, strong) NSDictionary *currentServer;
@property(nonatomic, assign) NSUInteger verificationGeneration;
@property(nonatomic, strong) NSDictionary *pendingClientUpdate;
@property(nonatomic, strong) RBClientUpdater *clientUpdater;
@property(nonatomic, strong) UIAlertView *updateAlert;
@property(nonatomic, strong) UIAlertView *updateInstalledAlert;
@property(nonatomic, assign) BOOL audioRequested;
@property(nonatomic, assign) BOOL applicationInBackground;
@property(nonatomic, assign) BOOL disconnectedForBackground;
@property(nonatomic, strong) NSTimer *backgroundDisconnectTimer;
@property(nonatomic, assign) UIBackgroundTaskIdentifier backgroundTaskIdentifier;
// Page state
@property(nonatomic, assign) BOOL loading;
@property(nonatomic, assign) BOOL fullscreen;
@property(nonatomic, assign) BOOL viewportTransitioning;
@property(nonatomic, assign) BOOL findVisible;
@property(nonatomic, assign) BOOL debugVisible;
@property(nonatomic, strong) NSArray *lastTabs;
@property(nonatomic, strong) NSMutableDictionary *tabThumbnails;
@property(nonatomic, strong) NSMutableArray *thumbnailLRU;
@property(nonatomic, strong) NSArray *bookmarks;
@property(nonatomic, copy) NSString *currentURL;
@property(nonatomic, copy) NSString *currentSecurity;
@property(nonatomic, assign) BOOL currentStarred;
@property(nonatomic, assign) BOOL canGoBack;
@property(nonatomic, assign) BOOL canGoForward;
@property(nonatomic, assign) BOOL awaitingPageFrame;
@property(nonatomic, assign) unsigned int awaitedSourceSequence;
// Physical page input
@property(nonatomic, strong) NSMutableDictionary *pageTouchIDs;
@property(nonatomic, assign) NSUInteger nextTouchID;
@property(nonatomic, assign) unsigned int presentedSurfaceGeneration;
@property(nonatomic, strong) UITouch *edgeTouch;
// Video lane
@property(nonatomic, strong) RBMediaPipeline *mediaPipeline;
@property(nonatomic, strong) RBDiagnostics *diagnostics;
@property(nonatomic, assign) BOOL videoActive;   // server confirmed video-config ok
@property(nonatomic, assign) BOOL videoStarting; // server is starting the automatic video lane
@property(nonatomic, copy) NSString *videoProfile;
@property(nonatomic, assign) CGSize remoteViewportSize;
// Keyboard avoidance (editable rect, viewport fractions)
@property(nonatomic, assign) CGRect editableRect;
@property(nonatomic, assign) BOOL editableHasRect;
@property(nonatomic, assign) BOOL keyboardVisible;
@property(nonatomic, assign) CGFloat keyboardTop;
@property(nonatomic, assign) CGFloat keyboardShift;
@property(nonatomic, assign) CFTimeInterval lastVideoLivenessRecoveryAt;
@property(nonatomic, copy) NSString *previousHiddenText;
@property(nonatomic, assign) BOOL inputCompositionActive;
// JS dialogs (M2.1)
@property(nonatomic, strong) UIAlertView *dialogAlert;
@property(nonatomic, copy) NSString *dialogKind;
@property(nonatomic, assign) BOOL dialogSuppressReply;
// Uploads (M2.2)
@property(nonatomic, assign) BOOL chooserPending;
@property(nonatomic, strong) UIPopoverController *uploadPopover;
@property(nonatomic, strong) UIImagePickerController *uploadPicker;
// Library (chrome rethink) / reader (M1.5)
@property(nonatomic, strong) RBLibraryController *libraryController;
@property(nonatomic, assign) BOOL readerPending;
// Pasteboard banner (M4.2)
@property(nonatomic, strong) UIAlertView *pasteboardAlert;
@property(nonatomic, copy) NSString *pasteboardURL;
// Host-controlled clipboard synchronization. Clipboard text stays in memory
// and on the two system pasteboards; it is never written to Surf's settings.
@property(nonatomic, assign) BOOL clipboardSyncEnabled;
@property(nonatomic, assign) NSInteger clipboardChangeCount;
@property(nonatomic, strong) NSTimer *clipboardSyncTimer;
// Edge swipes (M2.6): 0 none, -1 left edge (back), 1 right edge (forward)
@property(nonatomic, assign) int edgeSwipe;
@property(nonatomic, assign) CGPoint edgeStart;
- (NSNumber *)activeTabKey;
- (void)cacheThumbnailForTabKey:(NSNumber *)tabKey;
- (void)presentLibraryFromButton:(UIButton *)button;
- (void)presentPageSwitcher;
- (void)sendKeyName:(NSString *)name keyCode:(NSInteger)keyCode;
- (CGSize)constrainedSelectPopoverSize:(RBSelectController *)controller;
- (void)presentSelectMessage:(NSDictionary *)message;
- (void)dismissSelectControllerSendingCancel:(BOOL)sendCancel;
- (BOOL)browserBarAtBottom;
- (UIPopoverArrowDirection)browserChromeArrowDirection;
- (void)applyAppearance;
@end

@implementation RBRootViewController

- (BOOL)browserBarAtBottom {
    return [[NSUserDefaults standardUserDefaults] boolForKey:RBDefaultsBottomBrowserBarKey];
}

- (UIPopoverArrowDirection)browserChromeArrowDirection {
    return [self browserBarAtBottom] ? UIPopoverArrowDirectionDown : UIPopoverArrowDirectionUp;
}

- (id)init {
    self = [super init];
    if (self) self.backgroundTaskIdentifier = UIBackgroundTaskInvalid;
    return self;
}

- (void)dealloc {
    [self.backgroundDisconnectTimer invalidate];
    if (self.backgroundTaskIdentifier != UIBackgroundTaskInvalid) {
        [[UIApplication sharedApplication] endBackgroundTask:self.backgroundTaskIdentifier];
        self.backgroundTaskIdentifier = UIBackgroundTaskInvalid;
    }
    [self.clipboardSyncTimer invalidate];
    [[NSNotificationCenter defaultCenter] removeObserver:self];
}

// ---------------------------------------------------------------- lifecycle

- (void)viewDidLoad {
    [super viewDidLoad];
    [RBServerStore performBreakingMigrationIfNeeded];
    self.view.backgroundColor = [UIColor blackColor];
    [[NSNotificationCenter defaultCenter] addObserver:self selector:@selector(keyboardWillShow:)
                                                 name:UIKeyboardWillShowNotification object:nil];
    [[NSNotificationCenter defaultCenter] addObserver:self selector:@selector(keyboardWillHide:)
                                                 name:UIKeyboardWillHideNotification object:nil];
    [[NSNotificationCenter defaultCenter] addObserver:self selector:@selector(readerNavigate:)
                                                 name:@"RBReaderNavigate" object:nil];

    self.streamView = [[RBStreamView alloc] initWithFrame:CGRectZero];
    self.streamView.presentationDelegate = self;
    self.pageTouchIDs = [NSMutableDictionary dictionary];
    self.nextTouchID = 1;
    [self.view addSubview:self.streamView];

    self.startPageView = [[RBNewTabView alloc] initWithFrame:CGRectZero];
    self.startPageView.delegate = self;
    self.startPageView.hidden = YES;
    [self.view addSubview:self.startPageView];

    self.browserStateView = [[RBBrowserStateView alloc] initWithFrame:CGRectZero];
    self.browserStateView.delegate = self;
    [self.view addSubview:self.browserStateView];
    self.mediaPipeline = [[RBMediaPipeline alloc] init];
    self.mediaPipeline.delegate = self;
    [self.streamView installSystemDisplayLayer:self.mediaPipeline.systemDisplayLayer];
    self.diagnostics = [[RBDiagnostics alloc] initWithMediaPipeline:self.mediaPipeline
                                                         streamView:self.streamView];

    UITapGestureRecognizer *tripleTap = [[UITapGestureRecognizer alloc] initWithTarget:self action:@selector(toggleDebug:)];
    tripleTap.numberOfTapsRequired = 3;
    [self.streamView addGestureRecognizer:tripleTap];

    self.chromeBar = [[RBChromeBar alloc] initWithFrame:CGRectZero];
    self.chromeBar.delegate = self;
    self.chromeBar.omnibox.delegate = self;
    self.chromeBar.phoneLayout = !RBIsPad();
    self.chromeBar.bottomPositioned = [self browserBarAtBottom];
    [self.view addSubview:self.chromeBar];

    self.phoneToolbar = [[RBPhoneToolbar alloc] initWithFrame:CGRectZero];
    self.phoneToolbar.delegate = self;
    self.phoneToolbar.hidden = RBIsPad();
    [self.view addSubview:self.phoneToolbar];

    self.tabStrip = [[RBTabStrip alloc] initWithFrame:CGRectZero];
    self.tabStrip.delegate = self;
    self.tabStrip.hidden = !RBIsPad();
    self.tabStrip.frame = self.chromeBar.tabHostView.bounds;
    self.tabStrip.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
    [self.chromeBar.tabHostView addSubview:self.tabStrip];

    self.tabThumbnails = [NSMutableDictionary dictionary];
    self.thumbnailLRU = [NSMutableArray array];

    self.findBar = [[RBFindBar alloc] initWithFrame:CGRectZero];
    self.findBar.delegate = self;
    [self.findBar setPageBoundaryAtTop:[self browserBarAtBottom]];
    self.findBar.hidden = YES;
    [self.view addSubview:self.findBar];

    self.suggestPanel = [[RBSuggestPanel alloc] initWithFrame:CGRectZero];
    self.suggestPanel.delegate = self;
    [self.view addSubview:self.suggestPanel];

    self.restoreButton = [UIButton buttonWithType:UIButtonTypeCustom];
    self.restoreButton.backgroundColor = [[RBTheme deepTideColor] colorWithAlphaComponent:0.82];
    self.restoreButton.layer.cornerRadius = 11.0;
    [self.restoreButton setImage:[RBTheme icon:RBIconShrink size:20.0 color:[UIColor colorWithWhite:1.0 alpha:0.9]]
                        forState:UIControlStateNormal];
    [self.restoreButton addTarget:self action:@selector(toggleFullscreen) forControlEvents:UIControlEventTouchUpInside];
    self.restoreButton.hidden = YES;
    self.restoreButton.accessibilityLabel = @"Exit Fullscreen";
    [self.view addSubview:self.restoreButton];

    self.fullscreenBackButton = [UIButton buttonWithType:UIButtonTypeCustom];
    self.fullscreenBackButton.backgroundColor = [[RBTheme deepTideColor] colorWithAlphaComponent:0.82];
    self.fullscreenBackButton.layer.cornerRadius = 11.0;
    [self.fullscreenBackButton setImage:[RBTheme icon:RBIconBack size:20.0 color:[UIColor colorWithWhite:1.0 alpha:0.9]]
                                 forState:UIControlStateNormal];
    [self.fullscreenBackButton addTarget:self action:@selector(fullscreenBackTapped:)
                        forControlEvents:UIControlEventTouchUpInside];
    self.fullscreenBackButton.accessibilityLabel = @"Back";
    self.fullscreenBackButton.hidden = YES;
    [self.view addSubview:self.fullscreenBackButton];

    self.fullscreenForwardButton = [UIButton buttonWithType:UIButtonTypeCustom];
    self.fullscreenForwardButton.backgroundColor = [[RBTheme deepTideColor] colorWithAlphaComponent:0.82];
    self.fullscreenForwardButton.layer.cornerRadius = 11.0;
    [self.fullscreenForwardButton setImage:[RBTheme icon:RBIconForward size:20.0 color:[UIColor colorWithWhite:1.0 alpha:0.9]]
                                    forState:UIControlStateNormal];
    [self.fullscreenForwardButton addTarget:self action:@selector(fullscreenForwardTapped:)
                           forControlEvents:UIControlEventTouchUpInside];
    self.fullscreenForwardButton.accessibilityLabel = @"Forward";
    self.fullscreenForwardButton.hidden = YES;
    [self.view addSubview:self.fullscreenForwardButton];

    self.hiddenInput = [[UITextField alloc] initWithFrame:CGRectMake(-100.0, -100.0, 20.0, 20.0)];
    self.hiddenInput.delegate = self;
    self.hiddenInput.autocorrectionType = UITextAutocorrectionTypeNo;
    self.hiddenInput.autocapitalizationType = UITextAutocapitalizationTypeNone;
    self.hiddenInput.returnKeyType = UIReturnKeyGo;
    self.hiddenInput.text = @" ";
    self.previousHiddenText = @" ";
    UIToolbar *inputBar = [[UIToolbar alloc] initWithFrame:CGRectMake(0.0, 0.0, self.view.bounds.size.width, 38.0)];
    inputBar.barStyle = UIBarStyleBlack;
    inputBar.autoresizingMask = UIViewAutoresizingFlexibleWidth;
    UIBarButtonItem *escape = [[UIBarButtonItem alloc] initWithTitle:@"Esc" style:UIBarButtonItemStylePlain
                                                             target:self action:@selector(escapePageInput)];
    UIBarButtonItem *tab = [[UIBarButtonItem alloc] initWithTitle:@"Tab" style:UIBarButtonItemStylePlain
                                                          target:self action:@selector(tabPageInput)];
    UIBarButtonItem *space = [[UIBarButtonItem alloc] initWithBarButtonSystemItem:UIBarButtonSystemItemFlexibleSpace
                                                                          target:nil action:nil];
    self.pagePasteButton = [[UIBarButtonItem alloc] initWithTitle:@"Paste" style:UIBarButtonItemStyleDone
                                                          target:self action:@selector(pasteToPage)];
    inputBar.items = @[escape, tab, space, self.pagePasteButton];
    self.hiddenInput.inputAccessoryView = inputBar;
    [self.view addSubview:self.hiddenInput];
    [[NSNotificationCenter defaultCenter] addObserver:self selector:@selector(hiddenInputDidChange:)
                                                 name:UITextFieldTextDidChangeNotification object:self.hiddenInput];

    self.toastLabel = [[UILabel alloc] initWithFrame:CGRectZero];
    self.toastLabel.backgroundColor = [[RBTheme deepTideColor] colorWithAlphaComponent:0.92];
    self.toastLabel.textColor = [UIColor colorWithWhite:0.97 alpha:1.0];
    self.toastLabel.textAlignment = NSTextAlignmentCenter;
    self.toastLabel.font = [RBTheme fontOfSize:14.0 bold:NO];
    self.toastLabel.layer.cornerRadius = 14.0;
    self.toastLabel.layer.masksToBounds = YES;
    self.toastLabel.alpha = 0.0;
    [self.view addSubview:self.toastLabel];

    self.connectionPill = [[UILabel alloc] initWithFrame:CGRectZero];
    self.connectionPill.backgroundColor = [[RBTheme deepTideColor] colorWithAlphaComponent:0.88];
    self.connectionPill.textColor = [UIColor colorWithWhite:0.95 alpha:1.0];
    self.connectionPill.textAlignment = NSTextAlignmentCenter;
    self.connectionPill.font = [RBTheme fontOfSize:12.0 bold:NO];
    self.connectionPill.layer.cornerRadius = 11.0;
    self.connectionPill.layer.masksToBounds = YES;
    self.connectionPill.hidden = YES;
    [self.view addSubview:self.connectionPill];

    self.diagnosticsOverlay = [[RBDiagnosticsOverlay alloc] initWithFrame:CGRectZero];
    self.diagnosticsOverlay.delegate = self;
    self.diagnosticsOverlay.hidden = YES;
    [self.view addSubview:self.diagnosticsOverlay];
    [self setDebugVisible:[[NSUserDefaults standardUserDefaults] boolForKey:RBDefaultsDiagnosticsKey]];

    [self applyAppearance];

    [NSTimer scheduledTimerWithTimeInterval:1.0 target:self selector:@selector(watchdogTick:) userInfo:nil repeats:YES];

    RBLogEvent(@"application", @"info", @{@"compatibility": RBCompatibilityVersion ?: @""}, @"Browser interface loaded");
}

- (void)applyAppearance {
    [self.chromeBar applyAppearance];
    [self.phoneToolbar applyAppearance];
    [self.tabStrip applyAppearance];
    [self.findBar applyAppearance];
    [self.suggestPanel applyAppearance];
    [self.startPageView applyAppearance];
    [self.browserStateView applyAppearance];
    [self.diagnosticsOverlay applyAppearance];
    UIColor *floatingSurface = [[RBTheme deepTideColor] colorWithAlphaComponent:0.88];
    self.restoreButton.backgroundColor = floatingSurface;
    self.fullscreenBackButton.backgroundColor = floatingSurface;
    self.fullscreenForwardButton.backgroundColor = floatingSurface;
    self.toastLabel.backgroundColor = [[RBTheme deepTideColor] colorWithAlphaComponent:0.94];
    self.connectionPill.backgroundColor = floatingSurface;
    UIToolbar *inputBar = (UIToolbar *)self.hiddenInput.inputAccessoryView;
    inputBar.barStyle = [RBTheme isDarkMode] ? UIBarStyleBlack : UIBarStyleDefault;
    self.hiddenInput.keyboardAppearance = [RBTheme isDarkMode] ? UIKeyboardAppearanceDark
                                                               : UIKeyboardAppearanceDefault;
    [RBTheme styleNavigationBar:self.modalNavigationController.navigationBar];
    self.modalNavigationController.view.backgroundColor = [RBTheme pageBackgroundColor];
    [self.view setNeedsLayout];
}

- (void)finishBackgroundTask {
    [self.backgroundDisconnectTimer invalidate];
    self.backgroundDisconnectTimer = nil;
    if (self.backgroundTaskIdentifier == UIBackgroundTaskInvalid) return;
    UIBackgroundTaskIdentifier identifier = self.backgroundTaskIdentifier;
    self.backgroundTaskIdentifier = UIBackgroundTaskInvalid;
    [[UIApplication sharedApplication] endBackgroundTask:identifier];
}

- (void)disconnectAfterBackgroundTimeout:(NSTimer *)timer {
    (void)timer;
    if (!self.applicationInBackground) {
        [self finishBackgroundTask];
        return;
    }
    BOOL hadSession = self.session && self.session.state != RBSessionStateIdle;
    RBLogEvent(@"application", @"info",
               @{@"idle_seconds": @(kRBBackgroundDisconnectDelay),
                 @"connected": @(hadSession)},
               @"Background idle timeout reached");
    [self.mediaPipeline stop];
    self.streamView.videoActive = NO;
    [self.session shutdown];
    self.disconnectedForBackground = hadSession;
    [self finishBackgroundTask];
}

- (void)applicationDidEnterBackground {
    if (self.applicationInBackground) return;
    self.applicationInBackground = YES;

    // AudioQueue otherwise keeps asking for silent buffers forever. Stop and
    // deactivate it immediately, before the one-minute reconnect grace period.
    if (self.session.state == RBSessionStateOpen) {
        [self.session sendMessage:@{@"t": @"audio", @"on": @NO}];
    }
    self.audioRequested = NO;
    [self.mediaPipeline stopAudio];

    // Do not decode or present the video frames that may arrive during the
    // grace period. If Surf returns quickly, the live socket can be reused.
    self.streamView.videoActive = NO;
    [self.clipboardSyncTimer invalidate];
    self.clipboardSyncTimer = nil;

    RBLogEvent(@"application", @"info",
               @{@"disconnect_delay_seconds": @(kRBBackgroundDisconnectDelay)},
               @"Application backgrounded; media quiesced");

    __weak RBRootViewController *weakSelf = self;
    self.backgroundTaskIdentifier = [[UIApplication sharedApplication]
        beginBackgroundTaskWithExpirationHandler:^{
            RBRootViewController *controller = weakSelf;
            if (controller) [controller disconnectAfterBackgroundTimeout:nil];
        }];
    if (self.backgroundTaskIdentifier == UIBackgroundTaskInvalid) {
        [self disconnectAfterBackgroundTimeout:nil];
        return;
    }
    self.backgroundDisconnectTimer = [NSTimer timerWithTimeInterval:kRBBackgroundDisconnectDelay
                                                              target:self
                                                            selector:@selector(disconnectAfterBackgroundTimeout:)
                                                            userInfo:nil repeats:NO];
    [[NSRunLoop mainRunLoop] addTimer:self.backgroundDisconnectTimer forMode:NSRunLoopCommonModes];
}

- (void)applicationDidBecomeActive {
    BOOL wasBackgrounded = self.applicationInBackground;
    self.applicationInBackground = NO;
    [self finishBackgroundTask];
    if (!wasBackgrounded) return;

    if (self.clipboardSyncEnabled && !self.clipboardSyncTimer) {
        [self setClipboardSyncEnabled:YES];
    }
    if (self.disconnectedForBackground) {
        self.disconnectedForBackground = NO;
        NSDictionary *server = self.currentServer ?: [RBServerStore lastSelectedServer];
        if (server) [self connectToServer:server];
        return;
    }
    if (self.session.state == RBSessionStateOpen && self.videoActive) {
        // Frames are intentionally discarded while backgrounded. Re-enter at
        // an IDR instead of handing dependent P-frames to stale decoder state.
        [self.mediaPipeline recoverVideo];
        self.streamView.videoActive = YES;
        [self.session sendMessage:@{@"t": @"reqkeyframe"}];
    }
    RBLogEvent(@"application", @"info", @{}, @"Application resumed during background grace period");
}

- (void)viewDidAppear:(BOOL)animated {
    [super viewDidAppear:animated];
    if (self.session || self.settingsController || self.serversController) return;
    NSDictionary *server = [RBServerStore lastSelectedServer];
    if (server) [self connectToServer:server];
    else [self presentServersAllowingCancel:NO firstLaunch:YES message:nil];
}

- (void)didReceiveMemoryWarning {
    [super didReceiveMemoryWarning];
    [self.diagnostics noteMemoryWarning];
    [self.tabStrip purgeIconCache];
    [self.tabThumbnails removeAllObjects];
    [self.thumbnailLRU removeAllObjects];
    [self.pageSwitcherController updateTabs:self.lastTabs thumbnails:@{}];
}

// ------------------------------------------------------------------- layout

- (void)viewDidLayoutSubviews {
    [super viewDidLayoutSubviews];
    CGFloat w = self.view.bounds.size.width;
    CGFloat h = self.view.bounds.size.height;
    BOOL pad = RBIsPad();
    BOOL browserBarAtBottom = pad && [self browserBarAtBottom];
    CGFloat topBarHeight = kRBTopBarHeight;
    CGFloat bottomBarHeight = pad ? 0.0 : 48.0;

    self.chromeBar.phoneLayout = !pad;
    self.chromeBar.bottomPositioned = browserBarAtBottom;
    [self.findBar setPageBoundaryAtTop:browserBarAtBottom];
    self.chromeBar.hidden = self.fullscreen;
    self.phoneToolbar.hidden = self.fullscreen || pad;
    self.tabStrip.hidden = self.fullscreen || !pad;
    self.findBar.hidden = self.fullscreen || !self.findVisible;
    self.suggestPanel.hidden = self.fullscreen;
    self.restoreButton.hidden = !self.fullscreen;
    self.fullscreenBackButton.hidden = YES;
    self.fullscreenForwardButton.hidden = YES;
    self.fullscreenBackButton.enabled = self.canGoBack;
    self.fullscreenForwardButton.enabled = self.canGoForward;

    CGFloat contentTop = 0.0;
    CGFloat contentBottom = h;
    if (!self.fullscreen) {
        if (pad) {
            self.phoneToolbar.frame = CGRectZero;
            if (browserBarAtBottom) {
                CGFloat shelfBottom = h;
                if (self.keyboardVisible &&
                    (self.chromeBar.omnibox.editing || self.findBar.editing)) {
                    shelfBottom = MIN(h, MAX(topBarHeight, self.keyboardTop));
                }
                CGFloat chromeTop = shelfBottom - topBarHeight;
                self.chromeBar.frame = CGRectMake(0.0, chromeTop, w, topBarHeight);
                // Keep the remote viewport stable while the keyboard moves the
                // browser shelf. Resizing it here restarts capture, WebCodecs,
                // and VideoToolbox on every keyboard transition and creates a
                // visible one-second media interruption.
                contentBottom = h - topBarHeight;
                if (self.findVisible) {
                    contentBottom -= kRBFindBarHeight;
                    self.findBar.frame = CGRectMake(0.0, chromeTop - kRBFindBarHeight,
                                                    w, kRBFindBarHeight);
                }
            } else {
                self.chromeBar.frame = CGRectMake(0.0, 0.0, w, topBarHeight);
                contentTop = topBarHeight;
                if (self.findVisible) {
                    self.findBar.frame = CGRectMake(0.0, contentTop, w, kRBFindBarHeight);
                    contentTop += kRBFindBarHeight;
                }
            }
        } else {
            self.chromeBar.frame = CGRectMake(0.0, 0.0, w, topBarHeight);
            self.phoneToolbar.frame = CGRectMake(0.0, h - bottomBarHeight, w, bottomBarHeight);
            contentTop = topBarHeight;
            contentBottom = h - bottomBarHeight;
            if (self.findVisible) {
                self.findBar.frame = CGRectMake(0.0, contentTop, w, kRBFindBarHeight);
                contentTop += kRBFindBarHeight;
            }
        }
    } else {
        self.chromeBar.frame = CGRectZero;
        self.phoneToolbar.frame = CGRectZero;
        self.findBar.frame = CGRectZero;
    }

    CGFloat streamW = RBEvenExtent(w);
    CGFloat streamH = RBEvenExtent(MAX(2.0, contentBottom - contentTop));
    CGFloat streamX = floor((w - streamW) / 2.0);
    self.remoteViewportSize = CGSizeMake(streamW, streamH);
    // The keyboard and omnibox shelf may cover this surface, but must never
    // resize it. iOS's video compositor handles occlusion without restarting
    // capture, rebuilding a decoder, or reallocating an EAGL drawable.
    self.streamView.bounds = CGRectMake(0.0, 0.0, streamW, streamH);
    self.streamView.center = CGPointMake(streamX + streamW / 2.0,
                                         contentTop + streamH / 2.0);
    self.startPageView.frame = CGRectMake(streamX, contentTop, streamW, streamH);
    self.browserStateView.frame = CGRectMake(streamX, contentTop, streamW, streamH);

    [self.chromeBar setNeedsLayout];
    [self.chromeBar layoutIfNeeded];
    CGRect omniboxFrame = [self.chromeBar convertRect:self.chromeBar.omnibox.frame toView:self.view];
    if (pad && browserBarAtBottom) {
        CGFloat suggestBottom = MAX(0.0, CGRectGetMinY(self.chromeBar.frame) + 2.0);
        CGFloat suggestH = MIN([self.suggestPanel desiredHeight], suggestBottom);
        self.suggestPanel.frame = CGRectMake(omniboxFrame.origin.x, suggestBottom - suggestH,
                                             omniboxFrame.size.width, suggestH);
    } else {
        CGFloat suggestTop = MAX(0.0, CGRectGetMaxY(self.chromeBar.frame) - 2.0);
        CGFloat suggestH = MIN([self.suggestPanel desiredHeight],
                               MAX(0.0, contentBottom - suggestTop));
        self.suggestPanel.frame = CGRectMake(omniboxFrame.origin.x, suggestTop,
                                             omniboxFrame.size.width, suggestH);
    }

    self.restoreButton.frame = CGRectMake(w - 54.0, h - 54.0, 44.0, 44.0);
    self.fullscreenBackButton.frame = CGRectMake(10.0, h - 54.0, 44.0, 44.0);
    self.fullscreenForwardButton.frame = CGRectMake(62.0, h - 54.0, 44.0, 44.0);
    CGFloat toastW = MIN(320.0, MAX(120.0, w - 20.0));
    self.toastLabel.frame = CGRectMake(floorf((w - toastW) / 2.0), contentTop + 14.0, toastW, 28.0);
    self.connectionPill.frame = CGRectMake(10.0, contentBottom - 34.0, 150.0, 22.0);
    [self layoutDiagnosticsOverlayAnimated:NO];
    [self scheduleViewportUpdate];
}

- (BOOL)shouldAutorotateToInterfaceOrientation:(UIInterfaceOrientation)interfaceOrientation { return YES; }
- (NSUInteger)supportedInterfaceOrientations { return UIInterfaceOrientationMaskAll; }
- (BOOL)shouldAutorotate { return YES; }

- (void)willAnimateRotationToInterfaceOrientation:(UIInterfaceOrientation)toInterfaceOrientation duration:(NSTimeInterval)duration {
    [super willAnimateRotationToInterfaceOrientation:toInterfaceOrientation duration:duration];
    self.viewportTransitioning = YES;
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(sendCurrentViewportSize) object:nil];
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(sendCurrentViewportSizeForcedAfterTransition) object:nil];
    [self.view setNeedsLayout];
}

- (void)didRotateFromInterfaceOrientation:(UIInterfaceOrientation)fromInterfaceOrientation {
    [super didRotateFromInterfaceOrientation:fromInterfaceOrientation];
    [self finishViewportTransition];
}

- (void)viewWillTransitionToSize:(CGSize)size
       withTransitionCoordinator:(id<UIViewControllerTransitionCoordinator>)coordinator {
    [super viewWillTransitionToSize:size withTransitionCoordinator:coordinator];
    self.viewportTransitioning = YES;
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(sendCurrentViewportSize) object:nil];
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(sendCurrentViewportSizeForcedAfterTransition) object:nil];
    __weak RBRootViewController *weakSelf = self;
    [coordinator animateAlongsideTransition:nil completion:^(id<UIViewControllerTransitionCoordinatorContext> context) {
        [weakSelf finishViewportTransition];
    }];
}

- (void)finishViewportTransition {
    self.viewportTransitioning = NO;
    [self.view setNeedsLayout];
    [self.view layoutIfNeeded];
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(sendCurrentViewportSize) object:nil];
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(sendCurrentViewportSizeForcedAfterTransition) object:nil];
    [self performSelector:@selector(sendCurrentViewportSizeForcedAfterTransition)
               withObject:nil afterDelay:0.10];
    if (self.selectPopover.popoverVisible && self.selectController) {
        [self.selectPopover setPopoverContentSize:[self constrainedSelectPopoverSize:self.selectController]
                                         animated:YES];
    }
}

- (void)scheduleViewportUpdate {
    if (self.viewportTransitioning) return;
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(sendCurrentViewportSize) object:nil];
    [self performSelector:@selector(sendCurrentViewportSize) withObject:nil afterDelay:0.10];
}

- (void)sendCurrentViewportSize {
    [self sendCurrentViewportSizeForced:NO];
}

- (void)sendCurrentViewportSizeForcedAfterTransition {
    [self sendCurrentViewportSizeForced:YES];
}

- (void)sendCurrentViewportSizeForced:(BOOL)force {
    CGSize s = self.remoteViewportSize;
    if (s.width < 10.0 || s.height < 10.0) s = self.streamView.bounds.size;
    if (s.width < 10.0 || s.height < 10.0) return;
    [self.session updateViewportWidth:(NSInteger)(s.width + 0.5)
                                height:(NSInteger)(s.height + 0.5)
                                 force:force];
}

// ------------------------------------------------------------ connect flow

- (void)connectToServer:(NSDictionary *)server {
    NSString *endpoint = [RBServerStore normalizeEndpoint:[server objectForKey:@"lastEndpoint"]];
    if (!endpoint || ![[server objectForKey:@"fingerprint"] length]) {
        [self presentServersAllowingCancel:YES firstLaunch:NO message:@"Choose a valid paired Surf server."];
        return;
    }
    NSMutableDictionary *normalized = [server mutableCopy];
    [normalized setObject:endpoint forKey:@"lastEndpoint"];
    self.currentServer = normalized;
    [RBServerStore saveServer:normalized select:YES];
    [self leaveVideoMode];
    if (self.updateAlert) {
        self.updateAlert.delegate = nil;
        [self.updateAlert dismissWithClickedButtonIndex:self.updateAlert.cancelButtonIndex animated:NO];
        self.updateAlert = nil;
    }
    self.pendingClientUpdate = nil;
    RBSession *oldSession = self.session;
    oldSession.delegate = nil;
    [oldSession shutdown];
    [self.diagnostics resetConnection];
    self.session = [[RBSession alloc] initWithServer:normalized];
    self.session.delegate = self;
    self.session.interactionTracker.delegate = self;
    [self.session start];
}

- (void)interactionTracker:(RBInteractionTracker *)tracker
                 didSendID:(unsigned long long)interactionID {
}

- (void)presentSettingsAllowingCancel:(BOOL)allowsCancel message:(NSString *)message {
    if (self.settingsController && self.settingsController.view.window) return;
    self.settingsController = nil;
    self.serversController = nil;
    self.pairingController = nil;
    RBSettingsController *settings = [[RBSettingsController alloc] initWithSelectedServerID:[self.currentServer objectForKey:@"serverID"]];
    settings.delegate = self;
    settings.connected = self.session.state == RBSessionStateOpen;
    settings.diagnosticsVisible = self.debugVisible;
    settings.availableClientUpdate = self.pendingClientUpdate;
    self.settingsController = settings;
    UINavigationController *nav = [[RBModalNavigationController alloc] initWithRootViewController:settings];
    nav.modalPresentationStyle = RBIsPad() ? UIModalPresentationFormSheet : UIModalPresentationFullScreen;
    self.modalNavigationController = nav;
    [self presentViewController:nav animated:YES completion:nil];
}

- (void)presentServersAllowingCancel:(BOOL)allowsCancel firstLaunch:(BOOL)firstLaunch message:(NSString *)message {
    if (self.serversController && self.serversController.view.window) {
        if (message) [self.serversController setStatusText:message isError:YES];
        return;
    }
    RBServersController *servers = [[RBServersController alloc] initWithSelectedServerID:[self.currentServer objectForKey:@"serverID"]
                                                                             firstLaunch:firstLaunch];
    servers.delegate = self;
    servers.allowsCancel = allowsCancel;
    servers.connected = self.session.state == RBSessionStateOpen;
    self.serversController = servers;
    if (message) [servers setStatusText:message isError:YES];
    if (self.settingsController && self.settingsController.navigationController.view.window) {
        [self.settingsController.navigationController pushViewController:servers animated:YES];
        return;
    }
    self.settingsController = nil;
    self.pairingController = nil;
    UINavigationController *nav = [[RBModalNavigationController alloc] initWithRootViewController:servers];
    nav.modalPresentationStyle = RBIsPad() ? UIModalPresentationFormSheet : UIModalPresentationFullScreen;
    self.modalNavigationController = nav;
    [self presentViewController:nav animated:YES completion:nil];
}

// ---- settings delegate (chrome rethink) ----------------------------------

- (void)settings:(RBSettingsController *)settings clearData:(NSString *)what {
    [self.session sendMessage:@{@"t": @"clear", @"what": what ?: @""}];
}

- (void)settings:(RBSettingsController *)settings diagnosticsVisible:(BOOL)visible {
    [self setDebugVisible:visible];
}

- (void)settings:(RBSettingsController *)settings preference:(NSString *)key enabled:(BOOL)enabled {
    if ([key isEqualToString:RBDefaultsMobileLayoutKey]) {
        [self.session sendMessage:@{@"t": @"mobile", @"on": [NSNumber numberWithBool:enabled]}];
    } else if ([key isEqualToString:RBDefaultsDarkModeKey]) {
        [self applyAppearance];
        [self.session sendMessage:@{@"t": @"dark", @"on": [NSNumber numberWithBool:enabled]}];
    } else if ([key isEqualToString:RBDefaultsBottomBrowserBarKey]) {
        [self.view setNeedsLayout];
        [UIView animateWithDuration:0.20 animations:^{
            [self.view layoutIfNeeded];
        }];
    }
}

- (void)settingsWantsMediaControls:(RBSettingsController *)settings {
    self.settingsController = nil;
    self.serversController = nil;
    self.modalNavigationController = nil;
    [self dismissViewControllerAnimated:YES completion:^{
        UIButton *anchor = RBIsPad() ? self.chromeBar.moreButton : self.phoneToolbar.moreButton;
        [self presentMediaControlsFromButton:anchor];
    }];
}

- (void)settingsWantsDiagnosticsInspector:(RBSettingsController *)settings {
    self.settingsController = nil;
    self.serversController = nil;
    self.modalNavigationController = nil;
    [self dismissViewControllerAnimated:YES completion:^{
        [self setDebugVisible:YES];
        self.diagnosticsOverlay.displayMode = RBDiagnosticsOverlayExpanded;
        [self layoutDiagnosticsOverlayAnimated:YES];
    }];
}

- (void)settingsWantsClientUpdate:(RBSettingsController *)settings {
    NSDictionary *update = settings.availableClientUpdate;
    if (!update || self.updateAlert) return;
    self.pendingClientUpdate = update;
    self.settingsController = nil;
    self.serversController = nil;
    self.modalNavigationController = nil;
    [self dismissViewControllerAnimated:YES completion:^{
        double megabytes = [[update objectForKey:@"size"] doubleValue] / (1024.0 * 1024.0);
        NSString *message = [NSString stringWithFormat:@"Surf %@ is available for this iPad (%0.1f MB).",
                             [update objectForKey:@"version"] ?: @"?", megabytes];
        self.updateAlert = [[UIAlertView alloc] initWithTitle:@"Surf Update Available"
                                                      message:message delegate:self
                                            cancelButtonTitle:@"Not Now"
                                            otherButtonTitles:@"Update", nil];
        [self.updateAlert show];
    }];
}

- (void)settingsWantsServers:(RBSettingsController *)settings {
    [self presentServersAllowingCancel:NO firstLaunch:NO message:nil];
}

- (void)serversController:(RBServersController *)controller connectToServer:(NSDictionary *)server {
    [controller setConnectingServerID:[server objectForKey:@"serverID"]];
    [controller setStatusText:@"Connecting securely…" isError:NO];
    [self connectToServer:server];
}

- (void)serversController:(RBServersController *)controller
              pairEndpoint:(NSString *)endpoint
          expectedServerID:(NSString *)expectedServerID
         replacementServer:(NSDictionary *)replacementServer
                    qrToken:(NSString *)qrToken {
    RBPairingController *pairing = [[RBPairingController alloc] initWithEndpoint:endpoint
                                                                 expectedServerID:expectedServerID
                                                                replacementServer:replacementServer
                                                                           qrToken:qrToken];
    pairing.delegate = self;
    self.pairingController = pairing;
    [controller.navigationController pushViewController:pairing animated:YES];
}

- (void)serversControllerWantsQRScanner:(RBServersController *)controller {
    RBQRScannerController *scanner = [[RBQRScannerController alloc] init];
    scanner.delegate = self;
    UINavigationController *navigation = [[UINavigationController alloc] initWithRootViewController:scanner];
    [RBTheme styleNavigationBar:navigation.navigationBar];
    navigation.modalPresentationStyle = UIModalPresentationFullScreen;
    [controller presentViewController:navigation animated:YES completion:nil];
}

- (void)qrScannerDidCancel:(RBQRScannerController *)scanner {
    [scanner dismissViewControllerAnimated:YES completion:nil];
}

- (void)qrScanner:(RBQRScannerController *)scanner didScanValue:(NSString *)value {
    [scanner dismissViewControllerAnimated:YES completion:^{
        [self openPairingURL:[NSURL URLWithString:value]];
    }];
}

static NSString *RBPairQueryValue(NSURL *url, NSString *key) {
    for (NSString *item in [[url query] componentsSeparatedByString:@"&"]) {
        NSRange separator = [item rangeOfString:@"="];
        NSString *name = separator.location == NSNotFound ? item : [item substringToIndex:separator.location];
        if (![name isEqualToString:key]) continue;
        NSString *value = separator.location == NSNotFound ? @"" : [item substringFromIndex:separator.location + 1];
        return [value stringByReplacingPercentEscapesUsingEncoding:NSUTF8StringEncoding] ?: value;
    }
    return nil;
}

- (void)openPairingURL:(NSURL *)url {
    if (![[[url scheme] lowercaseString] isEqualToString:@"surf"] || ![[[url host] lowercaseString] isEqualToString:@"pair"]) return;
    NSString *host = RBPairQueryValue(url, @"h") ?: RBPairQueryValue(url, @"host");
    NSString *serverID = RBPairQueryValue(url, @"i") ?: RBPairQueryValue(url, @"id");
    NSString *token = RBPairQueryValue(url, @"t") ?: RBPairQueryValue(url, @"token");
    NSString *endpoint = [RBServerStore normalizeEndpoint:host];
    if (!endpoint || ![serverID length] || ![token length]) {
        [self presentServersAllowingCancel:YES firstLaunch:NO message:@"This is not a complete Surf pairing code."];
        return;
    }
    if (!self.serversController) [self presentServersAllowingCancel:YES firstLaunch:NO message:nil];
    [self serversController:self.serversController pairEndpoint:endpoint expectedServerID:serverID replacementServer:nil qrToken:token];
}

- (void)serversController:(RBServersController *)controller verifyAddress:(NSString *)endpoint forServer:(NSDictionary *)server {
    endpoint = [RBServerStore normalizeEndpoint:endpoint];
    if (!endpoint) { [controller setStatusText:@"Enter a valid HTTPS server address." isError:YES]; return; }
    NSUInteger generation = ++self.verificationGeneration;
    [controller setStatusText:@"Verifying the pinned server identity…" isError:NO];
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        NSError *error = nil;
        NSDictionary *info = [RBPairingClient inspectEndpoint:endpoint error:&error];
        BOOL valid = [[info objectForKey:@"serverID"] isEqualToString:[server objectForKey:@"serverID"]];
        dispatch_async(dispatch_get_main_queue(), ^{
            if (generation != self.verificationGeneration) return;
            if (!valid) {
                [controller setStatusText:[error localizedDescription] ?: @"The server at this address no longer matches the saved identity." isError:YES];
                return;
            }
            [RBServerStore addVerifiedEndpoint:endpoint transport:[info objectForKey:@"transport"] toServerID:[server objectForKey:@"serverID"]];
            NSDictionary *updated = [RBServerStore serverWithID:[server objectForKey:@"serverID"]];
            [controller reloadServers];
            [controller setStatusText:@"Address verified and saved." isError:NO];
            [self connectToServer:updated];
        });
    });
}

- (void)pairingController:(RBPairingController *)controller didPairServer:(NSDictionary *)server {
    self.serversController.pairingRequiredServerID = nil;
    [self.serversController reloadServers];
    [self.serversController setConnectingServerID:[server objectForKey:@"serverID"]];
    [self connectToServer:server];
}

- (void)pairingController:(RBPairingController *)controller foundKnownServer:(NSDictionary *)server endpoint:(NSString *)endpoint {
    [controller.navigationController popToViewController:self.serversController animated:YES];
    self.pairingController = nil;
    [self serversController:self.serversController verifyAddress:endpoint forServer:server];
}

- (void)pairingControllerDidCancel:(RBPairingController *)controller {
    self.pairingController = nil;
    if (self.serversController) [controller.navigationController popToViewController:self.serversController animated:YES];
    else [controller.navigationController popViewControllerAnimated:YES];
}

- (void)serversController:(RBServersController *)controller forgetServer:(NSDictionary *)server {
    NSString *serverID = [server objectForKey:@"serverID"];
    if ([[self.currentServer objectForKey:@"serverID"] isEqualToString:serverID]) {
        [self.session shutdown];
        self.session = nil;
        self.currentServer = nil;
        controller.connected = NO;
        [self.browserStateView showState:RBBrowserStateDisconnected detail:@"Choose or pair a Surf server."];
    }
    [RBServerStore forgetServerID:serverID];
    [controller reloadServers];
    [self.settingsController reloadServers];
}

- (void)serversControllerDidCancel:(RBServersController *)controller {
    self.serversController = nil;
    self.pairingController = nil;
    self.modalNavigationController = nil;
    [self dismissViewControllerAnimated:YES completion:nil];
}

- (void)settingsDismissed:(RBSettingsController *)settings {
    self.settingsController = nil;
    self.serversController = nil;
    self.pairingController = nil;
    self.modalNavigationController = nil;
    [self dismissViewControllerAnimated:YES completion:nil];
}

- (void)sessionDidAuthenticate:(RBSession *)session {
    if (session != self.session) return;
    self.pendingClientUpdate = session.availableClientUpdate;
    NSMutableDictionary *updated = [self.currentServer mutableCopy];
    [updated setObject:[NSDate date] forKey:@"lastConnected"];
    self.currentServer = updated;
    [RBServerStore saveServer:updated select:YES];
    [self.settingsController reloadServers];
    if (self.serversController) {
        self.serversController.connected = YES;
        [self.serversController setConnectingServerID:nil];
        [self.serversController setStatusText:@"Connected securely." isError:NO];
    }
    if (self.settingsController || self.serversController) {
        self.settingsController = nil;
        self.serversController = nil;
        self.pairingController = nil;
        self.modalNavigationController = nil;
        [self dismissViewControllerAnimated:YES completion:nil];
    }
}

- (void)sessionNeedsServer:(RBSession *)session message:(NSString *)message {
    if (session != self.session) return;
    NSString *resolvedMessage = message ?: @"Secure connection failed";
    if (session.requiresPairing) {
        resolvedMessage = @"Pairing Required. This device is no longer approved by the server. Tap the saved server to pair again.";
    }
    [self presentServersAllowingCancel:[[RBServerStore servers] count] > 0
                           firstLaunch:NO message:resolvedMessage];
    self.serversController.pairingRequiredServerID =
        session.requiresPairing ? [session.server objectForKey:@"serverID"] : nil;
}

- (void)session:(RBSession *)session requiresClientUpdate:(NSDictionary *)update {
    if (session != self.session || self.updateAlert) return;
    self.pendingClientUpdate = update;
    double megabytes = [[update objectForKey:@"size"] doubleValue] / (1024.0 * 1024.0);
    NSString *message = [NSString stringWithFormat:
                         @"This iPad has Surf %@. Update to Surf %@ (%0.1f MB) to connect.",
                         RBAppVersion, [update objectForKey:@"version"] ?: @"?", megabytes];
    self.updateAlert = [[UIAlertView alloc] initWithTitle:@"Surf Update Required"
                                                  message:message delegate:self
                                        cancelButtonTitle:@"Cancel"
                                        otherButtonTitles:@"Update", nil];
    [self.updateAlert show];
}

- (void)sessionRequiresServerUpdate:(RBSession *)session serverVersion:(NSString *)version {
    if (session != self.session) return;
    [[[UIAlertView alloc] initWithTitle:@"Server Update Required"
                               message:[NSString stringWithFormat:@"Update the Surf server (currently %@), then reconnect.", version ?: @"?"]
                              delegate:nil cancelButtonTitle:@"OK" otherButtonTitles:nil] show];
}

- (void)startClientUpdate {
    self.clientUpdater = [[RBClientUpdater alloc] initWithBaseURL:self.session.baseURL
                                                      fingerprint:[self.currentServer objectForKey:@"fingerprint"]
                                                           update:self.pendingClientUpdate];
    self.clientUpdater.delegate = self;
    self.connectionPill.hidden = NO;
    self.connectionPill.text = @"Downloading update…";
    [self.clientUpdater start];
}

- (void)clientUpdater:(RBClientUpdater *)updater progress:(double)progress {
    self.connectionPill.text = [NSString stringWithFormat:@"Downloading update… %d%%", (int)(progress * 100.0)];
}

- (void)clientUpdaterDidInstall:(RBClientUpdater *)updater {
    self.clientUpdater = nil;
    self.updateInstalledAlert = [[UIAlertView alloc] initWithTitle:@"Update Installed"
                                                           message:@"Surf must close to finish the update."
                                                          delegate:self cancelButtonTitle:@"Close Surf" otherButtonTitles:nil];
    [self.updateInstalledAlert show];
}

- (void)clientUpdater:(RBClientUpdater *)updater failed:(NSString *)message {
    self.clientUpdater = nil;
    self.connectionPill.hidden = YES;
    [[[UIAlertView alloc] initWithTitle:@"Update Failed" message:message delegate:nil
                     cancelButtonTitle:@"OK" otherButtonTitles:nil] show];
}

- (void)session:(RBSession *)session didChangeState:(RBSessionState)state {
    if (session != self.session) return;
    if (state != RBSessionStateOpen) {
        self.loading = NO;
        [self.chromeBar.omnibox setLoading:NO];
    }
    switch (state) {
        case RBSessionStateOpen:
            self.connectionPill.hidden = YES;
            if (self.browserStateView.state == RBBrowserStateConnecting ||
                self.browserStateView.state == RBBrowserStateStartingVideo ||
                self.browserStateView.state == RBBrowserStateReconnecting ||
                self.browserStateView.state == RBBrowserStateDisconnected) {
                [self.browserStateView showState:RBBrowserStateHidden detail:nil];
            }
            [self.view setNeedsLayout];
            [self.view layoutIfNeeded];
            [self sendCurrentViewportSizeForced:YES];
            [self.session sendMessage:@{@"t": @"mobile",
                                        @"on": [NSNumber numberWithBool:
                                                [[NSUserDefaults standardUserDefaults] boolForKey:RBDefaultsMobileLayoutKey]]}];
            [self.session sendMessage:@{@"t": @"dark",
                                        @"on": [NSNumber numberWithBool:
                                                [[NSUserDefaults standardUserDefaults] boolForKey:RBDefaultsDarkModeKey]]}];
            self.audioRequested = NO;
            break;
        case RBSessionStateConnecting:
            [self dismissSelectControllerSendingCancel:NO];
            self.connectionPill.hidden = YES;
            [self.browserStateView showState:RBBrowserStateConnecting detail:nil];
            [self leaveVideoMode];
            [self.mediaPipeline stopAudio];
            self.audioRequested = NO;
            break;
        case RBSessionStateRetrying:
            [self dismissSelectControllerSendingCancel:NO];
            self.connectionPill.hidden = YES;
            [self.browserStateView showState:RBBrowserStateReconnecting detail:nil];
            [self leaveVideoMode];
            self.audioRequested = NO;
            break;
        case RBSessionStateIdle:
            [self dismissSelectControllerSendingCancel:NO];
            self.connectionPill.hidden = YES;
            [self.browserStateView showState:RBBrowserStateDisconnected detail:nil];
            [self leaveVideoMode];
            [self.mediaPipeline stopAudio];
            self.audioRequested = NO;
            break;
    }
}

// ------------------------------------------------------------- video lane

// Local-only teardown (reconnects, server switches): the server side is
// cleaned up by its own disconnect path.
- (void)leaveVideoMode {
    if (!self.videoActive && !self.videoStarting) return;
    self.videoActive = NO;
    self.videoStarting = NO;
    self.videoProfile = nil;
    self.streamView.videoActive = NO;
    [self.mediaPipeline stopVideo];
}

- (void)handleVideoConfig:(NSDictionary *)message {
    NSString *state = [message objectForKey:@"state"];
    NSString *profile = [message objectForKey:@"profile"];
    if ([profile isKindOfClass:[NSString class]] && [profile length] > 0)
        self.videoProfile = profile;
    if ([state isEqualToString:@"starting"]) {
        self.videoStarting = YES;
        if (!self.currentURL || ![self.currentURL hasPrefix:@"about:blank#surf-new"])
            [self.browserStateView showState:RBBrowserStateStartingVideo detail:nil];
        return;
    }
    if ([state isEqualToString:@"unavailable"]) {
        [self leaveVideoMode];
        [self showVideoUnavailable:[[message objectForKey:@"reason"] description]];
        return;
    }
    if (![state isEqualToString:@"ready"] && ![[message objectForKey:@"ok"] boolValue]) return;
    if (![RBMediaPipeline videoAvailable]) {
        [self leaveVideoMode];
        [self showVideoUnavailable:@"videotoolbox-unavailable"];
        return;
    }
    [self.mediaPipeline configureVideoWidth:[[message objectForKey:@"w"] intValue]
                                     height:[[message objectForKey:@"h"] intValue]];
    [self.diagnostics resetVideoWindow];
    if (self.browserStateView.state == RBBrowserStateVideoUnavailable ||
        self.browserStateView.state == RBBrowserStateStartingVideo)
        [self.browserStateView showState:RBBrowserStateHidden detail:nil];
    self.videoActive = YES;
    self.streamView.videoActive = !self.applicationInBackground;
}

- (void)mediaPipeline:(RBMediaPipeline *)pipeline didDecodePixelBuffer:(CVPixelBufferRef)pixelBuffer
             metadata:(RBFrameMetadata *)metadata {
    if (!self.videoActive) return;
    [self.streamView displayVideoPixelBuffer:pixelBuffer metadata:metadata];
}

- (void)mediaPipeline:(RBMediaPipeline *)pipeline
 didAcceptSystemFrame:(RBFrameMetadata *)metadata {
    if (!self.videoActive) return;
    [self.streamView noteSystemFrameMetadata:metadata];
}

- (void)mediaPipeline:(RBMediaPipeline *)pipeline
didReplaceSystemDisplayLayer:(CALayer *)displayLayer {
    [self.streamView installSystemDisplayLayer:displayLayer];
}

- (void)streamView:(RBStreamView *)streamView didPresentMetadata:(RBFrameMetadata *)metadata {
    self.presentedSurfaceGeneration = metadata.encoderGeneration;
    if (self.awaitingPageFrame && self.awaitedSourceSequence > 0 &&
        metadata.sourceSequence >= self.awaitedSourceSequence) {
        self.awaitingPageFrame = NO;
        self.startPageView.hidden = YES;
    }
    // Starting AudioQueue while VideoToolbox, OpenGL, and the first IDR are
    // all initializing caused a short PCM burst and immediate drop-oldest
    // discontinuity on the A5. Start audio after the first displayed frame,
    // when the visual pipeline is already warm.
    if (!self.audioRequested && self.session.state == RBSessionStateOpen) {
        self.audioRequested = YES;
        [self.session sendMessage:@{@"t": @"audio", @"on": @YES}];
    }
    [self.session.interactionTracker didPresentInteractionID:metadata.interactionID];
}

- (void)mediaPipelineDidFailVideo:(RBMediaPipeline *)pipeline {
    RBLogEvent(@"media", @"error", @{@"lane": @"video", @"recovery": @"explicit_retry"}, @"Video decoder unavailable");
    [self leaveVideoMode];
    [self showVideoUnavailable:@"decoder-failed"];
}

- (void)showVideoUnavailable:(NSString *)reason {
    NSString *message = [NSString stringWithFormat:
                         @"The browser is still connected, but its video stream stopped (%@).",
                         [reason length] ? reason : @"unknown"];
    [self.browserStateView showState:RBBrowserStateVideoUnavailable detail:message];
}

- (void)mediaPipelineNeedsKeyframe:(RBMediaPipeline *)pipeline {
    [self.session sendMessage:@{@"t": @"reqkeyframe"}];
}

- (void)session:(RBSession *)session status:(NSString *)status {
    RBLogEvent(@"session", @"info", @{@"status": status ?: @""}, @"Session status changed");
}

// --------------------------------------------------------- incoming frames

- (void)session:(RBSession *)session didReceiveFrameData:(NSData *)data {
    if (self.applicationInBackground) return;
    [self.mediaPipeline consumeFrameData:data];
}

// ------------------------------------------------------- control messages

- (void)session:(RBSession *)session didReceiveControlMessage:(NSDictionary *)message {
    if ([self.diagnostics consumeControlMessage:message]) return;
    NSString *t = [message objectForKey:@"t"];
    if ([t isEqualToString:@"url"]) {
        NSString *url = [message objectForKey:@"url"];
        self.currentURL = url ?: @"";
        self.currentStarred = [[message objectForKey:@"starred"] boolValue];
        self.currentSecurity = [message objectForKey:@"security"];
        BOOL newTab = [self.currentURL hasPrefix:@"about:blank#surf-new"];
        if (newTab) {
            self.startPageView.hidden = NO;
            self.awaitingPageFrame = NO;
            self.awaitedSourceSequence = 0;
            if (self.browserStateView.state == RBBrowserStateStartingVideo)
                [self.browserStateView showState:RBBrowserStateHidden detail:nil];
            [self.chromeBar.omnibox setURLText:@""];
            [self.session sendMessage:@{@"t": @"hist"}];
        } else if (url) {
            if (!self.awaitingPageFrame) self.startPageView.hidden = YES;
            [self.chromeBar.omnibox setURLText:url];
        }
        [self.chromeBar.omnibox setStarred:self.currentStarred];
        [self.chromeBar.omnibox setSecurityState:self.currentSecurity];
        [self hideErrorCard];
    } else if ([t isEqualToString:@"histstate"]) {
        self.canGoBack = [[message objectForKey:@"back"] boolValue];
        self.canGoForward = [[message objectForKey:@"fwd"] boolValue];
        [self.chromeBar setCanGoBack:self.canGoBack forward:self.canGoForward];
        [self.phoneToolbar setCanGoBack:self.canGoBack forward:self.canGoForward];
        self.fullscreenBackButton.enabled = self.canGoBack;
        self.fullscreenForwardButton.enabled = self.canGoForward;
    } else if ([t isEqualToString:@"loading"]) {
        self.loading = [[message objectForKey:@"on"] boolValue];
        [self.chromeBar.omnibox setLoading:self.loading];
        if (self.loading) [self hideErrorCard];
    } else if ([t isEqualToString:@"editable"]) {
        if ([[message objectForKey:@"on"] boolValue]) {
            BOOL keyboardWasOpen = [self.hiddenInput isFirstResponder];
            [self configureKeyboardForKind:[message objectForKey:@"kind"] rect:[message objectForKey:@"rect"]];
            if (keyboardWasOpen || [[message objectForKey:@"show"] boolValue]) [self showKeyboard];
        } else {
            self.editableHasRect = NO;
            [self hidePageKeyboard];
        }
    } else if ([t isEqualToString:@"select"]) {
        [self presentSelectMessage:message];
    } else if ([t isEqualToString:@"video-config"]) {
        [self handleVideoConfig:message];
    } else if ([t isEqualToString:@"fullscreen"]) {
        [self setFullscreen:[[message objectForKey:@"on"] boolValue] notifyPage:NO];
    } else if ([t isEqualToString:@"audio-config"]) {
        if ([[message objectForKey:@"ok"] boolValue] && !self.applicationInBackground) {
            [self.mediaPipeline configureAudioSampleRate:[[message objectForKey:@"rate"] intValue]
                                                channels:[[message objectForKey:@"channels"] intValue]];
            RBLogEvent(@"media", @"info", @{@"lane": @"audio", @"state": @"ready"}, @"Audio lane ready");
        } else {
            [self.mediaPipeline stopAudio];
            RBLogEvent(@"media", @"info", @{@"lane": @"audio", @"state": @"stopped"}, @"Audio lane stopped");
        }
    } else if ([t isEqualToString:@"found"]) {
        [self.findBar setFound:[[message objectForKey:@"on"] boolValue]];
    } else if ([t isEqualToString:@"download"]) {
        [self showToast:[NSString stringWithFormat:@"Downloaded %@", [message objectForKey:@"name"] ?: @""]];
    } else if ([t isEqualToString:@"downloads"]) {
        [self.libraryController setDownloads:[message objectForKey:@"items"]];
    } else if ([t isEqualToString:@"tabs"]) {
        id tabs = [message objectForKey:@"tabs"];
        NSArray *nextTabs = [tabs isKindOfClass:[NSArray class]] ? tabs : nil;
        NSNumber *oldActiveKey = [self activeTabKey];
        NSNumber *nextActiveKey = nil;
        for (NSDictionary *tab in nextTabs) {
            if ([[tab objectForKey:@"active"] boolValue]) {
                nextActiveKey = [tab objectForKey:@"id"];
                break;
            }
        }
        if (!RBIsPad() && oldActiveKey && nextActiveKey && ![oldActiveKey isEqual:nextActiveKey]) {
            [self cacheThumbnailForTabKey:oldActiveKey];
        }
        self.lastTabs = nextTabs;
        [self.tabStrip setTabs:self.lastTabs baseURL:self.session.baseURL
                    fingerprint:[self.currentServer objectForKey:@"fingerprint"]];
        [self.phoneToolbar setTabCount:[self.lastTabs count]];
        NSMutableSet *liveTabIDs = [NSMutableSet set];
        NSString *activeTitle = nil;
        for (NSDictionary *tab in self.lastTabs) {
            NSNumber *tabID = [tab objectForKey:@"id"];
            if (tabID) [liveTabIDs addObject:tabID];
            if ([[tab objectForKey:@"active"] boolValue]) {
                activeTitle = [tab objectForKey:@"title"];
                NSString *activeURL = [tab objectForKey:@"url"];
                if ([activeURL hasPrefix:@"about:blank#surf-new"] ||
                    [activeTitle hasPrefix:@"about:blank#surf-new"]) {
                    activeTitle = @"New Tab";
                } else if (![activeTitle length] || [activeTitle isEqualToString:activeURL]) {
                    activeTitle = [[NSURL URLWithString:activeURL] host];
                    if (![activeTitle length]) activeTitle = @"New Tab";
                }
            }
        }
        self.chromeBar.pageTitle = activeTitle;
        for (NSNumber *tabID in [self.tabThumbnails allKeys]) {
            if (![liveTabIDs containsObject:tabID]) {
                [self.tabThumbnails removeObjectForKey:tabID];
                [self.thumbnailLRU removeObject:tabID];
            }
        }
        [self.pageSwitcherController updateTabs:self.lastTabs thumbnails:self.tabThumbnails];
    } else if ([t isEqualToString:@"hist"]) {
        self.bookmarks = [[message objectForKey:@"bookmarks"] isKindOfClass:[NSArray class]]
            ? [message objectForKey:@"bookmarks"] : @[];
        [self.libraryController setBookmarks:self.bookmarks];
        [self.startPageView setFavorites:self.bookmarks];
    } else if ([t isEqualToString:@"media-state"]) {
        [self.pageMediaController applyState:message];
    } else if ([t isEqualToString:@"pageframe"]) {
        self.awaitedSourceSequence = [[message objectForKey:@"sourceSeq"] unsignedIntValue];
        if (self.awaitingPageFrame &&
            self.streamView.lastPresentedSourceSequence >= self.awaitedSourceSequence) {
            self.awaitingPageFrame = NO;
            self.startPageView.hidden = YES;
        }
    } else if ([t isEqualToString:@"starred"]) {
        self.currentStarred = [[message objectForKey:@"on"] boolValue];
        [self.chromeBar.omnibox setStarred:self.currentStarred];
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
    } else if ([t isEqualToString:@"security"]) {
        self.currentSecurity = [message objectForKey:@"state"];
        [self.chromeBar.omnibox setSecurityState:self.currentSecurity];
    } else if ([t isEqualToString:@"pageerror"]) {
        [self showErrorCardForURL:[message objectForKey:@"url"]];
    } else if ([t isEqualToString:@"reader"]) {
        [self handleReaderReply:message];
    } else if ([t isEqualToString:@"history"]) {
        [self.libraryController consumeHistoryReply:message];
    } else if ([t isEqualToString:@"log-request"]) {
        [self.session uploadNativeLogNow];
    } else if ([t isEqualToString:@"log-clear"]) {
        __weak RBRootViewController *weakSelf = self;
        RBClearLogWithCompletion(^{
            RBRootViewController *controller = weakSelf;
            if (!controller || controller.session.state != RBSessionStateOpen) return;
            [controller.session sendMessage:@{@"t": @"log-cleared"}];
        });
    } else if ([t isEqualToString:@"clipboard-sync"]) {
        BOOL enabled = [[message objectForKey:@"enabled"] boolValue];
        BOOL known = [[message objectForKey:@"known"] boolValue];
        NSString *text = [message objectForKey:@"text"];
        [self setClipboardSyncEnabled:enabled];
        if (enabled && known && RBValidClipboardText(text)) {
            [UIPasteboard generalPasteboard].string = text;
            self.clipboardChangeCount = [UIPasteboard generalPasteboard].changeCount;
            RBLogEvent(@"clipboard", @"info",
                       @{ @"direction": @"host-to-device",
                          @"bytes": @([[text dataUsingEncoding:NSUTF8StringEncoding] length]) },
                       @"Synchronized host clipboard to device");
        } else if (enabled && !known) {
            [self sendCurrentClipboardIfValid];
        }
    } else if ([t isEqualToString:@"clipboard"]) {
        NSString *requestID = [message objectForKey:@"id"];
        NSString *text = [message objectForKey:@"text"];
        BOOL synchronized = [[message objectForKey:@"sync"] boolValue];
        BOOL valid = RBValidClipboardText(text) &&
                     (synchronized || ([requestID isKindOfClass:[NSString class]] && [requestID length]));
        if (valid) {
            [UIPasteboard generalPasteboard].string = text;
            self.clipboardChangeCount = [UIPasteboard generalPasteboard].changeCount;
            [self showToast:(synchronized ? @"Clipboard synchronized" : @"Copied to clipboard")];
            RBLogEvent(@"clipboard", @"info",
                       @{ @"direction": @"host-to-device",
                          @"bytes": @([[text dataUsingEncoding:NSUTF8StringEncoding] length]),
                          @"synchronized": @(synchronized),
                          @"expires_seconds": @(synchronized ? 0 : 120) },
                       synchronized ? @"Synchronized host clipboard to device" : @"Host text copied to device clipboard");
            if (!synchronized) {
                NSString *delivered = [text copy];
                dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(120.0 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
                    if ([[UIPasteboard generalPasteboard].string isEqualToString:delivered]) {
                        [UIPasteboard generalPasteboard].string = @"";
                        self.clipboardChangeCount = [UIPasteboard generalPasteboard].changeCount;
                        RBLogEvent(@"clipboard", @"info", @{}, @"Expired host-delivered clipboard text");
                    }
                });
            }
        }
        if ([requestID isKindOfClass:[NSString class]] && [requestID length]) {
            [self.session sendMessage:@{ @"t": @"clipboard-result", @"id": requestID, @"ok": @(valid) }];
        }
    }
}

// ----------------------------------------------------------- physical input

- (NSValue *)keyForPageTouch:(UITouch *)touch {
    return [NSValue valueWithNonretainedObject:touch];
}

- (NSDictionary *)wirePointForTouch:(UITouch *)touch identifier:(NSNumber *)identifier {
    CGSize size = self.remoteViewportSize;
    if (size.width < 1.0 || size.height < 1.0) size = self.streamView.bounds.size;
    CGPoint point = [touch locationInView:self.streamView];
    CGFloat width = MAX(1.0, size.width), height = MAX(1.0, size.height);
    CGFloat radius = 1.0;
    if ([touch respondsToSelector:@selector(majorRadius)]) radius = [touch majorRadius];
    return @{@"id": identifier,
             @"x": [NSNumber numberWithFloat:MIN(1.0, MAX(0.0, point.x / width))],
             @"y": [NSNumber numberWithFloat:MIN(1.0, MAX(0.0, point.y / height))],
             @"rx": [NSNumber numberWithFloat:MIN(1.0, radius / width)],
             @"ry": [NSNumber numberWithFloat:MIN(1.0, radius / height)],
             @"force": @0.5};
}

- (unsigned long long)wireTimestampForTouches:(NSSet *)touches {
    NSTimeInterval latest = 0.0;
    for (UITouch *touch in touches) latest = MAX(latest, touch.timestamp);
    if (latest <= 0.0) latest = CACurrentMediaTime();
    return (unsigned long long)(latest * 1000000000.0);
}

- (void)sendPageTouchPhase:(NSString *)phase touches:(NSSet *)touches {
    if (!self.presentedSurfaceGeneration) return;
    NSMutableArray *points = [NSMutableArray array];
    for (UITouch *touch in touches) {
        NSNumber *identifier = [self.pageTouchIDs objectForKey:[self keyForPageTouch:touch]];
        if (identifier) [points addObject:[self wirePointForTouch:touch identifier:identifier]];
    }
    if ([phase isEqualToString:@"cancel"]) [points removeAllObjects];
    if (![phase isEqualToString:@"cancel"] && ![points count]) return;
    [self.session sendTouchPhase:phase points:points
                       timestamp:[self wireTimestampForTouches:touches]
                         surface:self.presentedSurfaceGeneration];
}

- (void)streamView:(RBStreamView *)streamView touchesBegan:(NSSet *)touches withEvent:(UIEvent *)event {
    if (self.chromeBar.omnibox.editing) {
        [self.chromeBar.omnibox dismissKeyboard];
        [self.suggestPanel hide];
        return;
    }
    if (!self.edgeTouch && [self.pageTouchIDs count] == 0 && [touches count] == 1) {
        UITouch *candidate = [touches anyObject];
        CGPoint point = [candidate locationInView:self.streamView];
        CGFloat width = self.streamView.bounds.size.width;
        if (point.x < 24.0 || point.x > width - 24.0) {
            self.edgeTouch = candidate;
            self.edgeSwipe = point.x < 24.0 ? -1 : 1;
            self.edgeStart = point;
            return;
        }
    }
    BOOL wasEmpty = [self.pageTouchIDs count] == 0;
    for (UITouch *touch in touches) {
        if (touch == self.edgeTouch || !self.presentedSurfaceGeneration) continue;
        NSValue *key = [self keyForPageTouch:touch];
        if (![self.pageTouchIDs objectForKey:key]) {
            [self.pageTouchIDs setObject:[NSNumber numberWithUnsignedInteger:self.nextTouchID++] forKey:key];
        }
    }
    if (wasEmpty && [self.pageTouchIDs count]) [self.streamView beginMotionWindow];
    [self sendPageTouchPhase:@"start" touches:touches];
}

- (void)streamView:(RBStreamView *)streamView touchesMoved:(NSSet *)touches withEvent:(UIEvent *)event {
    // Move packets are complete snapshots of the page-owned contacts. Both
    // socket and server are then free to replace an older queued move without
    // losing a simultaneous finger that did not change in this UIEvent.
    NSSet *activeTouches = [event allTouches] ?: touches;
    [self sendPageTouchPhase:@"move" touches:activeTouches];
    if ([self.pageTouchIDs count]) [self.streamView continueMotionWindow];
}

- (void)streamView:(RBStreamView *)streamView touchesEnded:(NSSet *)touches withEvent:(UIEvent *)event {
    if (self.edgeTouch && [touches containsObject:self.edgeTouch]) {
        CGPoint point = [self.edgeTouch locationInView:self.streamView];
        CGFloat travel = point.x - self.edgeStart.x;
        if (self.edgeSwipe == -1 && travel > 70.0) [self.session sendMessage:@{@"t": @"back"}];
        else if (self.edgeSwipe == 1 && travel < -70.0) [self.session sendMessage:@{@"t": @"fwd"}];
        self.edgeTouch = nil;
        self.edgeSwipe = 0;
    }
    [self sendPageTouchPhase:@"end" touches:touches];
    for (UITouch *touch in touches) [self.pageTouchIDs removeObjectForKey:[self keyForPageTouch:touch]];
    if (![self.pageTouchIDs count]) [self.streamView endMotionWindow];
}

- (void)streamView:(RBStreamView *)streamView touchesCancelled:(NSSet *)touches withEvent:(UIEvent *)event {
    if ([self.pageTouchIDs count]) [self sendPageTouchPhase:@"cancel" touches:touches];
    [self.pageTouchIDs removeAllObjects];
    self.edgeTouch = nil;
    self.edgeSwipe = 0;
    [self.streamView endMotionWindow];
}

// ------------------------------------------------------------- chrome bar

- (void)chromeBack:(RBChromeBar *)bar { [self.session sendMessage:@{@"t": @"back"}]; }
- (void)chromeForward:(RBChromeBar *)bar { [self.session sendMessage:@{@"t": @"fwd"}]; }

- (void)chrome:(RBChromeBar *)bar shareFromButton:(UIButton *)button {
    [self presentShareFromButton:button];
}

- (void)chrome:(RBChromeBar *)bar moreFromButton:(UIButton *)button {
    [self presentMoreFromButton:button];
}

- (void)phoneToolbarBack:(RBPhoneToolbar *)toolbar {
    [self.session sendMessage:@{@"t": @"back"}];
}

- (void)phoneToolbarForward:(RBPhoneToolbar *)toolbar {
    [self.session sendMessage:@{@"t": @"fwd"}];
}

- (void)phoneToolbar:(RBPhoneToolbar *)toolbar shareFromButton:(UIButton *)button {
    [self presentShareFromButton:button];
}

- (void)phoneToolbar:(RBPhoneToolbar *)toolbar pagesFromButton:(UIButton *)button {
    [self presentPageSwitcher];
}

- (void)phoneToolbar:(RBPhoneToolbar *)toolbar moreFromButton:(UIButton *)button {
    [self presentMoreFromButton:button];
}

- (RBActionActivity *)activityForAction:(NSString *)action title:(NSString *)title icon:(RBIcon)icon {
    __weak RBRootViewController *weakSelf = self;
    return [[RBActionActivity alloc]
            initWithType:[@"space.seg6.surf." stringByAppendingString:action]
                   title:title
                   image:[RBTheme icon:icon size:48.0 color:[UIColor blackColor]]
                 handler:^{
        weakSelf.pendingActivityAction = action;
        RBLogEvent(@"activity", @"info", @{@"action": action ?: @""}, @"Page action selected");
    }];
}

- (void)activityControllerDidFinish {
    NSString *action = self.pendingActivityAction;
    self.pendingActivityAction = nil;
    self.activityController = nil;
    if ([action length]) {
        [self performSelector:@selector(performActivityActionWhenReady:)
                   withObject:action afterDelay:0.05];
    }
}

- (void)performActivityActionWhenReady:(NSString *)action {
    if (self.presentedViewController) {
        [self performSelector:@selector(performActivityActionWhenReady:)
                   withObject:action afterDelay:0.10];
        return;
    }
    // UIActivityViewController's iOS 6 completion handler can run after the
    // presented-controller link clears but before the dismissal animation is
    // fully settled. Give that legacy transition one final beat before trying
    // to present Media Controls or Surf Settings.
    [self performSelector:@selector(handlePageAction:) withObject:action afterDelay:0.35];
}

- (void)presentActivityController:(UIActivityViewController *)controller fromButton:(UIButton *)button {
    [self dismissPopover];
    self.pendingActivityAction = nil;
    self.activityController = controller;
    __weak RBRootViewController *weakSelf = self;
    if ([controller respondsToSelector:@selector(setCompletionWithItemsHandler:)]) {
        controller.completionWithItemsHandler = ^(NSString *activityType, BOOL completed,
                                                   NSArray *returnedItems, NSError *activityError) {
            [weakSelf activityControllerDidFinish];
        };
    } else {
        controller.completionHandler = ^(NSString *activityType, BOOL completed) {
            [weakSelf activityControllerDidFinish];
        };
    }
    if (RBIsPad()) {
        if ([controller respondsToSelector:@selector(popoverPresentationController)]) {
            UIPopoverPresentationController *presentation = controller.popoverPresentationController;
            presentation.sourceView = button;
            presentation.sourceRect = button.bounds;
            presentation.permittedArrowDirections = [self browserChromeArrowDirection];
            [self presentViewController:controller animated:YES completion:nil];
        } else {
            UIPopoverController *popover = [[UIPopoverController alloc] initWithContentViewController:controller];
            // System share destinations keep their native light sheet. Their
            // icons and labels are not authored for Surf's dark surface.
            popover.delegate = self;
            self.popover = popover;
            CGRect anchor = [button convertRect:button.bounds toView:self.view];
            [popover presentPopoverFromRect:anchor inView:self.view
                   permittedArrowDirections:[self browserChromeArrowDirection] animated:YES];
        }
    } else {
        [self presentViewController:controller animated:YES completion:nil];
    }
}

- (void)presentShareFromButton:(UIButton *)button {
    NSString *urlText = self.currentURL ?: [self.chromeBar.omnibox currentText];
    NSURL *url = [NSURL URLWithString:urlText ?: @""];
    id shareItem = url ?: (urlText ?: @"");
    NSString *pageTitle = self.chromeBar.pageTitle;
    NSMutableArray *shareItems = [NSMutableArray array];
    if ([pageTitle length]) [shareItems addObject:pageTitle];
    [shareItems addObject:shareItem];
    RBActionActivity *bookmark = [self activityForAction:@"bookmark"
                                                   title:(self.currentStarred ? @"Remove Bookmark" : @"Add Bookmark")
                                                    icon:(self.currentStarred ? RBIconStarFill : RBIconStar)];
    UIActivityViewController *controller = [[UIActivityViewController alloc]
                                             initWithActivityItems:shareItems
                                             applicationActivities:@[bookmark]];
    [self presentActivityController:controller fromButton:button];
}

- (void)presentMoreFromButton:(UIButton *)button {
    [self dismissPopover];
    [self dismissActionMenuAnimated:NO performingAction:nil];
    NSArray *items = @[
        [RBActionMenuItem itemWithTitle:@"Library" action:@"library" icon:RBIconBook],
        [RBActionMenuItem itemWithTitle:@"Reader" action:@"reader" icon:RBIconReader],
        [RBActionMenuItem itemWithTitle:@"Find on Page" action:@"find" icon:RBIconSearch],
        [RBActionMenuItem itemWithTitle:@"Media Controls" action:@"media" icon:RBIconMedia],
        [RBActionMenuItem itemWithTitle:(self.fullscreen ? @"Exit Fullscreen" : @"Fullscreen")
                                  action:@"fullscreen"
                                    icon:(self.fullscreen ? RBIconShrink : RBIconExpand)],
        [RBActionMenuItem itemWithTitle:@"Settings" action:@"settings" icon:RBIconSliders]
    ];
    BOOL phone = !RBIsPad();
    RBActionMenuController *controller = [[RBActionMenuController alloc] initWithItems:items phoneLayout:phone];
    __weak RBRootViewController *weakSelf = self;
    controller.onSelect = ^(NSString *action) {
        [weakSelf dismissActionMenuAnimated:YES performingAction:action];
    };
    controller.onDismiss = ^{
        [weakSelf dismissActionMenuAnimated:YES performingAction:nil];
    };
    self.actionMenuController = controller;
    if (phone) {
        [self addChildViewController:controller];
        controller.view.frame = self.view.bounds;
        controller.view.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
        [self.view addSubview:controller.view];
        [controller didMoveToParentViewController:self];
        [controller showAnimated:YES];
    } else {
        CGSize menuSize = [controller preferredSize];
        controller.contentSizeForViewInPopover = menuSize;
        if ([controller respondsToSelector:@selector(setPreferredContentSize:)]) {
            controller.preferredContentSize = menuSize;
        }
        UIPopoverController *popover = [[UIPopoverController alloc] initWithContentViewController:controller];
        popover.popoverContentSize = menuSize;
        [RBTheme stylePopoverController:popover];
        popover.delegate = self;
        self.popover = popover;
        CGRect anchor = [button convertRect:button.bounds toView:self.view];
        [popover presentPopoverFromRect:anchor inView:self.view
               permittedArrowDirections:[self browserChromeArrowDirection] animated:YES];
    }
}

- (void)dismissActionMenuAnimated:(BOOL)animated performingAction:(NSString *)action {
    RBActionMenuController *controller = self.actionMenuController;
    if (!controller) {
        if ([action length]) [self handlePageAction:action];
        return;
    }
    self.actionMenuController = nil;
    if (RBIsPad()) {
        if (self.popover.popoverVisible) [self.popover dismissPopoverAnimated:animated];
        self.popover = nil;
        if ([action length]) {
            [self performSelector:@selector(handlePageAction:) withObject:action
                       afterDelay:(animated ? 0.22 : 0.0)];
        }
        return;
    }
    [controller dismissAnimated:animated completion:^{
        [controller willMoveToParentViewController:nil];
        [controller.view removeFromSuperview];
        [controller removeFromParentViewController];
        if ([action length]) [self handlePageAction:action];
    }];
}

- (void)handlePageAction:(NSString *)action {
    RBLogEvent(@"activity", @"info", @{@"action": action ?: @""}, @"Page action executing");
    if ([action isEqualToString:@"forward"]) {
        [self.session sendMessage:@{@"t": @"fwd"}];
    } else if ([action isEqualToString:@"library"]) {
        [self presentLibrary];
    } else if ([action isEqualToString:@"settings"]) {
        [self presentSettingsAllowingCancel:YES message:nil];
    } else if ([action isEqualToString:@"media"]) {
        UIButton *anchor = RBIsPad() ? self.chromeBar.moreButton : self.phoneToolbar.moreButton;
        [self presentMediaControlsFromButton:anchor];
    } else if ([action isEqualToString:@"reader"]) {
        self.readerPending = YES;
        [self.session sendMessage:@{@"t": @"reader"}];
        [self showToast:@"Preparing reader…"];
    } else if ([action isEqualToString:@"find"]) {
        self.findVisible = YES;
        [self.view setNeedsLayout];
        [self.view layoutIfNeeded];
        [self.findBar focusField];
    } else if ([action isEqualToString:@"bookmark"]) {
        [self.session sendMessage:@{@"t": @"bookmark"}];
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

- (void)presentMediaControlsFromButton:(UIButton *)button {
    [self dismissPopover];
    RBMediaController *controller = [[RBMediaController alloc] init];
    controller.delegate = self;
    self.pageMediaController = controller;
    if (RBIsPad()) {
        UIPopoverController *popover = [[UIPopoverController alloc] initWithContentViewController:controller];
        [RBTheme stylePopoverController:popover];
        popover.delegate = self;
        self.popover = popover;
        CGRect anchor = [button convertRect:button.bounds toView:self.view];
        [popover presentPopoverFromRect:anchor inView:self.view
               permittedArrowDirections:[self browserChromeArrowDirection] animated:YES];
    } else {
        controller.navigationItem.rightBarButtonItem =
            [[UIBarButtonItem alloc] initWithBarButtonSystemItem:UIBarButtonSystemItemDone
                                                          target:self action:@selector(compactPopoverDone:)];
        UINavigationController *nav = [[UINavigationController alloc] initWithRootViewController:controller];
        [RBTheme styleNavigationBar:nav.navigationBar];
        nav.modalPresentationStyle = UIModalPresentationFullScreen;
        self.compactPopoverController = nav;
        [self presentViewController:nav animated:YES completion:nil];
    }
    [self.session sendMessage:@{@"t": @"media-query"}];
}

- (void)mediaControllerTogglePlayback:(RBMediaController *)controller {
    [self.session sendMessage:@{@"t": @"media-playpause"}];
}

- (void)mediaControllerToggleMute:(RBMediaController *)controller {
    [self.session sendMessage:@{@"t": @"media-mute"}];
}

- (void)mediaController:(RBMediaController *)controller setVolume:(CGFloat)volume {
    [self.session sendMessage:@{@"t": @"media-volume", @"value": [NSNumber numberWithFloat:volume]}];
}

- (void)mediaControllerRequestsRefresh:(RBMediaController *)controller {
    [self.session sendMessage:@{@"t": @"media-query"}];
}

- (void)mailCurrentPage {
    if (![MFMailComposeViewController canSendMail]) {
        [self showToast:@"Mail is not set up on this device"];
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
    [self presentLibraryFromButton:button];
}

// Settings (gear): straight to settings — there is no menu.
- (void)pasteToPage {
    NSString *text = [UIPasteboard generalPasteboard].string;
    if (![text length] || !RBValidClipboardText(text)) {
        [self showToast:@"Clipboard is empty"];
        return;
    }
    [self.session sendMessage:@{@"t": @"paste", @"text": text}];
    [self showToast:@"Pasted to page"];
}

- (void)escapePageInput {
    [self sendKeyName:@"Escape" keyCode:27];
}

- (void)tabPageInput {
    [self sendKeyName:@"Tab" keyCode:9];
}

- (void)toggleFullscreen {
    [self setFullscreen:!self.fullscreen notifyPage:YES];
}

- (void)setFullscreen:(BOOL)fullscreen notifyPage:(BOOL)notifyPage {
    if (self.fullscreen == fullscreen) {
        if (notifyPage)
            [self.session sendMessage:@{@"t": @"fullscreen", @"on": [NSNumber numberWithBool:fullscreen]}];
        return;
    }
    self.viewportTransitioning = YES;
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(sendCurrentViewportSize) object:nil];
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(sendCurrentViewportSizeForcedAfterTransition) object:nil];
    self.fullscreen = fullscreen;
    [self.view setNeedsLayout];
    [self.view layoutIfNeeded];
    [self finishViewportTransition];
    if (notifyPage)
        [self.session sendMessage:@{@"t": @"fullscreen", @"on": [NSNumber numberWithBool:fullscreen]}];
}

- (void)fullscreenBackTapped:(id)sender {
    [self.session sendMessage:@{@"t": @"back"}];
}

- (void)fullscreenForwardTapped:(id)sender {
    [self.session sendMessage:@{@"t": @"fwd"}];
}

// ---------------------------------------------------------------- omnibox

- (void)omnibox:(RBOmnibox *)omnibox navigateTo:(NSString *)text {
    [self.suggestPanel hide];
    self.awaitingPageFrame = !self.startPageView.hidden;
    self.awaitedSourceSequence = 0;
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

- (void)omniboxEditingBegan:(RBOmnibox *)omnibox {
    BOOL classic = [RBTheme usesClassicAppearance];
    RBLogEvent(@"omnibox", @"info", @{@"phase": @"begin", @"classic": @(classic)},
               @"Address editing began");
    if (!classic) {
        [self.chromeBar setOmniboxExpanded:YES animated:YES];
        return;
    }
    // iOS 6 is still assembling UITextField's responder graph when its begin
    // delegate runs. Resizing the field in that callback corrupts UIKit's
    // private selection/touch state, so cross the run-loop boundary first.
    __weak RBRootViewController *weakSelf = self;
    dispatch_async(dispatch_get_main_queue(), ^{
        RBRootViewController *controller = weakSelf;
        if (controller && omnibox.editing)
            [controller.chromeBar setOmniboxExpanded:YES animated:NO];
    });
}

- (void)omniboxEditingEnded:(RBOmnibox *)omnibox {
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(fireSuggest) object:nil];
    [self.suggestPanel hide];
    BOOL classic = [RBTheme usesClassicAppearance];
    RBLogEvent(@"omnibox", @"info", @{@"phase": @"end", @"classic": @(classic)},
               @"Address editing ended");
    [self.chromeBar setOmniboxExpanded:NO animated:!classic];
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
    self.awaitingPageFrame = !self.startPageView.hidden;
    self.awaitedSourceSequence = 0;
    [self.session sendMessage:@{@"t": @"nav", @"url": url}];
}

// ------------------------------------------------------------ native new tab

- (void)newTabViewWantsOmnibox:(RBNewTabView *)view {
    [self.chromeBar.omnibox focus];
}

- (void)newTabView:(RBNewTabView *)view openURL:(NSString *)url {
    self.awaitingPageFrame = YES;
    self.awaitedSourceSequence = 0;
    [self.session sendMessage:@{@"t": @"nav", @"url": url ?: @""}];
}

- (void)newTabViewWantsLibrary:(RBNewTabView *)view {
    [self presentLibrary];
}

// ------------------------------------------------------------ browser states

- (void)reconnectCurrentServer {
    NSDictionary *server = self.currentServer ?: [RBServerStore lastSelectedServer];
    if (server) [self connectToServer:server];
    else [self presentServersAllowingCancel:NO firstLaunch:YES message:@"Add a Surf server to reconnect."];
}

- (void)browserStateViewPrimaryAction:(RBBrowserStateView *)view {
    switch (view.state) {
        case RBBrowserStatePageError:
            [view showState:RBBrowserStateHidden detail:nil];
            [self.session sendMessage:@{@"t": @"reload"}];
            break;
        case RBBrowserStateVideoUnavailable:
            [view showState:RBBrowserStateConnecting detail:@"Restarting the video stream…"];
            [self.session sendMessage:@{@"t": @"video-retry"}];
            break;
        case RBBrowserStateReconnecting:
        case RBBrowserStateDisconnected:
            [self reconnectCurrentServer];
            break;
        default:
            break;
    }
}

- (void)browserStateViewSecondaryAction:(RBBrowserStateView *)view {
    switch (view.state) {
        case RBBrowserStatePageError:
            [view showState:RBBrowserStateHidden detail:nil];
            [self.session sendMessage:@{@"t": @"back"}];
            break;
        case RBBrowserStateVideoUnavailable:
            [self reconnectCurrentServer];
            break;
        default:
            [self presentServersAllowingCancel:YES firstLaunch:NO message:nil];
            break;
    }
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

// --------------------------------------------------------- phone pages

- (NSNumber *)activeTabKey {
    for (NSDictionary *tab in self.lastTabs) {
        if ([[tab objectForKey:@"active"] boolValue]) return [tab objectForKey:@"id"];
    }
    return nil;
}

- (void)cacheThumbnailForTabKey:(NSNumber *)tabKey {
    if (!tabKey) return;
    UIImage *image = [self.streamView snapshotImageWithMaximumSize:CGSizeMake(280.0, 360.0)];
    if (!image) return;
    [self.tabThumbnails setObject:image forKey:tabKey];
    [self.thumbnailLRU removeObject:tabKey];
    [self.thumbnailLRU addObject:tabKey];
    while ([self.thumbnailLRU count] > 12) {
        NSNumber *oldest = [self.thumbnailLRU objectAtIndex:0];
        [self.thumbnailLRU removeObjectAtIndex:0];
        [self.tabThumbnails removeObjectForKey:oldest];
    }
}

- (void)cacheActiveTabThumbnail {
    [self cacheThumbnailForTabKey:[self activeTabKey]];
}

- (void)presentPageSwitcher {
    if (RBIsPad() || self.pageSwitcherController) return;
    [self.chromeBar.omnibox dismissKeyboard];
    [self cacheActiveTabThumbnail];
    RBPageSwitcherController *controller = [[RBPageSwitcherController alloc]
                                             initWithTabs:self.lastTabs
                                             thumbnails:self.tabThumbnails
                                             baseURL:self.session.baseURL
                                             fingerprint:[self.currentServer objectForKey:@"fingerprint"]];
    controller.delegate = self;
    self.pageSwitcherController = controller;
    [self presentViewController:controller animated:YES completion:nil];
}

- (void)dismissPageSwitcherSelectingTab:(NSInteger)tabID {
    self.pageSwitcherController = nil;
    [self dismissViewControllerAnimated:YES completion:^{
        if (tabID != NSNotFound) {
            [self.session sendMessage:@{@"t": @"tab", @"action": @"select",
                                        @"id": [NSNumber numberWithInteger:tabID]}];
        }
    }];
}

- (void)pageSwitcher:(RBPageSwitcherController *)controller selectTab:(NSInteger)tabID {
    [self dismissPageSwitcherSelectingTab:tabID];
}

- (void)pageSwitcher:(RBPageSwitcherController *)controller closeTab:(NSInteger)tabID {
    NSNumber *tabKey = [NSNumber numberWithInteger:tabID];
    [self.tabThumbnails removeObjectForKey:tabKey];
    [self.thumbnailLRU removeObject:tabKey];
    [self.session sendMessage:@{@"t": @"tab", @"action": @"close", @"id": tabKey}];
}

- (void)pageSwitcherNewTab:(RBPageSwitcherController *)controller {
    self.pageSwitcherController = nil;
    [self dismissViewControllerAnimated:YES completion:^{
        [self.session sendMessage:@{@"t": @"tab", @"action": @"new"}];
    }];
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
    if (RBIsPad()) {
        list.contentSizeForViewInPopover = [list preferredSize];
        UIPopoverController *popover = [[UIPopoverController alloc] initWithContentViewController:list];
        [RBTheme stylePopoverController:popover];
        popover.delegate = self;
        self.popover = popover;
        CGRect anchor = [button convertRect:button.bounds toView:self.view];
        [popover presentPopoverFromRect:anchor inView:self.view
               permittedArrowDirections:[self browserChromeArrowDirection] animated:YES];
    } else {
        list.title = @"Actions";
        list.navigationItem.rightBarButtonItem =
            [[UIBarButtonItem alloc] initWithBarButtonSystemItem:UIBarButtonSystemItemDone
                                                          target:self action:@selector(compactPopoverDone:)];
        UINavigationController *nav = [[UINavigationController alloc] initWithRootViewController:list];
        [RBTheme styleNavigationBar:nav.navigationBar];
        nav.modalPresentationStyle = UIModalPresentationFullScreen;
        self.compactPopoverController = nav;
        [self presentViewController:nav animated:YES completion:nil];
    }
}

- (void)dismissPopover {
    if (self.popover.popoverVisible) [self.popover dismissPopoverAnimated:NO];
    if (self.compactPopoverController) {
        [self dismissViewControllerAnimated:NO completion:nil];
    }
    self.popover = nil;
    self.compactPopoverController = nil;
    self.pageMediaController = nil;
}

- (void)compactPopoverDone:(id)sender {
    [self dismissPopover];
}

- (void)popoverControllerDidDismissPopover:(UIPopoverController *)popoverController {
    if (popoverController == self.popover) {
        self.popover = nil;
        self.pageMediaController = nil;
        self.libraryController = nil;
        self.activityController = nil;
        self.actionMenuController = nil;
    }
    if (popoverController == self.uploadPopover) {
        // Swiped away without picking: cancel the pending server chooser.
        self.uploadPopover = nil;
        if (self.chooserPending) {
            self.chooserPending = NO;
            [self postUploadData:nil filename:nil];
        }
    }
    if (popoverController == self.selectPopover) {
        RBSelectController *controller = self.selectController;
        self.selectPopover = nil;
        self.selectNavigationController = nil;
        self.selectController = nil;
        if (controller) {
            [self.session sendMessage:@{@"t": @"selectreply", @"id": controller.requestID ?: @"",
                                        @"cancel": @YES}];
        }
    }
}

// ---------------------------------------------------------------- library

- (void)presentLibrary {
    UIButton *button = RBIsPad() ? self.chromeBar.libraryButton : nil;
    [self presentLibraryFromButton:button];
}

- (void)dismissLibraryAnimated:(BOOL)animated {
    if (self.popover.popoverVisible) {
        [self.popover dismissPopoverAnimated:animated];
        self.popover = nil;
    } else if (self.libraryController) {
        [self dismissViewControllerAnimated:animated completion:nil];
    }
    self.libraryController = nil;
}

- (void)presentLibraryFromButton:(UIButton *)button {
    [self dismissPopover];
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
        [weakSelf dismissLibraryAnimated:YES];
        [weakSelf.session sendMessage:@{@"t": @"nav", @"url": url}];
    };
    library.onDismiss = ^{
        [weakSelf dismissLibraryAnimated:YES];
    };
    library.onNeedsData = ^(NSString *kind) {
        if ([kind isEqualToString:@"bookmarks"]) [weakSelf.session sendMessage:@{@"t": @"hist"}];
        else if ([kind isEqualToString:@"downloads"]) [weakSelf.session sendMessage:@{@"t": @"downloads"}];
    };
    self.libraryController = library;
    UINavigationController *nav = [[UINavigationController alloc] initWithRootViewController:library];
    [RBTheme styleNavigationBar:nav.navigationBar];
    nav.view.backgroundColor = [RBTheme pageBackgroundColor];
    if (RBIsPad()) {
        CGSize librarySize = CGSizeMake(420.0, 520.0);
        library.contentSizeForViewInPopover = librarySize;
        nav.contentSizeForViewInPopover = librarySize;
        UIPopoverController *popover = [[UIPopoverController alloc] initWithContentViewController:nav];
        [RBTheme stylePopoverController:popover];
        // iOS 6 does not reliably forward a navigation controller's preferred
        // size into an already-created popover. Set the owning controller too,
        // otherwise UIKit falls back to an almost full-screen table height.
        popover.popoverContentSize = librarySize;
        popover.delegate = self;
        self.popover = popover;
        CGRect anchor = [button convertRect:button.bounds toView:self.view];
        [popover presentPopoverFromRect:anchor inView:self.view
               permittedArrowDirections:[self browserChromeArrowDirection] animated:YES];
    } else {
        nav.modalPresentationStyle = UIModalPresentationFullScreen;
        [self presentViewController:nav animated:YES completion:nil];
    }
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
    NSURL *url = [NSURL URLWithString:[@"/api/v1/downloads/" stringByAppendingString:escaped] relativeToURL:self.session.baseURL];
    if (!url) return;
    [self showToast:[NSString stringWithFormat:@"Fetching %@…", name]];
    RBLogEvent(@"download", @"info", @{@"host": [url host] ?: @"", @"name": name ?: @""}, @"Download started");
    NSURLRequest *request = [NSURLRequest requestWithURL:url cachePolicy:NSURLRequestReloadIgnoringLocalCacheData timeoutInterval:120.0];
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        RBSecureHTTPClient *client = [RBSecureHTTPClient clientForServer:self.currentServer];
        NSHTTPURLResponse *response = nil; NSError *error = nil;
        NSData *data = [client sendRequest:request response:&response error:&error];
        dispatch_async(dispatch_get_main_queue(), ^{
        NSInteger status = [response isKindOfClass:[NSHTTPURLResponse class]] ? [(NSHTTPURLResponse *)response statusCode] : 0;
        RBLogEvent(@"download", (error || status >= 400) ? @"error" : @"info",
                   @{@"status": @(status), @"bytes": @([data length]),
                     @"error": [error localizedDescription] ?: @""}, @"Download request completed");
        if (error || status >= 400 || ![data length]) {
            [self showToast:@"Download failed"];
            return;
        }
        NSString *dir = [RBLogDirectory stringByAppendingPathComponent:@"Downloads"];
        NSError *dirError = nil;
        if (![[NSFileManager defaultManager] createDirectoryAtPath:dir withIntermediateDirectories:YES attributes:nil error:&dirError]) {
            RBLogEvent(@"download", @"error", @{@"operation": @"create_directory",
                       @"error": [dirError localizedDescription] ?: @""}, @"Download directory creation failed");
        }
        NSString *path = [dir stringByAppendingPathComponent:name];
        if (![data writeToFile:path atomically:YES]) {
            RBLogEvent(@"download", @"error", @{@"operation": @"write_file", @"name": [path lastPathComponent] ?: @""}, @"Downloaded file could not be saved");
            [self showToast:@"Could not save file"];
            return;
        }
        self.docController = [UIDocumentInteractionController interactionControllerWithURL:[NSURL fileURLWithPath:path]];
        self.docController.delegate = self;
        RBLogEvent(@"download", @"info", @{@"name": [path lastPathComponent] ?: @"",
                   @"uti": self.docController.UTI ?: @"",
                   @"handlers": @([self.docController.gestureRecognizers count])}, @"Download open menu prepared");
        // Present over the Library if it's up, else from the library button.
        UIView *host = self.presentedViewController ? self.presentedViewController.view : self.view;
        CGRect anchor = self.presentedViewController
            ? CGRectMake(host.bounds.size.width / 2.0 - 22.0, 40.0, 44.0, 44.0)
            : [self.chromeBar.libraryButton convertRect:self.chromeBar.libraryButton.bounds toView:self.view];
        BOOL presented = [self.docController presentOpenInMenuFromRect:anchor inView:host animated:YES];
        RBLogEvent(@"download", presented ? @"info" : @"warn", @{@"presented": @(presented)}, @"Download open menu requested");
        if (!presented) {
            [self showToast:@"No app can open this file"];
        }
        });
    });
}

// These three are optional on the delegate and Apple only calls them if the
// OS actually attempts the hand-off — logging them tells us whether tapping
// an app in the "Open In" list ever reaches the OS at all, or dies earlier
// (e.g. in the popover itself).
- (void)documentInteractionController:(UIDocumentInteractionController *)controller willBeginSendingToApplication:(NSString *)application {
    RBLogEvent(@"download", @"info", @{@"application": application ?: @""}, @"Sending download to application");
}

- (void)documentInteractionController:(UIDocumentInteractionController *)controller didEndSendingToApplication:(NSString *)application {
    RBLogEvent(@"download", @"info", @{@"application": application ?: @""}, @"Download handoff completed");
}

- (void)documentInteractionControllerDidDismissOpenInMenu:(UIDocumentInteractionController *)controller {
    RBLogEvent(@"download", @"info", @{@"state": @"dismissed"}, @"Download open menu dismissed");
}

// ----------------------------------------------------------- keyboard shim

- (void)resetHiddenInput {
    self.hiddenInput.text = @" ";
    self.previousHiddenText = @" ";
    UITextPosition *end = [self.hiddenInput endOfDocument];
    if (end) self.hiddenInput.selectedTextRange = [self.hiddenInput textRangeFromPosition:end toPosition:end];
}

- (void)showKeyboard {
    NSString *clipboard = [UIPasteboard generalPasteboard].string;
    self.pagePasteButton.enabled = [clipboard length] && RBValidClipboardText(clipboard);
    if (![self.hiddenInput isFirstResponder]) {
        [self resetHiddenInput];
        [self.hiddenInput becomeFirstResponder];
    }
    [self updateKeyboardAvoidance];
}

- (void)hidePageKeyboard {
    if (![self.hiddenInput isFirstResponder]) return;
    if (self.inputCompositionActive) {
        [self.session sendMessage:@{@"t": @"compose", @"phase": @"cancel", @"text": @""}];
        self.inputCompositionActive = NO;
        [self resetHiddenInput];
    }
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
    else if ([kind isEqualToString:@"tel"]) type = UIKeyboardTypePhonePad;
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
    if (RBIsPad() && (self.chromeBar.omnibox.editing || self.findBar.editing)) {
        NSTimeInterval duration = [[[note userInfo] objectForKey:UIKeyboardAnimationDurationUserInfoKey] doubleValue];
        UIViewAnimationCurve curve = (UIViewAnimationCurve)[[[note userInfo]
            objectForKey:UIKeyboardAnimationCurveUserInfoKey] integerValue];
        [UIView animateWithDuration:(duration > 0.0 ? duration : 0.25)
                              delay:0.0
                            options:UIViewAnimationOptionBeginFromCurrentState |
                                    ((UIViewAnimationOptions)curve << 16)
                         animations:^{
            [self.view setNeedsLayout];
            [self.view layoutIfNeeded];
        } completion:nil];
    }
}

- (void)keyboardWillHide:(NSNotification *)note {
    self.keyboardVisible = NO;
    [self updateKeyboardAvoidance];
    if (RBIsPad()) {
        NSTimeInterval duration = [[[note userInfo] objectForKey:UIKeyboardAnimationDurationUserInfoKey] doubleValue];
        UIViewAnimationCurve curve = (UIViewAnimationCurve)[[[note userInfo]
            objectForKey:UIKeyboardAnimationCurveUserInfoKey] integerValue];
        [UIView animateWithDuration:(duration > 0.0 ? duration : 0.25)
                              delay:0.0
                            options:UIViewAnimationOptionBeginFromCurrentState |
                                    ((UIViewAnimationOptions)curve << 16)
                         animations:^{
            [self.view setNeedsLayout];
            [self.view layoutIfNeeded];
        } completion:nil];
    }
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
    return YES;
}

- (void)hiddenInputDidChange:(NSNotification *)notification {
    NSString *current = self.hiddenInput.text ?: @"";
    UITextRange *marked = self.hiddenInput.markedTextRange;
    if (marked) {
        NSInteger markedStart = [self.hiddenInput offsetFromPosition:self.hiddenInput.beginningOfDocument
                                                          toPosition:marked.start];
        NSInteger markedLength = [self.hiddenInput offsetFromPosition:marked.start toPosition:marked.end];
        if (markedStart < 0 || markedLength < 0 || (NSUInteger)(markedStart + markedLength) > [current length]) return;
        NSString *text = [current substringWithRange:NSMakeRange((NSUInteger)markedStart, (NSUInteger)markedLength)];
        UITextRange *selection = self.hiddenInput.selectedTextRange;
        NSInteger start = selection ? [self.hiddenInput offsetFromPosition:marked.start toPosition:selection.start] : markedLength;
        NSInteger end = selection ? [self.hiddenInput offsetFromPosition:marked.start toPosition:selection.end] : markedLength;
        start = MAX(0, MIN(markedLength, start));
        end = MAX(start, MIN(markedLength, end));
        self.inputCompositionActive = YES;
        self.previousHiddenText = current;
        [self.session sendMessage:@{@"t": @"compose", @"phase": @"update", @"text": text,
                                    @"start": [NSNumber numberWithInteger:start],
                                    @"end": [NSNumber numberWithInteger:end]}];
        return;
    }
    if (self.inputCompositionActive) {
        NSString *committed = [current hasPrefix:@" "] ? [current substringFromIndex:1] : current;
        self.inputCompositionActive = NO;
        [self.session sendMessage:@{@"t": @"compose", @"phase": [committed length] ? @"commit" : @"cancel",
                                    @"text": committed ?: @""}];
        [self resetHiddenInput];
        return;
    }

    NSString *previous = self.previousHiddenText ?: @" ";
    NSUInteger prefix = 0;
    NSUInteger common = MIN([previous length], [current length]);
    while (prefix < common && [previous characterAtIndex:prefix] == [current characterAtIndex:prefix]) prefix++;
    NSUInteger oldTail = [previous length], newTail = [current length];
    while (oldTail > prefix && newTail > prefix &&
           [previous characterAtIndex:oldTail - 1] == [current characterAtIndex:newTail - 1]) {
        oldTail--; newTail--;
    }
    if (oldTail > prefix) [self sendKeyName:@"Backspace" keyCode:8];
    if (newTail > prefix) {
        NSString *inserted = [current substringWithRange:NSMakeRange(prefix, newTail - prefix)];
        [self.session sendMessage:@{@"t": @"key", @"text": inserted}];
    }
    [self resetHiddenInput];
}

- (void)sendKeyName:(NSString *)name keyCode:(NSInteger)keyCode {
    [self.session sendMessage:@{@"t": @"key", @"down": @YES, @"key": name, @"code": name, @"keyCode": [NSNumber numberWithInteger:keyCode]}];
    [self.session sendMessage:@{@"t": @"key", @"down": @NO, @"key": name, @"code": name, @"keyCode": [NSNumber numberWithInteger:keyCode]}];
}

// ----------------------------------------------------- native page selects

- (CGSize)constrainedSelectPopoverSize:(RBSelectController *)controller {
    CGSize preferred = [controller preferredPopoverSize];
    CGSize available = self.view.bounds.size;
    CGFloat width = MIN(preferred.width, MAX(220.0, available.width - 24.0));
    CGFloat height = MIN(preferred.height, MAX(132.0, available.height - 48.0));
    return CGSizeMake(width, height);
}

- (CGRect)selectAnchorForRectValue:(id)rectValue {
    NSArray *values = [rectValue isKindOfClass:[NSArray class]] ? rectValue : nil;
    CGRect bounds = self.streamView.bounds;
    CGRect local = CGRectMake(CGRectGetMidX(bounds), CGRectGetMidY(bounds), 2.0, 2.0);
    if ([values count] == 4) {
        CGFloat x = [[values objectAtIndex:0] floatValue] * bounds.size.width;
        CGFloat y = [[values objectAtIndex:1] floatValue] * bounds.size.height;
        CGFloat width = [[values objectAtIndex:2] floatValue] * bounds.size.width;
        CGFloat height = [[values objectAtIndex:3] floatValue] * bounds.size.height;
        CGRect candidate = CGRectIntersection(bounds, CGRectMake(x, y, width, height));
        if (!CGRectIsNull(candidate) && candidate.size.width > 0.0 && candidate.size.height > 0.0) local = candidate;
    }
    return [self.streamView convertRect:local toView:self.view];
}

- (void)presentSelectMessage:(NSDictionary *)message {
    NSString *requestID = [message objectForKey:@"id"];
    NSArray *options = [[message objectForKey:@"options"] isKindOfClass:[NSArray class]]
        ? [message objectForKey:@"options"] : nil;
    if (![requestID isKindOfClass:[NSString class]] || ![requestID length] || ![options count]) return;

    // A newer request supersedes the old server-side token. Do not answer the
    // stale one while replacing its UI.
    [self dismissSelectControllerSendingCancel:NO];
    [self hidePageKeyboard];
    RBSelectController *controller = [[RBSelectController alloc]
        initWithRequestID:requestID title:[message objectForKey:@"title"] options:options
                 multiple:[[message objectForKey:@"multiple"] boolValue]];
    controller.delegate = self;
    UINavigationController *navigation = [[UINavigationController alloc] initWithRootViewController:controller];
    [RBTheme styleNavigationBar:navigation.navigationBar];
    navigation.view.backgroundColor = [RBTheme pageBackgroundColor];
    self.selectController = controller;
    self.selectNavigationController = navigation;

    if (RBIsPad()) {
        CGSize size = [self constrainedSelectPopoverSize:controller];
        navigation.contentSizeForViewInPopover = size;
        UIPopoverController *popover = [[UIPopoverController alloc] initWithContentViewController:navigation];
        [RBTheme stylePopoverController:popover];
        popover.delegate = self;
        popover.popoverContentSize = size;
        self.selectPopover = popover;
        CGRect anchor = [self selectAnchorForRectValue:[message objectForKey:@"rect"]];
        [popover presentPopoverFromRect:anchor inView:self.view
               permittedArrowDirections:UIPopoverArrowDirectionAny animated:YES];
    } else {
        navigation.modalPresentationStyle = UIModalPresentationFullScreen;
        [self presentViewController:navigation animated:YES completion:nil];
    }
}

- (void)dismissSelectControllerSendingCancel:(BOOL)sendCancel {
    RBSelectController *controller = self.selectController;
    UIPopoverController *popover = self.selectPopover;
    UINavigationController *navigation = self.selectNavigationController;
    self.selectController = nil;
    self.selectPopover = nil;
    self.selectNavigationController = nil;
    if (popover.popoverVisible) [popover dismissPopoverAnimated:YES];
    if (navigation && !RBIsPad()) [self dismissViewControllerAnimated:YES completion:nil];
    if (sendCancel && controller) {
        [self.session sendMessage:@{@"t": @"selectreply", @"id": controller.requestID ?: @"",
                                    @"cancel": @YES}];
    }
}

- (void)selectController:(RBSelectController *)controller choseIndices:(NSArray *)indices {
    if (controller != self.selectController) return;
    NSString *requestID = [controller.requestID copy];
    [self dismissSelectControllerSendingCancel:NO];
    [self.session sendMessage:@{@"t": @"selectreply", @"id": requestID ?: @"",
                                @"indices": indices ?: @[]}];
}

- (void)selectControllerDidCancel:(RBSelectController *)controller {
    if (controller == self.selectController) [self dismissSelectControllerSendingCancel:YES];
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
    if (alertView == self.updateAlert) {
        self.updateAlert = nil;
        if (buttonIndex != alertView.cancelButtonIndex) [self startClientUpdate];
        return;
    }
    if (alertView == self.updateInstalledAlert) {
        self.updateInstalledAlert = nil;
        exit(0);
    }
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
    if (RBIsPad()) {
        // iPad rule: the photo library picker must live in a popover.
        UIPopoverController *popover = [[UIPopoverController alloc] initWithContentViewController:picker];
        // UIImagePickerController is system-owned and expects the native
        // popover chrome just like the Share sheet.
        popover.delegate = self;
        self.uploadPopover = popover;
        CGRect anchor = [self.chromeBar.moreButton convertRect:self.chromeBar.moreButton.bounds toView:self.view];
        [popover presentPopoverFromRect:anchor inView:self.view
               permittedArrowDirections:[self browserChromeArrowDirection] animated:YES];
    } else {
        self.uploadPicker = picker;
        [self presentViewController:picker animated:YES completion:nil];
    }
    [self showToast:@"Pick a photo to upload"];
}

- (void)dismissUploadPopover {
    if (self.uploadPopover.popoverVisible) [self.uploadPopover dismissPopoverAnimated:YES];
    if (self.uploadPicker) [self dismissViewControllerAnimated:YES completion:nil];
    self.uploadPopover = nil;
    self.uploadPicker = nil;
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

// postUploadData ships one file (or nothing = cancel) through the pinned,
// authenticated API. The device session rides in the shared cookie jar.
- (void)postUploadData:(NSData *)data filename:(NSString *)filename {
    NSURL *url = [NSURL URLWithString:@"/api/v1/uploads" relativeToURL:self.session.baseURL];
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
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        RBSecureHTTPClient *client = [RBSecureHTTPClient clientForServer:self.currentServer];
        NSHTTPURLResponse *response = nil; NSError *error = nil;
        [client sendRequest:request response:&response error:&error];
        dispatch_async(dispatch_get_main_queue(), ^{
        if (wasCancel) return;
        NSInteger status = [response isKindOfClass:[NSHTTPURLResponse class]] ? [(NSHTTPURLResponse *)response statusCode] : 0;
        if (error || status >= 400) {
            RBLogEvent(@"upload", @"error", @{@"status": @(status), @"error": [error localizedDescription] ?: @""}, @"Upload failed");
            [self showToast:@"Upload failed"];
        } else {
            [self showToast:@"Photo attached"];
        }
        });
    });
}

// ---------------------------------------------------------- page error state

- (void)showErrorCardForURL:(NSString *)url {
    NSString *detail = [url length]
        ? [NSString stringWithFormat:@"Surf could not open %@.", url]
        : @"Surf could not open this page.";
    [self.browserStateView showState:RBBrowserStatePageError detail:detail];
}

- (void)hideErrorCard {
    if (self.browserStateView.state == RBBrowserStatePageError)
        [self.browserStateView showState:RBBrowserStateHidden detail:nil];
}

// ------------------------------------------------------------ reader (M1.5)

- (void)handleReaderReply:(NSDictionary *)message {
    if (!self.readerPending) return;
    self.readerPending = NO;
    if (![[message objectForKey:@"ok"] boolValue]) {
        [self showToast:@"No article found on this page"];
        return;
    }
    RBReaderController *reader = [[RBReaderController alloc]
                                  initWithTitle:[message objectForKey:@"title"]
                                           html:[message objectForKey:@"html"]
                                            url:[message objectForKey:@"url"]];
    [self presentViewController:reader animated:YES completion:nil];
}

- (void)readerNavigate:(NSNotification *)note {
    NSString *url = [note.object isKindOfClass:[NSString class]] ? note.object : nil;
    if ([url length]) [self.session sendMessage:@{@"t": @"nav", @"url": url}];
}

// ---------------------------------------- device integration (M4.1 / M4.2)

- (void)setClipboardSyncEnabled:(BOOL)enabled {
    _clipboardSyncEnabled = enabled;
    self.clipboardChangeCount = [UIPasteboard generalPasteboard].changeCount;
    [self.clipboardSyncTimer invalidate];
    self.clipboardSyncTimer = nil;
    if (!enabled) return;
    self.clipboardSyncTimer = [NSTimer timerWithTimeInterval:0.6 target:self
                                                   selector:@selector(pollClipboardSync:)
                                                   userInfo:nil repeats:YES];
    [[NSRunLoop mainRunLoop] addTimer:self.clipboardSyncTimer forMode:NSRunLoopCommonModes];
}

- (void)sendCurrentClipboardIfValid {
    if (!self.clipboardSyncEnabled || self.session.state != RBSessionStateOpen) return;
    NSString *text = [UIPasteboard generalPasteboard].string;
    if (!RBValidClipboardText(text)) return;
    self.clipboardChangeCount = [UIPasteboard generalPasteboard].changeCount;
    [self.session sendMessage:@{ @"t": @"clipboard-change", @"text": text }];
    RBLogEvent(@"clipboard", @"info",
               @{ @"direction": @"device-to-host",
                  @"bytes": @([[text dataUsingEncoding:NSUTF8StringEncoding] length]) },
               @"Synchronized device clipboard to host");
}

- (void)pollClipboardSync:(NSTimer *)timer {
    (void)timer;
    if (!self.clipboardSyncEnabled || self.session.state != RBSessionStateOpen) return;
    NSInteger changeCount = [UIPasteboard generalPasteboard].changeCount;
    if (changeCount == self.clipboardChangeCount) return;
    self.clipboardChangeCount = changeCount;
    [self sendCurrentClipboardIfValid];
}

- (void)openURLString:(NSString *)url {
    if (![url length]) return;
    [self.session sendMessage:@{@"t": @"nav", @"url": url}];
}

- (void)checkPasteboard {
    if (![[NSUserDefaults standardUserDefaults] boolForKey:RBDefaultsOfferCopiedLinksKey]) return;
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

- (void)syncNativeLog { [self.session uploadNativeLogNow]; }

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
    [[NSUserDefaults standardUserDefaults] setBool:debugVisible forKey:RBDefaultsDiagnosticsKey];
    [[NSUserDefaults standardUserDefaults] synchronize];
    self.diagnosticsOverlay.hidden = !debugVisible;
    if (debugVisible) {
        CFTimeInterval presented = self.streamView.lastPresentationAt;
        [self refreshDebugOverlayWithAge:(presented > 0.0 ? CACurrentMediaTime() - presented : 0.0)];
        [self.view bringSubviewToFront:self.diagnosticsOverlay];
        [self layoutDiagnosticsOverlayAnimated:NO];
    } else {
        self.diagnosticsOverlay.displayMode = RBDiagnosticsOverlayCompact;
    }
}

- (void)toggleDebug:(UITapGestureRecognizer *)tap {
    if (tap.state != UIGestureRecognizerStateEnded) return;
    [self setDebugVisible:!self.debugVisible];
}

- (CGRect)diagnosticsOverlayFrame {
    CGRect streamFrame = self.streamView.frame;
    CGFloat availableWidth = MAX(1.0, streamFrame.size.width - 20.0);
    CGFloat availableHeight = MAX(1.0, streamFrame.size.height - 20.0);
    if (self.diagnosticsOverlay.displayMode == RBDiagnosticsOverlayCompact) {
        CGFloat width = MIN(280.0, availableWidth);
        CGFloat height = MIN(38.0, availableHeight);
        return CGRectMake(CGRectGetMaxX(streamFrame) - width - 10.0,
                          CGRectGetMinY(streamFrame) + 10.0, width, height);
    }
    CGFloat width = RBIsPad() ? MIN(372.0, availableWidth) : availableWidth;
    CGFloat height = MIN([self.diagnosticsOverlay preferredExpandedHeightForWidth:width],
                         availableHeight);
    CGFloat x = RBIsPad() ? CGRectGetMaxX(streamFrame) - width - 10.0
                          : CGRectGetMinX(streamFrame) + 10.0;
    CGFloat y = CGRectGetMinY(streamFrame) + 10.0;
    return CGRectMake(x, y, width, height);
}

- (void)layoutDiagnosticsOverlayAnimated:(BOOL)animated {
    CGRect frame = [self diagnosticsOverlayFrame];
    if (!animated) {
        self.diagnosticsOverlay.frame = frame;
        return;
    }
    [UIView animateWithDuration:0.18 animations:^{ self.diagnosticsOverlay.frame = frame; }];
}

- (void)diagnosticsOverlayDidChangeMode:(RBDiagnosticsOverlay *)overlay {
    [self layoutDiagnosticsOverlayAnimated:YES];
}

- (void)diagnosticsOverlayDidRequestClose:(RBDiagnosticsOverlay *)overlay {
    [self setDebugVisible:NO];
}

- (void)refreshDebugOverlayWithAge:(double)age {
    if (!self.debugVisible) return;
    NSString *state = self.session.state == RBSessionStateOpen ? @"open" : (self.session.state == RBSessionStateConnecting ? @"connecting" : @"idle");
    NSString *lane = self.videoActive ?
        [NSString stringWithFormat:@"H.264 %@", self.videoProfile ?: @"ready"] :
        (self.videoStarting ? @"H.264 starting" : @"video idle");
    NSString *server = [self.currentServer objectForKey:@"name"];
    if (server.length == 0) server = self.session.baseURL.host;
    if (server.length == 0) server = @"Surf";
    RBDiagnosticsSnapshot *snapshot = [self.diagnostics overlaySnapshotForServer:server
                                                                          version:RBAppVersion
                                                                    compatibility:RBCompatibilityVersion
                                                                           stream:lane
                                                                            state:state
                                                                          latency:self.session.interactionTracker.lastInteractionToPresentMS
                                                                              age:age];
    [self.diagnosticsOverlay updateWithSnapshot:snapshot];
}

- (void)watchdogTick:(NSTimer *)timer {
    if (self.applicationInBackground) return;
    CFTimeInterval presentedAt = self.streamView.lastPresentationAt;
    double age = presentedAt > 0.0 ? CACurrentMediaTime() - presentedAt : 0.0;
    // Decoder errors request an IDR directly; elapsed wall time alone is not
    // a fault signal because capture can be quiet on static pages.
    // Keep a recent low-RTT sample for the performance overlay.
    if (self.session.state == RBSessionStateOpen) {
        NSDictionary *probe = [self.diagnostics clockProbeIfIdle];
        if (probe) [self.session sendMessage:probe];
    }

    CFTimeInterval now = CACurrentMediaTime();
    RBDiagnosticsReport *report = [self.diagnostics reportAtTime:now age:age];
    if (report) {
        int queued = self.mediaPipeline.queuedAUs;
        [self.session sendMessage:[report mediaStatsMessage]];
        BOOL rendererWarning = report.rendererRecoveries > 0 || report.rendererFailures > 0;
        RBLogEvent(@"performance", report.recentGapMS >= 100.0 || rendererWarning ? @"warn" : @"info",
                   @{@"lane": self.videoActive ? @"video" : @"starting",
                     @"presented_fps": @(report.presentedFPS), @"image_fps": @(report.imageFPS),
                     @"au_rate": @(report.AURate), @"decode_rate": @(report.decodeRate),
                     @"video_queue": @(queued), @"drop_delta": @(report.dropDelta),
                     @"overwritten_frames": @(self.streamView.overwrittenVideoFrames),
                     @"max_gap_ms": @(report.recentGapMS), @"rtt_ms": @(self.diagnostics.lastRTTMS),
                     @"decode_submit_ms": @(self.mediaPipeline.averageSubmitMS),
                     @"decode_callback_ms": @(self.mediaPipeline.averageCallbackMS),
                     @"main_handoff_ms": @(self.mediaPipeline.averageHandoffMS),
                     @"error_delta": @(report.errorDelta),
                     @"audio_queue": @(self.mediaPipeline.audioQueuedBuffers),
                     @"audio_dropped_pcm": @(self.mediaPipeline.audioDroppedPCM),
                     @"audio_underruns": @(self.mediaPipeline.audioUnderruns),
                     @"audio_restarts": @(self.mediaPipeline.audioRestartCount),
                     @"frame_age_seconds": @(report.ageSeconds),
                     @"stream_profile": self.videoProfile ?: @"unknown",
                     @"renderer": self.mediaPipeline.rendererMode,
                     @"renderer_ms": @(self.mediaPipeline.averageRendererMS),
                     @"renderer_backpressure_delta": @(report.rendererBackpressure),
                     @"renderer_recoveries_delta": @(report.rendererRecoveries),
                     @"renderer_failures_delta": @(report.rendererFailures),
                     @"view_width": @(self.remoteViewportSize.width),
                     @"view_height": @(self.remoteViewportSize.height)}, @"Performance sample");

        // VideoToolbox on legacy devices can accept AUs without an error yet
        // stop issuing output callbacks. It can also leave the client waiting
        // for an event-driven IDR after a congestion resync. Continuous input
        // with no decode or presentation is therefore a real liveness fault,
        // unlike an intentionally quiet/static capture source.
        BOOL legacyRenderer = [self.mediaPipeline.rendererMode isEqualToString:@"legacy-gl"];
        BOOL stalled = legacyRenderer && self.videoActive && report.AURate >= 10.0 &&
            report.decodeRate < 1.0 && report.presentedFPS < 1.0 &&
            (presentedAt <= 0.0 || age >= 1.5);
        if (stalled && now - self.lastVideoLivenessRecoveryAt >= 2.0) {
            self.lastVideoLivenessRecoveryAt = now;
            RBLogEvent(@"decoder", @"warn", @{ @"frame_age_seconds": @(age),
                @"au_rate": @(report.AURate), @"recovery": @"session_reset" },
                @"Decoder liveness watchdog fired");
            [self.mediaPipeline recoverVideo];
            [self.diagnostics resetVideoWindow];
            [self.session sendMessage:@{ @"t": @"reqkeyframe" }];
        }
    }
    [self refreshDebugOverlayWithAge:age];
}

@end
