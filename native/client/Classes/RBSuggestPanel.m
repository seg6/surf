#import "RBSuggestPanel.h"
#import "RBTheme.h"

#import <QuartzCore/QuartzCore.h>

static const CGFloat kRBSuggestRowHeight = 44.0;

@interface RBSuggestPanel () <UITableViewDataSource, UITableViewDelegate>
@property(nonatomic, strong) UITableView *table;
@property(nonatomic, strong) NSArray *items;
@end

@implementation RBSuggestPanel

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [UIColor whiteColor];
        self.layer.borderWidth = 1.0;
        self.layer.borderColor = [[UIColor colorWithWhite:0.55 alpha:1.0] CGColor];
        self.layer.shadowColor = [[UIColor blackColor] CGColor];
        self.layer.shadowOpacity = 0.35;
        self.layer.shadowOffset = CGSizeMake(0.0, 3.0);
        self.layer.shadowRadius = 6.0;
        self.hidden = YES;

        self.table = [[UITableView alloc] initWithFrame:self.bounds style:UITableViewStylePlain];
        self.table.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
        self.table.dataSource = self;
        self.table.delegate = self;
        self.table.rowHeight = kRBSuggestRowHeight;
        [self addSubview:self.table];
    }
    return self;
}

- (void)showItems:(NSArray *)items {
    self.items = [items isKindOfClass:[NSArray class]] ? items : nil;
    if (![self.items count]) {
        [self hide];
        return;
    }
    self.hidden = NO;
    [self.table reloadData];
}

- (void)hide {
    self.hidden = YES;
    self.items = nil;
}

- (CGFloat)desiredHeight {
    return MIN(6, (NSInteger)[self.items count]) * kRBSuggestRowHeight;
}

- (NSInteger)tableView:(UITableView *)tableView numberOfRowsInSection:(NSInteger)section {
    return (NSInteger)[self.items count];
}

- (UITableViewCell *)tableView:(UITableView *)tableView cellForRowAtIndexPath:(NSIndexPath *)indexPath {
    UITableViewCell *cell = [tableView dequeueReusableCellWithIdentifier:@"sugg"];
    if (!cell) {
        cell = [[UITableViewCell alloc] initWithStyle:UITableViewCellStyleSubtitle reuseIdentifier:@"sugg"];
        cell.textLabel.font = [RBTheme fontOfSize:15.0 bold:NO];
        cell.detailTextLabel.font = [RBTheme fontOfSize:12.0 bold:NO];
        cell.detailTextLabel.textColor = [UIColor colorWithRed:0.25 green:0.40 blue:0.60 alpha:1.0];
    }
    NSDictionary *item = [self.items objectAtIndex:(NSUInteger)indexPath.row];
    NSString *title = [item objectForKey:@"title"];
    NSString *url = [item objectForKey:@"url"] ?: @"";
    cell.textLabel.text = [title length] ? title : url;
    cell.detailTextLabel.text = url;
    return cell;
}

- (void)tableView:(UITableView *)tableView didSelectRowAtIndexPath:(NSIndexPath *)indexPath {
    [tableView deselectRowAtIndexPath:indexPath animated:NO];
    NSDictionary *item = [self.items objectAtIndex:(NSUInteger)indexPath.row];
    NSString *url = [item objectForKey:@"url"];
    if ([url length]) [self.delegate suggestPanel:self pickedURL:url];
}

@end
