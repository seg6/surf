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
    self.tableView.rowHeight = 44.0;
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

- (NSString *)tableView:(UITableView *)tableView titleForHeaderInSection:(NSInteger)section {
    return [self titleForSection:section];
}

- (NSString *)tableView:(UITableView *)tableView titleForFooterInSection:(NSInteger)section {
    if (section == RBSettingsAppearanceSection) return @"Dark Mode also requests dark colors from websites that support them.";
    if (section == RBSettingsBrowsingSection) return @"Offer Copied Links asks before opening a web address from the clipboard.";
    if (section == RBSettingsDataSection) {
        return self.connected ? @"Clearing cookies signs you out of websites on this server."
                              : @"Connect to a Surf server to manage its browsing data.";
    }
    return nil;
}

- (void)tableView:(UITableView *)tableView willDisplayHeaderView:(UIView *)view forSection:(NSInteger)section {
    [RBTheme styleTableSectionView:view];
}

- (void)tableView:(UITableView *)tableView willDisplayFooterView:(UIView *)view forSection:(NSInteger)section {
    [RBTheme styleTableSectionView:view];
}

- (CGFloat)tableView:(UITableView *)tableView heightForRowAtIndexPath:(NSIndexPath *)indexPath {
    return indexPath.section == RBSettingsServerSection ? 60.0 : 44.0;
}

- (void)tableView:(UITableView *)tableView willDisplayCell:(UITableViewCell *)cell
 forRowAtIndexPath:(NSIndexPath *)indexPath {
    // UIKit installs/updates its grouped background when inserting a cell.
    // Apply the fill afterwards, on every display (including reused rows),
    // without removing the views that own rounded edges and selection.
    cell.backgroundColor = [RBTheme groupedCellColor];
}

- (UITableViewCell *)cellWithIdentifier:(NSString *)identifier
                                title:(NSString *)title
                                value:(NSString *)value
                              enabled:(BOOL)enabled {
    UITableViewCellStyle style = [identifier isEqualToString:@"server"]
        ? UITableViewCellStyleSubtitle : UITableViewCellStyleValue1;
    UITableViewCell *cell = [self.tableView dequeueReusableCellWithIdentifier:identifier];
    if (!cell) cell = [[UITableViewCell alloc] initWithStyle:style reuseIdentifier:identifier];
    cell.textLabel.text = title;
    cell.detailTextLabel.text = value;
    cell.accessoryType = UITableViewCellAccessoryNone;
    cell.accessoryView = nil;
    cell.selectionStyle = enabled ? UITableViewCellSelectionStyleBlue : UITableViewCellSelectionStyleNone;
    cell.userInteractionEnabled = enabled;
    cell.accessibilityTraits = enabled ? UIAccessibilityTraitNone : UIAccessibilityTraitNotEnabled;
    cell.textLabel.textColor = enabled ? [RBTheme primaryTextColor] : [UIColor grayColor];
    cell.detailTextLabel.textColor = [RBTheme secondaryTextColor];
    // Do not clear backgroundView/selectedBackgroundView on reuse: on classic
    // UIKit these are the system's grouped-row backgrounds, not our decoration.
    return cell;
}

- (UISwitch *)switchOn:(BOOL)on target:(SEL)action label:(NSString *)label {
    UISwitch *toggle = [[UISwitch alloc] initWithFrame:CGRectZero];
    toggle.on = on;
    toggle.accessibilityLabel = label;
    [toggle addTarget:self action:action forControlEvents:UIControlEventValueChanged];
    return toggle;
}

