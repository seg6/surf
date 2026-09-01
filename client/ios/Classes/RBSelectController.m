#import "RBSelectController.h"
#import "RBTheme.h"

@interface RBSelectController ()
@property(nonatomic, copy, readwrite) NSString *requestID;
@property(nonatomic, assign, readwrite) BOOL multiple;
@property(nonatomic, strong) NSArray *options;
@property(nonatomic, strong) NSMutableIndexSet *selectedIndices;
@end

@implementation RBSelectController

- (id)initWithRequestID:(NSString *)requestID
                  title:(NSString *)title
                options:(NSArray *)options
               multiple:(BOOL)multiple {
    self = [super initWithStyle:UITableViewStylePlain];
    if (self) {
        self.requestID = requestID ?: @"";
        self.multiple = multiple;
        self.options = options ?: @[];
        self.selectedIndices = [NSMutableIndexSet indexSet];
        for (NSUInteger index = 0; index < [self.options count]; index++) {
            NSDictionary *option = [self.options objectAtIndex:index];
            if ([[option objectForKey:@"selected"] boolValue]) [self.selectedIndices addIndex:index];
        }
        self.title = [title length] ? title : (multiple ? @"Choose options" : @"Choose an option");
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    [RBTheme styleTableView:self.tableView];
    [RBTheme styleNavigationBar:self.navigationController.navigationBar];
    self.view.backgroundColor = [RBTheme pageBackgroundColor];
    self.tableView.backgroundColor = [RBTheme pageBackgroundColor];
    self.tableView.separatorColor = [RBTheme separatorColor];
    self.tableView.rowHeight = 44.0;
    self.tableView.tableFooterView = [[UIView alloc] initWithFrame:CGRectZero];
    self.navigationItem.leftBarButtonItem = [[UIBarButtonItem alloc]
        initWithBarButtonSystemItem:UIBarButtonSystemItemCancel target:self action:@selector(cancelTapped:)];
    if (self.multiple) {
        self.navigationItem.rightBarButtonItem = [[UIBarButtonItem alloc]
            initWithBarButtonSystemItem:UIBarButtonSystemItemDone target:self action:@selector(doneTapped:)];
    }
}

- (void)viewWillAppear:(BOOL)animated {
    [super viewWillAppear:animated];
    [RBTheme styleTableView:self.tableView];
    [RBTheme styleNavigationBar:self.navigationController.navigationBar];
    self.view.backgroundColor = [RBTheme pageBackgroundColor];
    [self.tableView reloadData];
}

- (CGSize)preferredPopoverSize {
    CGFloat height = MIN(520.0, MAX(132.0, 44.0 * [self.options count] + 44.0));
    return CGSizeMake(360.0, height);
}

- (NSInteger)tableView:(UITableView *)tableView numberOfRowsInSection:(NSInteger)section {
    return (NSInteger)[self.options count];
}

- (UITableViewCell *)tableView:(UITableView *)tableView cellForRowAtIndexPath:(NSIndexPath *)indexPath {
    UITableViewCell *cell = [tableView dequeueReusableCellWithIdentifier:@"option"];
    if (!cell) {
        cell = [[UITableViewCell alloc] initWithStyle:UITableViewCellStyleDefault reuseIdentifier:@"option"];
        cell.textLabel.font = [RBTheme fontOfSize:16.0 bold:NO];
    }
    NSDictionary *option = [self.options objectAtIndex:(NSUInteger)indexPath.row];
    BOOL disabled = [[option objectForKey:@"disabled"] boolValue];
    cell.textLabel.text = [option objectForKey:@"label"] ?: @"";
    cell.textLabel.textColor = disabled ? [RBTheme secondaryTextColor] : [RBTheme primaryTextColor];
    cell.backgroundColor = [RBTheme surfaceColor];
    if (!cell.selectedBackgroundView) {
        cell.selectedBackgroundView = [[UIView alloc] initWithFrame:CGRectZero];
    }
    cell.selectedBackgroundView.backgroundColor = [[RBTheme separatorColor] colorWithAlphaComponent:0.62];
    cell.selectionStyle = disabled ? UITableViewCellSelectionStyleNone : UITableViewCellSelectionStyleBlue;
    if ([cell respondsToSelector:@selector(setTintColor:)]) cell.tintColor = [RBTheme accentColor];
    cell.accessoryType = [self.selectedIndices containsIndex:(NSUInteger)indexPath.row]
        ? UITableViewCellAccessoryCheckmark : UITableViewCellAccessoryNone;
    return cell;
}

- (void)tableView:(UITableView *)tableView didSelectRowAtIndexPath:(NSIndexPath *)indexPath {
    NSDictionary *option = [self.options objectAtIndex:(NSUInteger)indexPath.row];
    if ([[option objectForKey:@"disabled"] boolValue]) return;
    [tableView deselectRowAtIndexPath:indexPath animated:NO];
    NSUInteger index = (NSUInteger)indexPath.row;
    if (!self.multiple) {
        [self.delegate selectController:self choseIndices:@[[NSNumber numberWithUnsignedInteger:index]]];
        return;
    }
    if ([self.selectedIndices containsIndex:index]) [self.selectedIndices removeIndex:index];
    else [self.selectedIndices addIndex:index];
    [tableView reloadRowsAtIndexPaths:@[indexPath] withRowAnimation:UITableViewRowAnimationNone];
}

- (void)cancelTapped:(id)sender { [self.delegate selectControllerDidCancel:self]; }

- (void)doneTapped:(id)sender {
    NSMutableArray *indices = [NSMutableArray array];
    [self.selectedIndices enumerateIndexesUsingBlock:^(NSUInteger index, BOOL *stop) {
        [indices addObject:[NSNumber numberWithUnsignedInteger:index]];
    }];
    [self.delegate selectController:self choseIndices:indices];
}

@end
