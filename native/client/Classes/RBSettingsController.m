#import "RBSettingsController.h"
#import "RBConfig.h"
#import "RBLogViewController.h"
#import "RBServerStore.h"
#import "RBTheme.h"

#import <QuartzCore/QuartzCore.h>

static const NSInteger kRBClearDataAlert = 4101;
static const CGFloat kRBSettingsInset = 14.0;
static const CGFloat kRBSettingsRowHeight = 62.0;

@interface RBSettingsActionRow : UIControl
@property(nonatomic, strong) UIImageView *iconView;
@property(nonatomic, strong) UILabel *titleLabel;
@property(nonatomic, strong) UILabel *detailLabel;
@property(nonatomic, strong) UIImageView *chevronView;
@property(nonatomic, strong) UISwitch *toggle;
@property(nonatomic, strong) UIView *separator;
- (id)initWithTitle:(NSString *)title detail:(NSString *)detail icon:(RBIcon)icon;
- (void)setShowsChevron:(BOOL)showsChevron;
- (UISwitch *)installSwitchOn:(BOOL)on target:(id)target action:(SEL)action;
- (void)setDestructive:(BOOL)destructive;
- (void)setEmphasized:(BOOL)emphasized;
@end

@implementation RBSettingsActionRow

- (id)initWithTitle:(NSString *)title detail:(NSString *)detail icon:(RBIcon)icon {
    self = [super initWithFrame:CGRectZero];
    if (self) {
        self.backgroundColor = [UIColor whiteColor];
        self.accessibilityTraits = UIAccessibilityTraitButton;

        self.iconView = [[UIImageView alloc] initWithImage:
            [RBTheme icon:icon size:21.0 color:[RBTheme accentColor]]];
        self.iconView.contentMode = UIViewContentModeCenter;
        self.iconView.userInteractionEnabled = NO;
        [self addSubview:self.iconView];

        self.titleLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.titleLabel.backgroundColor = [UIColor clearColor];
        self.titleLabel.font = [RBTheme fontOfSize:15.0 bold:NO];
        self.titleLabel.textColor = [RBTheme primaryTextColor];
        self.titleLabel.text = title;
        self.titleLabel.lineBreakMode = NSLineBreakByTruncatingTail;
        [self addSubview:self.titleLabel];

        self.detailLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.detailLabel.backgroundColor = [UIColor clearColor];
        self.detailLabel.font = [RBTheme fontOfSize:12.0 bold:NO];
        self.detailLabel.textColor = [RBTheme secondaryTextColor];
        self.detailLabel.text = detail;
        self.detailLabel.lineBreakMode = NSLineBreakByTruncatingTail;
        [self addSubview:self.detailLabel];

        self.chevronView = [[UIImageView alloc] initWithImage:
            [RBTheme icon:RBIconForward size:15.0 color:[RBTheme secondaryTextColor]]];
        self.chevronView.contentMode = UIViewContentModeCenter;
        self.chevronView.hidden = YES;
        [self addSubview:self.chevronView];

        self.separator = [[UIView alloc] initWithFrame:CGRectZero];
        self.separator.backgroundColor = [RBTheme separatorColor];
        self.separator.userInteractionEnabled = NO;
        [self addSubview:self.separator];

        self.isAccessibilityElement = YES;
        self.accessibilityLabel = title;
        self.accessibilityValue = detail;
    }
    return self;
}

- (void)setShowsChevron:(BOOL)showsChevron {
    self.chevronView.hidden = !showsChevron;
    self.accessibilityTraits = showsChevron ? UIAccessibilityTraitButton : UIAccessibilityTraitNone;
    [self setNeedsLayout];
}

- (UISwitch *)installSwitchOn:(BOOL)on target:(id)target action:(SEL)action {
    [self.toggle removeFromSuperview];
    self.toggle = [[UISwitch alloc] initWithFrame:CGRectZero];
    self.toggle.on = on;
    if ([self.toggle respondsToSelector:@selector(setOnTintColor:)]) {
        self.toggle.onTintColor = [RBTheme accentColor];
    }
    [self.toggle addTarget:target action:action forControlEvents:UIControlEventValueChanged];
    [self addSubview:self.toggle];
    self.chevronView.hidden = YES;
    self.isAccessibilityElement = NO;
    [self setNeedsLayout];
    return self.toggle;
}

