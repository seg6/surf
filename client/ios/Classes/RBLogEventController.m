#import "RBLogEventController.h"
#import "RBTheme.h"

enum {
    RBLogEventOverviewSection = 0,
    RBLogEventFieldsSection,
    RBLogEventMessageSection,
    RBLogEventRawSection,
    RBLogEventSectionCount
};

@interface RBLogEventController ()
@property(nonatomic, strong) NSDictionary *event;
@property(nonatomic, strong) NSArray *fieldKeys;
@end

@implementation RBLogEventController

- (id)initWithEvent:(NSDictionary *)event {
    self = [super initWithStyle:UITableViewStyleGrouped];
    if (self) {
        self.event = event;
        self.fieldKeys = [[[event objectForKey:@"fields"] allKeys] sortedArrayUsingSelector:@selector(compare:)];
        self.title = [event objectForKey:@"component"] ?: @"Event";
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    [RBTheme styleTableView:self.tableView];
    self.tableView.backgroundColor = [RBTheme pageBackgroundColor];
    self.navigationItem.rightBarButtonItem = [[UIBarButtonItem alloc] initWithTitle:@"Copy Event"
        style:UIBarButtonItemStyleBordered target:self action:@selector(copyTapped:)];
}

- (NSInteger)numberOfSectionsInTableView:(UITableView *)tableView { return RBLogEventSectionCount; }

- (NSInteger)tableView:(UITableView *)tableView numberOfRowsInSection:(NSInteger)section {
    if (section == RBLogEventOverviewSection) return 4;
    if (section == RBLogEventFieldsSection) return MAX(1, (NSInteger)[self.fieldKeys count]);
    return 1;
}

- (NSString *)tableView:(UITableView *)tableView titleForHeaderInSection:(NSInteger)section {
    static NSString *const titles[] = {@"Overview", @"Fields", @"Message", @"Raw Record"};
    return titles[section];
}

- (UITableViewCell *)valueCell:(UITableView *)tableView {
    UITableViewCell *cell = [tableView dequeueReusableCellWithIdentifier:@"value"];
    if (!cell) cell = [[UITableViewCell alloc] initWithStyle:UITableViewCellStyleValue1 reuseIdentifier:@"value"];
    cell.backgroundColor = [RBTheme surfaceColor];
    cell.selectionStyle = UITableViewCellSelectionStyleNone;
    cell.textLabel.font = [RBTheme fontOfSize:13.0 bold:NO];
    cell.detailTextLabel.font = [RBTheme monospacedFontOfSize:11.0 bold:NO];
    cell.textLabel.textColor = [RBTheme secondaryTextColor];
    cell.detailTextLabel.textColor = [RBTheme primaryTextColor];
    return cell;
}

- (UITableViewCell *)tableView:(UITableView *)tableView cellForRowAtIndexPath:(NSIndexPath *)indexPath {
    if (indexPath.section == RBLogEventOverviewSection) {
        static NSString *const labels[] = {@"Level", @"Component", @"Date", @"Time"};
        static NSString *const keys[] = {@"level", @"component", @"date", @"time"};
        UITableViewCell *cell = [self valueCell:tableView];
        cell.textLabel.text = labels[indexPath.row];
        cell.detailTextLabel.text = [[self.event objectForKey:keys[indexPath.row]] description] ?: @"";
        return cell;
    }
    if (indexPath.section == RBLogEventFieldsSection) {
        UITableViewCell *cell = [self valueCell:tableView];
        if (![self.fieldKeys count]) {
            cell.textLabel.text = @"No fields";
            cell.detailTextLabel.text = nil;
        } else {
            NSString *key = [self.fieldKeys objectAtIndex:indexPath.row];
            cell.textLabel.text = key;
            cell.detailTextLabel.text = [[[self.event objectForKey:@"fields"] objectForKey:key] description];
        }
        return cell;
    }
    UITableViewCell *cell = [tableView dequeueReusableCellWithIdentifier:@"text"];
    if (!cell) cell = [[UITableViewCell alloc] initWithStyle:UITableViewCellStyleDefault reuseIdentifier:@"text"];
    cell.backgroundColor = [RBTheme surfaceColor];
    cell.selectionStyle = UITableViewCellSelectionStyleNone;
    cell.textLabel.numberOfLines = 0;
    cell.textLabel.font = indexPath.section == RBLogEventRawSection
        ? [RBTheme monospacedFontOfSize:10.0 bold:NO]
        : [RBTheme fontOfSize:13.0 bold:NO];
    cell.textLabel.textColor = [RBTheme primaryTextColor];
    cell.textLabel.text = indexPath.section == RBLogEventMessageSection
        ? [self.event objectForKey:@"message"] : [self.event objectForKey:@"raw"];
    return cell;
}

- (CGFloat)tableView:(UITableView *)tableView heightForRowAtIndexPath:(NSIndexPath *)indexPath {
    if (indexPath.section == RBLogEventMessageSection) return 88.0;
    if (indexPath.section == RBLogEventRawSection) return 132.0;
    return 44.0;
}

- (void)copyTapped:(id)sender {
    [UIPasteboard generalPasteboard].string = [self.event objectForKey:@"raw"] ?: @"";
}

@end
