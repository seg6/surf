#import "RBListPopover.h"
#import "RBTheme.h"

@implementation RBListItem

+ (RBListItem *)itemWithTitle:(NSString *)title subtitle:(NSString *)subtitle payload:(id)payload {
    RBListItem *item = [[RBListItem alloc] init];
    item.title = title;
    item.subtitle = subtitle;
    item.payload = payload;
    return item;
}

@end

@interface RBListPopover ()
@property(nonatomic, strong) NSArray *sections;
@end

@implementation RBListPopover

- (void)viewDidLoad {
    [super viewDidLoad];
    [RBTheme styleTableView:self.tableView];
    self.view.backgroundColor = [RBTheme pageBackgroundColor];
    self.tableView.tableFooterView = [[UIView alloc] initWithFrame:CGRectZero];
}

- (id)initWithSections:(NSArray *)sections {
    self = [super initWithStyle:UITableViewStylePlain];
    if (self) {
        self.sections = sections;
    }
    return self;
}

- (CGSize)preferredSize {
    NSInteger rows = 0;
    BOOL subtitles = NO, headers = NO;
    for (NSDictionary *section in self.sections) {
        NSArray *items = [section objectForKey:@"items"];
        rows += (NSInteger)[items count];
        if ([[section objectForKey:@"title"] length]) headers = YES;
        for (RBListItem *item in items) {
            if ([item.subtitle length]) subtitles = YES;
        }
    }
    CGFloat rowH = subtitles ? 52.0 : 44.0;
    CGFloat height = rows * rowH + (headers ? [self.sections count] * 28.0 : 0.0);
    return CGSizeMake(subtitles ? 360.0 : 260.0, MIN(560.0, MAX(44.0, height)));
}

- (NSInteger)numberOfSectionsInTableView:(UITableView *)tableView {
    return (NSInteger)[self.sections count];
}

- (NSString *)tableView:(UITableView *)tableView titleForHeaderInSection:(NSInteger)section {
    NSString *title = [[self.sections objectAtIndex:(NSUInteger)section] objectForKey:@"title"];
    return [title length] ? title : nil;
}

- (CGFloat)tableView:(UITableView *)tableView heightForHeaderInSection:(NSInteger)section {
    return [[self tableView:tableView titleForHeaderInSection:section] length] ? 28.0 : 0.0;
}

- (UIView *)tableView:(UITableView *)tableView viewForHeaderInSection:(NSInteger)section {
    NSString *title = [self tableView:tableView titleForHeaderInSection:section];
    if (![title length]) return nil;
    UIView *header = [[UIView alloc] initWithFrame:CGRectZero];
    header.backgroundColor = [RBTheme pageBackgroundColor];
    UILabel *label = [[UILabel alloc] initWithFrame:CGRectMake(12.0, 0.0,
                                                               MAX(1.0, tableView.bounds.size.width - 24.0), 28.0)];
    label.autoresizingMask = UIViewAutoresizingFlexibleWidth;
    label.backgroundColor = [UIColor clearColor];
    label.font = [RBTheme fontOfSize:11.0 bold:YES];
    label.textColor = [RBTheme secondaryTextColor];
    label.text = [title uppercaseString];
    [header addSubview:label];
    return header;
}

- (NSInteger)tableView:(UITableView *)tableView numberOfRowsInSection:(NSInteger)section {
    return (NSInteger)[[[self.sections objectAtIndex:(NSUInteger)section] objectForKey:@"items"] count];
}

- (RBListItem *)itemAtIndexPath:(NSIndexPath *)indexPath {
    NSArray *items = [[self.sections objectAtIndex:(NSUInteger)indexPath.section] objectForKey:@"items"];
    return [items objectAtIndex:(NSUInteger)indexPath.row];
}

- (CGFloat)tableView:(UITableView *)tableView heightForRowAtIndexPath:(NSIndexPath *)indexPath {
    return [[self itemAtIndexPath:indexPath].subtitle length] ? 52.0 : 44.0;
}

- (UITableViewCell *)tableView:(UITableView *)tableView cellForRowAtIndexPath:(NSIndexPath *)indexPath {
    RBListItem *item = [self itemAtIndexPath:indexPath];
    NSString *reuse = [item.subtitle length] ? @"sub" : @"plain";
    UITableViewCell *cell = [tableView dequeueReusableCellWithIdentifier:reuse];
    if (!cell) {
        UITableViewCellStyle style = [item.subtitle length] ? UITableViewCellStyleSubtitle : UITableViewCellStyleDefault;
        cell = [[UITableViewCell alloc] initWithStyle:style reuseIdentifier:reuse];
        cell.textLabel.font = [RBTheme fontOfSize:15.0 bold:NO];
        cell.detailTextLabel.font = [RBTheme fontOfSize:12.0 bold:NO];
    }
    cell.backgroundColor = [RBTheme surfaceColor];
    if (!cell.selectedBackgroundView) {
        cell.selectedBackgroundView = [[UIView alloc] initWithFrame:CGRectZero];
    }
    cell.selectedBackgroundView.backgroundColor = [[RBTheme separatorColor] colorWithAlphaComponent:0.62];
    cell.textLabel.textColor = [RBTheme primaryTextColor];
    cell.detailTextLabel.textColor = [RBTheme secondaryTextColor];
    cell.textLabel.text = item.title;
    cell.detailTextLabel.text = item.subtitle;
    return cell;
}

- (void)tableView:(UITableView *)tableView didSelectRowAtIndexPath:(NSIndexPath *)indexPath {
    [tableView deselectRowAtIndexPath:indexPath animated:NO];
    if (self.onSelect) self.onSelect([self itemAtIndexPath:indexPath]);
}

@end
