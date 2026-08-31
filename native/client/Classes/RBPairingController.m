#import "RBPairingController.h"
#import "RBDeviceIdentity.h"
#import "RBPairingClient.h"
#import "RBServerStore.h"
#import "RBTheme.h"
#import <QuartzCore/QuartzCore.h>

typedef enum {
    RBPairingViewContacting,
    RBPairingViewCode,
    RBPairingViewComparing,
    RBPairingViewWaiting,
    RBPairingViewError
} RBPairingViewState;

@interface RBPairingCodeField : UITextField
@end

@implementation RBPairingCodeField

- (CGRect)insetRectForBounds:(CGRect)bounds {
    return CGRectInset(bounds, 10.0, 2.0);
}

- (CGRect)textRectForBounds:(CGRect)bounds { return [self insetRectForBounds:bounds]; }
- (CGRect)editingRectForBounds:(CGRect)bounds { return [self insetRectForBounds:bounds]; }
- (CGRect)placeholderRectForBounds:(CGRect)bounds { return [self insetRectForBounds:bounds]; }

@end

@interface RBPairingController ()
@property(nonatomic, copy) NSString *endpoint;
@property(nonatomic, copy) NSString *expectedServerID;
@property(nonatomic, copy) NSString *qrToken;
@property(nonatomic, strong) NSDictionary *replacementServer;
@property(nonatomic, strong) NSDictionary *serverInfo;
@property(nonatomic, strong) NSDictionary *pendingPairing;
@property(nonatomic, strong) UILabel *titleLabel;
@property(nonatomic, strong) UILabel *detailLabel;
@property(nonatomic, strong) UILabel *phraseLabel;
@property(nonatomic, strong) UITextField *codeField;
@property(nonatomic, strong) UIActivityIndicatorView *spinner;
@property(nonatomic, strong) UIButton *primaryButton;
@property(nonatomic, assign) RBPairingViewState pairingState;
@property(nonatomic, assign) NSUInteger generation;
@property(nonatomic, assign) NSUInteger pollAttempt;
@property(nonatomic, assign) BOOL started;
@property(nonatomic, assign) BOOL relevant;
@property(nonatomic, assign) BOOL completed;
@property(nonatomic, assign) BOOL cancelRequested;
@end

@implementation RBPairingController

