#import "RBSettingsController.h"
#import "RBConfig.h"
#import "RBTheme.h"

#import <QuartzCore/QuartzCore.h>

@interface RBSettingsController () <UITextFieldDelegate>
@property(nonatomic, copy) NSString *initialURL;
@property(nonatomic, copy) NSString *initialPassword;
@property(nonatomic, strong) UITextField *urlField;
@property(nonatomic, strong) UITextField *passwordField;
@property(nonatomic, strong) UILabel *videoLabel;
@property(nonatomic, strong) UISwitch *videoSwitch;
@property(nonatomic, strong) UIButton *connectButton;
@property(nonatomic, strong) UIButton *cancelButton;
@property(nonatomic, strong) UILabel *statusLabel;
@property(nonatomic, strong) UILabel *versionLabel;
@property(nonatomic, strong) UIView *card;
@end

@implementation RBSettingsController

- (id)initWithServerURL:(NSString *)serverURL password:(NSString *)password {
    self = [super init];
    if (self) {
        self.initialURL = serverURL;
        self.initialPassword = password;
        self.modalPresentationStyle = UIModalPresentationFormSheet;
        self.modalTransitionStyle = UIModalTransitionStyleCoverVertical;
    }
    return self;
}

- (UITextField *)formFieldWithPlaceholder:(NSString *)placeholder {
    UITextField *field = [[UITextField alloc] initWithFrame:CGRectZero];
    field.delegate = self;
    field.borderStyle = UITextBorderStyleRoundedRect;
    field.font = [RBTheme fontOfSize:16.0 bold:NO];
    field.placeholder = placeholder;
    field.autocorrectionType = UITextAutocorrectionTypeNo;
    field.autocapitalizationType = UITextAutocapitalizationTypeNone;
    field.contentVerticalAlignment = UIControlContentVerticalAlignmentCenter;
    return field;
}

- (UILabel *)formLabel:(NSString *)text {
    UILabel *label = [[UILabel alloc] initWithFrame:CGRectZero];
    label.backgroundColor = [UIColor clearColor];
    label.font = [RBTheme fontOfSize:13.0 bold:YES];
    label.textColor = [UIColor colorWithWhite:0.35 alpha:1.0];
    label.text = text;
    return label;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    self.view.backgroundColor = [UIColor colorWithRed:0.90 green:0.91 blue:0.93 alpha:1.0];

    self.card = [[UIView alloc] initWithFrame:CGRectZero];
    self.card.backgroundColor = [UIColor whiteColor];
    self.card.layer.cornerRadius = 8.0;
    self.card.layer.borderWidth = 1.0;
    self.card.layer.borderColor = [[UIColor colorWithWhite:0.72 alpha:1.0] CGColor];
    [self.view addSubview:self.card];

	UILabel *title = [self formLabel:@"Surf — Remote Browser"];
    title.font = [RBTheme fontOfSize:19.0 bold:YES];
    title.textColor = [UIColor colorWithWhite:0.15 alpha:1.0];
    title.tag = 100;
    [self.card addSubview:title];

    UILabel *urlLabel = [self formLabel:@"SERVER"];
    urlLabel.tag = 101;
    [self.card addSubview:urlLabel];
    self.urlField = [self formFieldWithPlaceholder:@"http://server"];
    self.urlField.keyboardType = UIKeyboardTypeURL;
    self.urlField.returnKeyType = UIReturnKeyNext;
    self.urlField.text = self.initialURL;
    [self.card addSubview:self.urlField];

    UILabel *passwordLabel = [self formLabel:@"PASSWORD"];
    passwordLabel.tag = 102;
    [self.card addSubview:passwordLabel];
    self.passwordField = [self formFieldWithPlaceholder:@"password"];
    self.passwordField.secureTextEntry = YES;
    self.passwordField.returnKeyType = UIReturnKeyGo;
    self.passwordField.text = self.initialPassword;
    [self.card addSubview:self.passwordField];

    self.videoLabel = [self formLabel:@"VIDEO STREAMING (H.264)"];
    [self.card addSubview:self.videoLabel];
    self.videoSwitch = [[UISwitch alloc] initWithFrame:CGRectZero];
    NSNumber *videoDefault = [[NSUserDefaults standardUserDefaults] objectForKey:RBDefaultsVideoKey];
    self.videoSwitch.on = videoDefault == nil || [videoDefault boolValue];
    [self.videoSwitch addTarget:self action:@selector(videoToggled:) forControlEvents:UIControlEventValueChanged];
    [self.card addSubview:self.videoSwitch];

    self.connectButton = [UIButton buttonWithType:UIButtonTypeCustom];
    self.connectButton.backgroundColor = [UIColor colorWithRed:0.28 green:0.42 blue:0.62 alpha:1.0];
    self.connectButton.layer.cornerRadius = 6.0;
    self.connectButton.titleLabel.font = [RBTheme fontOfSize:16.0 bold:YES];
    [self.connectButton setTitle:@"Connect" forState:UIControlStateNormal];
    [self.connectButton setTitleColor:[UIColor whiteColor] forState:UIControlStateNormal];
    [self.connectButton setTitleColor:[UIColor colorWithWhite:1.0 alpha:0.5] forState:UIControlStateHighlighted];
    [self.connectButton addTarget:self action:@selector(connectTapped:) forControlEvents:UIControlEventTouchUpInside];
    [self.card addSubview:self.connectButton];

    self.cancelButton = [UIButton buttonWithType:UIButtonTypeCustom];
    self.cancelButton.titleLabel.font = [RBTheme fontOfSize:15.0 bold:NO];
    [self.cancelButton setTitle:@"Cancel" forState:UIControlStateNormal];
    [self.cancelButton setTitleColor:[UIColor colorWithWhite:0.40 alpha:1.0] forState:UIControlStateNormal];
    [self.cancelButton addTarget:self action:@selector(cancelTapped:) forControlEvents:UIControlEventTouchUpInside];
    [self.card addSubview:self.cancelButton];

    self.statusLabel = [[UILabel alloc] initWithFrame:CGRectZero];
    self.statusLabel.backgroundColor = [UIColor clearColor];
    self.statusLabel.font = [RBTheme fontOfSize:13.0 bold:NO];
    self.statusLabel.textColor = [UIColor colorWithWhite:0.40 alpha:1.0];
    self.statusLabel.textAlignment = NSTextAlignmentCenter;
    self.statusLabel.numberOfLines = 2;
    [self.card addSubview:self.statusLabel];

    self.versionLabel = [[UILabel alloc] initWithFrame:CGRectZero];
    self.versionLabel.backgroundColor = [UIColor clearColor];
    self.versionLabel.font = [RBTheme fontOfSize:11.0 bold:NO];
    self.versionLabel.textColor = [UIColor colorWithWhite:0.55 alpha:1.0];
    self.versionLabel.textAlignment = NSTextAlignmentCenter;
    self.versionLabel.text = [NSString stringWithFormat:@"native %@", RBNativeVersion];
    [self.view addSubview:self.versionLabel];
}

