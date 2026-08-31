#import "RBServersController.h"
#import "RBAddServerController.h"
#import "RBLog.h"
#import "RBServerDetailController.h"
#import "RBServerStore.h"
#import "RBTheme.h"

#import <AVFoundation/AVFoundation.h>
#import <arpa/inet.h>
#import <netinet/in.h>

enum {
    RBServersSavedSection = 0,
    RBServersNearbySection,
    RBServersAddSection,
    RBServersSectionCount
};

@interface RBServersController () <RBAddServerControllerDelegate, RBServerDetailControllerDelegate,
                                    NSNetServiceBrowserDelegate, NSNetServiceDelegate>
@property(nonatomic, copy) NSString *selectedServerID;
@property(nonatomic, copy) NSString *connectingServerID;
@property(nonatomic, assign) BOOL firstLaunch;
@property(nonatomic, strong) NSArray *savedServers;
@property(nonatomic, strong) NSMutableArray *nearbyServers;
@property(nonatomic, strong) NSMutableArray *services;
@property(nonatomic, strong) NSNetServiceBrowser *serviceBrowser;
@property(nonatomic, assign) BOOL searching;
@property(nonatomic, assign) NSUInteger searchGeneration;
@property(nonatomic, copy) NSString *statusText;
@property(nonatomic, assign) BOOL statusIsError;
@property(nonatomic, strong) UILabel *headerLabel;
@property(nonatomic, strong) RBServerDetailController *detailController;
@end

@implementation RBServersController