- (UITableViewCell *)tableView:(UITableView *)tableView
         cellForRowAtIndexPath:(NSIndexPath *)indexPath {
    NSInteger section = indexPath.section;
    NSInteger row = indexPath.row;
    UITableViewCell *cell = nil;

    if (section == RBSettingsServerSection) {
        NSString *name = [self.selectedServer objectForKey:@"name"];
        NSString *status = [name length] ? (self.connected ? @"Connected" : @"Not connected") : @"Add or select a server";
        cell = [self cellWithIdentifier:@"server" title:[name length] ? name : @"Choose a Server"
                                 value:status enabled:YES];
        cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
    } else if (section == RBSettingsAppearanceSection) {
        BOOL dark = row == RBSettingsAppearanceDarkModeRow;
        NSString *title = dark ? @"Dark Mode" : @"Bottom Browser Bar";
        NSString *key = dark ? RBDefaultsDarkModeKey : RBDefaultsBottomBrowserBarKey;
        cell = [self cellWithIdentifier:@"toggle" title:title value:nil enabled:YES];
        cell.accessoryView = [self switchOn:[[NSUserDefaults standardUserDefaults] boolForKey:key]
                                    target:dark ? @selector(darkModeChanged:) : @selector(bottomBrowserBarChanged:)
                                     label:title];
        cell.selectionStyle = UITableViewCellSelectionStyleNone;
    } else if (section == RBSettingsBrowsingSection) {
        if (row == RBSettingsBrowsingMediaRow) {
            cell = [self cellWithIdentifier:@"action" title:@"Page Media Controls" value:nil enabled:self.connected];
            if (self.connected) cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
        } else {
            BOOL mobile = row == RBSettingsBrowsingMobileRow;
            NSString *title = mobile ? @"Request Mobile Sites" : @"Offer Copied Links";
            NSString *key = mobile ? RBDefaultsMobileLayoutKey : RBDefaultsOfferCopiedLinksKey;
            cell = [self cellWithIdentifier:@"toggle" title:title value:nil enabled:YES];
            cell.accessoryView = [self switchOn:[[NSUserDefaults standardUserDefaults] boolForKey:key]
                                        target:mobile ? @selector(mobileChanged:) : @selector(copiedLinksChanged:)
                                         label:title];
            cell.selectionStyle = UITableViewCellSelectionStyleNone;
        }
    } else if (section == RBSettingsPerformanceSection) {
        if (row == RBSettingsPerformanceOverlayRow) {
            cell = [self cellWithIdentifier:@"toggle" title:@"Performance Overlay" value:nil enabled:YES];
            cell.accessoryView = [self switchOn:self.diagnosticsVisible target:@selector(diagnosticsChanged:) label:@"Performance Overlay"];
            cell.selectionStyle = UITableViewCellSelectionStyleNone;
        } else {
            cell = [self cellWithIdentifier:@"action"
                title:row == RBSettingsPerformanceInspectorRow ? @"Live Inspector" : @"Event Log" value:nil enabled:YES];
            cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
        }
    } else if (section == RBSettingsDataSection) {
        static NSString *const titles[] = {@"Clear History", @"Clear Cookies", @"Clear Cache"};
        cell = [self cellWithIdentifier:@"data" title:titles[row] value:nil enabled:self.connected];
        if (self.connected) cell.textLabel.textColor = [UIColor colorWithRed:1.0 green:0.23 blue:0.19 alpha:1.0];
    } else if (row == RBSettingsAboutClientRow || row == RBSettingsAboutCompatibilityRow) {
        BOOL version = row == RBSettingsAboutClientRow;
        cell = [self cellWithIdentifier:@"about" title:version ? @"Version" : @"Compatibility"
                                 value:version ? RBAppVersion : RBCompatibilityVersion enabled:YES];
        cell.selectionStyle = UITableViewCellSelectionStyleNone;
    } else if (row == RBSettingsAboutUpdateRow) {
        NSDictionary *update = self.availableClientUpdate;
        if (update) {
            cell = [self cellWithIdentifier:@"action" title:@"Client Update"
                                     value:[update objectForKey:@"version"] ?: @"" enabled:YES];
            cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
        } else {
            cell = [self cellWithIdentifier:@"about" title:@"Client Updates" value:@"Up to Date" enabled:YES];
            cell.selectionStyle = UITableViewCellSelectionStyleNone;
        }
    } else {
        cell = [self cellWithIdentifier:@"action" title:@"Third-Party Licenses" value:nil enabled:YES];
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
