#import "RBLibraryController.h"
#import "RBTheme.h"

typedef enum {
    RBLibraryTabHistory = 0,
    RBLibraryTabBookmarks = 1,
    RBLibraryTabDownloads = 2
} RBLibraryTab;

@interface RBLibraryController () <UISearchBarDelegate>
@property(nonatomic, strong) UISegmentedControl *segments;
@property(nonatomic, strong) UISearchBar *searchBar;
// history
@property(nonatomic, strong) NSMutableArray *history; // {url,title,ts}
@property(nonatomic, assign) NSInteger historyTotal;
@property(nonatomic, assign) BOOL historyInFlight;
@property(nonatomic, copy) NSString *query;
// bookmarks
@property(nonatomic, strong) NSMutableArray *bookmarkItems; // {url,title,ts}
@property(nonatomic, assign) BOOL bookmarksLoaded;
// downloads
@property(nonatomic, strong) NSMutableArray *downloadItems; // {name,size,ts}
@property(nonatomic, assign) BOOL downloadsLoaded;
@property(nonatomic, strong) NSMutableDictionary *dlProgress; // name -> pct NSNumber
@end

@implementation RBLibraryController

- (id)init {
    self = [super initWithStyle:UITableViewStylePlain];
    if (self) {
        _history = [NSMutableArray array];
        _bookmarkItems = [NSMutableArray array];
        _downloadItems = [NSMutableArray array];
        _dlProgress = [NSMutableDictionary dictionary];
        _query = @"";
    }
    return self;
}

- (RBLibraryTab)tab {
    return (RBLibraryTab)self.segments.selectedSegmentIndex;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    self.segments = [[UISegmentedControl alloc] initWithItems:@[@"History", @"Bookmarks", @"Downloads"]];
    self.segments.segmentedControlStyle = UISegmentedControlStyleBar;
    self.segments.selectedSegmentIndex = RBLibraryTabHistory;
    [self.segments addTarget:self action:@selector(segmentChanged:) forControlEvents:UIControlEventValueChanged];
    self.navigationItem.titleView = self.segments;
    self.navigationItem.rightBarButtonItem =
        [[UIBarButtonItem alloc] initWithBarButtonSystemItem:UIBarButtonSystemItemDone
                                                      target:self action:@selector(doneTapped:)];
    [self refreshLeftButton];

    self.searchBar = [[UISearchBar alloc] initWithFrame:CGRectMake(0.0, 0.0, self.view.bounds.size.width, 44.0)];
    self.searchBar.delegate = self;
    self.searchBar.placeholder = @"Search history";
    self.tableView.tableHeaderView = self.searchBar;
    self.tableView.rowHeight = 54.0;
}

- (void)viewDidAppear:(BOOL)animated {
    [super viewDidAppear:animated];
    if (![self.history count]) [self requestHistoryFrom:0];
}

- (void)doneTapped:(id)sender {
    [self.presentingViewController dismissViewControllerAnimated:YES completion:nil];
}

- (void)refreshLeftButton {
    if ([self tab] == RBLibraryTabHistory) {
        self.navigationItem.leftBarButtonItem =
            [[UIBarButtonItem alloc] initWithTitle:@"Clear" style:UIBarButtonItemStylePlain
                                            target:self action:@selector(clearHistoryTapped:)];
    } else {
        self.navigationItem.leftBarButtonItem = nil;
    }
}

- (void)segmentChanged:(id)sender {
    [self refreshLeftButton];
    self.tableView.tableHeaderView = [self tab] == RBLibraryTabHistory ? self.searchBar : nil;
    if ([self tab] == RBLibraryTabBookmarks && !self.bookmarksLoaded && self.onNeedsData) self.onNeedsData(@"bookmarks");
    if ([self tab] == RBLibraryTabDownloads && !self.downloadsLoaded && self.onNeedsData) self.onNeedsData(@"downloads");
    [self.tableView reloadData];
}

- (void)clearHistoryTapped:(id)sender {
    if (self.onClearHistory) self.onClearHistory();
    [self.history removeAllObjects];
    self.historyTotal = 0;
    [self.tableView reloadData];
}

// ---- data in -------------------------------------------------------------

- (void)requestHistoryFrom:(NSInteger)offset {
    if (self.historyInFlight || !self.onRequestHistoryPage) return;
    self.historyInFlight = YES;
    self.onRequestHistoryPage(self.query, offset);
}

