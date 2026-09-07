#import "RBLogViewController.h"
#import "RBLog.h"
#import "RBLogEventController.h"
#import "RBTheme.h"
#include <limits.h>

@interface RBLogViewController ()
@property(nonatomic, strong) NSArray *dates;
@property(nonatomic, strong) NSDictionary *eventsByDate;
@property(nonatomic, strong) NSTimer *timer;
@property(nonatomic, copy) NSString *rawText;
@property(nonatomic, assign) unsigned long long displayedSize;
@property(nonatomic, assign) BOOL firstLoad;
@property(nonatomic, assign) BOOL reading;
@end

@implementation RBLogViewController

- (id)init {
    self = [super initWithStyle:UITableViewStylePlain];
    if (self) {
        self.title = @"Logs";
        self.dates = @[];
        self.eventsByDate = @{};
        self.firstLoad = YES;
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    [RBTheme styleTableView:self.tableView];
    self.tableView.backgroundColor = [RBTheme pageBackgroundColor];
    self.tableView.separatorColor = [RBTheme separatorColor];
    self.tableView.rowHeight = 62.0;
    UIBarButtonItem *copy = [[UIBarButtonItem alloc] initWithTitle:@"Copy All" style:UIBarButtonItemStyleBordered target:self action:@selector(copyTapped:)];
    UIBarButtonItem *clear = [[UIBarButtonItem alloc] initWithTitle:@"Clear" style:UIBarButtonItemStyleBordered target:self action:@selector(clearTapped:)];
    self.navigationItem.rightBarButtonItems = @[copy, clear];
}

- (void)viewWillAppear:(BOOL)animated {
    [super viewWillAppear:animated];
    [self refresh];
    self.timer = [NSTimer scheduledTimerWithTimeInterval:0.75 target:self selector:@selector(refresh) userInfo:nil repeats:YES];
}

- (void)viewWillDisappear:(BOOL)animated {
    [super viewWillDisappear:animated];
    [self.timer invalidate];
    self.timer = nil;
}

- (void)dealloc { [self.timer invalidate]; }

+ (NSDictionary *)eventForLine:(NSString *)line {
    if (![line length]) return nil;
    NSData *lineData = [line dataUsingEncoding:NSUTF8StringEncoding];
    id decoded = [NSJSONSerialization JSONObjectWithData:lineData options:0 error:nil];
    if ([decoded isKindOfClass:[NSDictionary class]]) {
        NSDictionary *record = decoded;
        NSString *timestamp = [record objectForKey:@"ts"] ?: @"";
        NSString *date = [timestamp length] >= 10 ? [timestamp substringToIndex:10] : @"Unknown date";
        NSString *time = [timestamp length] >= 23 ? [timestamp substringWithRange:NSMakeRange(11, 12)] : timestamp;
        NSDictionary *fields = [[record objectForKey:@"fields"] isKindOfClass:[NSDictionary class]]
            ? [record objectForKey:@"fields"] : @{};
        return @{@"date": date, @"time": time,
                 @"level": [record objectForKey:@"level"] ?: @"info",
                 @"component": [record objectForKey:@"component"] ?: @"app",
                 @"message": [record objectForKey:@"message"] ?: @"",
                 @"fields": fields, @"raw": line};
    }
    return nil;
}

+ (NSDictionary *)parseText:(NSString *)text {
    NSMutableArray *dates = [NSMutableArray array];
    NSMutableDictionary *groups = [NSMutableDictionary dictionary];
    NSArray *lines = [text componentsSeparatedByCharactersInSet:[NSCharacterSet newlineCharacterSet]];
    BOOL complete = [text hasSuffix:@"\n"] || [text hasSuffix:@"\r"];
    for (NSUInteger index = 0; index < [lines count]; index++) {
        NSString *line = [lines objectAtIndex:index];
        if (!complete && index == [lines count] - 1) break;
        NSDictionary *event = [self eventForLine:line];
        if (!event) continue;
        NSString *date = [event objectForKey:@"date"];
        NSMutableArray *events = [groups objectForKey:date];
        if (!events) {
            events = [NSMutableArray array];
            [groups setObject:events forKey:date];
            [dates addObject:date];
        }
        [events addObject:event];
    }
    return @{@"dates": dates, @"groups": groups};
}

- (BOOL)isFollowingEnd {
    if (self.firstLoad) return YES;
    NSIndexPath *last = [[self.tableView indexPathsForVisibleRows] lastObject];
    if (!last || ![self.dates count]) return YES;
    NSArray *events = [self.eventsByDate objectForKey:[self.dates lastObject]];
    return last.section == (NSInteger)[self.dates count] - 1 && last.row >= (NSInteger)[events count] - 3;
}

- (void)scrollToEnd {
    if (![self.dates count]) return;
    NSArray *events = [self.eventsByDate objectForKey:[self.dates lastObject]];
    if (![events count]) return;
    NSIndexPath *last = [NSIndexPath indexPathForRow:[events count] - 1 inSection:[self.dates count] - 1];
    [self.tableView scrollToRowAtIndexPath:last atScrollPosition:UITableViewScrollPositionBottom animated:NO];
}

- (void)refresh {
    if (self.reading) return;
    NSString *path = RBCurrentLogPath();
    NSDictionary *attributes = [[NSFileManager defaultManager] attributesOfItemAtPath:path error:nil];
    unsigned long long size = [[attributes objectForKey:NSFileSize] unsignedLongLongValue];
    if (size == self.displayedSize && self.rawText) return;
    BOOL follow = [self isFollowingEnd];
    self.reading = YES;
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_LOW, 0), ^{
        NSData *data = [NSData dataWithContentsOfFile:path] ?: [NSData data];
        const NSUInteger limit = 256 * 1024;
        if ([data length] > limit) data = [data subdataWithRange:NSMakeRange([data length] - limit, limit)];
        NSString *text = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding] ?: @"";
        NSDictionary *parsed = [[self class] parseText:text];
        dispatch_async(dispatch_get_main_queue(), ^{
            self.reading = NO;
            self.displayedSize = size;
            self.rawText = text;
            self.dates = [parsed objectForKey:@"dates"];
            self.eventsByDate = [parsed objectForKey:@"groups"];
            [self.tableView reloadData];
            if (follow) [self scrollToEnd];
            self.firstLoad = NO;
        });
    });
}

