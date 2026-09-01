#import "RBServerDetailController.h"
#import "RBLog.h"
#import "RBSession.h"
#import "RBServerStore.h"
#import "RBTheme.h"

enum {
    RBServerDetailOverviewSection = 0,
    RBServerDetailAddressesSection,
    RBServerDetailIdentitySection,
    RBServerDetailActionsSection,
    RBServerDetailSectionCount
};

static const NSInteger kRBServerRenameAlert = 3201;
static const NSInteger kRBServerAddAddressAlert = 3202;
static const NSInteger kRBServerForgetAlert = 3203;

@interface RBServerDetailController () <UIAlertViewDelegate>
@property(nonatomic, strong) NSDictionary *server;
@property(nonatomic, copy, readwrite) NSString *serverID;
@property(nonatomic, copy) NSString *statusText;
@property(nonatomic, assign) BOOL statusIsError;
@end

@implementation RBServerDetailController

- (void)viewDidLoad {
    [super viewDidLoad];
    [RBTheme styleTableView:self.tableView];
}

- (id)initWithServer:(NSDictionary *)server {
    self = [super initWithStyle:UITableViewStyleGrouped];
    if (self) {
        self.server = server;
        self.serverID = [server objectForKey:@"serverID"];
        self.title = [server objectForKey:@"name"] ?: @"Surf Server";
    }
    return self;
}

- (void)reloadServer {
    NSDictionary *updated = [RBServerStore serverWithID:self.serverID];
    if (updated) self.server = updated;
    self.title = [self.server objectForKey:@"name"] ?: @"Surf Server";
    if ([self isViewLoaded]) [self.tableView reloadData];
}

- (void)setStatusText:(NSString *)status isError:(BOOL)isError {
    self.statusText = status;
    self.statusIsError = isError;
    if ([self isViewLoaded]) [self.tableView reloadData];
}

- (void)setRequiresPairing:(BOOL)requiresPairing {
    if (_requiresPairing == requiresPairing) return;
    _requiresPairing = requiresPairing;
    if ([self isViewLoaded]) [self.tableView reloadData];
}

- (NSArray *)endpoints {
    id value = [self.server objectForKey:@"endpoints"];
    if ([value isKindOfClass:[NSArray class]] && [value count]) return value;
    NSString *last = [self.server objectForKey:@"lastEndpoint"];
    return [last length] ? @[last] : @[];
}

- (NSInteger)numberOfSectionsInTableView:(UITableView *)tableView { return RBServerDetailSectionCount; }

- (NSInteger)tableView:(UITableView *)tableView numberOfRowsInSection:(NSInteger)section {
    if (section == RBServerDetailOverviewSection) return (self.statusText || self.requiresPairing) ? 3 : 2;
    if (section == RBServerDetailAddressesSection) return [self endpoints].count + 1;
    if (section == RBServerDetailIdentitySection) return 1;
    return 3;
}

- (NSString *)tableView:(UITableView *)tableView titleForHeaderInSection:(NSInteger)section {
    static NSString *const titles[] = {@"Server", @"Verified Addresses", @"Security", @"Actions"};
    return titles[section];
}

- (NSString *)tableView:(UITableView *)tableView titleForFooterInSection:(NSInteger)section {
    if (section == RBServerDetailAddressesSection) return @"New addresses must match this server.";
    if (section == RBServerDetailIdentitySection) return @"Pinned server identity.";
    if (section == RBServerDetailActionsSection && self.requiresPairing)
        return @"Pair again to reconnect.";
    if (section == RBServerDetailActionsSection)
        return @"Forgetting also revokes this device when the server is online.";
    return nil;
}

- (UITableViewCell *)cell:(NSString *)identifier style:(UITableViewCellStyle)style {
    UITableViewCell *cell = [self.tableView dequeueReusableCellWithIdentifier:identifier];
    if (!cell) cell = [[UITableViewCell alloc] initWithStyle:style reuseIdentifier:identifier];
    cell.accessoryType = UITableViewCellAccessoryNone;
    cell.accessoryView = nil;
    cell.selectionStyle = UITableViewCellSelectionStyleBlue;
    cell.backgroundColor = [RBTheme surfaceColor];
    cell.textLabel.textColor = [RBTheme primaryTextColor];
    cell.detailTextLabel.textColor = [RBTheme secondaryTextColor];
    cell.detailTextLabel.text = nil;
    return cell;
}

