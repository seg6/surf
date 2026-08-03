#import "RBSettingsController.h"
#import "RBConfig.h"
#import "RBLogViewController.h"
#import "RBServerStore.h"
#import "RBTheme.h"

enum {
    RBSettingsConnectionSection = 0,
    RBSettingsBrowsingSection,
    RBSettingsDiagnosticsSection,
    RBSettingsDataSection,
    RBSettingsAboutSection,
    RBSettingsSectionCount
};

static const NSInteger kRBClearDataAlert = 4101;

@interface RBSettingsController () <UIAlertViewDelegate>
@property(nonatomic, copy) NSString *selectedServerID;
@property(nonatomic, strong) NSDictionary *selectedServer;
@property(nonatomic, copy) NSString *pendingClearData;
@end

@implementation RBSettingsController

- (id)initWithSelectedServerID:(NSString *)serverID {
    self = [super initWithStyle:UITableViewStyleGrouped];
    if (self) {
        self.selectedServerID = serverID;
        self.title = @"Surf Settings";
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    self.navigationItem.rightBarButtonItem = [[UIBarButtonItem alloc] initWithBarButtonSystemItem:UIBarButtonSystemItemDone
                                                                                          target:self action:@selector(doneTapped:)];
    [self reloadServers];
}

- (void)reloadServers {
    self.selectedServer = [RBServerStore serverWithID:self.selectedServerID] ?: [RBServerStore lastSelectedServer];
    self.selectedServerID = [self.selectedServer objectForKey:@"serverID"];
    if ([self isViewLoaded]) [self.tableView reloadData];
}

- (void)doneTapped:(id)sender { [self.delegate settingsDismissed:self]; }

- (void)diagnosticsChanged:(UISwitch *)sender {
    self.diagnosticsVisible = sender.on;
    [[NSUserDefaults standardUserDefaults] setBool:sender.on forKey:RBDefaultsDiagnosticsKey];
    [[NSUserDefaults standardUserDefaults] synchronize];
    if ([self.delegate respondsToSelector:@selector(settings:diagnosticsVisible:)]) [self.delegate settings:self diagnosticsVisible:sender.on];
}

- (void)preferenceChanged:(UISwitch *)sender {
    NSString *key = sender.tag == 1 ? RBDefaultsMobileLayoutKey : RBDefaultsOfferCopiedLinksKey;
    [[NSUserDefaults standardUserDefaults] setBool:sender.on forKey:key];
    [[NSUserDefaults standardUserDefaults] synchronize];
    if ([self.delegate respondsToSelector:@selector(settings:preference:enabled:)]) [self.delegate settings:self preference:key enabled:sender.on];
}

- (NSInteger)numberOfSectionsInTableView:(UITableView *)tableView { return RBSettingsSectionCount; }

- (NSInteger)tableView:(UITableView *)tableView numberOfRowsInSection:(NSInteger)section {
    if (section == RBSettingsConnectionSection) return 1;
    if (section == RBSettingsBrowsingSection) return 3;
    if (section == RBSettingsDiagnosticsSection) return 2;
    if (section == RBSettingsDataSection) return 3;
    return 1;
}

- (NSString *)tableView:(UITableView *)tableView titleForHeaderInSection:(NSInteger)section {
    static NSString *const titles[] = {@"Connection", @"Browsing", @"Diagnostics", @"Data", @"About"};
    return titles[section];
}

- (NSString *)tableView:(UITableView *)tableView titleForFooterInSection:(NSInteger)section {
    if (section == RBSettingsDiagnosticsSection) return @"Inspect live performance and structured application events.";
    if (section == RBSettingsDataSection && !self.connected) return @"Connect to a server to manage its browsing data.";
    return nil;
}

- (UITableViewCell *)cell:(NSString *)identifier style:(UITableViewCellStyle)style {
    UITableViewCell *cell = [self.tableView dequeueReusableCellWithIdentifier:identifier];
    if (!cell) cell = [[UITableViewCell alloc] initWithStyle:style reuseIdentifier:identifier];
    cell.accessoryType = UITableViewCellAccessoryNone;
    cell.accessoryView = nil;
    cell.selectionStyle = UITableViewCellSelectionStyleBlue;
    cell.textLabel.textColor = [RBTheme primaryTextColor];
    cell.detailTextLabel.textColor = [RBTheme secondaryTextColor];
    cell.detailTextLabel.text = nil;
    return cell;
}

- (UITableViewCell *)tableView:(UITableView *)tableView cellForRowAtIndexPath:(NSIndexPath *)indexPath {
    NSInteger section = indexPath.section, row = indexPath.row;
    if (section == RBSettingsConnectionSection) {
        UITableViewCell *cell = [self cell:@"servers" style:UITableViewCellStyleValue1];
        cell.textLabel.text = @"Servers";
        NSString *name = [self.selectedServer objectForKey:@"name"];
        if ([name length]) cell.detailTextLabel.text = self.connected ? [NSString stringWithFormat:@"%@ \u00b7 Connected", name] : name;
        else cell.detailTextLabel.text = @"None";
        cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
        return cell;
    }
    if (section == RBSettingsBrowsingSection) {
        UITableViewCell *cell = [self cell:@"browsing" style:UITableViewCellStyleSubtitle];
        if (row == 0) {
            cell.textLabel.text = @"Page Media Controls";
            cell.detailTextLabel.text = @"playback, mute, and volume";
            cell.selectionStyle = self.connected ? UITableViewCellSelectionStyleBlue : UITableViewCellSelectionStyleNone;
            return cell;
        }
        BOOL mobile = row == 1;
        cell.textLabel.text = mobile ? @"Request Mobile Sites" : @"Offer Copied Links";
        cell.detailTextLabel.text = mobile ? @"identify as mobile Chrome" : @"ask to open copied web addresses";
        cell.selectionStyle = UITableViewCellSelectionStyleNone;
        UISwitch *toggle = [[UISwitch alloc] initWithFrame:CGRectZero];
        toggle.tag = mobile ? 1 : 2;
        toggle.on = [[NSUserDefaults standardUserDefaults] boolForKey:(mobile ? RBDefaultsMobileLayoutKey : RBDefaultsOfferCopiedLinksKey)];
        [toggle addTarget:self action:@selector(preferenceChanged:) forControlEvents:UIControlEventValueChanged];
        cell.accessoryView = toggle;
        return cell;
    }
    if (section == RBSettingsDiagnosticsSection) {
        if (row == 1) {
            UITableViewCell *cell = [self cell:@"live-log" style:UITableViewCellStyleSubtitle];
            cell.textLabel.text = @"Logs";
            cell.detailTextLabel.text = @"application events, warnings, and errors";
            cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
            return cell;
        }
        UITableViewCell *cell = [self cell:@"diagnostics" style:UITableViewCellStyleDefault];
        cell.textLabel.text = @"Performance Overlay";
        cell.selectionStyle = UITableViewCellSelectionStyleNone;
        UISwitch *toggle = [[UISwitch alloc] initWithFrame:CGRectZero];
        toggle.on = self.diagnosticsVisible;
        [toggle addTarget:self action:@selector(diagnosticsChanged:) forControlEvents:UIControlEventValueChanged];
        cell.accessoryView = toggle;
        return cell;
    }
    if (section == RBSettingsDataSection) {
        static NSString *const labels[] = {@"Clear History", @"Clear Cookies", @"Clear Cache"};
        UITableViewCell *cell = [self cell:@"data" style:UITableViewCellStyleDefault];
        cell.textLabel.text = labels[row];
        cell.textLabel.textColor = self.connected ? [UIColor colorWithRed:.62 green:.12 blue:.12 alpha:1] : [UIColor colorWithWhite:.6 alpha:1];
        cell.selectionStyle = self.connected ? UITableViewCellSelectionStyleBlue : UITableViewCellSelectionStyleNone;
        return cell;
    }
    UITableViewCell *cell = [self cell:@"about" style:UITableViewCellStyleValue1];
    cell.selectionStyle = UITableViewCellSelectionStyleNone;
    cell.textLabel.text = @"Version";
    cell.detailTextLabel.text = [NSString stringWithFormat:@"%@ \u00b7 protocol %@", RBAppVersion, RBNativeVersion];
    return cell;
}

- (void)tableView:(UITableView *)tableView didSelectRowAtIndexPath:(NSIndexPath *)indexPath {
    [tableView deselectRowAtIndexPath:indexPath animated:YES];
    if (indexPath.section == RBSettingsConnectionSection) {
        [self.delegate settingsWantsServers:self];
        return;
    }
    if (indexPath.section == RBSettingsBrowsingSection && indexPath.row == 0 && self.connected &&
        [self.delegate respondsToSelector:@selector(settingsWantsMediaControls:)]) {
        [self.delegate settingsWantsMediaControls:self];
        return;
    }
    if (indexPath.section == RBSettingsDiagnosticsSection && indexPath.row == 1) {
        [self.navigationController pushViewController:[[RBLogViewController alloc] init] animated:YES];
        return;
    }
    if (indexPath.section == RBSettingsDataSection && self.connected) {
        static NSString *const values[] = {@"history", @"cookies", @"cache"};
        static NSString *const titles[] = {@"Clear History?", @"Clear Cookies?", @"Clear Cache?"};
        self.pendingClearData = values[indexPath.row];
        UIAlertView *alert = [[UIAlertView alloc] initWithTitle:titles[indexPath.row]
                                                       message:@"This affects the connected Surf server."
                                                      delegate:self cancelButtonTitle:@"Cancel" otherButtonTitles:@"Clear", nil];
        alert.tag = kRBClearDataAlert;
        [alert show];
    }
}

- (void)alertView:(UIAlertView *)alertView clickedButtonAtIndex:(NSInteger)buttonIndex {
    if (buttonIndex == alertView.cancelButtonIndex || alertView.tag != kRBClearDataAlert) return;
    if ([self.delegate respondsToSelector:@selector(settings:clearData:)]) [self.delegate settings:self clearData:self.pendingClearData];
    self.pendingClearData = nil;
}

@end