- (id)initWithSelectedServerID:(NSString *)serverID firstLaunch:(BOOL)firstLaunch {
    self = [super initWithStyle:UITableViewStyleGrouped];
    if (self) {
        self.selectedServerID = serverID;
        self.firstLaunch = firstLaunch;
        self.title = firstLaunch ? @"Connect to Surf" : @"Servers";
        self.nearbyServers = [NSMutableArray array];
        self.services = [NSMutableArray array];
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    [RBTheme styleTableView:self.tableView];
    if (self.allowsCancel) {
        self.navigationItem.leftBarButtonItem = [[UIBarButtonItem alloc] initWithBarButtonSystemItem:UIBarButtonSystemItemCancel
                                                                                             target:self action:@selector(cancelTapped:)];
    }
    self.headerLabel = [[UILabel alloc] initWithFrame:CGRectZero];
    self.headerLabel.backgroundColor = [UIColor clearColor];
    self.headerLabel.font = [RBTheme fontOfSize:14.0 bold:NO];
    self.headerLabel.textAlignment = NSTextAlignmentCenter;
    self.headerLabel.numberOfLines = 0;
    [self reloadServers];
}

- (void)viewWillAppear:(BOOL)animated {
    [super viewWillAppear:animated];
    [self reloadServers];
    [self startDiscovery];
}

- (void)viewWillDisappear:(BOOL)animated {
    [super viewWillDisappear:animated];
    [self stopDiscovery];
}

- (void)dealloc { [self stopDiscovery]; }

- (void)cancelTapped:(id)sender { [self.delegate serversControllerDidCancel:self]; }

- (BOOL)canScanQR {
    return NSClassFromString(@"AVCaptureVideoDataOutput") != nil &&
           [AVCaptureDevice defaultDeviceWithMediaType:AVMediaTypeVideo] != nil;
}

- (void)reloadServers {
    self.savedServers = [RBServerStore servers];
    NSDictionary *selected = [RBServerStore lastSelectedServer];
    if (!self.selectedServerID && selected) self.selectedServerID = [selected objectForKey:@"serverID"];
    if ([self isViewLoaded]) {
        [self updateHeader];
        [self.tableView reloadData];
    }
    self.detailController.requiresPairing =
        [self.detailController.serverID isEqualToString:self.pairingRequiredServerID];
    [self.detailController reloadServer];
}

- (void)setPairingRequiredServerID:(NSString *)serverID {
    if ((_pairingRequiredServerID == serverID) || [_pairingRequiredServerID isEqualToString:serverID]) return;
    _pairingRequiredServerID = [serverID copy];
    self.detailController.requiresPairing =
        [self.detailController.serverID isEqualToString:_pairingRequiredServerID];
    if ([self isViewLoaded]) {
        [self.tableView reloadSections:[NSIndexSet indexSetWithIndex:RBServersSavedSection]
                       withRowAnimation:UITableViewRowAnimationNone];
    }
}

- (void)setStatusText:(NSString *)status isError:(BOOL)isError {
    self.statusText = status;
    self.statusIsError = isError;
    [self.detailController setStatusText:status isError:isError];
    if ([self isViewLoaded]) [self updateHeader];
}

- (void)setConnectingServerID:(NSString *)serverID {
    _connectingServerID = [serverID copy];
    if ([serverID length]) self.selectedServerID = serverID;
    if ([self isViewLoaded]) [self.tableView reloadSections:[NSIndexSet indexSetWithIndex:RBServersSavedSection]
                                           withRowAnimation:UITableViewRowAnimationNone];
}

- (void)updateHeader {
    NSString *message = self.statusText;
    if (![message length] && ![self.savedServers count]) {
        message = @"Choose a server or add one.";
    }
    if (![message length]) {
        self.tableView.tableHeaderView = [[UIView alloc] initWithFrame:CGRectMake(0, 0, 1, 1)];
        return;
    }
    self.headerLabel.text = message;
    self.headerLabel.textColor = self.statusIsError ? [UIColor colorWithRed:.62 green:.12 blue:.12 alpha:1] : [RBTheme secondaryTextColor];
    self.headerLabel.frame = CGRectMake(20.0, 8.0, MAX(1.0, self.tableView.bounds.size.width - 40.0), 64.0);
    UIView *header = [[UIView alloc] initWithFrame:CGRectMake(0, 0, self.tableView.bounds.size.width, 76.0)];
    self.headerLabel.autoresizingMask = UIViewAutoresizingFlexibleWidth;
    [header addSubview:self.headerLabel];
    self.tableView.tableHeaderView = header;
}

- (void)startDiscovery {
    [self stopDiscovery];
    [self.services removeAllObjects];
    [self.nearbyServers removeAllObjects];
    self.searching = YES;
    NSUInteger generation = ++self.searchGeneration;
    self.serviceBrowser = [[NSNetServiceBrowser alloc] init];
    self.serviceBrowser.delegate = self;
    RBLogEvent(@"discovery", @"info", @{@"service": @"_surf._tcp", @"domain": @"local"}, @"Nearby server search started");
    [self.serviceBrowser searchForServicesOfType:@"_surf._tcp." inDomain:@"local."];
    [self.tableView reloadSections:[NSIndexSet indexSetWithIndex:RBServersNearbySection] withRowAnimation:UITableViewRowAnimationNone];
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(3.0 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
        if (generation != self.searchGeneration) return;
        self.searching = NO;
        if ([self isViewLoaded]) [self.tableView reloadSections:[NSIndexSet indexSetWithIndex:RBServersNearbySection] withRowAnimation:UITableViewRowAnimationNone];
    });
}

- (void)stopDiscovery {
    self.searchGeneration++;
    self.searching = NO;
    for (NSNetService *service in self.services) service.delegate = nil;
    [self.serviceBrowser stop];
    self.serviceBrowser.delegate = nil;
    self.serviceBrowser = nil;
}

- (void)netServiceBrowser:(NSNetServiceBrowser *)browser didFindService:(NSNetService *)service moreComing:(BOOL)moreComing {
    service.delegate = self;
    [self.services addObject:service];
    [service resolveWithTimeout:5.0];
}

- (void)netServiceBrowser:(NSNetServiceBrowser *)browser didRemoveService:(NSNetService *)service moreComing:(BOOL)moreComing {
    [self.services removeObject:service];
    NSIndexSet *indexes = [self.nearbyServers indexesOfObjectsPassingTest:^BOOL(NSDictionary *item, NSUInteger index, BOOL *stop) {
        return [item objectForKey:@"service"] == service;
    }];
    if ([indexes count]) [self.nearbyServers removeObjectsAtIndexes:indexes];
    if (!moreComing) [self.tableView reloadSections:[NSIndexSet indexSetWithIndex:RBServersNearbySection] withRowAnimation:UITableViewRowAnimationNone];
}

