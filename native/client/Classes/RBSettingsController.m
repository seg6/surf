#import "RBSettingsController.h"
#import "RBConfig.h"
#import "RBTheme.h"

#import <arpa/inet.h>
#import <netinet/in.h>

enum {
    RBSectionServer = 0,
    RBSectionSaved,
    RBSectionStream,
    RBSectionData,
    RBSectionAbout,
    RBSectionCount
};

static NSString *const kProfiles[] = {@"sharp", @"smooth", @"balanced", @"fast", @"potato", @"max"};
static NSString *const kProfileTitles[] = {@"Sharp 30", @"Smooth 60", @"Balanced 30", @"Fast 45", @"Low Data", @"Max 60"};
static const int kProfileCount = 6;

@interface RBSettingsController () <UITextFieldDelegate, NSNetServiceBrowserDelegate, NSNetServiceDelegate>
@property(nonatomic, copy) NSString *initialURL;
@property(nonatomic, copy) NSString *initialPassword;
@property(nonatomic, strong) UITextField *urlField;
@property(nonatomic, strong) UITextField *passwordField;
@property(nonatomic, strong) UISwitch *videoSwitch;
@property(nonatomic, strong) UISwitch *diagSwitch;
@property(nonatomic, copy) NSString *statusText;
@property(nonatomic, assign) BOOL statusIsError;
@property(nonatomic, strong) NSNetServiceBrowser *serviceBrowser;
@property(nonatomic, strong) NSMutableArray *services;
@property(nonatomic, strong) NSArray *savedServers; // RBListItem-free: [{title,url}]
@end

@implementation RBSettingsController

