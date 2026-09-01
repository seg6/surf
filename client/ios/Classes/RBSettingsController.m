#import "RBSettingsController.h"
#import "RBConfig.h"
#import "RBLogViewController.h"
#import "RBServerStore.h"
#import "RBTheme.h"

enum {
    RBSettingsServerSection = 0,
    RBSettingsAppearanceSection,
    RBSettingsBrowsingSection,
    RBSettingsPerformanceSection,
    RBSettingsDataSection,
    RBSettingsAboutSection,
    RBSettingsSectionCount
};

enum {
    RBSettingsAppearanceDarkModeRow = 0,
    RBSettingsAppearanceBottomBarRow
};

enum {
    RBSettingsBrowsingMobileRow = 0,
    RBSettingsBrowsingCopiedLinksRow,
    RBSettingsBrowsingMediaRow
};

enum {
    RBSettingsPerformanceOverlayRow = 0,
    RBSettingsPerformanceInspectorRow,
    RBSettingsPerformanceLogRow
};

enum {
    RBSettingsAboutClientRow = 0,
    RBSettingsAboutCompatibilityRow,
    RBSettingsAboutUpdateRow,
    RBSettingsAboutLicensesRow
};

static const NSInteger kRBClearDataAlert = 4101;
static const CGFloat kRBSettingsRowHeight = 66.0;

@interface RBSettingsCell : UITableViewCell
@property(nonatomic, strong) UIImageView *settingIconView;
@property(nonatomic, strong) UILabel *settingTitleLabel;
@property(nonatomic, strong) UILabel *settingDetailLabel;
- (void)configureWithTitle:(NSString *)title
                    detail:(NSString *)detail
                      icon:(RBIcon)icon
                 iconColor:(UIColor *)iconColor
                   enabled:(BOOL)enabled;
@end

@implementation RBSettingsCell

- (id)initWithReuseIdentifier:(NSString *)reuseIdentifier {
    self = [super initWithStyle:UITableViewCellStyleDefault reuseIdentifier:reuseIdentifier];
    if (self) {
        self.backgroundColor = [RBTheme surfaceColor];
        self.backgroundView = [[UIView alloc] initWithFrame:CGRectZero];
        self.backgroundView.backgroundColor = [RBTheme surfaceColor];

        UIView *selectedBackground = [[UIView alloc] initWithFrame:CGRectZero];
        selectedBackground.backgroundColor = [[RBTheme seaGlassColor] colorWithAlphaComponent:0.16];
        self.selectedBackgroundView = selectedBackground;

        self.settingIconView = [[UIImageView alloc] initWithFrame:CGRectZero];
        self.settingIconView.contentMode = UIViewContentModeCenter;
        [self.contentView addSubview:self.settingIconView];

        self.settingTitleLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.settingTitleLabel.backgroundColor = [UIColor clearColor];
        self.settingTitleLabel.font = [RBTheme fontOfSize:15.0 bold:NO];
        self.settingTitleLabel.lineBreakMode = NSLineBreakByTruncatingTail;
        [self.contentView addSubview:self.settingTitleLabel];

        self.settingDetailLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.settingDetailLabel.backgroundColor = [UIColor clearColor];
        self.settingDetailLabel.font = [RBTheme fontOfSize:12.0 bold:NO];
        self.settingDetailLabel.lineBreakMode = NSLineBreakByWordWrapping;
        self.settingDetailLabel.numberOfLines = 2;
        [self.contentView addSubview:self.settingDetailLabel];
    }
    return self;
}

- (void)prepareForReuse {
    [super prepareForReuse];
    self.accessoryType = UITableViewCellAccessoryNone;
    self.accessoryView = nil;
    self.selectionStyle = UITableViewCellSelectionStyleBlue;
    self.userInteractionEnabled = YES;
    self.alpha = 1.0;
    self.isAccessibilityElement = YES;
    self.accessibilityTraits = UIAccessibilityTraitNone;
}