- (void)netServiceBrowser:(NSNetServiceBrowser *)browser didNotSearch:(NSDictionary *)errorDict {
    self.searching = NO;
    [self setStatusText:@"Nearby server discovery failed. You can still add a server address." isError:YES];
    [self.tableView reloadSections:[NSIndexSet indexSetWithIndex:RBServersNearbySection] withRowAnimation:UITableViewRowAnimationNone];
}

- (NSString *)addressForService:(NSNetService *)service {
    NSString *ipv6Address = nil;
    for (NSData *data in service.addresses) {
        const struct sockaddr *address = [data bytes];
        if (!address) continue;
        if (address->sa_family == AF_INET) {
            char buffer[INET_ADDRSTRLEN];
            const struct sockaddr_in *ipv4 = (const struct sockaddr_in *)address;
            if (inet_ntop(AF_INET, &ipv4->sin_addr, buffer, sizeof(buffer))) return [NSString stringWithUTF8String:buffer];
        }
        if (address->sa_family == AF_INET6 && !ipv6Address) {
            char buffer[INET6_ADDRSTRLEN];
            const struct sockaddr_in6 *ipv6 = (const struct sockaddr_in6 *)address;
            if (inet_ntop(AF_INET6, &ipv6->sin6_addr, buffer, sizeof(buffer))) ipv6Address = [NSString stringWithUTF8String:buffer];
        }
    }
    if ([ipv6Address length]) return [NSString stringWithFormat:@"[%@]", ipv6Address];
    NSString *host = service.hostName;
    return [host hasSuffix:@"."] ? [host substringToIndex:[host length] - 1] : host;
}

- (void)netServiceDidResolveAddress:(NSNetService *)service {
    NSString *host = [self addressForService:service];
    if (![host length] || service.port <= 0) return;
    NSString *endpoint = [RBServerStore normalizeEndpoint:[NSString stringWithFormat:@"%@:%ld", host, (long)service.port]];
    NSDictionary *txt = [NSNetService dictionaryFromTXTRecordData:service.TXTRecordData ?: [NSData data]];
    NSString *(^decode)(NSString *) = ^NSString *(NSString *key) {
        NSData *value = [txt objectForKey:key];
        return [value isKindOfClass:[NSData class]] ? [[NSString alloc] initWithData:value encoding:NSUTF8StringEncoding] : nil;
    };
    NSString *api = decode(@"api") ?: @"";
    if ([api length] && ![api isEqualToString:@"v1"]) return;
    NSDictionary *nearby = @{@"name": decode(@"name") ?: service.name ?: @"Surf",
                             @"serverID": decode(@"id") ?: @"", @"api": api,
                             @"endpoint": endpoint ?: @"", @"service": service};
    if (![endpoint length]) return;
    NSUInteger existingIndex = NSNotFound;
    NSString *serverID = [nearby objectForKey:@"serverID"];
    for (NSUInteger i = 0; i < [self.nearbyServers count]; i++) {
        NSDictionary *existing = [self.nearbyServers objectAtIndex:i];
        BOOL sameID = [serverID length] && [[existing objectForKey:@"serverID"] isEqualToString:serverID];
        BOOL sameEndpoint = [[existing objectForKey:@"endpoint"] isEqualToString:endpoint];
        if (sameID || sameEndpoint) { existingIndex = i; break; }
    }
    if (existingIndex == NSNotFound) [self.nearbyServers addObject:nearby];
    else [self.nearbyServers replaceObjectAtIndex:existingIndex withObject:nearby];
    self.searching = NO;
    [self.tableView reloadSections:[NSIndexSet indexSetWithIndex:RBServersNearbySection] withRowAnimation:UITableViewRowAnimationNone];
}

- (void)netService:(NSNetService *)service didNotResolve:(NSDictionary *)errorDict {
    [self.services removeObject:service];
}

- (NSInteger)numberOfSectionsInTableView:(UITableView *)tableView { return RBServersSectionCount; }

