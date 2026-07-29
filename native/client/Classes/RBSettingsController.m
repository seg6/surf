#import "RBSettingsController.h"
#import "RBConfig.h"
#import "RBTheme.h"

#import <arpa/inet.h>
#import <netinet/in.h>

enum {
    RBSectionServer = 0,
    RBSectionSaved,
    RBSectionBrowsing,
    RBSectionDiagnostics,
    RBSectionData,
    RBSectionAbout,
    RBSectionCount
};

static const NSInteger kEditServerAlertTag = 1001;
static const NSInteger kClearDataAlertTag = 1002;

@interface RBSettingsController () <UITextFieldDelegate, UIAlertViewDelegate, NSNetServiceBrowserDelegate, NSNetServiceDelegate>
@property(nonatomic, copy) NSString *initialURL;
@property(nonatomic, copy) NSString *initialPassword;
@property(nonatomic, strong) UITextField *urlField;
@property(nonatomic, strong) UITextField *passwordField;
@property(nonatomic, copy) NSString *statusText;
@property(nonatomic, assign) BOOL statusIsError;
@property(nonatomic, strong) NSNetServiceBrowser *serviceBrowser;
@property(nonatomic, strong) NSMutableArray *services;
@property(nonatomic, strong) NSArray *savedServers; // [{title,url,password?}]
@property(nonatomic, assign) NSInteger editingServerRow;
@property(nonatomic, copy) NSString *pendingClearData;
@end

@implementation RBSettingsController