- (void)consumeHistoryReply:(NSDictionary *)message {
    self.historyInFlight = NO;
    NSArray *items = [[message objectForKey:@"items"] isKindOfClass:[NSArray class]] ? [message objectForKey:@"items"] : @[];
    NSInteger offset = [[message objectForKey:@"offset"] integerValue];
    self.historyTotal = [[message objectForKey:@"total"] integerValue];
    if (offset == 0) [self.history removeAllObjects];
    for (NSDictionary *entry in items) {
        if ([entry isKindOfClass:[NSDictionary class]]) [self.history addObject:entry];
    }
    if ([self tab] == RBLibraryTabHistory) [self.tableView reloadData];
}

- (void)setBookmarks:(NSArray *)bookmarks {
    self.bookmarksLoaded = YES;
    [self.bookmarkItems removeAllObjects];
    for (NSDictionary *entry in bookmarks ?: @[]) {
        if ([entry isKindOfClass:[NSDictionary class]]) [self.bookmarkItems addObject:entry];
    }
    if ([self isViewLoaded] && [self tab] == RBLibraryTabBookmarks) [self.tableView reloadData];
}

- (void)setDownloads:(NSArray *)items {
    self.downloadsLoaded = YES;
    [self.downloadItems removeAllObjects];
    for (NSDictionary *entry in items ?: @[]) {
        if ([entry isKindOfClass:[NSDictionary class]]) [self.downloadItems addObject:entry];
    }
    if ([self isViewLoaded] && [self tab] == RBLibraryTabDownloads) [self.tableView reloadData];
}

- (void)updateDownloadProgress:(NSString *)name pct:(NSInteger)pct {
    if (![name length]) return;
    [self.dlProgress setObject:[NSNumber numberWithInteger:pct] forKey:name];
    if (pct >= 100) [self.dlProgress removeObjectForKey:name];
    if ([self isViewLoaded] && [self tab] == RBLibraryTabDownloads) [self.tableView reloadData];
}

// ---- search (history only) -----------------------------------------------

- (void)searchBar:(UISearchBar *)searchBar textDidChange:(NSString *)searchText {
    self.query = searchText ?: @"";
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(fireSearch) object:nil];
    [self performSelector:@selector(fireSearch) withObject:nil afterDelay:0.3];
}

- (void)fireSearch {
    self.historyInFlight = NO;
    [self requestHistoryFrom:0];
}

- (void)searchBarSearchButtonClicked:(UISearchBar *)searchBar {
    [searchBar resignFirstResponder];
}

// ---- table ---------------------------------------------------------------

- (BOOL)historyHasMore {
    return (NSInteger)[self.history count] < self.historyTotal;
}

- (NSArray *)currentRows {
    switch ([self tab]) {
        case RBLibraryTabBookmarks: return self.bookmarkItems;
        case RBLibraryTabDownloads: return self.downloadItems;
        default: return self.history;
    }
}

- (NSInteger)tableView:(UITableView *)tableView numberOfRowsInSection:(NSInteger)section {
    NSInteger n = (NSInteger)[[self currentRows] count];
    if ([self tab] == RBLibraryTabHistory && [self historyHasMore]) n++;
    return n;
}

static NSString *RBLibFormatSize(long long bytes) {
    if (bytes >= 1024 * 1024) return [NSString stringWithFormat:@"%.1f MB", bytes / (1024.0 * 1024.0)];
    if (bytes >= 1024) return [NSString stringWithFormat:@"%.0f KB", bytes / 1024.0];
    return [NSString stringWithFormat:@"%lld B", bytes];
}