- (NSInteger)numberOfSectionsInTableView:(UITableView *)tableView { return [self.dates count]; }
- (NSInteger)tableView:(UITableView *)tableView numberOfRowsInSection:(NSInteger)section {
    return [[self.eventsByDate objectForKey:[self.dates objectAtIndex:section]] count];
}
- (NSString *)tableView:(UITableView *)tableView titleForHeaderInSection:(NSInteger)section {
    return [self.dates objectAtIndex:section];
}

- (UIColor *)colorForLevel:(NSString *)level {
    if ([level isEqualToString:@"error"]) return [UIColor colorWithRed:.72 green:.12 blue:.12 alpha:1];
    if ([level isEqualToString:@"warn"]) return [UIColor colorWithRed:.72 green:.42 blue:.04 alpha:1];
    if ([level isEqualToString:@"debug"]) return [UIColor colorWithRed:.35 green:.35 blue:.42 alpha:1];
    return [UIColor colorWithRed:.12 green:.46 blue:.28 alpha:1];
}

- (NSDictionary *)eventAtIndexPath:(NSIndexPath *)indexPath {
    return [[self.eventsByDate objectForKey:[self.dates objectAtIndex:indexPath.section]] objectAtIndex:indexPath.row];
}

- (UITableViewCell *)tableView:(UITableView *)tableView cellForRowAtIndexPath:(NSIndexPath *)indexPath {
    UITableViewCell *cell = [tableView dequeueReusableCellWithIdentifier:@"event"];
    if (!cell) cell = [[UITableViewCell alloc] initWithStyle:UITableViewCellStyleSubtitle reuseIdentifier:@"event"];
    NSDictionary *event = [self eventAtIndexPath:indexPath];
    cell.textLabel.text = [event objectForKey:@"message"];
    cell.textLabel.font = [RBTheme fontOfSize:13.0 bold:NO];
    cell.textLabel.numberOfLines = 2;
    NSString *level = [[event objectForKey:@"level"] uppercaseString];
    cell.detailTextLabel.text = [NSString stringWithFormat:@"%@   %@   %@",
                                 [event objectForKey:@"time"], level,
                                 [[event objectForKey:@"component"] uppercaseString]];
    cell.detailTextLabel.font = [RBTheme monospacedFontOfSize:10.0 bold:YES];
    cell.detailTextLabel.textColor = [self colorForLevel:[event objectForKey:@"level"]];
    cell.textLabel.textColor = [RBTheme primaryTextColor];
    cell.backgroundColor = [RBTheme surfaceColor];
    cell.accessoryType = UITableViewCellAccessoryDisclosureIndicator;
    return cell;
}

- (void)tableView:(UITableView *)tableView didSelectRowAtIndexPath:(NSIndexPath *)indexPath {
    [tableView deselectRowAtIndexPath:indexPath animated:YES];
    [self.navigationController pushViewController:[[RBLogEventController alloc] initWithEvent:[self eventAtIndexPath:indexPath]] animated:YES];
}

- (void)copyTapped:(id)sender { [UIPasteboard generalPasteboard].string = self.rawText ?: @""; }
- (void)clearTapped:(id)sender {
    RBClearLog();
    self.displayedSize = ULLONG_MAX;
    self.rawText = nil;
    self.dates = @[];
    self.eventsByDate = @{};
    [self.tableView reloadData];
    [self refresh];
}

@end