- (id)initWithEndpoint:(NSString *)endpoint
      expectedServerID:(NSString *)expectedServerID
     replacementServer:(NSDictionary *)replacementServer
                qrToken:(NSString *)qrToken {
    self = [super init];
    if (self) {
        self.endpoint = [endpoint copy];
        self.expectedServerID = [[expectedServerID lowercaseString] copy];
        self.qrToken = [qrToken copy];
        self.replacementServer = replacementServer;
        self.title = replacementServer ? @"Pair Again" : @"Pair with Surf";
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    self.view.backgroundColor = [RBTheme pageBackgroundColor];
    self.navigationItem.leftBarButtonItem = [[UIBarButtonItem alloc] initWithBarButtonSystemItem:UIBarButtonSystemItemCancel
                                                                                         target:self action:@selector(cancelTapped:)];

    self.titleLabel = [[UILabel alloc] initWithFrame:CGRectZero];
    self.titleLabel.backgroundColor = [UIColor clearColor];
    self.titleLabel.textAlignment = NSTextAlignmentCenter;
    self.titleLabel.font = [RBTheme displayFontOfSize:22.0];
    self.titleLabel.textColor = [RBTheme primaryTextColor];
    self.titleLabel.numberOfLines = 2;
    [self.view addSubview:self.titleLabel];

    self.detailLabel = [[UILabel alloc] initWithFrame:CGRectZero];
    self.detailLabel.backgroundColor = [UIColor clearColor];
    self.detailLabel.textAlignment = NSTextAlignmentCenter;
    self.detailLabel.font = [RBTheme fontOfSize:15.0 bold:NO];
    self.detailLabel.textColor = [RBTheme secondaryTextColor];
    self.detailLabel.numberOfLines = 0;
    [self.view addSubview:self.detailLabel];

    self.phraseLabel = [[UILabel alloc] initWithFrame:CGRectZero];
    self.phraseLabel.backgroundColor = [RBTheme surfaceColor];
    self.phraseLabel.textAlignment = NSTextAlignmentCenter;
    self.phraseLabel.font = [RBTheme monospacedFontOfSize:18.0 bold:YES];
    self.phraseLabel.textColor = [RBTheme primaryTextColor];
    self.phraseLabel.numberOfLines = 3;
    self.phraseLabel.layer.cornerRadius = 8.0;
    self.phraseLabel.layer.borderWidth = 1.0;
    self.phraseLabel.layer.borderColor = [[RBTheme mistColor] CGColor];
    self.phraseLabel.layer.masksToBounds = YES;
    [self.view addSubview:self.phraseLabel];

    self.codeField = [[RBPairingCodeField alloc] initWithFrame:CGRectZero];
    self.codeField.borderStyle = UITextBorderStyleRoundedRect;
    self.codeField.backgroundColor = [RBTheme surfaceColor];
    self.codeField.textColor = [RBTheme primaryTextColor];
    self.codeField.keyboardAppearance = [RBTheme isDarkMode] ? UIKeyboardAppearanceDark
                                                             : UIKeyboardAppearanceDefault;
    self.codeField.textAlignment = NSTextAlignmentCenter;
    self.codeField.font = [RBTheme fontOfSize:23.0 bold:YES];
    self.codeField.contentVerticalAlignment = UIControlContentVerticalAlignmentCenter;
    self.codeField.keyboardType = UIKeyboardTypeNumberPad;
    self.codeField.placeholder = @"6-digit code";
    self.codeField.hidden = YES;
    [self.view addSubview:self.codeField];

    self.spinner = [[UIActivityIndicatorView alloc] initWithActivityIndicatorStyle:UIActivityIndicatorViewStyleGray];
    [self.view addSubview:self.spinner];

    self.primaryButton = [UIButton buttonWithType:UIButtonTypeCustom];
    [RBTheme stylePrimaryButton:self.primaryButton];
    self.primaryButton.titleLabel.font = [RBTheme displayFontOfSize:17.0];
    [self.primaryButton addTarget:self action:@selector(primaryTapped:) forControlEvents:UIControlEventTouchUpInside];
    [self.view addSubview:self.primaryButton];

    [[NSNotificationCenter defaultCenter] addObserver:self selector:@selector(applicationDidBecomeActive:)
                                                 name:UIApplicationDidBecomeActiveNotification object:nil];
    [[NSNotificationCenter defaultCenter] addObserver:self selector:@selector(applicationDidEnterBackground:)
                                                 name:UIApplicationDidEnterBackgroundNotification object:nil];
    [self showContacting];
}

- (void)dealloc { [[NSNotificationCenter defaultCenter] removeObserver:self]; }

- (void)viewDidAppear:(BOOL)animated {
    [super viewDidAppear:animated];
    self.relevant = YES;
    if (!self.started) {
        self.started = YES;
        [self beginPairing];
    } else if (self.pairingState == RBPairingViewWaiting) {
        [self schedulePollAfter:0.0];
    }
}

- (void)viewWillDisappear:(BOOL)animated {
    [super viewWillDisappear:animated];
    self.relevant = NO;
    if (!self.completed && !self.cancelRequested && [self isMovingFromParentViewController]) [self cancelPairingSilently];
}

- (void)viewDidLayoutSubviews {
    [super viewDidLayoutSubviews];
    CGFloat width = self.view.bounds.size.width;
    CGFloat height = self.view.bounds.size.height;
    BOOL pad = UI_USER_INTERFACE_IDIOM() == UIUserInterfaceIdiomPad;
    CGFloat contentWidth = MIN(pad ? 420.0 : 360.0, width - (pad ? 48.0 : 28.0));
    CGFloat x = floorf((width - contentWidth) / 2.0);
    CGFloat buttonWidth = MIN(280.0, contentWidth);
    CGFloat buttonX = floorf((width - buttonWidth) / 2.0);
    if (self.pairingState == RBPairingViewCode) {
        CGFloat y = pad ? 38.0 : 4.0;
        self.spinner.frame = CGRectMake(0, 0, 0, 0);
        self.titleLabel.frame = CGRectMake(x, y, contentWidth, 42.0); y += 45.0;
        self.detailLabel.frame = CGRectMake(x, y, contentWidth, pad ? 60.0 : 46.0); y += pad ? 72.0 : 54.0;
        CGFloat fieldWidth = MIN(280.0, contentWidth);
        self.codeField.frame = CGRectMake(floorf((width - fieldWidth) / 2.0), y, fieldWidth, 38.0); y += 54.0;
        self.primaryButton.frame = CGRectMake(buttonX, y, buttonWidth, 44.0);
        return;
    }
    BOOL hasVisual = !self.phraseLabel.hidden;
    BOOL hasButton = !self.primaryButton.hidden;
    CGFloat total = (!self.spinner.hidden ? 34.0 : 0.0) + 46.0 + 68.0 +
                    (hasVisual ? 124.0 : 0.0) + (hasButton ? 60.0 : 0.0);
    CGFloat y = MAX(24.0, floorf((height - total) / 2.0) - 8.0);
    self.spinner.frame = CGRectMake(floorf((width - 24.0) / 2.0), y, 24.0, 24.0);
    if (!self.spinner.hidden) y += 34.0;
    self.titleLabel.frame = CGRectMake(x, y, contentWidth, 46.0); y += 50.0;
    self.detailLabel.frame = CGRectMake(x, y, contentWidth, 64.0); y += 68.0;
    self.phraseLabel.frame = CGRectMake(x, y, contentWidth, 110.0);
    if (hasVisual) y += 124.0;
    self.codeField.frame = CGRectMake(0, 0, 0, 0);
    self.primaryButton.frame = CGRectMake(buttonX, y, buttonWidth, 44.0);
}

- (NSString *)displayEndpoint {
    return [self.endpoint hasPrefix:@"https://"] ? [self.endpoint substringFromIndex:8] : self.endpoint;
}

- (NSString *)formattedPhrase:(NSString *)phrase {
    NSArray *parts = [phrase componentsSeparatedByCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
    NSMutableArray *words = [NSMutableArray array];
    for (NSString *part in parts) if ([part length]) [words addObject:part];
    if ([words count] != 6) return phrase ?: @"";
    return [NSString stringWithFormat:@"%@  %@\n%@  %@\n%@  %@",
            [words objectAtIndex:0], [words objectAtIndex:1],
            [words objectAtIndex:2], [words objectAtIndex:3],
            [words objectAtIndex:4], [words objectAtIndex:5]];
}

- (void)showContacting {
    self.pairingState = RBPairingViewContacting;
    self.titleLabel.text = self.replacementServer ? @"Pairing Again" : @"Contacting Surf";
    self.detailLabel.text = [self displayEndpoint];
    self.phraseLabel.hidden = YES;
    self.codeField.hidden = YES;
    self.primaryButton.hidden = YES;
    [self.spinner startAnimating];
    self.spinner.hidden = NO;
    [self.view setNeedsLayout];
}

- (void)showCodeEntry {
    self.pairingState = RBPairingViewCode;
    NSString *name = [self.serverInfo objectForKey:@"name"] ?: @"Surf";
    self.titleLabel.text = @"Enter Pairing Code";
    self.detailLabel.text = [NSString stringWithFormat:@"Enter the code shown on %@.", name];
    self.phraseLabel.hidden = YES;
    self.codeField.hidden = NO;
    self.codeField.text = @"";
    [self.spinner stopAnimating];
    self.spinner.hidden = YES;
    [self.primaryButton setTitle:@"Pair" forState:UIControlStateNormal];
    self.primaryButton.hidden = NO;
    self.primaryButton.enabled = YES;
    [self.codeField becomeFirstResponder];
    [self.view setNeedsLayout];
}

- (void)showComparison {
    self.pairingState = RBPairingViewComparing;
    NSString *name = [self.pendingPairing objectForKey:@"serverName"] ?: @"this Surf server";
    self.titleLabel.text = @"Compare These Words";
    self.detailLabel.text = [NSString stringWithFormat:@"Match these words on %@.", name];
    self.phraseLabel.text = [self formattedPhrase:[self.pendingPairing objectForKey:@"phrase"]];
    self.phraseLabel.hidden = NO;
    self.codeField.hidden = YES;
    [self.codeField resignFirstResponder];
    [self.view endEditing:YES];
    [self.spinner stopAnimating];
    self.spinner.hidden = YES;
    [self.primaryButton setTitle:@"Words Match" forState:UIControlStateNormal];
    self.primaryButton.hidden = NO;
    self.primaryButton.enabled = YES;
    [self.view setNeedsLayout];
}

- (void)showWaiting:(NSString *)message {
    self.pairingState = RBPairingViewWaiting;
    NSString *name = [self.pendingPairing objectForKey:@"serverName"] ?: @"Surf";
    self.titleLabel.text = [NSString stringWithFormat:@"Finishing with %@", name];
    self.detailLabel.text = [message length] ? message : @"Saving device…";
    self.phraseLabel.hidden = YES;
    self.codeField.hidden = YES;
    [self.codeField resignFirstResponder];
    [self.view endEditing:YES];
    [self.spinner startAnimating];
    self.spinner.hidden = NO;
    self.primaryButton.hidden = YES;
    [self.view setNeedsLayout];
}

- (void)showErrorTitle:(NSString *)title detail:(NSString *)detail {
    self.pairingState = RBPairingViewError;
    self.titleLabel.text = [title length] ? title : @"Couldn\u2019t Pair";
    self.detailLabel.text = detail;
    self.phraseLabel.hidden = YES;
    self.codeField.hidden = YES;
    [self.codeField resignFirstResponder];
    [self.view endEditing:YES];
    [self.spinner stopAnimating];
    self.spinner.hidden = YES;
    [self.primaryButton setTitle:@"Try Again" forState:UIControlStateNormal];
    self.primaryButton.hidden = NO;
    self.primaryButton.enabled = YES;
    [self.view setNeedsLayout];
}

- (void)beginPairing {
    NSUInteger generation = ++self.generation;
    self.pendingPairing = nil;
    self.pollAttempt = 0;
    [self showContacting];
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        NSError *error = nil;
        NSDictionary *info = [RBPairingClient inspectEndpoint:self.endpoint error:&error];
        NSString *actualServerID = [[info objectForKey:@"serverID"] lowercaseString];
        BOOL expectedIdentity = ![self.expectedServerID length] ||
            ([self.expectedServerID length] >= 32 && [actualServerID hasPrefix:self.expectedServerID]);
        if (info && !expectedIdentity) {
            info = nil;
            error = [NSError errorWithDomain:@"SurfPairing" code:9 userInfo:@{
                NSLocalizedDescriptionKey: @"The pairing code identifies a different Surf server."
            }];
        }
        NSDictionary *known = info ? [RBServerStore serverWithID:[info objectForKey:@"serverID"]] : nil;
        dispatch_async(dispatch_get_main_queue(), ^{
            if (generation != self.generation || self.cancelRequested) return;
            if (known && !self.replacementServer) {
                self.completed = YES;
                [self.delegate pairingController:self foundKnownServer:known endpoint:self.endpoint];
                return;
            }
            if (!info) {
                NSString *title = error.code == 9 ? @"Server Identity Changed" : @"Couldn\u2019t Reach Server";
                NSString *detail = [error localizedDescription] ?: @"Check that Surf is running and this address is reachable.";
                [self showErrorTitle:title detail:detail];
                return;
            }
            self.serverInfo = info;
            if (![[info objectForKey:@"pairing"] boolValue]) {
                [self showErrorTitle:@"Pairing Not Started"
                              detail:@"Choose Pair Device on the server first."];
                return;
            }
            if ([self.qrToken length]) [self requestPairingWithCode:nil];
            else [self showCodeEntry];
        });
    });
}