- (id)initWithServerURL:(NSString *)serverURL password:(NSString *)password {
    self = [super initWithStyle:UITableViewStyleGrouped];
    if (self) {
        self.initialURL = serverURL;
        self.initialPassword = password;
        self.title = @"Settings";
        self.editingServerRow = -1;
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
    for (NSDictionary *entry in [[NSUserDefaults standardUserDefaults] arrayForKey:RBDefaultsServersKey] ?: @[]) {
        NSString *url = [entry objectForKey:@"url"];
        if (![url length]) continue;
        [items addObject:entry];
    }
    self.savedServers = items;
}

- (NSString *)normalizedServerURL:(NSString *)raw {
    NSString *url = [raw stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
    if (![url length]) return @"";
    if ([url rangeOfString:@"://"].location == NSNotFound) url = [@"http://" stringByAppendingString:url];
    return url;
}

- (NSString *)titleForServerURL:(NSString *)url {
    if ([url isEqualToString:RBDefaultServerURL]) return @"Surf VPS";
    NSString *host = [[NSURL URLWithString:url] host];
    return [host length] ? host : url;
}

- (NSString *)titleForServerEntry:(NSDictionary *)entry {
    NSString *url = [entry objectForKey:@"url"];
    if ([url isEqualToString:RBDefaultServerURL]) return @"Surf VPS";
    NSString *title = [entry objectForKey:@"title"];
    return [title length] ? title : [self titleForServerURL:url];
}

- (NSDictionary *)serverEntryWithURL:(NSString *)url password:(NSString *)password {
    return @{ @"title": [self titleForServerURL:url] ?: url, @"url": url ?: @"", @"password": password ?: @"" };
}

- (NSString *)passwordForServerEntry:(NSDictionary *)entry {
    id password = [entry objectForKey:@"password"];
    if ([password isKindOfClass:[NSString class]]) return password;
    NSString *url = [entry objectForKey:@"url"];
    NSString *currentURL = [self normalizedServerURL:self.urlField.text ?: @""];
    if ([url isEqualToString:currentURL]) return self.passwordField.text ?: @"";
    if ([url isEqualToString:RBDefaultServerURL]) return RBDefaultPassword;
    return @"";
}

- (BOOL)isEditableSavedRow:(NSInteger)row {
    return row >= 0 && row < (NSInteger)[self.savedServers count];
}

- (BOOL)isDeletableSavedRow:(NSInteger)row {
    return [self isEditableSavedRow:row];
}

- (void)applySavedServer:(NSDictionary *)entry {
    NSString *url = [entry objectForKey:@"url"];
    if ([url length]) self.urlField.text = url;
    self.passwordField.text = [self passwordForServerEntry:entry];
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

- (void)diagnosticsChanged:(UISwitch *)sender {
    self.diagnosticsVisible = sender.on;
    NSUserDefaults *defaults = [NSUserDefaults standardUserDefaults];
    [defaults setBool:sender.on forKey:RBDefaultsDiagnosticsKey];
    [defaults synchronize];
    if ([self.delegate respondsToSelector:@selector(settings:diagnosticsVisible:)]) {
        [self.delegate settings:self diagnosticsVisible:sender.on];
    }
}

- (void)preferenceChanged:(UISwitch *)sender {
    NSString *key = sender.tag == 1 ? RBDefaultsMobileLayoutKey : RBDefaultsOfferCopiedLinksKey;
    [[NSUserDefaults standardUserDefaults] setBool:sender.on forKey:key];
    [[NSUserDefaults standardUserDefaults] synchronize];
    if ([self.delegate respondsToSelector:@selector(settings:preference:enabled:)]) {
        [self.delegate settings:self preference:key enabled:sender.on];
    }
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
        case RBSectionBrowsing: return 3;
        case RBSectionDiagnostics: return 1;
        case RBSectionData: return 3;
        case RBSectionAbout: return 1;
    }
    return 0;
}

- (NSString *)tableView:(UITableView *)tableView titleForHeaderInSection:(NSInteger)section {
    switch (section) {
        case RBSectionServer: return @"Server";
        case RBSectionSaved: return @"Saved Servers";
        case RBSectionBrowsing: return @"Browsing";
        case RBSectionDiagnostics: return @"Diagnostics";
        case RBSectionData: return @"Data";
        case RBSectionAbout: return @"About";
    }
    return nil;
}

- (NSString *)tableView:(UITableView *)tableView titleForFooterInSection:(NSInteger)section {
    if (section == RBSectionDiagnostics) return @"Shows live video, latency, decoder, network, and audio health over the page.";
    if (section == RBSectionBrowsing) return @"Request Mobile Sites uses a mobile Chrome identity and touch viewport. Changing it reloads the current page.";
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
            if (field.superview != cell.contentView) {
                [field removeFromSuperview];
                [cell.contentView addSubview:field];
            }
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
        cell.textLabel.text = [self titleForServerEntry:entry];
        cell.detailTextLabel.text = [entry objectForKey:@"url"];
        NSString *current = [self.urlField.text stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
        if ([self isEditableSavedRow:r]) {
            cell.accessoryType = UITableViewCellAccessoryDetailDisclosureButton;
        } else {
            cell.accessoryType = [current isEqualToString:[entry objectForKey:@"url"]]
                ? UITableViewCellAccessoryCheckmark : UITableViewCellAccessoryNone;
        }
        return cell;
    }

    if (s == RBSectionBrowsing) {
        UITableViewCell *cell = [self cellWithID:@"browsing" style:UITableViewCellStyleSubtitle];
        if (r == 0) {
            cell.textLabel.text = @"Page Media Controls";
            cell.detailTextLabel.text = @"playback, mute, and volume";
            cell.accessoryType = self.connected ? UITableViewCellAccessoryDisclosureIndicator : UITableViewCellAccessoryNone;
            cell.textLabel.textColor = self.connected ? [UIColor blackColor] : [UIColor colorWithWhite:0.6 alpha:1.0];
            cell.selectionStyle = self.connected ? UITableViewCellSelectionStyleBlue : UITableViewCellSelectionStyleNone;
            return cell;
        }
        BOOL mobile = r == 1;
        cell.textLabel.text = mobile ? @"Request Mobile Sites" : @"Offer Copied Links";
        cell.detailTextLabel.text = mobile ? @"identify as mobile Chrome" : @"ask to open copied web addresses";
        cell.selectionStyle = UITableViewCellSelectionStyleNone;
        UISwitch *toggle = [[UISwitch alloc] initWithFrame:CGRectZero];
        toggle.tag = mobile ? 1 : 2;
        toggle.on = [[NSUserDefaults standardUserDefaults] boolForKey:
                     mobile ? RBDefaultsMobileLayoutKey : RBDefaultsOfferCopiedLinksKey];
        [toggle addTarget:self action:@selector(preferenceChanged:) forControlEvents:UIControlEventValueChanged];
        cell.accessoryView = toggle;
        return cell;
    }

    if (s == RBSectionDiagnostics) {
        UITableViewCell *cell = [self cellWithID:@"diagnostics" style:UITableViewCellStyleDefault];
        cell.textLabel.text = @"Performance Overlay";
        cell.selectionStyle = UITableViewCellSelectionStyleNone;
        UISwitch *toggle = [[UISwitch alloc] initWithFrame:CGRectZero];
        toggle.on = self.diagnosticsVisible;
        [toggle addTarget:self action:@selector(diagnosticsChanged:) forControlEvents:UIControlEventValueChanged];
        cell.accessoryView = toggle;
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
    cell.textLabel.text = @"Version";
    cell.detailTextLabel.text = [NSString stringWithFormat:@"%@ · protocol %@", RBAppVersion, RBNativeVersion];
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
        [self applySavedServer:[self.savedServers objectAtIndex:(NSUInteger)r]];
        [self setStatusText:@"Server selected — edit above or tap Connect" isError:NO];
        [tableView reloadSections:[NSIndexSet indexSetWithIndex:RBSectionSaved]
                 withRowAnimation:UITableViewRowAnimationNone];
        return;
    }
    if (s == RBSectionBrowsing && r == 0 && self.connected) {
        if ([self.delegate respondsToSelector:@selector(settingsWantsMediaControls:)]) {
            [self.delegate settingsWantsMediaControls:self];
        }
        return;
    }
    if (s == RBSectionData && self.connected) {
        static NSString *const whats[] = {@"history", @"cookies", @"cache"};
        static NSString *const titles[] = {@"Clear History?", @"Clear Cookies?", @"Clear Cache?"};
        self.pendingClearData = whats[r];
        UIAlertView *alert = [[UIAlertView alloc] initWithTitle:titles[r]
                                                       message:@"This affects the browser on the connected Surf server."
                                                      delegate:self cancelButtonTitle:@"Cancel"
                                             otherButtonTitles:@"Clear", nil];
        alert.tag = kClearDataAlertTag;
        [alert show];
        return;
    }
}

- (void)tableView:(UITableView *)tableView accessoryButtonTappedForRowWithIndexPath:(NSIndexPath *)indexPath {
    if (indexPath.section != RBSectionSaved || ![self isEditableSavedRow:indexPath.row]) return;
    NSDictionary *entry = [self.savedServers objectAtIndex:(NSUInteger)indexPath.row];
    self.editingServerRow = indexPath.row;
    UIAlertView *alert = [[UIAlertView alloc] initWithTitle:@"Edit Server"
                                                    message:@"Update the saved URL and password."
                                                   delegate:self
                                          cancelButtonTitle:@"Cancel"
                                          otherButtonTitles:@"Save", nil];
    alert.tag = kEditServerAlertTag;
    alert.alertViewStyle = UIAlertViewStyleLoginAndPasswordInput;
    UITextField *urlField = [alert textFieldAtIndex:0];
    urlField.placeholder = @"http://server";
    urlField.text = [entry objectForKey:@"url"] ?: @"";
    urlField.keyboardType = UIKeyboardTypeURL;
    urlField.autocorrectionType = UITextAutocorrectionTypeNo;
    urlField.autocapitalizationType = UITextAutocapitalizationTypeNone;
    UITextField *passwordField = [alert textFieldAtIndex:1];
    passwordField.placeholder = @"password";
    passwordField.secureTextEntry = YES;
    passwordField.text = [self passwordForServerEntry:entry];
    [alert show];
}

- (BOOL)tableView:(UITableView *)tableView canEditRowAtIndexPath:(NSIndexPath *)indexPath {
    return indexPath.section == RBSectionSaved && [self isDeletableSavedRow:indexPath.row];
}

- (void)tableView:(UITableView *)tableView commitEditingStyle:(UITableViewCellEditingStyle)editingStyle
forRowAtIndexPath:(NSIndexPath *)indexPath {
    if (editingStyle != UITableViewCellEditingStyleDelete || indexPath.section != RBSectionSaved || ![self isDeletableSavedRow:indexPath.row]) return;
    NSString *deleteURL = [[self.savedServers objectAtIndex:(NSUInteger)indexPath.row] objectForKey:@"url"];
    NSMutableArray *servers = [NSMutableArray array];
    for (NSDictionary *entry in [[NSUserDefaults standardUserDefaults] arrayForKey:RBDefaultsServersKey] ?: @[]) {
        NSString *url = [entry objectForKey:@"url"];
        if (![url length] || [url isEqualToString:deleteURL]) continue;
        [servers addObject:entry];
    }
    [[NSUserDefaults standardUserDefaults] setObject:servers forKey:RBDefaultsServersKey];
    [[NSUserDefaults standardUserDefaults] synchronize];
    [self reloadSavedServers];
    self.statusText = @"Saved server removed";
    self.statusIsError = NO;
    [tableView reloadData];
}

- (void)alertView:(UIAlertView *)alertView clickedButtonAtIndex:(NSInteger)buttonIndex {
    if (alertView.tag == kClearDataAlertTag) {
        NSString *what = self.pendingClearData;
        self.pendingClearData = nil;
        if (buttonIndex != alertView.cancelButtonIndex &&
            [self.delegate respondsToSelector:@selector(settings:clearData:)]) {
            [self.delegate settings:self clearData:what];
        }
        return;
    }
    if (alertView.tag != kEditServerAlertTag) return;
    NSInteger row = self.editingServerRow;
    self.editingServerRow = -1;
    if (buttonIndex == alertView.cancelButtonIndex || ![self isEditableSavedRow:row]) return;

    NSString *oldURL = [[self.savedServers objectAtIndex:(NSUInteger)row] objectForKey:@"url"];
    NSString *url = [self normalizedServerURL:[alertView textFieldAtIndex:0].text ?: @""];
    NSString *password = [alertView textFieldAtIndex:1].text ?: @"";
    if (![url length]) {
        [self setStatusText:@"Server URL is required" isError:YES];
        return;
    }

    NSDictionary *updated = [self serverEntryWithURL:url password:password];
    NSMutableArray *servers = [NSMutableArray array];
    BOOL replaced = NO;
    for (NSDictionary *entry in [[NSUserDefaults standardUserDefaults] arrayForKey:RBDefaultsServersKey] ?: @[]) {
        NSString *entryURL = [entry objectForKey:@"url"];
        if (![entryURL length]) continue;
        if (!replaced && [entryURL isEqualToString:oldURL]) {
            [servers addObject:updated];
            replaced = YES;
            continue;
        }
        if ([entryURL isEqualToString:url]) continue;
        [servers addObject:entry];
    }
    if (!replaced) [servers insertObject:updated atIndex:0];
    [[NSUserDefaults standardUserDefaults] setObject:servers forKey:RBDefaultsServersKey];
    [[NSUserDefaults standardUserDefaults] synchronize];

    self.urlField.text = url;
    self.passwordField.text = password;
    [self reloadSavedServers];
    [self setStatusText:@"Server updated — tap Connect" isError:NO];
    [self.tableView reloadSections:[NSIndexSet indexSetWithIndex:RBSectionSaved]
                   withRowAnimation:UITableViewRowAnimationNone];
}

// ---- text fields ----------------------------------------------------------

- (BOOL)textFieldShouldReturn:(UITextField *)textField {
    if (textField == self.urlField) [self.passwordField becomeFirstResponder];
    else [self connect];
    return NO;
}

@end