- (void)setDestructive:(BOOL)destructive {
    UIColor *color = destructive ? [UIColor colorWithRed:0.68 green:0.18 blue:0.20 alpha:1.0]
                                 : [RBTheme primaryTextColor];
    self.titleLabel.textColor = color;
    if (destructive) {
        self.iconView.image = [RBTheme icon:RBIconWarning size:21.0 color:color];
    }
}

- (void)setEmphasized:(BOOL)emphasized {
    self.titleLabel.font = emphasized ? [RBTheme displayFontOfSize:17.0]
                                      : [RBTheme fontOfSize:15.0 bold:NO];
}

- (void)setEnabled:(BOOL)enabled {
    [super setEnabled:enabled];
    self.alpha = enabled ? 1.0 : 0.42;
}

- (void)setHighlighted:(BOOL)highlighted {
    [super setHighlighted:highlighted];
    if (!self.enabled || self.toggle) return;
    self.backgroundColor = highlighted ? [[RBTheme seaGlassColor] colorWithAlphaComponent:0.12]
                                       : [UIColor whiteColor];
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat width = self.bounds.size.width;
    CGFloat height = self.bounds.size.height;
    CGFloat accessoryWidth = self.toggle ? 66.0 : (self.chevronView.hidden ? 12.0 : 30.0);
    self.iconView.frame = CGRectMake(10.0, floorf((height - 34.0) / 2.0), 34.0, 34.0);
    CGFloat textX = 50.0;
    CGFloat textWidth = MAX(20.0, width - textX - accessoryWidth - 8.0);
    BOOL hasDetail = [self.detailLabel.text length] > 0;
    self.titleLabel.frame = CGRectMake(textX, hasDetail ? 10.0 : 0.0,
                                       textWidth, hasDetail ? 22.0 : height);
    self.detailLabel.frame = CGRectMake(textX, 31.0, textWidth, 19.0);
    self.chevronView.frame = CGRectMake(MAX(0.0, width - 28.0),
                                        floorf((height - 28.0) / 2.0), 20.0, 28.0);
    if (self.toggle) {
        CGSize switchSize = self.toggle.bounds.size;
        self.toggle.frame = CGRectMake(width - switchSize.width - 10.0,
                                       floorf((height - switchSize.height) / 2.0),
                                       switchSize.width, switchSize.height);
    }
    self.separator.frame = CGRectMake(textX, MAX(0.0, height - 1.0),
                                      MAX(0.0, width - textX), 1.0);
}

@end

@interface RBSettingsCard : UIView
@property(nonatomic, strong) NSArray *rows;
- (void)setRows:(NSArray *)rows;
- (CGFloat)desiredHeight;
@end

@implementation RBSettingsCard

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [UIColor whiteColor];
        self.layer.cornerRadius = 11.0;
        self.layer.borderWidth = 1.0;
        self.layer.borderColor = [[RBTheme mistColor] CGColor];
        self.layer.masksToBounds = YES;
    }
    return self;
}

- (void)setRows:(NSArray *)rows {
    for (UIView *view in _rows) [view removeFromSuperview];
    _rows = rows;
    for (UIView *view in rows) [self addSubview:view];
    [self setNeedsLayout];
}

- (CGFloat)desiredHeight { return kRBSettingsRowHeight * [self.rows count]; }

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat rowHeight = [self.rows count] ? self.bounds.size.height / [self.rows count] : 0.0;
    for (NSUInteger i = 0; i < [self.rows count]; i++) {
        RBSettingsActionRow *row = [self.rows objectAtIndex:i];
        row.frame = CGRectMake(0.0, floorf(i * rowHeight), self.bounds.size.width,
                               ceilf(rowHeight));
        row.separator.hidden = i + 1 == [self.rows count];
    }
}

@end

static UILabel *RBSettingsSectionTitle(NSString *text) {
    UILabel *label = [[UILabel alloc] initWithFrame:CGRectZero];
    label.backgroundColor = [UIColor clearColor];
    label.font = [RBTheme fontOfSize:12.0 bold:YES];
    label.textColor = [RBTheme accentColor];
    label.text = [text uppercaseString];
    return label;
}

