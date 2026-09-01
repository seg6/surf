#import "RBAddServerController.h"
#import "RBServerStore.h"
#import "RBTheme.h"

@interface RBAddServerController () <UITextFieldDelegate>
@property(nonatomic, strong) UITextField *addressField;
@property(nonatomic, strong) UIBarButtonItem *nextButton;
@property(nonatomic, copy) NSString *message;
@property(nonatomic, assign) BOOL errorMessage;
@property(nonatomic, assign) BOOL busy;
@end

@implementation RBAddServerController

- (id)initWithEndpoint:(NSString *)endpoint {
    self = [super initWithStyle:UITableViewStyleGrouped];
    if (self) {
        self.title = @"Add Server";
        _message = @"Enter the address shown by Surf on the server.";
        if ([endpoint length]) {
            NSString *display = [endpoint hasPrefix:@"https://"] ? [endpoint substringFromIndex:8] : endpoint;
            _addressField = [[UITextField alloc] initWithFrame:CGRectZero];
            _addressField.text = display;
        }
    }
    return self;
}

- (NSString *)endpoint { return [RBServerStore normalizeEndpoint:self.addressField.text]; }

- (void)viewDidLoad {
    [super viewDidLoad];
    [RBTheme styleTableView:self.tableView];
    if (!self.addressField) self.addressField = [[UITextField alloc] initWithFrame:CGRectZero];
    self.addressField.delegate = self;
    self.addressField.font = [RBTheme fontOfSize:16.0 bold:NO];
    self.addressField.textColor = [RBTheme primaryTextColor];
    self.addressField.keyboardAppearance = [RBTheme isDarkMode] ? UIKeyboardAppearanceDark
                                                                : UIKeyboardAppearanceDefault;
    self.addressField.placeholder = @"192.168.1.25:7777";
    self.addressField.keyboardType = UIKeyboardTypeURL;
    self.addressField.returnKeyType = UIReturnKeyGo;
    self.addressField.autocorrectionType = UITextAutocorrectionTypeNo;
    self.addressField.autocapitalizationType = UITextAutocapitalizationTypeNone;
    self.addressField.clearButtonMode = UITextFieldViewModeWhileEditing;
    self.addressField.contentVerticalAlignment = UIControlContentVerticalAlignmentCenter;
    [self.addressField addTarget:self action:@selector(addressChanged:) forControlEvents:UIControlEventEditingChanged];

    self.nextButton = [[UIBarButtonItem alloc] initWithTitle:@"Next" style:UIBarButtonItemStyleDone
                                                      target:self action:@selector(nextTapped:)];
    self.navigationItem.rightBarButtonItem = self.nextButton;
    [self updateNextButton];
}

- (void)viewDidAppear:(BOOL)animated {
    [super viewDidAppear:animated];
    if (!self.busy) [self.addressField becomeFirstResponder];
}

- (NSInteger)numberOfSectionsInTableView:(UITableView *)tableView { return 1; }
- (NSInteger)tableView:(UITableView *)tableView numberOfRowsInSection:(NSInteger)section { return 1; }

- (CGFloat)tableView:(UITableView *)tableView heightForRowAtIndexPath:(NSIndexPath *)indexPath {
    return 48.0;
}

- (NSString *)tableView:(UITableView *)tableView titleForHeaderInSection:(NSInteger)section {
    return @"Server Address";
}

- (NSString *)tableView:(UITableView *)tableView titleForFooterInSection:(NSInteger)section {
    return self.message;
}

- (UITableViewCell *)tableView:(UITableView *)tableView cellForRowAtIndexPath:(NSIndexPath *)indexPath {
    static NSString *identifier = @"address";
    UITableViewCell *cell = [tableView dequeueReusableCellWithIdentifier:identifier];
    if (!cell) cell = [[UITableViewCell alloc] initWithStyle:UITableViewCellStyleDefault reuseIdentifier:identifier];
    cell.backgroundColor = [RBTheme surfaceColor];
    cell.selectionStyle = UITableViewCellSelectionStyleNone;
    self.addressField.frame = CGRectMake(15.0, 5.0, cell.contentView.bounds.size.width - 30.0,
                                         MAX(22.0, cell.contentView.bounds.size.height - 10.0));
    self.addressField.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
    if (self.addressField.superview != cell.contentView) {
        [self.addressField removeFromSuperview];
        [cell.contentView addSubview:self.addressField];
    }
    return cell;
}

- (void)addressChanged:(id)sender {
    if (self.errorMessage) {
        self.message = @"Enter the address shown by Surf on the server.";
        self.errorMessage = NO;
        [self.tableView reloadData];
    }
    [self updateNextButton];
}

- (void)updateNextButton {
    self.nextButton.enabled = !self.busy && self.endpoint != nil;
    self.addressField.enabled = !self.busy;
}

- (void)nextTapped:(id)sender {
    NSString *endpoint = self.endpoint;
    if (!endpoint) {
        [self showError:@"Enter a valid hostname, IP address, or bracketed IPv6 address."];
        return;
    }
    [self.addressField resignFirstResponder];
    [self.delegate addServerController:self didSubmitEndpoint:endpoint];
}

- (BOOL)textFieldShouldReturn:(UITextField *)textField {
    if (self.nextButton.enabled) [self nextTapped:self.nextButton];
    return NO;
}

- (void)setBusy:(BOOL)busy message:(NSString *)message {
    self.busy = busy;
    self.errorMessage = NO;
    self.message = [message length] ? message : @"Contacting the Surf server\u2026";
    if ([self isViewLoaded]) {
        [self updateNextButton];
        [self.tableView reloadData];
    }
}

- (void)showError:(NSString *)message {
    self.busy = NO;
    self.errorMessage = YES;
    self.message = [message length] ? message : @"Couldn\u2019t reach the Surf server.";
    if ([self isViewLoaded]) {
        [self updateNextButton];
        [self.tableView reloadData];
    }
}

@end