- (NSString *)lastConnectedText {
    NSDate *date = [self.server objectForKey:@"lastConnected"];
    if (![date isKindOfClass:[NSDate class]]) return @"Never connected";
    NSDateFormatter *formatter = [[NSDateFormatter alloc] init];
    formatter.dateStyle = NSDateFormatterMediumStyle;
    formatter.timeStyle = NSDateFormatterShortStyle;
    return [formatter stringFromDate:date];
}

- (UITableViewCell *)tableView:(UITableView *)tableView cellForRowAtIndexPath:(NSIndexPath *)indexPath {
    NSInteger section = indexPath.section, row = indexPath.row;
    if (section == RBServerDetailOverviewSection) {
        UITableViewCell *cell = [self cell:@"overview" style:UITableViewCellStyleValue1];
        if (row == 0) { cell.textLabel.text = @"Name"; cell.detailTextLabel.text = [self.server objectForKey:@"name"] ?: @"Surf"; }
        else if (row == 1) { cell.textLabel.text = @"Status"; cell.detailTextLabel.text = self.connected ? @"Connected" : [self lastConnectedText]; }
        else {
            cell.textLabel.text = self.statusText ?: @"Pairing required. This device must be approved again.";
            cell.textLabel.font = [RBTheme fontOfSize:13.0 bold:NO];
            cell.textLabel.textColor = (self.statusIsError || self.requiresPairing) ? [UIColor colorWithRed:.62 green:.12 blue:.12 alpha:1] : [RBTheme secondaryTextColor];
        }
        cell.selectionStyle = UITableViewCellSelectionStyleNone;
        return cell;
    }
    if (section == RBServerDetailAddressesSection) {
        NSArray *endpoints = [self endpoints];
        if (row < (NSInteger)[endpoints count]) {
            UITableViewCell *cell = [self cell:@"endpoint" style:UITableViewCellStyleSubtitle];
            NSString *endpoint = [endpoints objectAtIndex:row];
            cell.textLabel.text = [endpoint hasPrefix:@"https://"] ? [endpoint substringFromIndex:8] : endpoint;
            cell.detailTextLabel.text = [endpoint isEqualToString:[self.server objectForKey:@"lastEndpoint"]] ? @"Preferred address" : @"Tap to connect";
            cell.accessoryType = [endpoint isEqualToString:[self.server objectForKey:@"lastEndpoint"]] ? UITableViewCellAccessoryCheckmark : UITableViewCellAccessoryNone;
            return cell;
        }
        UITableViewCell *cell = [self cell:@"add-address" style:UITableViewCellStyleDefault];
        cell.textLabel.text = @"Add Address\u2026";
        cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
        return cell;
    }
    if (section == RBServerDetailIdentitySection) {
        UITableViewCell *cell = [self cell:@"identity" style:UITableViewCellStyleValue1];
        NSString *identity = [self.server objectForKey:@"fingerprint"] ?: @"";
        cell.textLabel.text = @"Server Identity";
        cell.detailTextLabel.text = [identity length] > 12 ? [NSString stringWithFormat:@"%@\u2026%@", [identity substringToIndex:6], [identity substringFromIndex:[identity length] - 6]] : identity;
        return cell;
    }
    UITableViewCell *cell = [self cell:@"action" style:UITableViewCellStyleDefault];
    if (row == 0) { cell.textLabel.text = @"Rename Server\u2026"; cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator; }
    else if (row == 1) {
        cell.textLabel.text = self.requiresPairing ? @"Pair Again Now\u2026" : @"Pair Again\u2026";
        cell.textLabel.textColor = self.requiresPairing ? [RBTheme accentColor] : [RBTheme primaryTextColor];
        cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
    }
    else { cell.textLabel.text = @"Forget Server"; cell.textLabel.textColor = [UIColor colorWithRed:.62 green:.12 blue:.12 alpha:1]; }
    return cell;
}