typedef enum {
    RBSettingsPageBrowsing,
    RBSettingsPageDiagnostics,
    RBSettingsPageData,
    RBSettingsPageAbout
} RBSettingsPage;

@class RBSettingsDetailController;

@interface RBSettingsController () <UIAlertViewDelegate>
@property(nonatomic, copy) NSString *selectedServerID;
@property(nonatomic, strong) NSDictionary *selectedServer;
@property(nonatomic, copy) NSString *pendingClearData;
@property(nonatomic, strong) UIScrollView *scrollView;
@property(nonatomic, strong) UILabel *serverSectionLabel;
@property(nonatomic, strong) UILabel *quickSectionLabel;
@property(nonatomic, strong) UILabel *manageSectionLabel;
@property(nonatomic, strong) RBSettingsCard *serverCard;
@property(nonatomic, strong) RBSettingsCard *quickCard;
@property(nonatomic, strong) RBSettingsCard *manageCard;
@property(nonatomic, strong) RBSettingsActionRow *serverRow;
@property(nonatomic, strong) UIView *serverStatusBar;
@property(nonatomic, strong) UISwitch *mobileSwitch;
@property(nonatomic, strong) UISwitch *diagnosticsSwitch;
- (void)mobileChanged:(UISwitch *)sender;
- (void)copiedLinksChanged:(UISwitch *)sender;
- (void)diagnosticsChanged:(UISwitch *)sender;
- (void)openMediaControls;
- (void)openDiagnosticsInspector;
- (void)openEventLog;
- (void)requestClearData:(NSString *)what;
- (void)showLicenses;
@end

@interface RBSettingsDetailController : UIViewController
@property(nonatomic, assign) RBSettingsController *owner;
@property(nonatomic, assign) RBSettingsPage page;
@property(nonatomic, strong) UIScrollView *scrollView;
@property(nonatomic, strong) UILabel *introLabel;
@property(nonatomic, strong) NSArray *cards;
@property(nonatomic, strong) UIView *brandHeader;
@property(nonatomic, strong) UISwitch *copiedLinksSwitch;
- (id)initWithPage:(RBSettingsPage)page owner:(RBSettingsController *)owner;
@end

@interface RBLicensesController : UIViewController
@property(nonatomic, strong) UITextView *textView;
@end

@implementation RBSettingsController