- (void)configureWithTitle:(NSString *)title
                    detail:(NSString *)detail
                      icon:(RBIcon)icon
                 iconColor:(UIColor *)iconColor
                   enabled:(BOOL)enabled {
    self.settingTitleLabel.text = title;
    self.backgroundColor = [RBTheme surfaceColor];
    self.backgroundView.backgroundColor = [RBTheme surfaceColor];
    self.selectedBackgroundView.backgroundColor = [[RBTheme seaGlassColor] colorWithAlphaComponent:0.16];
    self.settingTitleLabel.textColor = enabled ? [RBTheme primaryTextColor]
                                               : [[RBTheme secondaryTextColor] colorWithAlphaComponent:0.72];
    self.settingDetailLabel.text = detail;
    self.settingDetailLabel.textColor = enabled ? [RBTheme secondaryTextColor]
                                                : [[RBTheme secondaryTextColor] colorWithAlphaComponent:0.64];
    self.settingIconView.image = [RBTheme icon:icon size:21.0
                                      color:enabled ? iconColor
                                                    : [[RBTheme secondaryTextColor] colorWithAlphaComponent:0.55]];
    self.userInteractionEnabled = enabled;
    self.selectionStyle = enabled ? UITableViewCellSelectionStyleBlue
                                  : UITableViewCellSelectionStyleNone;
    self.accessibilityLabel = title;
    self.accessibilityValue = detail;
    [self setNeedsLayout];
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGRect bounds = self.contentView.bounds;
    CGFloat iconX = 12.0;
    CGFloat textX = 50.0;
    CGFloat textWidth = MAX(24.0, bounds.size.width - textX - 10.0);
    BOOL hasDetail = [self.settingDetailLabel.text length] > 0;

    self.settingIconView.frame = CGRectMake(iconX, floorf((bounds.size.height - 34.0) / 2.0),
                                            30.0, 34.0);
    if (hasDetail) {
        self.settingTitleLabel.frame = CGRectMake(textX, 8.0, textWidth, 21.0);
        self.settingDetailLabel.frame = CGRectMake(textX, 29.0, textWidth,
                                                   MAX(20.0, bounds.size.height - 34.0));
    } else {
        self.settingTitleLabel.frame = CGRectMake(textX, 0.0, textWidth, bounds.size.height);
        self.settingDetailLabel.frame = CGRectZero;
    }
}

@end

@interface RBSettingsController () <UIAlertViewDelegate>
@property(nonatomic, copy) NSString *selectedServerID;
@property(nonatomic, strong) NSDictionary *selectedServer;
@property(nonatomic, copy) NSString *pendingClearData;
- (void)mobileChanged:(UISwitch *)sender;
- (void)copiedLinksChanged:(UISwitch *)sender;
- (void)diagnosticsChanged:(UISwitch *)sender;
- (void)darkModeChanged:(UISwitch *)sender;
- (void)requestClearDataAtRow:(NSInteger)row;
- (void)showLicenses;
@end

@interface RBLicensesController : UIViewController
@property(nonatomic, strong) UITextView *textView;
@end

@implementation RBSettingsController