- (void)tableView:(UITableView *)tableView didSelectRowAtIndexPath:(NSIndexPath *)indexPath {
    [tableView deselectRowAtIndexPath:indexPath animated:YES];
    if (indexPath.section == RBServerDetailAddressesSection) {
        NSArray *endpoints = [self endpoints];
        if (indexPath.row < (NSInteger)[endpoints count]) {
            NSString *endpoint = [endpoints objectAtIndex:indexPath.row];
            NSMutableDictionary *updated = [self.server mutableCopy];
            [updated setObject:endpoint forKey:@"lastEndpoint"];
            [RBServerStore saveServer:updated select:NO];
            self.server = updated;
            [self.tableView reloadData];
            [self.delegate serverDetailController:self connectToServer:updated];
        } else {
            UIAlertView *alert = [[UIAlertView alloc] initWithTitle:@"Add Address"
                                                           message:@"Enter another address for this same Surf server."
                                                          delegate:self cancelButtonTitle:@"Cancel" otherButtonTitles:@"Verify", nil];
            alert.tag = kRBServerAddAddressAlert;
            alert.alertViewStyle = UIAlertViewStylePlainTextInput;
            [alert textFieldAtIndex:0].keyboardType = UIKeyboardTypeURL;
            [alert show];
        }
        return;
    }
    if (indexPath.section == RBServerDetailIdentitySection) {
        [UIPasteboard generalPasteboard].string = [self.server objectForKey:@"fingerprint"] ?: @"";
        [self setStatusText:@"Server identity copied" isError:NO];
        return;
    }
    if (indexPath.section != RBServerDetailActionsSection) return;
    if (indexPath.row == 0) {
        UIAlertView *alert = [[UIAlertView alloc] initWithTitle:@"Rename Server" message:nil delegate:self cancelButtonTitle:@"Cancel" otherButtonTitles:@"Save", nil];
        alert.tag = kRBServerRenameAlert;
        alert.alertViewStyle = UIAlertViewStylePlainTextInput;
        [alert textFieldAtIndex:0].text = [self.server objectForKey:@"name"];
        [alert show];
    } else if (indexPath.row == 1) {
        [self.delegate serverDetailController:self pairAgainServer:self.server];
    } else {
        UIAlertView *alert = [[UIAlertView alloc] initWithTitle:@"Forget Server?"
                                                       message:@"This also revokes this device when the server is online."
                                                      delegate:self cancelButtonTitle:@"Cancel" otherButtonTitles:@"Forget Server", nil];
        alert.tag = kRBServerForgetAlert;
        [alert show];
    }
}

- (void)alertView:(UIAlertView *)alertView clickedButtonAtIndex:(NSInteger)buttonIndex {
    if (buttonIndex == alertView.cancelButtonIndex) return;
    if (alertView.tag == kRBServerRenameAlert) {
        [RBServerStore renameServerID:self.serverID name:[alertView textFieldAtIndex:0].text];
        [self reloadServer];
        [self.delegate serverDetailControllerDidChangeServer:self];
    } else if (alertView.tag == kRBServerAddAddressAlert) {
        NSString *address = [alertView textFieldAtIndex:0].text;
    NSString *loggedEndpoint = [RBServerStore normalizeEndpoint:address] ?: @"";
    RBLogEvent(@"verification", @"info", @{@"phase": @"tap", @"endpoint": loggedEndpoint},
               @"Verify address requested");
        [self.delegate serverDetailController:self verifyAddress:address forServer:self.server];
    } else if (alertView.tag == kRBServerForgetAlert) {
        self.tableView.userInteractionEnabled = NO;
        [self setStatusText:@"Forgetting this server\u2026" isError:NO];
        NSDictionary *server = self.server;
        dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
            NSString *error = nil;
            RBSession *session = [[RBSession alloc] initWithServer:server];
            [session revokeThisDevice:&error];
            dispatch_async(dispatch_get_main_queue(), ^{
                self.tableView.userInteractionEnabled = YES;
                [self.delegate serverDetailController:self forgetServer:server];
            });
        });
    }
}

@end