- (void)requestPairingWithCode:(NSString *)code {
    NSUInteger generation = self.generation;
    [self.codeField resignFirstResponder];
    [self showContacting];
    self.titleLabel.text = [self.qrToken length] ? @"Using Pairing Invitation" : @"Checking Pairing Code";
    NSDictionary *info = self.serverInfo;
    BOOL qrPairing = [self.qrToken length] > 0;
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        NSError *error = nil;
        NSDictionary *pairing = [RBPairingClient requestPairAtEndpoint:self.endpoint serverInfo:info
                                                                   code:code qrToken:self.qrToken error:&error];
        if (pairing && qrPairing) pairing = [RBPairingClient confirmPairing:pairing error:&error];
        dispatch_async(dispatch_get_main_queue(), ^{
            if (generation != self.generation || self.cancelRequested) return;
            if (!pairing) {
                NSString *title = error.code == 429 ? @"Pairing Cancelled" : @"Pairing Not Accepted";
                NSString *detail = [error localizedDescription] ?: @"Start a new pairing on the server.";
                [self showErrorTitle:title detail:detail];
                return;
            }
            self.pendingPairing = pairing;
            if (qrPairing) {
                if ([[pairing objectForKey:@"paired"] boolValue]) [self finishPairing:pairing];
                else { [self showWaiting:@"Saving device…"]; [self schedulePollAfter:1.0]; }
            } else {
                [self showComparison];
            }
        });
    });
}