- (NSInteger)tableView:(UITableView *)tableView numberOfRowsInSection:(NSInteger)section {
    if (section == RBServersSavedSection) return [self.savedServers count];
    if (section == RBServersNearbySection) return MAX(1, (NSInteger)[self.nearbyServers count]);
    return [self canScanQR] ? 2 : 1;
}

- (NSString *)tableView:(UITableView *)tableView titleForHeaderInSection:(NSInteger)section {
    static NSString *const titles[] = {@"Saved Servers", @"Nearby Surf Servers", @"Add a Server"};
    return titles[section];
}

- (NSString *)tableView:(UITableView *)tableView titleForFooterInSection:(NSInteger)section {
    if (section == RBServersSavedSection && [self.savedServers count])
        return @"Tap to connect. Use the details button to edit.";
    if (section == RBServersNearbySection) return nil;
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

- (UITableViewCell *)tableView:(UITableView *)tableView cellForRowAtIndexPath:(NSIndexPath *)indexPath {
    if (indexPath.section == RBServersSavedSection) {
        UITableViewCell *cell = [self cell:@"saved" style:UITableViewCellStyleSubtitle];
        NSDictionary *server = [self.savedServers objectAtIndex:indexPath.row];
        NSString *serverID = [server objectForKey:@"serverID"];
        cell.textLabel.text = [server objectForKey:@"name"] ?: @"Surf";
        cell.detailTextLabel.text = [server objectForKey:@"lastEndpoint"];
        // Keep details available even for the selected/connecting server. The
        // old checkmark and spinner accessories hid the only editing affordance.
        cell.accessoryType = UITableViewCellAccessoryDetailDisclosureButton;
        if ([serverID isEqualToString:self.pairingRequiredServerID]) {
            cell.detailTextLabel.text = @"Pairing required \u00b7 Tap to pair again";
            cell.detailTextLabel.textColor = [UIColor colorWithRed:.62 green:.12 blue:.12 alpha:1];
        } else if ([serverID isEqualToString:self.connectingServerID]) {
            cell.detailTextLabel.text = @"Connecting\u2026";
        } else if ([serverID isEqualToString:self.selectedServerID]) {
            if (self.connected) cell.detailTextLabel.text = @"Connected";
        }
        return cell;
    }
    if (indexPath.section == RBServersNearbySection) {
        UITableViewCell *cell = [self cell:@"nearby" style:UITableViewCellStyleSubtitle];
        if (![self.nearbyServers count]) {
            cell.textLabel.text = self.searching ? @"Searching\u2026" : @"No nearby servers found";
            cell.detailTextLabel.text = self.searching ? @"Looking on the local network" : @"Tap to search again";
            if (self.searching) {
                UIActivityIndicatorView *spinner = [[UIActivityIndicatorView alloc] initWithActivityIndicatorStyle:UIActivityIndicatorViewStyleGray];
                [spinner startAnimating];
                cell.accessoryView = spinner;
                cell.selectionStyle = UITableViewCellSelectionStyleNone;
            }
            return cell;
        }
        NSDictionary *nearby = [self.nearbyServers objectAtIndex:indexPath.row];
        cell.textLabel.text = [nearby objectForKey:@"name"] ?: @"Surf";
        NSDictionary *known = [RBServerStore serverWithID:[nearby objectForKey:@"serverID"]];
        NSString *endpoint = [nearby objectForKey:@"endpoint"];
        NSArray *knownEndpoints = [known objectForKey:@"endpoints"];
        BOOL savedAddress = [knownEndpoints isKindOfClass:[NSArray class]] && [knownEndpoints containsObject:endpoint];
        if (!savedAddress && [[known objectForKey:@"lastEndpoint"] isEqualToString:endpoint]) savedAddress = YES;
        cell.detailTextLabel.text = known && !savedAddress ? @"Known server at a new address" : endpoint;
        cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
        return cell;
    }
    UITableViewCell *cell = [self cell:@"add" style:UITableViewCellStyleDefault];
    cell.textLabel.text = indexPath.row == 0 ? @"Add Server Address\u2026" : @"Scan Pairing Code\u2026";
    cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
    return cell;
}

- (void)tableView:(UITableView *)tableView didSelectRowAtIndexPath:(NSIndexPath *)indexPath {
    [tableView deselectRowAtIndexPath:indexPath animated:YES];
    if (indexPath.section == RBServersSavedSection) {
        NSDictionary *server = [self.savedServers objectAtIndex:indexPath.row];
        self.selectedServerID = [server objectForKey:@"serverID"];
        if ([self.selectedServerID isEqualToString:self.pairingRequiredServerID]) {
            [self.delegate serversController:self pairEndpoint:[server objectForKey:@"lastEndpoint"]
                              expectedServerID:self.selectedServerID replacementServer:server qrToken:nil];
            return;
        }
        [self setConnectingServerID:self.selectedServerID];
        [self.delegate serversController:self connectToServer:server];
        return;
    }
    if (indexPath.section == RBServersNearbySection) {
        if (![self.nearbyServers count]) {
            if (!self.searching) [self startDiscovery];
            return;
        }
        NSDictionary *nearby = [self.nearbyServers objectAtIndex:indexPath.row];
        NSDictionary *known = [RBServerStore serverWithID:[nearby objectForKey:@"serverID"]];
        if (known) [self.delegate serversController:self verifyAddress:[nearby objectForKey:@"endpoint"] forServer:known];
        else [self.delegate serversController:self pairEndpoint:[nearby objectForKey:@"endpoint"]
                              expectedServerID:[nearby objectForKey:@"serverID"] replacementServer:nil qrToken:nil];
        return;
    }
    if (indexPath.row == 0) {
        RBAddServerController *add = [[RBAddServerController alloc] initWithEndpoint:nil];
        add.delegate = self;
        [self.navigationController pushViewController:add animated:YES];
    } else {
        [self.delegate serversControllerWantsQRScanner:self];
    }
}

- (void)tableView:(UITableView *)tableView accessoryButtonTappedForRowWithIndexPath:(NSIndexPath *)indexPath {
    if (indexPath.section != RBServersSavedSection || indexPath.row >= (NSInteger)[self.savedServers count]) return;
    NSDictionary *server = [self.savedServers objectAtIndex:indexPath.row];
    RBServerDetailController *detail = [[RBServerDetailController alloc] initWithServer:server];
    detail.delegate = self;
    detail.connected = self.connected && [[server objectForKey:@"serverID"] isEqualToString:self.selectedServerID];
    detail.requiresPairing = [[server objectForKey:@"serverID"] isEqualToString:self.pairingRequiredServerID];
    self.detailController = detail;
    [self.navigationController pushViewController:detail animated:YES];
}

- (void)addServerController:(RBAddServerController *)controller didSubmitEndpoint:(NSString *)endpoint {
    [self.delegate serversController:self pairEndpoint:endpoint expectedServerID:nil replacementServer:nil qrToken:nil];
}

- (void)serverDetailController:(RBServerDetailController *)controller connectToServer:(NSDictionary *)server {
    self.selectedServerID = [server objectForKey:@"serverID"];
    [self.delegate serversController:self connectToServer:server];
}

- (void)serverDetailController:(RBServerDetailController *)controller verifyAddress:(NSString *)endpoint forServer:(NSDictionary *)server {
    [self.delegate serversController:self verifyAddress:endpoint forServer:server];
}

- (void)serverDetailController:(RBServerDetailController *)controller pairAgainServer:(NSDictionary *)server {
    [self.delegate serversController:self pairEndpoint:[server objectForKey:@"lastEndpoint"]
                      expectedServerID:[server objectForKey:@"serverID"] replacementServer:server qrToken:nil];
}

- (void)serverDetailController:(RBServerDetailController *)controller forgetServer:(NSDictionary *)server {
    [self.delegate serversController:self forgetServer:server];
    self.detailController = nil;
    [self.navigationController popViewControllerAnimated:YES];
    [self reloadServers];
}

- (void)serverDetailControllerDidChangeServer:(RBServerDetailController *)controller { [self reloadServers]; }

@end