- (id)initWithSelectedServerID:(NSString *)serverID {
    self = [super initWithStyle:UITableViewStyleGrouped];
    if (self) {
        self.selectedServerID = serverID;
        self.title = @"Settings";
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    [RBTheme styleTableView:self.tableView];
    self.tableView.rowHeight = kRBSettingsRowHeight;
    self.navigationItem.rightBarButtonItem =
        [[UIBarButtonItem alloc] initWithBarButtonSystemItem:UIBarButtonSystemItemDone
                                                     target:self action:@selector(doneTapped:)];
    [self reloadServers];
}

- (void)viewWillAppear:(BOOL)animated {
    [super viewWillAppear:animated];
    [RBTheme styleTableView:self.tableView];
    [RBTheme styleNavigationBar:self.navigationController.navigationBar];
    self.navigationController.view.backgroundColor = [RBTheme pageBackgroundColor];
    [self reloadServers];
}

- (void)setConnected:(BOOL)connected {
    _connected = connected;
    if ([self isViewLoaded]) [self.tableView reloadData];
}

- (void)setDiagnosticsVisible:(BOOL)diagnosticsVisible {
    _diagnosticsVisible = diagnosticsVisible;
    if ([self isViewLoaded]) [self.tableView reloadData];
}

- (void)reloadServers {
    self.selectedServer = [RBServerStore serverWithID:self.selectedServerID] ?:
        [RBServerStore lastSelectedServer];
    self.selectedServerID = [self.selectedServer objectForKey:@"serverID"];
    if ([self isViewLoaded]) [self.tableView reloadData];
}

- (void)doneTapped:(id)sender { [self.delegate settingsDismissed:self]; }

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

- (void)darkModeChanged:(UISwitch *)sender {
    [[NSUserDefaults standardUserDefaults] setBool:sender.on forKey:RBDefaultsDarkModeKey];
    [[NSUserDefaults standardUserDefaults] synchronize];
    [RBTheme styleNavigationBar:self.navigationController.navigationBar];
    self.navigationController.view.backgroundColor = [RBTheme pageBackgroundColor];
    [RBTheme styleTableView:self.tableView];
    [self.tableView reloadData];
    if ([self.delegate respondsToSelector:@selector(settings:preference:enabled:)]) {
        [self.delegate settings:self preference:RBDefaultsDarkModeKey enabled:sender.on];
    }
}

- (void)bottomBrowserBarChanged:(UISwitch *)sender {
    [[NSUserDefaults standardUserDefaults] setBool:sender.on forKey:RBDefaultsBottomBrowserBarKey];
    [[NSUserDefaults standardUserDefaults] synchronize];
    if ([self.delegate respondsToSelector:@selector(settings:preference:enabled:)]) {
        [self.delegate settings:self preference:RBDefaultsBottomBrowserBarKey enabled:sender.on];
    }
}

- (NSInteger)numberOfSectionsInTableView:(UITableView *)tableView {
    return RBSettingsSectionCount;
}

- (NSInteger)tableView:(UITableView *)tableView numberOfRowsInSection:(NSInteger)section {
    if (section == RBSettingsServerSection) return 1;
    if (section == RBSettingsAppearanceSection) {
        return UI_USER_INTERFACE_IDIOM() == UIUserInterfaceIdiomPad ? 2 : 1;
    }
    if (section == RBSettingsAboutSection) return 4;
    return 3;
}

- (NSString *)titleForSection:(NSInteger)section {
    static NSString *const titles[] = {
        @"Current Server", @"Appearance", @"Browsing", @"Performance", @"Data & Privacy", @"About"
    };
    return titles[section];
}

- (CGFloat)tableView:(UITableView *)tableView heightForHeaderInSection:(NSInteger)section {
    return section == RBSettingsServerSection ? 38.0 : 34.0;
}

- (UIView *)tableView:(UITableView *)tableView viewForHeaderInSection:(NSInteger)section {
    UIView *header = [[UIView alloc] initWithFrame:CGRectZero];
    header.backgroundColor = [UIColor clearColor];
    UILabel *label = [[UILabel alloc] initWithFrame:CGRectZero];
    label.tag = 1;
    label.backgroundColor = [UIColor clearColor];
    label.font = [RBTheme fontOfSize:12.0 bold:YES];
    label.textColor = [RBTheme accentColor];
    label.text = [[self titleForSection:section] uppercaseString];
    [header addSubview:label];
    return header;
}

- (void)tableView:(UITableView *)tableView willDisplayHeaderView:(UIView *)view
       forSection:(NSInteger)section {
    UILabel *label = (UILabel *)[view viewWithTag:1];
    CGFloat top = section == RBSettingsServerSection ? 14.0 : 10.0;
    label.frame = CGRectMake(16.0, top, MAX(1.0, view.bounds.size.width - 32.0), 18.0);
}

- (NSString *)tableView:(UITableView *)tableView titleForFooterInSection:(NSInteger)section {
    if (section == RBSettingsDataSection && !self.connected) {
        return @"Connect to a Surf server to manage its browsing data.";
    }
    return nil;
}

- (RBSettingsCell *)cellWithIdentifier:(NSString *)identifier
                                  title:(NSString *)title
                                 detail:(NSString *)detail
                                   icon:(RBIcon)icon
                              iconColor:(UIColor *)iconColor
                                enabled:(BOOL)enabled {
    RBSettingsCell *cell = (RBSettingsCell *)[self.tableView dequeueReusableCellWithIdentifier:identifier];
    if (!cell) cell = [[RBSettingsCell alloc] initWithReuseIdentifier:identifier];
    cell.accessoryType = UITableViewCellAccessoryNone;
    cell.accessoryView = nil;
    cell.selectionStyle = UITableViewCellSelectionStyleBlue;
    cell.isAccessibilityElement = YES;
    cell.accessibilityTraits = UIAccessibilityTraitNone;
    [cell configureWithTitle:title detail:detail icon:icon iconColor:iconColor enabled:enabled];
    return cell;
}

- (UISwitch *)switchOn:(BOOL)on target:(SEL)action label:(NSString *)label {
    UISwitch *toggle = [[UISwitch alloc] initWithFrame:CGRectZero];
    toggle.on = on;
    toggle.accessibilityLabel = label;
    if ([toggle respondsToSelector:@selector(setOnTintColor:)]) {
        toggle.onTintColor = [RBTheme accentColor];
    }
    [toggle addTarget:self action:action forControlEvents:UIControlEventValueChanged];
    return toggle;
}

- (UITableViewCell *)tableView:(UITableView *)tableView
         cellForRowAtIndexPath:(NSIndexPath *)indexPath {
    NSInteger section = indexPath.section;
    NSInteger row = indexPath.row;
    UIColor *accent = [RBTheme accentColor];
    RBSettingsCell *cell = nil;

    if (section == RBSettingsServerSection) {
        NSString *name = [self.selectedServer objectForKey:@"name"];
        NSString *title = [name length] ? name : @"Choose a Server";
        NSString *detail = ![name length] ? @"Add or select a Surf server" :
            (self.connected ? @"Connected" : @"Selected — tap to connect or manage");
        cell = [self cellWithIdentifier:@"server" title:title detail:detail icon:RBIconServer
                              iconColor:self.connected ? [RBTheme seaGlassColor] : accent enabled:YES];
        cell.settingTitleLabel.font = [RBTheme fontOfSize:16.0 bold:YES];
        cell.settingDetailLabel.textColor = self.connected ? [RBTheme accentColor]
                                                          : [RBTheme secondaryTextColor];
        cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
        cell.accessibilityTraits = UIAccessibilityTraitButton;
        return cell;
    }

    if (section == RBSettingsAppearanceSection) {
        if (row == RBSettingsAppearanceDarkModeRow) {
            cell = [self cellWithIdentifier:@"toggle" title:@"Dark Mode"
                detail:@"Use dark surfaces in Surf and websites" icon:RBIconMoon
                iconColor:accent enabled:YES];
            BOOL on = [[NSUserDefaults standardUserDefaults] boolForKey:RBDefaultsDarkModeKey];
            cell.accessoryView = [self switchOn:on target:@selector(darkModeChanged:) label:@"Dark Mode"];
        } else {
            cell = [self cellWithIdentifier:@"toggle" title:@"Bottom Browser Bar"
                detail:@"Keep the address, tabs, and actions near the bottom" icon:RBIconSliders
                iconColor:accent enabled:YES];
            BOOL on = [[NSUserDefaults standardUserDefaults] boolForKey:RBDefaultsBottomBrowserBarKey];
            cell.accessoryView = [self switchOn:on target:@selector(bottomBrowserBarChanged:)
                                              label:@"Bottom Browser Bar"];
        }
        cell.selectionStyle = UITableViewCellSelectionStyleNone;
        return cell;
    }

    if (section == RBSettingsBrowsingSection) {
        if (row == RBSettingsBrowsingMobileRow) {
            cell = [self cellWithIdentifier:@"toggle" title:@"Request Mobile Sites"
                detail:@"Identify Surf as mobile Chrome" icon:RBIconSliders
                iconColor:accent enabled:YES];
            BOOL on = [[NSUserDefaults standardUserDefaults] boolForKey:RBDefaultsMobileLayoutKey];
            cell.accessoryView = [self switchOn:on target:@selector(mobileChanged:)
                                          label:@"Request Mobile Sites"];
        } else if (row == RBSettingsBrowsingCopiedLinksRow) {
            cell = [self cellWithIdentifier:@"toggle" title:@"Offer Copied Links"
                detail:@"Ask before opening copied web addresses" icon:RBIconShare
                iconColor:accent enabled:YES];
            BOOL on = [[NSUserDefaults standardUserDefaults] boolForKey:RBDefaultsOfferCopiedLinksKey];
            cell.accessoryView = [self switchOn:on target:@selector(copiedLinksChanged:)
                                          label:@"Offer Copied Links"];
        } else {
            cell = [self cellWithIdentifier:@"action" title:@"Page Media Controls"
                detail:@"Playback, mute, and page volume" icon:RBIconMedia
                iconColor:accent enabled:self.connected];
            if (self.connected) cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
        }
        if (row != RBSettingsBrowsingMediaRow) cell.selectionStyle = UITableViewCellSelectionStyleNone;
        return cell;
    }

    if (section == RBSettingsPerformanceSection) {
        if (row == RBSettingsPerformanceOverlayRow) {
            cell = [self cellWithIdentifier:@"toggle" title:@"Performance Overlay"
                detail:@"Show live stream health over the page" icon:RBIconGauge
                iconColor:accent enabled:YES];
            cell.accessoryView = [self switchOn:self.diagnosticsVisible
                                         target:@selector(diagnosticsChanged:)
                                          label:@"Performance Overlay"];
            cell.selectionStyle = UITableViewCellSelectionStyleNone;
        } else if (row == RBSettingsPerformanceInspectorRow) {
            cell = [self cellWithIdentifier:@"action" title:@"Live Inspector"
                detail:@"Open the expanded performance panel" icon:RBIconGauge
                iconColor:accent enabled:YES];
            cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
        } else {
            cell = [self cellWithIdentifier:@"action" title:@"Event Log"
                detail:@"Application events, warnings, and errors" icon:RBIconReader
                iconColor:accent enabled:YES];
            cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
        }
        return cell;
    }

    if (section == RBSettingsDataSection) {
        static NSString *const titles[] = {@"Clear History", @"Clear Cookies", @"Clear Cache"};
        static NSString *const details[] = {
            @"Remove visited-page records from this server",
            @"Sign out of websites on this server",
            @"Remove temporary website files from this server"
        };
        UIColor *destructive = [RBTheme isDarkMode]
            ? [UIColor colorWithRed:0.96 green:0.39 blue:0.42 alpha:1.0]
            : [UIColor colorWithRed:0.68 green:0.18 blue:0.20 alpha:1.0];
        cell = [self cellWithIdentifier:@"data" title:titles[row] detail:details[row]
                                  icon:RBIconWarning iconColor:destructive enabled:self.connected];
        if (self.connected) cell.settingTitleLabel.textColor = destructive;
        return cell;
    }

    if (row == RBSettingsAboutClientRow) {
        cell = [self cellWithIdentifier:@"about" title:@"Surf Client" detail:RBAppVersion
                                  icon:RBIconGear iconColor:accent enabled:YES];
        cell.selectionStyle = UITableViewCellSelectionStyleNone;
    } else if (row == RBSettingsAboutCompatibilityRow) {
        cell = [self cellWithIdentifier:@"about" title:@"Compatibility" detail:RBCompatibilityVersion
                                  icon:RBIconServer iconColor:accent enabled:YES];
        cell.selectionStyle = UITableViewCellSelectionStyleNone;
    } else if (row == RBSettingsAboutUpdateRow) {
        NSDictionary *update = self.availableClientUpdate;
        if (update) {
            double megabytes = [[update objectForKey:@"size"] doubleValue] / (1024.0 * 1024.0);
            cell = [self cellWithIdentifier:@"action"
                                      title:[NSString stringWithFormat:@"Update to Surf %@",
                                             [update objectForKey:@"version"] ?: @"?"]
                                     detail:[NSString stringWithFormat:@"Optional client update · %0.1f MB", megabytes]
                                       icon:RBIconDownload iconColor:accent enabled:YES];
            cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
        } else {
            cell = [self cellWithIdentifier:@"about" title:@"Client Updates"
                                     detail:[NSString stringWithFormat:@"Surf %@ is current", RBAppVersion]
                                       icon:RBIconDownload iconColor:accent enabled:YES];
            cell.selectionStyle = UITableViewCellSelectionStyleNone;
        }
    } else {
        cell = [self cellWithIdentifier:@"action" title:@"Third-Party Licenses"
            detail:@"Deta Surf artwork and Lucide icons" icon:RBIconBook
            iconColor:accent enabled:YES];
        cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
    }
    return cell;
}

- (NSIndexPath *)tableView:(UITableView *)tableView willSelectRowAtIndexPath:(NSIndexPath *)indexPath {
    if (indexPath.section == RBSettingsAppearanceSection) return nil;
    if (indexPath.section == RBSettingsBrowsingSection &&
        indexPath.row != RBSettingsBrowsingMediaRow) return nil;
    if (indexPath.section == RBSettingsBrowsingSection && !self.connected) return nil;
    if (indexPath.section == RBSettingsPerformanceSection &&
        indexPath.row == RBSettingsPerformanceOverlayRow) return nil;
    if (indexPath.section == RBSettingsDataSection && !self.connected) return nil;
    if (indexPath.section == RBSettingsAboutSection &&
        indexPath.row != RBSettingsAboutLicensesRow &&
        !(indexPath.row == RBSettingsAboutUpdateRow && self.availableClientUpdate)) return nil;
    return indexPath;
}

- (void)tableView:(UITableView *)tableView didSelectRowAtIndexPath:(NSIndexPath *)indexPath {
    [tableView deselectRowAtIndexPath:indexPath animated:YES];
    if (indexPath.section == RBSettingsServerSection) {
        [self.delegate settingsWantsServers:self];
        return;
    }
    if (indexPath.section == RBSettingsBrowsingSection &&
        indexPath.row == RBSettingsBrowsingMediaRow &&
        [self.delegate respondsToSelector:@selector(settingsWantsMediaControls:)]) {
        [self.delegate settingsWantsMediaControls:self];
        return;
    }
    if (indexPath.section == RBSettingsPerformanceSection) {
        if (indexPath.row == RBSettingsPerformanceInspectorRow) {
            self.diagnosticsVisible = YES;
            [[NSUserDefaults standardUserDefaults] setBool:YES forKey:RBDefaultsDiagnosticsKey];
            [[NSUserDefaults standardUserDefaults] synchronize];
            if ([self.delegate respondsToSelector:@selector(settings:diagnosticsVisible:)]) {
                [self.delegate settings:self diagnosticsVisible:YES];
            }
            if ([self.delegate respondsToSelector:@selector(settingsWantsDiagnosticsInspector:)]) {
                [self.delegate settingsWantsDiagnosticsInspector:self];
            }
        } else if (indexPath.row == RBSettingsPerformanceLogRow) {
            [self.navigationController pushViewController:[[RBLogViewController alloc] init]
                                                 animated:YES];
        }
        return;
    }
    if (indexPath.section == RBSettingsDataSection) {
        [self requestClearDataAtRow:indexPath.row];
        return;
    }
    if (indexPath.section == RBSettingsAboutSection &&
        indexPath.row == RBSettingsAboutUpdateRow && self.availableClientUpdate) {
        if ([self.delegate respondsToSelector:@selector(settingsWantsClientUpdate:)]) {
            [self.delegate settingsWantsClientUpdate:self];
        }
        return;
    }
    if (indexPath.section == RBSettingsAboutSection &&
        indexPath.row == RBSettingsAboutLicensesRow) {
        [self showLicenses];
    }
}

- (void)requestClearDataAtRow:(NSInteger)row {
    if (!self.connected) return;
    static NSString *const values[] = {@"history", @"cookies", @"cache"};
    static NSString *const titles[] = {@"Clear History?", @"Clear Cookies?", @"Clear Cache?"};
    self.pendingClearData = values[row];
    UIAlertView *alert = [[UIAlertView alloc] initWithTitle:titles[row]
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
    self.textView.backgroundColor = [RBTheme surfaceColor];
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