- (void)primaryTapped:(id)sender {
    if (self.pairingState == RBPairingViewError) {
        [self beginPairing];
        return;
    }
    if (self.pairingState == RBPairingViewCode) {
        NSString *code = [[self.codeField text] stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
        NSCharacterSet *notDigits = [[NSCharacterSet decimalDigitCharacterSet] invertedSet];
        if ([code length] != 6 || [code rangeOfCharacterFromSet:notDigits].location != NSNotFound) {
            self.detailLabel.text = @"Enter the six-digit code shown on the server.";
            return;
        }
        [self requestPairingWithCode:code];
        return;
    }
    if (self.pairingState != RBPairingViewComparing || !self.pendingPairing) return;
    self.primaryButton.enabled = NO;
    [self showWaiting:nil];
    NSUInteger generation = self.generation;
    NSDictionary *pairing = self.pendingPairing;
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        NSError *error = nil;
        NSDictionary *status = [RBPairingClient confirmPairing:pairing error:&error];
        dispatch_async(dispatch_get_main_queue(), ^{
            if (generation != self.generation || self.cancelRequested) return;
            if ([[status objectForKey:@"paired"] boolValue]) {
                [self finishPairing:status];
            } else if (!status && error.code == 404) {
                [self showErrorTitle:@"Pairing Request Ended"
                              detail:@"Start a new pairing on the server."];
            } else {
                if (!status) [self showWaiting:@"Reconnecting…"];
                [self schedulePollAfter:1.0];
            }
        });
    });
}