- (id)initWithSelectedServerID:(NSString *)serverID {
    self = [super initWithNibName:nil bundle:nil];
    if (self) {
        self.selectedServerID = serverID;
        self.title = @"Settings";
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    self.view.backgroundColor = [RBTheme pageBackgroundColor];
    self.navigationItem.rightBarButtonItem =
        [[UIBarButtonItem alloc] initWithBarButtonSystemItem:UIBarButtonSystemItemDone
                                                     target:self action:@selector(doneTapped:)];

    self.scrollView = [[UIScrollView alloc] initWithFrame:self.view.bounds];
    self.scrollView.backgroundColor = [UIColor clearColor];
    self.scrollView.alwaysBounceVertical = YES;
    self.scrollView.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
    [self.view addSubview:self.scrollView];

    self.serverSectionLabel = RBSettingsSectionTitle(@"Current server");
    self.quickSectionLabel = RBSettingsSectionTitle(@"Quick controls");
    self.manageSectionLabel = RBSettingsSectionTitle(@"Manage");
    [self.scrollView addSubview:self.serverSectionLabel];
    [self.scrollView addSubview:self.quickSectionLabel];
    [self.scrollView addSubview:self.manageSectionLabel];

    self.serverRow = [[RBSettingsActionRow alloc] initWithTitle:@"Choose a server"
                                                         detail:@"Add or select a Surf server"
                                                           icon:RBIconServer];
    [self.serverRow setShowsChevron:YES];
    [self.serverRow setEmphasized:YES];
    [self.serverRow addTarget:self action:@selector(serverTapped:) forControlEvents:UIControlEventTouchUpInside];
    self.serverCard = [[RBSettingsCard alloc] initWithFrame:CGRectZero];
    [self.serverCard setRows:@[self.serverRow]];
    [self.scrollView addSubview:self.serverCard];
    self.serverStatusBar = [[UIView alloc] initWithFrame:CGRectZero];
    [self.serverCard addSubview:self.serverStatusBar];

    BOOL mobile = [[NSUserDefaults standardUserDefaults] boolForKey:RBDefaultsMobileLayoutKey];
    RBSettingsActionRow *mobileRow = [[RBSettingsActionRow alloc]
        initWithTitle:@"Request Mobile Sites" detail:@"Identify as mobile Chrome" icon:RBIconSliders];
    self.mobileSwitch = [mobileRow installSwitchOn:mobile target:self action:@selector(mobileChanged:)];
    RBSettingsActionRow *diagnosticsRow = [[RBSettingsActionRow alloc]
        initWithTitle:@"Performance Monitor"
               detail:@"Show connection and stream health over the page" icon:RBIconGauge];
    self.diagnosticsSwitch = [diagnosticsRow installSwitchOn:self.diagnosticsVisible
                                                      target:self action:@selector(diagnosticsChanged:)];
    self.quickCard = [[RBSettingsCard alloc] initWithFrame:CGRectZero];
    [self.quickCard setRows:@[mobileRow, diagnosticsRow]];
    [self.scrollView addSubview:self.quickCard];

    NSArray *manageInfo = @[
        @[@"Browsing & Links", @"Media controls and copied addresses", @(RBIconShare)],
        @[@"Diagnostics & Logs", @"Live inspector and application events", @(RBIconGauge)],
        @[@"Data & Privacy", @"History, cookies, and cache", @(RBIconHistory)],
        @[@"About Surf", @"Version, protocol, and licenses", @(RBIconGear)]
    ];
    NSMutableArray *manageRows = [NSMutableArray array];
    for (NSUInteger i = 0; i < [manageInfo count]; i++) {
        NSArray *info = [manageInfo objectAtIndex:i];
        RBSettingsActionRow *row = [[RBSettingsActionRow alloc]
            initWithTitle:[info objectAtIndex:0] detail:[info objectAtIndex:1]
                     icon:(RBIcon)[[info objectAtIndex:2] integerValue]];
        row.tag = (NSInteger)i;
        [row setShowsChevron:YES];
        [row addTarget:self action:@selector(manageTapped:) forControlEvents:UIControlEventTouchUpInside];
        [manageRows addObject:row];
    }
    self.manageCard = [[RBSettingsCard alloc] initWithFrame:CGRectZero];
    [self.manageCard setRows:manageRows];
    [self.scrollView addSubview:self.manageCard];
    [self reloadServers];
}

- (void)viewWillAppear:(BOOL)animated {
    [super viewWillAppear:animated];
    [self reloadServers];
    self.mobileSwitch.on = [[NSUserDefaults standardUserDefaults] boolForKey:RBDefaultsMobileLayoutKey];
    self.diagnosticsSwitch.on = self.diagnosticsVisible;
}

- (void)viewDidLayoutSubviews {
    [super viewDidLayoutSubviews];
    CGFloat width = self.scrollView.bounds.size.width;
    CGFloat contentWidth = MAX(1.0, width - kRBSettingsInset * 2.0);
    CGFloat y = 12.0;
    self.serverSectionLabel.frame = CGRectMake(kRBSettingsInset + 4.0, y, contentWidth - 8.0, 18.0);
    y += 23.0;
    self.serverCard.frame = CGRectMake(kRBSettingsInset, y, contentWidth, 78.0);
    self.serverStatusBar.frame = CGRectMake(0.0, 0.0, 4.0, self.serverCard.bounds.size.height);
    y += 92.0;

    CGFloat quickHeight = [self.quickCard desiredHeight];
    CGFloat manageHeight = [self.manageCard desiredHeight];
    if (width >= 520.0) {
        CGFloat gap = 14.0;
        CGFloat columnWidth = floorf((contentWidth - gap) / 2.0);
        self.quickSectionLabel.frame = CGRectMake(kRBSettingsInset + 4.0, y, columnWidth - 8.0, 18.0);
        self.manageSectionLabel.frame = CGRectMake(kRBSettingsInset + columnWidth + gap + 4.0,
                                                   y, columnWidth - 8.0, 18.0);
        y += 23.0;
        self.quickCard.frame = CGRectMake(kRBSettingsInset, y, columnWidth, quickHeight);
        self.manageCard.frame = CGRectMake(kRBSettingsInset + columnWidth + gap, y,
                                           columnWidth, manageHeight);
        y += MAX(quickHeight, manageHeight) + 20.0;
    } else {
        self.quickSectionLabel.frame = CGRectMake(kRBSettingsInset + 4.0, y, contentWidth - 8.0, 18.0);
        y += 23.0;
        self.quickCard.frame = CGRectMake(kRBSettingsInset, y, contentWidth, quickHeight);
        y += quickHeight + 16.0;
        self.manageSectionLabel.frame = CGRectMake(kRBSettingsInset + 4.0, y, contentWidth - 8.0, 18.0);
        y += 23.0;
        self.manageCard.frame = CGRectMake(kRBSettingsInset, y, contentWidth, manageHeight);
        y += manageHeight + 20.0;
    }
    self.scrollView.contentSize = CGSizeMake(width, MAX(y, self.scrollView.bounds.size.height + 1.0));
}

- (void)updateServerCard {
    NSString *name = [self.selectedServer objectForKey:@"name"];
    if ([name length]) {
        self.serverRow.titleLabel.text = name;
        self.serverRow.detailLabel.text = self.connected ? @"Connected — manage server"
                                                         : @"Selected — tap to connect or manage";
    } else {
        self.serverRow.titleLabel.text = @"Choose a server";
        self.serverRow.detailLabel.text = @"Add or select a Surf server";
    }
    self.serverRow.accessibilityLabel = self.serverRow.titleLabel.text;
    self.serverRow.accessibilityValue = self.serverRow.detailLabel.text;
    self.serverRow.detailLabel.textColor = self.connected ? [RBTheme accentColor]
                                                          : [RBTheme secondaryTextColor];
    self.serverStatusBar.backgroundColor = self.connected ? [RBTheme seaGlassColor]
                                                           : [RBTheme mistColor];
}

- (void)setConnected:(BOOL)connected {
    _connected = connected;
    if ([self isViewLoaded]) [self updateServerCard];
}

- (void)setDiagnosticsVisible:(BOOL)diagnosticsVisible {
    _diagnosticsVisible = diagnosticsVisible;
    if ([self isViewLoaded]) self.diagnosticsSwitch.on = diagnosticsVisible;
}

- (void)reloadServers {
    self.selectedServer = [RBServerStore serverWithID:self.selectedServerID] ?:
        [RBServerStore lastSelectedServer];
    self.selectedServerID = [self.selectedServer objectForKey:@"serverID"];
    if ([self isViewLoaded]) [self updateServerCard];
}

- (void)doneTapped:(id)sender { [self.delegate settingsDismissed:self]; }
- (void)serverTapped:(id)sender { [self.delegate settingsWantsServers:self]; }

- (void)manageTapped:(RBSettingsActionRow *)row {
    RBSettingsDetailController *controller = [[RBSettingsDetailController alloc]
        initWithPage:(RBSettingsPage)row.tag owner:self];
    [self.navigationController pushViewController:controller animated:YES];
}

- (void)mobileChanged:(UISwitch *)sender {
    [[NSUserDefaults standardUserDefaults] setBool:sender.on forKey:RBDefaultsMobileLayoutKey];
    [[NSUserDefaults standardUserDefaults] synchronize];
    if ([self.delegate respondsToSelector:@selector(settings:preference:enabled:)]) {
        [self.delegate settings:self preference:RBDefaultsMobileLayoutKey enabled:sender.on];
    }
}

- (void)copiedLinksChanged:(UISwitch *)sender {
    [[NSUserDefaults standardUserDefaults] setBool:sender.on forKey:RBDefaultsOfferCopiedLinksKey];
    [[NSUserDefaults standardUserDefaults] synchronize];
    if ([self.delegate respondsToSelector:@selector(settings:preference:enabled:)]) {
        [self.delegate settings:self preference:RBDefaultsOfferCopiedLinksKey enabled:sender.on];
    }
}

- (void)diagnosticsChanged:(UISwitch *)sender {
    self.diagnosticsVisible = sender.on;
    [[NSUserDefaults standardUserDefaults] setBool:sender.on forKey:RBDefaultsDiagnosticsKey];
    [[NSUserDefaults standardUserDefaults] synchronize];
    if ([self.delegate respondsToSelector:@selector(settings:diagnosticsVisible:)]) {
        [self.delegate settings:self diagnosticsVisible:sender.on];
    }
}

- (void)openMediaControls {
    if (!self.connected) return;
    if ([self.delegate respondsToSelector:@selector(settingsWantsMediaControls:)]) {
        [self.delegate settingsWantsMediaControls:self];
    }
}

- (void)openDiagnosticsInspector {
    self.diagnosticsVisible = YES;
    [[NSUserDefaults standardUserDefaults] setBool:YES forKey:RBDefaultsDiagnosticsKey];
    [[NSUserDefaults standardUserDefaults] synchronize];
    if ([self.delegate respondsToSelector:@selector(settings:diagnosticsVisible:)]) {
        [self.delegate settings:self diagnosticsVisible:YES];
    }
    if ([self.delegate respondsToSelector:@selector(settingsWantsDiagnosticsInspector:)]) {
        [self.delegate settingsWantsDiagnosticsInspector:self];
    }
}

- (void)openEventLog {
    [self.navigationController pushViewController:[[RBLogViewController alloc] init] animated:YES];
}

- (void)requestClearData:(NSString *)what {
    if (!self.connected) return;
    NSDictionary *titles = @{@"history": @"Clear History?", @"cookies": @"Clear Cookies?",
                             @"cache": @"Clear Cache?"};
    self.pendingClearData = what;
    UIAlertView *alert = [[UIAlertView alloc] initWithTitle:[titles objectForKey:what]
                                                   message:@"This affects only the connected Surf server."
                                                  delegate:self cancelButtonTitle:@"Cancel"
                                         otherButtonTitles:@"Clear", nil];
    alert.tag = kRBClearDataAlert;
    [alert show];
}

- (void)alertView:(UIAlertView *)alertView clickedButtonAtIndex:(NSInteger)buttonIndex {
    if (buttonIndex == alertView.cancelButtonIndex || alertView.tag != kRBClearDataAlert) return;
    if ([self.delegate respondsToSelector:@selector(settings:clearData:)]) {
        [self.delegate settings:self clearData:self.pendingClearData];
    }
    self.pendingClearData = nil;
}

- (void)showLicenses {
    [self.navigationController pushViewController:[[RBLicensesController alloc] init] animated:YES];
}

@end

@implementation RBSettingsDetailController

- (id)initWithPage:(RBSettingsPage)page owner:(RBSettingsController *)owner {
    self = [super initWithNibName:nil bundle:nil];
    if (self) {
        self.page = page;
        self.owner = owner;
        static NSString *const titles[] = {@"Browsing & Links", @"Diagnostics & Logs",
                                           @"Data & Privacy", @"About Surf"};
        self.title = titles[page];
    }
    return self;
}

- (RBSettingsActionRow *)row:(NSString *)title detail:(NSString *)detail icon:(RBIcon)icon
                       action:(SEL)action {
    RBSettingsActionRow *row = [[RBSettingsActionRow alloc] initWithTitle:title detail:detail icon:icon];
    if (action) {
        [row setShowsChevron:YES];
        [row addTarget:self action:action forControlEvents:UIControlEventTouchUpInside];
    }
    return row;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    self.view.backgroundColor = [RBTheme pageBackgroundColor];
    self.scrollView = [[UIScrollView alloc] initWithFrame:self.view.bounds];
    self.scrollView.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
    self.scrollView.alwaysBounceVertical = YES;
    [self.view addSubview:self.scrollView];
    self.introLabel = [[UILabel alloc] initWithFrame:CGRectZero];
    self.introLabel.backgroundColor = [UIColor clearColor];
    self.introLabel.font = [RBTheme fontOfSize:14.0 bold:NO];
    self.introLabel.textColor = [RBTheme secondaryTextColor];
    self.introLabel.numberOfLines = 0;
    [self.scrollView addSubview:self.introLabel];

    NSMutableArray *cards = [NSMutableArray array];
    if (self.page == RBSettingsPageBrowsing) {
        self.introLabel.text = @"Control how remote pages identify themselves and hand links or media back to Surf.";
        RBSettingsActionRow *media = [self row:@"Media Controls"
            detail:@"Playback, mute, and page volume" icon:RBIconMedia action:@selector(mediaTapped:)];
        media.enabled = self.owner.connected;
        RBSettingsActionRow *links = [self row:@"Offer Copied Links"
            detail:@"Ask before opening copied web addresses" icon:RBIconShare action:nil];
        BOOL on = [[NSUserDefaults standardUserDefaults] boolForKey:RBDefaultsOfferCopiedLinksKey];
        self.copiedLinksSwitch = [links installSwitchOn:on target:self.owner
                                                 action:@selector(copiedLinksChanged:)];
        RBSettingsCard *card = [[RBSettingsCard alloc] initWithFrame:CGRectZero];
        [card setRows:@[media, links]];
        [cards addObject:card];
    } else if (self.page == RBSettingsPageDiagnostics) {
        self.introLabel.text = @"Inspect live browser health or review structured events when something feels wrong.";
        RBSettingsActionRow *live = [self row:@"Open Live Inspector"
            detail:@"Return to the browser with performance details expanded"
              icon:RBIconGauge action:@selector(inspectorTapped:)];
        RBSettingsActionRow *events = [self row:@"Event Log"
            detail:@"Application events, warnings, and errors"
              icon:RBIconReader action:@selector(eventsTapped:)];
        RBSettingsCard *card = [[RBSettingsCard alloc] initWithFrame:CGRectZero];
        [card setRows:@[live, events]];
        [cards addObject:card];
    } else if (self.page == RBSettingsPageData) {
        self.introLabel.text = self.owner.connected ?
            @"Remove browsing data from the connected Surf server. Each action asks before it runs." :
            @"Connect to a Surf server before managing its browsing data.";
        NSArray *titles = @[@"Clear History", @"Clear Cookies", @"Clear Cache"];
        NSArray *details = @[@"Remove visited-page records", @"Sign out of websites",
                             @"Remove temporary website files"];
        NSMutableArray *rows = [NSMutableArray array];
        for (NSUInteger i = 0; i < [titles count]; i++) {
            RBSettingsActionRow *row = [self row:[titles objectAtIndex:i]
                detail:[details objectAtIndex:i] icon:RBIconWarning action:@selector(clearTapped:)];
            row.tag = (NSInteger)i;
            [row setDestructive:YES];
            row.enabled = self.owner.connected;
            [rows addObject:row];
        }
        RBSettingsCard *card = [[RBSettingsCard alloc] initWithFrame:CGRectZero];
        [card setRows:rows];
        [cards addObject:card];
    } else {
        self.introLabel.text = @"A native remote browser built for the devices that still deserve excellent software.";
        self.brandHeader = [[UIView alloc] initWithFrame:CGRectZero];
        self.brandHeader.backgroundColor = [UIColor whiteColor];
        self.brandHeader.layer.cornerRadius = 11.0;
        self.brandHeader.layer.borderWidth = 1.0;
        self.brandHeader.layer.borderColor = [[RBTheme mistColor] CGColor];
        UIImageView *mark = [[UIImageView alloc] initWithImage:[UIImage imageNamed:@"brand-mark.png"]];
        mark.tag = 1;
        mark.contentMode = UIViewContentModeScaleAspectFit;
        [self.brandHeader addSubview:mark];
        UILabel *name = [[UILabel alloc] initWithFrame:CGRectZero];
        name.tag = 2;
        name.backgroundColor = [UIColor clearColor];
        name.font = [RBTheme displayFontOfSize:22.0];
        name.textColor = [RBTheme primaryTextColor];
        name.text = @"Surf";
        [self.brandHeader addSubview:name];
        UILabel *version = [[UILabel alloc] initWithFrame:CGRectZero];
        version.tag = 3;
        version.backgroundColor = [UIColor clearColor];
        version.font = [RBTheme fontOfSize:13.0 bold:NO];
        version.textColor = [RBTheme secondaryTextColor];
        version.text = [NSString stringWithFormat:@"Version %@ · protocol %@", RBAppVersion, RBNativeVersion];
        [self.brandHeader addSubview:version];
        [self.scrollView addSubview:self.brandHeader];

        RBSettingsActionRow *licenses = [self row:@"Third-Party Licenses"
            detail:@"Deta Surf artwork and Lucide icons" icon:RBIconBook action:@selector(licensesTapped:)];
        RBSettingsCard *card = [[RBSettingsCard alloc] initWithFrame:CGRectZero];
        [card setRows:@[licenses]];
        [cards addObject:card];
    }
    self.cards = cards;
    for (UIView *card in cards) [self.scrollView addSubview:card];
}

- (void)viewDidLayoutSubviews {
    [super viewDidLayoutSubviews];
    CGFloat width = self.scrollView.bounds.size.width;
    CGFloat contentWidth = MAX(1.0, MIN(620.0, width - kRBSettingsInset * 2.0));
    CGFloat x = floorf((width - contentWidth) / 2.0);
    CGFloat y = 14.0;
    CGSize introSize = [self.introLabel.text sizeWithFont:self.introLabel.font
                                       constrainedToSize:CGSizeMake(contentWidth - 8.0, 1000.0)
                                           lineBreakMode:NSLineBreakByWordWrapping];
    self.introLabel.frame = CGRectMake(x + 4.0, y, contentWidth - 8.0, ceilf(introSize.height));
    y += ceilf(introSize.height) + 16.0;
    if (self.brandHeader) {
        self.brandHeader.frame = CGRectMake(x, y, contentWidth, 104.0);
        ((UIView *)[self.brandHeader viewWithTag:1]).frame = CGRectMake(12.0, 10.0, 84.0, 84.0);
        ((UIView *)[self.brandHeader viewWithTag:2]).frame = CGRectMake(108.0, 24.0,
                                                                      contentWidth - 120.0, 28.0);
        ((UIView *)[self.brandHeader viewWithTag:3]).frame = CGRectMake(108.0, 52.0,
                                                                      contentWidth - 120.0, 24.0);
        y += 118.0;
    }
    for (RBSettingsCard *card in self.cards) {
        CGFloat height = [card desiredHeight];
        card.frame = CGRectMake(x, y, contentWidth, height);
        y += height + 14.0;
    }
    self.scrollView.contentSize = CGSizeMake(width, MAX(y, self.scrollView.bounds.size.height + 1.0));
}

- (void)mediaTapped:(id)sender { [self.owner openMediaControls]; }
- (void)inspectorTapped:(id)sender { [self.owner openDiagnosticsInspector]; }
- (void)eventsTapped:(id)sender { [self.owner openEventLog]; }
- (void)licensesTapped:(id)sender { [self.owner showLicenses]; }
- (void)clearTapped:(RBSettingsActionRow *)row {
    static NSString *const values[] = {@"history", @"cookies", @"cache"};
    [self.owner requestClearData:values[row.tag]];
}

@end

@implementation RBLicensesController

- (id)init {
    self = [super initWithNibName:nil bundle:nil];
    if (self) self.title = @"Licenses";
    return self;
}

- (NSString *)contentsForName:(NSString *)name extension:(NSString *)extension {
    NSString *path = [[NSBundle mainBundle] pathForResource:name ofType:extension
                                                inDirectory:@"ThirdPartyNotices"];
    NSData *data = path ? [NSData dataWithContentsOfFile:path] : nil;
    return data ? [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding] : @"";
}

- (void)viewDidLoad {
    [super viewDidLoad];
    self.view.backgroundColor = [RBTheme pageBackgroundColor];
    self.textView = [[UITextView alloc] initWithFrame:self.view.bounds];
    self.textView.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
    self.textView.backgroundColor = [UIColor whiteColor];
    self.textView.textColor = [RBTheme primaryTextColor];
    self.textView.font = [RBTheme fontOfSize:12.0 bold:NO];
    self.textView.editable = NO;
    self.textView.alwaysBounceVertical = YES;
    NSArray *documents = @[
        [self contentsForName:@"README" extension:@"md"],
        [self contentsForName:@"DETA-SURF-LICENSE" extension:@"txt"],
        [self contentsForName:@"LUCIDE-LICENSE" extension:@"txt"]
    ];
    self.textView.text = [documents componentsJoinedByString:@"\n\n————————————————\n\n"];
    [self.view addSubview:self.textView];
}

@end