- (id)initWithServerURL:(NSString *)serverURL password:(NSString *)password {
    self = [super initWithStyle:UITableViewStyleGrouped];
    if (self) {
        self.initialURL = serverURL;
        self.initialPassword = password;
        self.title = @"Settings";
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    self.navigationItem.rightBarButtonItem =
        [[UIBarButtonItem alloc] initWithBarButtonSystemItem:UIBarButtonSystemItemDone
                                                      target:self action:@selector(doneTapped:)];

    self.urlField = [self fieldWithPlaceholder:@"http://server" text:self.initialURL];
    self.urlField.keyboardType = UIKeyboardTypeURL;
    self.urlField.returnKeyType = UIReturnKeyNext;
    self.passwordField = [self fieldWithPlaceholder:@"password" text:self.initialPassword];
    self.passwordField.secureTextEntry = YES;
    self.passwordField.returnKeyType = UIReturnKeyGo;

    self.videoSwitch = [[UISwitch alloc] initWithFrame:CGRectZero];
    NSNumber *videoDefault = [[NSUserDefaults standardUserDefaults] objectForKey:RBDefaultsVideoKey];
    self.videoSwitch.on = videoDefault == nil || [videoDefault boolValue];
    [self.videoSwitch addTarget:self action:@selector(videoToggled:) forControlEvents:UIControlEventValueChanged];

    self.diagSwitch = [[UISwitch alloc] initWithFrame:CGRectZero];
    self.diagSwitch.on = self.diagnosticsVisible;
    [self.diagSwitch addTarget:self action:@selector(diagToggled:) forControlEvents:UIControlEventValueChanged];

    [self reloadSavedServers];
}

- (UITextField *)fieldWithPlaceholder:(NSString *)placeholder text:(NSString *)text {
    UITextField *field = [[UITextField alloc] initWithFrame:CGRectMake(0.0, 0.0, 300.0, 24.0)];
    field.delegate = self;
    field.font = [RBTheme fontOfSize:15.0 bold:NO];
    field.placeholder = placeholder;
    field.text = text;
    field.autocorrectionType = UITextAutocorrectionTypeNo;
    field.autocapitalizationType = UITextAutocapitalizationTypeNone;
    field.contentVerticalAlignment = UIControlContentVerticalAlignmentCenter;
    field.clearButtonMode = UITextFieldViewModeWhileEditing;
    return field;
}

- (void)reloadSavedServers {
    NSMutableArray *items = [NSMutableArray array];
    [items addObject:@{@"title": @"Surf VPS", @"url": RBDefaultServerURL}];
    for (NSDictionary *entry in [[NSUserDefaults standardUserDefaults] arrayForKey:RBDefaultsServersKey] ?: @[]) {
        NSString *url = [entry objectForKey:@"url"];
        if (![url length] || [url isEqualToString:RBDefaultServerURL]) continue;
        [items addObject:entry];
    }
    self.savedServers = items;
}

- (void)doneTapped:(id)sender {
    [self.view endEditing:YES];
    [self.delegate settingsDismissed:self];
}

- (void)setStatusText:(NSString *)status isError:(BOOL)isError {
    self.statusText = status;
    self.statusIsError = isError;
    if ([self isViewLoaded]) {
        [self.tableView reloadSections:[NSIndexSet indexSetWithIndex:RBSectionServer]
                      withRowAnimation:UITableViewRowAnimationNone];
    }
}

// ---- actions -------------------------------------------------------------

- (void)connect {
    [self.view endEditing:YES];
    NSString *url = [self.urlField.text stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
    if (![url length]) {
        [self setStatusText:@"Server URL is required" isError:YES];
        return;
    }
    if ([url rangeOfString:@"://"].location == NSNotFound) url = [@"http://" stringByAppendingString:url];
    [self setStatusText:@"Connecting…" isError:NO];
    [self.delegate settings:self connectToURL:url password:self.passwordField.text ?: @""];
}

- (void)videoToggled:(id)sender {
    [[NSUserDefaults standardUserDefaults] setObject:[NSNumber numberWithBool:self.videoSwitch.on]
                                              forKey:RBDefaultsVideoKey];
    [[NSUserDefaults standardUserDefaults] synchronize];
    if ([self.delegate respondsToSelector:@selector(settingsStreamChanged:)]) {
        [self.delegate settingsStreamChanged:self];
    }
}

- (void)diagToggled:(id)sender {
    self.diagnosticsVisible = self.diagSwitch.on;
    if ([self.delegate respondsToSelector:@selector(settings:setDiagnosticsVisible:)]) {
        [self.delegate settings:self setDiagnosticsVisible:self.diagSwitch.on];
    }
}

- (NSString *)currentProfile {
    NSString *p = [[NSUserDefaults standardUserDefaults] stringForKey:RBDefaultsStreamProfileKey];
    return [p length] ? p : @"balanced";
}

// ---- discovery -----------------------------------------------------------

- (void)startDiscovery {
    [self.serviceBrowser stop];
    self.services = [NSMutableArray array];
    self.serviceBrowser = [[NSNetServiceBrowser alloc] init];
    self.serviceBrowser.delegate = self;
    [self setStatusText:@"Looking for Surf on this Wi-Fi…" isError:NO];
    [self.serviceBrowser searchForServicesOfType:@"_surf._tcp." inDomain:@"local."];
}

- (void)netServiceBrowser:(NSNetServiceBrowser *)browser didFindService:(NSNetService *)service moreComing:(BOOL)moreComing {
    service.delegate = self;
    [self.services addObject:service];
    [self setStatusText:[NSString stringWithFormat:@"Found %@, resolving…", service.name] isError:NO];
    [service resolveWithTimeout:4.0];
}

- (void)netServiceBrowser:(NSNetServiceBrowser *)browser didNotSearch:(NSDictionary *)errorDict {
    [self setStatusText:@"Local discovery failed" isError:YES];
}

- (void)netServiceDidResolveAddress:(NSNetService *)service {
    [self.serviceBrowser stop];
    NSString *host = [self IPv4AddressForService:service] ?: service.hostName ?: @"";
    if ([host hasSuffix:@"."]) host = [host substringToIndex:[host length] - 1];
    if (![host length] || service.port <= 0) {
        [self setStatusText:@"Found Surf but could not read its address" isError:YES];
        return;
    }
    self.urlField.text = [NSString stringWithFormat:@"http://%@:%ld", host, (long)service.port];
    [self setStatusText:[NSString stringWithFormat:@"Found local Surf: %@ — tap Connect", service.name] isError:NO];
}

- (void)netService:(NSNetService *)service didNotResolve:(NSDictionary *)errorDict {
    [self setStatusText:@"Found Surf but could not resolve it" isError:YES];
}

- (NSString *)IPv4AddressForService:(NSNetService *)service {
    for (NSData *data in service.addresses) {
        const struct sockaddr *addr = (const struct sockaddr *)[data bytes];
        if (!addr || addr->sa_family != AF_INET) continue;
        char buf[INET_ADDRSTRLEN];
        const struct sockaddr_in *in = (const struct sockaddr_in *)addr;
        if (inet_ntop(AF_INET, &(in->sin_addr), buf, sizeof(buf))) return [NSString stringWithUTF8String:buf];
    }
    return nil;
}

// ---- table ---------------------------------------------------------------

- (NSInteger)numberOfSectionsInTableView:(UITableView *)tableView { return RBSectionCount; }

- (NSInteger)tableView:(UITableView *)tableView numberOfRowsInSection:(NSInteger)section {
    switch (section) {
        case RBSectionServer: return [self.statusText length] ? 4 : 3; // url, password, connect, (status)
        case RBSectionSaved: return (NSInteger)[self.savedServers count] + 1; // + Find Local Surf
        case RBSectionStream: return 1 + kProfileCount; // video switch + profiles
        case RBSectionData: return 3;
        case RBSectionAbout: return 2; // version, diagnostics
    }
    return 0;
}

- (NSString *)tableView:(UITableView *)tableView titleForHeaderInSection:(NSInteger)section {
    switch (section) {
        case RBSectionServer: return @"Server";
        case RBSectionSaved: return @"Saved Servers";
        case RBSectionStream: return @"Stream";
        case RBSectionData: return @"Data";
        case RBSectionAbout: return @"About";
    }
    return nil;
}

- (NSString *)tableView:(UITableView *)tableView titleForFooterInSection:(NSInteger)section {
    if (section == RBSectionData && !self.connected) return @"Connect to a server to manage its data.";
    return nil;
}

- (UITableViewCell *)cellWithID:(NSString *)cellID style:(UITableViewCellStyle)style {
    UITableViewCell *cell = [self.tableView dequeueReusableCellWithIdentifier:cellID];
    if (!cell) cell = [[UITableViewCell alloc] initWithStyle:style reuseIdentifier:cellID];
    cell.accessoryType = UITableViewCellAccessoryNone;
    cell.accessoryView = nil;
    cell.textLabel.textAlignment = NSTextAlignmentLeft;
    cell.textLabel.textColor = [UIColor blackColor];
    cell.selectionStyle = UITableViewCellSelectionStyleBlue;
    return cell;
}

- (UITableViewCell *)tableView:(UITableView *)tableView cellForRowAtIndexPath:(NSIndexPath *)indexPath {
    NSInteger s = indexPath.section, r = indexPath.row;

    if (s == RBSectionServer) {
        if (r == 0 || r == 1) {
            UITableViewCell *cell = [self cellWithID:(r == 0 ? @"url" : @"pass") style:UITableViewCellStyleDefault];
            cell.selectionStyle = UITableViewCellSelectionStyleNone;
            UITextField *field = r == 0 ? self.urlField : self.passwordField;
            field.frame = CGRectMake(15.0, 8.0, cell.contentView.bounds.size.width - 30.0, 28.0);
            field.autoresizingMask = UIViewAutoresizingFlexibleWidth;
            if (!field.superview) [cell.contentView addSubview:field];
            return cell;
        }
        if (r == 2) {
            UITableViewCell *cell = [self cellWithID:@"connect" style:UITableViewCellStyleDefault];
            cell.textLabel.text = @"Connect";
            cell.textLabel.textAlignment = NSTextAlignmentCenter;
            cell.textLabel.textColor = [UIColor colorWithRed:0.22 green:0.33 blue:0.53 alpha:1.0];
            return cell;
        }
        UITableViewCell *cell = [self cellWithID:@"status" style:UITableViewCellStyleDefault];
        cell.selectionStyle = UITableViewCellSelectionStyleNone;
        cell.textLabel.font = [RBTheme fontOfSize:13.0 bold:NO];
        cell.textLabel.textColor = self.statusIsError
            ? [UIColor colorWithRed:0.62 green:0.12 blue:0.12 alpha:1.0]
            : [UIColor colorWithWhite:0.35 alpha:1.0];
        cell.textLabel.text = self.statusText;
        return cell;
    }

    if (s == RBSectionSaved) {
        UITableViewCell *cell = [self cellWithID:@"saved" style:UITableViewCellStyleSubtitle];
        if (r == (NSInteger)[self.savedServers count]) {
            cell.textLabel.text = @"Find Local Surf";
            cell.detailTextLabel.text = @"search this Wi-Fi (Bonjour)";
            return cell;
        }
        NSDictionary *entry = [self.savedServers objectAtIndex:(NSUInteger)r];
        cell.textLabel.text = [entry objectForKey:@"title"] ?: [entry objectForKey:@"url"];
        cell.detailTextLabel.text = [entry objectForKey:@"url"];
        NSString *current = [self.urlField.text stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
        cell.accessoryType = [current isEqualToString:[entry objectForKey:@"url"]]
            ? UITableViewCellAccessoryCheckmark : UITableViewCellAccessoryNone;
        return cell;
    }

    if (s == RBSectionStream) {
        if (r == 0) {
            UITableViewCell *cell = [self cellWithID:@"video" style:UITableViewCellStyleDefault];
            cell.selectionStyle = UITableViewCellSelectionStyleNone;
            cell.textLabel.text = @"H.264 Video";
            cell.accessoryView = self.videoSwitch;
            return cell;
        }
        UITableViewCell *cell = [self cellWithID:@"profile" style:UITableViewCellStyleDefault];
        cell.textLabel.text = kProfileTitles[r - 1];
        cell.accessoryType = [[self currentProfile] isEqualToString:kProfiles[r - 1]]
            ? UITableViewCellAccessoryCheckmark : UITableViewCellAccessoryNone;
        return cell;
    }

    if (s == RBSectionData) {
        UITableViewCell *cell = [self cellWithID:@"data" style:UITableViewCellStyleDefault];
        static NSString *const titles[] = {@"Clear History", @"Clear Cookies", @"Clear Cache"};
        cell.textLabel.text = titles[r];
        cell.textLabel.textColor = self.connected ? [UIColor colorWithRed:0.62 green:0.12 blue:0.12 alpha:1.0]
                                                  : [UIColor colorWithWhite:0.6 alpha:1.0];
        cell.selectionStyle = self.connected ? UITableViewCellSelectionStyleBlue : UITableViewCellSelectionStyleNone;
        return cell;
    }

    // About
    UITableViewCell *cell = [self cellWithID:@"about" style:UITableViewCellStyleValue1];
    cell.selectionStyle = UITableViewCellSelectionStyleNone;
    if (r == 0) {
        cell.textLabel.text = @"Version";
        cell.detailTextLabel.text = [NSString stringWithFormat:@"native %@", RBNativeVersion];
    } else {
        cell.textLabel.text = @"Diagnostics Overlay";
        cell.detailTextLabel.text = nil;
        self.diagSwitch.on = self.diagnosticsVisible;
        cell.accessoryView = self.diagSwitch;
    }
    return cell;
}

- (void)tableView:(UITableView *)tableView didSelectRowAtIndexPath:(NSIndexPath *)indexPath {
    [tableView deselectRowAtIndexPath:indexPath animated:YES];
    NSInteger s = indexPath.section, r = indexPath.row;

    if (s == RBSectionServer && r == 2) {
        [self connect];
        return;
    }
    if (s == RBSectionSaved) {
        if (r == (NSInteger)[self.savedServers count]) {
            [self startDiscovery];
            return;
        }
        self.urlField.text = [[self.savedServers objectAtIndex:(NSUInteger)r] objectForKey:@"url"];
        [self setStatusText:@"Server selected — tap Connect" isError:NO];
        [tableView reloadSections:[NSIndexSet indexSetWithIndex:RBSectionSaved]
                 withRowAnimation:UITableViewRowAnimationNone];
        return;
    }
    if (s == RBSectionStream && r > 0) {
        [[NSUserDefaults standardUserDefaults] setObject:kProfiles[r - 1] forKey:RBDefaultsStreamProfileKey];
        [[NSUserDefaults standardUserDefaults] synchronize];
        [tableView reloadSections:[NSIndexSet indexSetWithIndex:RBSectionStream]
                 withRowAnimation:UITableViewRowAnimationNone];
        if ([self.delegate respondsToSelector:@selector(settingsStreamChanged:)]) {
            [self.delegate settingsStreamChanged:self];
        }
        return;
    }
    if (s == RBSectionData && self.connected) {
        static NSString *const whats[] = {@"history", @"cookies", @"cache"};
        if ([self.delegate respondsToSelector:@selector(settings:clearData:)]) {
            [self.delegate settings:self clearData:whats[r]];
        }
        return;
    }
}

// ---- text fields ----------------------------------------------------------

- (BOOL)textFieldShouldReturn:(UITextField *)textField {
    if (textField == self.urlField) [self.passwordField becomeFirstResponder];
    else [self connect];
    return NO;
}

@end