- (void)schedulePollAfter:(NSTimeInterval)delay {
    if (!self.relevant || self.completed || self.cancelRequested || self.pairingState != RBPairingViewWaiting) return;
    NSUInteger generation = self.generation;
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(delay * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
        if (generation == self.generation && self.relevant && !self.completed && !self.cancelRequested) [self pollPairing];
    });
}

- (void)pollPairing {
    NSDictionary *pairing = self.pendingPairing;
    if (!pairing) return;
    NSUInteger generation = self.generation;
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        NSError *error = nil;
        NSDictionary *status = [RBPairingClient statusForPairing:pairing error:&error];
        dispatch_async(dispatch_get_main_queue(), ^{
            if (generation != self.generation || !self.relevant || self.cancelRequested) return;
            if ([[status objectForKey:@"paired"] boolValue]) {
                [self finishPairing:status];
                return;
            }
            if (!status && error.code == 404) {
                [self showErrorTitle:@"Pairing Ended"
                              detail:@"Start a new pairing on the server."];
                return;
            }
            self.pollAttempt++;
            NSTimeInterval delay = self.pollAttempt < 4 ? 1.0 : (self.pollAttempt < 12 ? 2.0 : 5.0);
            if (!status) [self showWaiting:@"Reconnecting…"];
            [self schedulePollAfter:delay];
        });
    });
}