- (void)viewWillLayoutSubviews {
    [super viewWillLayoutSubviews];
    CGFloat w = self.view.bounds.size.width;
    CGFloat h = self.view.bounds.size.height;
    CGFloat cardW = MIN(420.0, w - 40.0);
    CGFloat cardH = 384.0;
    self.card.frame = CGRectMake((w - cardW) / 2.0, MAX(16.0, (h - cardH) / 2.0 - 30.0), cardW, cardH);

    CGFloat pad = 24.0;
    CGFloat fw = cardW - pad * 2.0;
    UIView *title = [self.card viewWithTag:100];
    UIView *urlLabel = [self.card viewWithTag:101];
    UIView *passwordLabel = [self.card viewWithTag:102];
    title.frame = CGRectMake(pad, 20.0, fw, 24.0);
    urlLabel.frame = CGRectMake(pad, 62.0, fw, 16.0);
    self.urlField.frame = CGRectMake(pad, 80.0, fw, 36.0);
    passwordLabel.frame = CGRectMake(pad, 128.0, fw, 16.0);
    self.passwordField.frame = CGRectMake(pad, 146.0, fw, 36.0);
    self.videoLabel.frame = CGRectMake(pad, 198.0, fw - 90.0, 28.0);
    self.videoSwitch.frame = CGRectMake(cardW - pad - 79.0, 194.0, 79.0, 27.0);
    self.connectButton.frame = CGRectMake(pad, 236.0, fw, 42.0);
    self.statusLabel.frame = CGRectMake(pad, 284.0, fw, 36.0);
    self.cancelButton.frame = CGRectMake(pad, 322.0, fw, 30.0);
    self.cancelButton.hidden = !self.allowsCancel;
    self.versionLabel.frame = CGRectMake(0.0, h - 26.0, w, 16.0);
}

- (void)setStatusText:(NSString *)status isError:(BOOL)isError {
    self.statusLabel.text = status ?: @"";
    self.statusLabel.textColor = isError ? [UIColor colorWithRed:0.62 green:0.12 blue:0.12 alpha:1.0]
                                         : [UIColor colorWithWhite:0.40 alpha:1.0];
}

- (void)connectTapped:(id)sender {
    NSString *url = [self.urlField.text stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
    NSString *password = self.passwordField.text ?: @"";
    if (![url length]) {
        [self setStatusText:@"Server URL is required" isError:YES];
        return;
    }
    if ([url rangeOfString:@"://"].location == NSNotFound) url = [@"http://" stringByAppendingString:url];
    [self.urlField resignFirstResponder];
    [self.passwordField resignFirstResponder];
    [self setStatusText:@"Connecting…" isError:NO];
    [self.delegate settings:self connectToURL:url password:password];
}

- (void)videoToggled:(id)sender {
    [[NSUserDefaults standardUserDefaults] setObject:[NSNumber numberWithBool:self.videoSwitch.on]
                                              forKey:RBDefaultsVideoKey];
    [[NSUserDefaults standardUserDefaults] synchronize];
}

- (void)cancelTapped:(id)sender {
    [self.delegate settingsDismissed:self];
}

- (BOOL)textFieldShouldReturn:(UITextField *)textField {
    if (textField == self.urlField) {
        [self.passwordField becomeFirstResponder];
    } else {
        [self connectTapped:nil];
    }
    return NO;
}

@end