- (UITableViewCell *)tableView:(UITableView *)tableView cellForRowAtIndexPath:(NSIndexPath *)indexPath {
    NSArray *rows = [self currentRows];
    if ([self tab] == RBLibraryTabHistory && [self historyHasMore] && indexPath.row == (NSInteger)[rows count]) {
        UITableViewCell *cell = [tableView dequeueReusableCellWithIdentifier:@"more"];
        if (!cell) {
            cell = [[UITableViewCell alloc] initWithStyle:UITableViewCellStyleDefault reuseIdentifier:@"more"];
            cell.textLabel.textAlignment = NSTextAlignmentCenter;
            cell.textLabel.textColor = [UIColor colorWithWhite:0.45 alpha:1.0];
            cell.textLabel.font = [RBTheme fontOfSize:14.0 bold:NO];
        }
        cell.textLabel.text = [NSString stringWithFormat:@"Load more (%ld of %ld)…",
                               (long)[rows count], (long)self.historyTotal];
        return cell;
    }
    UITableViewCell *cell = [tableView dequeueReusableCellWithIdentifier:@"row"];
    if (!cell) {
        cell = [[UITableViewCell alloc] initWithStyle:UITableViewCellStyleSubtitle reuseIdentifier:@"row"];
        cell.textLabel.font = [RBTheme fontOfSize:15.0 bold:NO];
        cell.detailTextLabel.font = [RBTheme fontOfSize:12.0 bold:NO];
        cell.detailTextLabel.textColor = [UIColor colorWithWhite:0.5 alpha:1.0];
    }
    NSDictionary *entry = [rows objectAtIndex:(NSUInteger)indexPath.row];
    if ([self tab] == RBLibraryTabDownloads) {
        NSString *name = [entry objectForKey:@"name"] ?: @"download";
        NSNumber *pct = [self.dlProgress objectForKey:name];
        cell.textLabel.text = name;
        cell.detailTextLabel.text = pct
            ? [NSString stringWithFormat:@"downloading… %@%%", pct]
            : RBLibFormatSize([[entry objectForKey:@"size"] longLongValue]);
    } else {
        NSString *title = [entry objectForKey:@"title"];
        NSString *url = [entry objectForKey:@"url"] ?: @"";
        cell.textLabel.text = [title length] ? title : url;
        cell.detailTextLabel.text = url;
    }
    return cell;
}

- (void)tableView:(UITableView *)tableView didSelectRowAtIndexPath:(NSIndexPath *)indexPath {
    [tableView deselectRowAtIndexPath:indexPath animated:YES];
    NSArray *rows = [self currentRows];
    if ([self tab] == RBLibraryTabHistory && [self historyHasMore] && indexPath.row == (NSInteger)[rows count]) {
        [self requestHistoryFrom:(NSInteger)[rows count]];
        return;
    }
    NSDictionary *entry = [rows objectAtIndex:(NSUInteger)indexPath.row];
    if ([self tab] == RBLibraryTabDownloads) {
        NSString *name = [entry objectForKey:@"name"];
        if ([name length] && self.onOpenDownload) self.onOpenDownload(name);
        return;
    }
    NSString *url = [entry objectForKey:@"url"];
    if ([url length] && self.onPick) self.onPick(url);
}

- (BOOL)tableView:(UITableView *)tableView canEditRowAtIndexPath:(NSIndexPath *)indexPath {
    if ([self tab] == RBLibraryTabHistory && [self historyHasMore] &&
        indexPath.row == (NSInteger)[[self currentRows] count]) return NO;
    return YES;
}

- (void)tableView:(UITableView *)tableView commitEditingStyle:(UITableViewCellEditingStyle)editingStyle
                                            forRowAtIndexPath:(NSIndexPath *)indexPath {
    if (editingStyle != UITableViewCellEditingStyleDelete) return;
    NSUInteger row = (NSUInteger)indexPath.row;
    switch ([self tab]) {
        case RBLibraryTabHistory: {
            if (row >= [self.history count]) return;
            NSDictionary *entry = [self.history objectAtIndex:row];
            [self.history removeObjectAtIndex:row];
            if (self.historyTotal > 0) self.historyTotal--;
            if (self.onDeleteHistory) self.onDeleteHistory(entry);
            break;
        }
        case RBLibraryTabBookmarks: {
            if (row >= [self.bookmarkItems count]) return;
            NSString *url = [[self.bookmarkItems objectAtIndex:row] objectForKey:@"url"] ?: @"";
            [self.bookmarkItems removeObjectAtIndex:row];
            if (self.onDeleteBookmark) self.onDeleteBookmark(url);
            break;
        }
        case RBLibraryTabDownloads: {
            if (row >= [self.downloadItems count]) return;
            NSString *name = [[self.downloadItems objectAtIndex:row] objectForKey:@"name"] ?: @"";
            [self.downloadItems removeObjectAtIndex:row];
            if (self.onDeleteDownload) self.onDeleteDownload(name);
            break;
        }
    }
    [self.tableView reloadData];
}

@end