- (NSDictionary *)mergedServerFromPairing:(NSDictionary *)pairing {
    NSMutableDictionary *server = [[RBPairingClient savedServerFromPairing:pairing] mutableCopy];
    if (!self.replacementServer) return server;
    NSString *savedName = [self.replacementServer objectForKey:@"name"];
    if ([savedName length]) [server setObject:savedName forKey:@"name"];
    NSMutableArray *endpoints = [NSMutableArray array];
    id oldEndpoints = [self.replacementServer objectForKey:@"endpoints"];
    if ([oldEndpoints isKindOfClass:[NSArray class]]) [endpoints addObjectsFromArray:oldEndpoints];
    NSString *endpoint = [server objectForKey:@"lastEndpoint"];
    if ([endpoint length] && ![endpoints containsObject:endpoint]) [endpoints addObject:endpoint];
    [server setObject:endpoints forKey:@"endpoints"];
    NSMutableArray *tunnels = [NSMutableArray array];
    id oldTunnels = [self.replacementServer objectForKey:@"tunnelEndpoints"];
    if ([oldTunnels isKindOfClass:[NSArray class]]) [tunnels addObjectsFromArray:oldTunnels];
    id newTunnels = [server objectForKey:@"tunnelEndpoints"];
    if ([newTunnels isKindOfClass:[NSArray class]]) {
        for (NSString *value in newTunnels) if (![tunnels containsObject:value]) [tunnels addObject:value];
    }
    [server setObject:tunnels forKey:@"tunnelEndpoints"];
    return server;
}

- (void)finishPairing:(NSDictionary *)status {
    if (self.completed) return;
    self.completed = YES;
    self.generation++;
    NSDictionary *server = [self mergedServerFromPairing:status];
    [RBServerStore saveServer:server select:YES];
    NSDictionary *ackPairing = status;
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        [RBPairingClient acknowledgePairing:ackPairing error:nil];
    });
    self.titleLabel.text = @"Paired Securely";
    self.detailLabel.text = @"Ready to connect.";
    self.phraseLabel.hidden = YES;
    [self.spinner stopAnimating];
    self.spinner.hidden = YES;
    self.primaryButton.hidden = YES;
    [self.delegate pairingController:self didPairServer:server];
}

- (void)cancelPairingSilently {
    if (self.completed || self.cancelRequested) return;
    self.cancelRequested = YES;
    self.generation++;
    NSDictionary *pairing = self.pendingPairing;
    if (!pairing) return;
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        NSError *error = nil;
        NSDictionary *status = [RBPairingClient cancelPairing:pairing error:&error];
        if (!self.replacementServer && status && ![[status objectForKey:@"paired"] boolValue]) {
            [RBDeviceIdentity deleteKeyForServerID:[pairing objectForKey:@"serverID"]];
        }
    });
}

- (void)cancelTapped:(id)sender {
    [self cancelPairingSilently];
    [self.delegate pairingControllerDidCancel:self];
}

- (void)applicationDidEnterBackground:(NSNotification *)notification { self.relevant = NO; }

- (void)applicationDidBecomeActive:(NSNotification *)notification {
    if (self.view.window && !self.completed && !self.cancelRequested) {
        self.relevant = YES;
        if (self.pairingState == RBPairingViewWaiting) [self schedulePollAfter:0.0];
    }
}

@end
