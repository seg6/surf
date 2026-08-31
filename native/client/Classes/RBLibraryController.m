#import "RBLibraryController.h"
#import "RBTheme.h"

typedef enum {
    RBLibraryTabBookmarks = 0,
    RBLibraryTabHistory = 1,
    RBLibraryTabDownloads = 2
} RBLibraryTab;

static UITextField *RBLibrarySearchTextField(UIView *view) {
    if ([view isKindOfClass:[UITextField class]]) return (UITextField *)view;
    for (UIView *subview in view.subviews) {
        UITextField *field = RBLibrarySearchTextField(subview);
        if (field) return field;
    }
    return nil;
}

@interface RBLibraryController () <UISearchBarDelegate, UIAlertViewDelegate>
@property(nonatomic, strong) UISegmentedControl *segments;
@property(nonatomic, strong) UISearchBar *searchBar;
@property(nonatomic, strong) UIView *libraryHeader;
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
@property(nonatomic, strong) UILabel *emptyLabel;
- (void)applyAppearance;
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

- (void)applyAppearance {
    [RBTheme styleTableView:self.tableView];
    [RBTheme styleNavigationBar:self.navigationController.navigationBar];
    self.view.backgroundColor = [RBTheme pageBackgroundColor];
    self.libraryHeader.backgroundColor = [RBTheme pageBackgroundColor];
    self.emptyLabel.textColor = [RBTheme secondaryTextColor];

    UIColor *segmentTint = [RBTheme isDarkMode] ? [RBTheme separatorColor]
                                                 : [RBTheme accentColor];
    self.segments.tintColor = segmentTint;
    [self.segments setTitleTextAttributes:@{
        UITextAttributeTextColor: [RBTheme primaryTextColor],
        UITextAttributeFont: [RBTheme fontOfSize:12.0 bold:YES]
    } forState:UIControlStateNormal];
    [self.segments setTitleTextAttributes:@{
        UITextAttributeTextColor: [UIColor whiteColor],
        UITextAttributeFont: [RBTheme fontOfSize:12.0 bold:YES]
    } forState:UIControlStateSelected];

    self.searchBar.barStyle = [RBTheme isDarkMode] ? UIBarStyleBlack : UIBarStyleDefault;
    self.searchBar.translucent = NO;
    self.searchBar.tintColor = [RBTheme accentColor];
    self.searchBar.backgroundColor = [RBTheme pageBackgroundColor];
    [self.searchBar setBackgroundImage:[RBTheme solidImage:[RBTheme pageBackgroundColor]
                                                 cornerRadius:0.0]];
    UITextField *field = RBLibrarySearchTextField(self.searchBar);
    field.backgroundColor = [RBTheme surfaceColor];
    field.textColor = [RBTheme primaryTextColor];
    if ([field respondsToSelector:@selector(setTintColor:)]) field.tintColor = [RBTheme accentColor];
    field.keyboardAppearance = [RBTheme isDarkMode] ? UIKeyboardAppearanceDark
                                                     : UIKeyboardAppearanceDefault;
    field.layer.cornerRadius = 7.0;
    field.layer.masksToBounds = YES;
    [self.tableView reloadData];
}

- (void)viewDidLoad {
    [super viewDidLoad];
    [RBTheme styleTableView:self.tableView];
    self.title = @"Library";
    self.segments = [[UISegmentedControl alloc] initWithItems:@[@"Bookmarks", @"History", @"Downloads"]];
    self.segments.segmentedControlStyle = UISegmentedControlStyleBar;
    self.segments.selectedSegmentIndex = RBLibraryTabBookmarks;
    [self.segments addTarget:self action:@selector(segmentChanged:) forControlEvents:UIControlEventValueChanged];
    self.navigationItem.rightBarButtonItem =
        [[UIBarButtonItem alloc] initWithBarButtonSystemItem:UIBarButtonSystemItemDone
                                                      target:self action:@selector(doneTapped:)];
    [self refreshLeftButton];

    self.searchBar = [[UISearchBar alloc] initWithFrame:CGRectMake(0.0, 0.0, self.view.bounds.size.width, 44.0)];
    self.searchBar.delegate = self;
    self.searchBar.placeholder = @"Search bookmarks";
    self.libraryHeader = [[UIView alloc] initWithFrame:CGRectMake(0.0, 0.0,
                                                                  self.view.bounds.size.width, 78.0)];
    self.libraryHeader.backgroundColor = [RBTheme foamColor];
    [self.libraryHeader addSubview:self.segments];
    [self.libraryHeader addSubview:self.searchBar];
    self.tableView.tableHeaderView = self.libraryHeader;
    self.tableView.rowHeight = 54.0;

    self.emptyLabel = [[UILabel alloc] initWithFrame:self.tableView.bounds];
    self.emptyLabel.backgroundColor = [UIColor clearColor];
    self.emptyLabel.textAlignment = NSTextAlignmentCenter;
    self.emptyLabel.numberOfLines = 2;
    self.emptyLabel.textColor = [RBTheme secondaryTextColor];
    self.emptyLabel.font = [RBTheme fontOfSize:15.0 bold:NO];
    self.tableView.tableFooterView = [[UIView alloc] initWithFrame:CGRectZero];
    [self applyAppearance];
}

- (void)viewDidLayoutSubviews {
    [super viewDidLayoutSubviews];
    CGFloat width = self.tableView.bounds.size.width;
    self.libraryHeader.frame = CGRectMake(0.0, 0.0, width, 78.0);
    self.segments.frame = CGRectMake(6.0, 5.0, MAX(1.0, width - 12.0), 29.0);
    self.searchBar.frame = CGRectMake(0.0, 34.0, width, 44.0);
}

- (void)viewDidAppear:(BOOL)animated {
    [super viewDidAppear:animated];
    if (!self.bookmarksLoaded && self.onNeedsData) self.onNeedsData(@"bookmarks");
}

- (void)viewWillAppear:(BOOL)animated {
    [super viewWillAppear:animated];
    [self applyAppearance];
}

- (void)doneTapped:(id)sender {
    if (self.onDismiss) {
        self.onDismiss();
    } else {
        [self.presentingViewController dismissViewControllerAnimated:YES completion:nil];
    }
}

- (void)refreshLeftButton {
    if ([self tab] == RBLibraryTabHistory) {
        self.navigationItem.leftBarButtonItem =
            [[UIBarButtonItem alloc] initWithTitle:@"Clear" style:UIBarButtonItemStylePlain
                                            target:self action:@selector(clearHistoryTapped:)];
    } else {
        self.navigationItem.leftBarButtonItem = self.editButtonItem;
    }
}

- (void)segmentChanged:(id)sender {
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(fireSearch) object:nil];
    self.historyInFlight = NO;
    [self refreshLeftButton];
    [self setEditing:NO animated:NO];
    self.searchBar.text = @"";
    self.query = @"";
    static NSString *const placeholders[] = {@"Search bookmarks", @"Search history", @"Search downloads"};
    self.searchBar.placeholder = placeholders[[self tab]];
    self.tableView.tableHeaderView = self.libraryHeader;
    if ([self tab] == RBLibraryTabBookmarks && !self.bookmarksLoaded && self.onNeedsData) self.onNeedsData(@"bookmarks");
    if ([self tab] == RBLibraryTabHistory && ![self.history count]) [self requestHistoryFrom:0];
    if ([self tab] == RBLibraryTabDownloads && !self.downloadsLoaded && self.onNeedsData) self.onNeedsData(@"downloads");
    [self.tableView reloadData];
}

- (void)clearHistoryTapped:(id)sender {
    UIAlertView *alert = [[UIAlertView alloc] initWithTitle:@"Clear Browsing History?"
                                                    message:@"This removes all history stored by the Surf server."
                                                   delegate:self cancelButtonTitle:@"Cancel"
                                          otherButtonTitles:@"Clear", nil];
    [alert show];
}

- (void)alertView:(UIAlertView *)alertView clickedButtonAtIndex:(NSInteger)buttonIndex {
    if (buttonIndex == alertView.cancelButtonIndex) return;
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
    NSString *replyQuery = [message objectForKey:@"q"] ?: @"";
    if (![replyQuery isEqualToString:self.query ?: @""]) return;
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

// ---- search ---------------------------------------------------------------

- (void)searchBar:(UISearchBar *)searchBar textDidChange:(NSString *)searchText {
    self.query = searchText ?: @"";
    [NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(fireSearch) object:nil];
    if ([self tab] == RBLibraryTabHistory) {
        [self performSelector:@selector(fireSearch) withObject:nil afterDelay:0.3];
    } else {
        [self.tableView reloadData];
    }
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
    NSArray *source = nil;
    switch ([self tab]) {
        case RBLibraryTabBookmarks: source = self.bookmarkItems; break;
        case RBLibraryTabDownloads: source = self.downloadItems; break;
        default: return self.history;
    }
    NSString *needle = [self.query lowercaseString];
    if (![needle length]) return source;
    NSMutableArray *filtered = [NSMutableArray array];
    for (NSDictionary *entry in source) {
        NSString *primary = [self tab] == RBLibraryTabDownloads
            ? [entry objectForKey:@"name"] : [entry objectForKey:@"title"];
        NSString *secondary = [entry objectForKey:@"url"];
        NSString *primaryText = primary ?: @"";
        NSString *secondaryText = secondary ?: @"";
        if ([[primaryText lowercaseString] rangeOfString:needle].location != NSNotFound ||
            [[secondaryText lowercaseString] rangeOfString:needle].location != NSNotFound) {
            [filtered addObject:entry];
        }
    }
    return filtered;
}

- (NSInteger)tableView:(UITableView *)tableView numberOfRowsInSection:(NSInteger)section {
    NSInteger n = (NSInteger)[[self currentRows] count];
    if ([self tab] == RBLibraryTabHistory && [self historyHasMore]) n++;
    if (n == 0) {
        static NSString *const emptyText[] = {
            @"No bookmarks yet\nUse Share to add the current page.",
            @"No browsing history",
            @"No downloads"
        };
        self.emptyLabel.text = emptyText[[self tab]];
        self.tableView.backgroundView = self.emptyLabel;
    } else {
        self.tableView.backgroundView = nil;
    }
    return n;
}

static NSString *RBLibFormatSize(long long bytes) {
    if (bytes >= 1024 * 1024) return [NSString stringWithFormat:@"%.1f MB", bytes / (1024.0 * 1024.0)];
    if (bytes >= 1024) return [NSString stringWithFormat:@"%.0f KB", bytes / 1024.0];
    return [NSString stringWithFormat:@"%lld B", bytes];
}

static NSString *RBLibFormatDate(long long timestamp) {
    if (timestamp <= 0) return @"";
    NSDate *date = [NSDate dateWithTimeIntervalSince1970:timestamp];
    NSDateFormatter *formatter = [[NSDateFormatter alloc] init];
    formatter.dateStyle = NSDateFormatterShortStyle;
    formatter.timeStyle = NSDateFormatterShortStyle;
    return [formatter stringFromDate:date] ?: @"";
}

- (UITableViewCell *)tableView:(UITableView *)tableView cellForRowAtIndexPath:(NSIndexPath *)indexPath {
    NSArray *rows = [self currentRows];
    if ([self tab] == RBLibraryTabHistory && [self historyHasMore] && indexPath.row == (NSInteger)[rows count]) {
        UITableViewCell *cell = [tableView dequeueReusableCellWithIdentifier:@"more"];
        if (!cell) {
            cell = [[UITableViewCell alloc] initWithStyle:UITableViewCellStyleDefault reuseIdentifier:@"more"];
            cell.textLabel.textAlignment = NSTextAlignmentCenter;
            cell.textLabel.font = [RBTheme fontOfSize:14.0 bold:NO];
        }
        cell.backgroundColor = [RBTheme surfaceColor];
        cell.textLabel.textColor = [RBTheme secondaryTextColor];
        cell.textLabel.text = [NSString stringWithFormat:@"Load more (%ld of %ld)…",
                               (long)[rows count], (long)self.historyTotal];
        return cell;
    }
    UITableViewCell *cell = [tableView dequeueReusableCellWithIdentifier:@"row"];
    if (!cell) {
        cell = [[UITableViewCell alloc] initWithStyle:UITableViewCellStyleSubtitle reuseIdentifier:@"row"];
        cell.textLabel.font = [RBTheme fontOfSize:15.0 bold:NO];
        cell.detailTextLabel.font = [RBTheme fontOfSize:12.0 bold:NO];
    }
    cell.backgroundColor = [RBTheme surfaceColor];
    cell.textLabel.textColor = [RBTheme primaryTextColor];
    cell.detailTextLabel.textColor = [RBTheme secondaryTextColor];
    if (!cell.selectedBackgroundView) {
        cell.selectedBackgroundView = [[UIView alloc] initWithFrame:CGRectZero];
    }
    cell.selectedBackgroundView.backgroundColor = [[RBTheme separatorColor] colorWithAlphaComponent:0.62];
    NSDictionary *entry = [rows objectAtIndex:(NSUInteger)indexPath.row];
    if ([self tab] == RBLibraryTabDownloads) {
        NSString *name = [entry objectForKey:@"name"] ?: @"download";
        NSNumber *pct = [self.dlProgress objectForKey:name];
        cell.textLabel.text = name;
        cell.detailTextLabel.text = pct
            ? [NSString stringWithFormat:@"downloading… %@%%", pct]
            : [NSString stringWithFormat:@"%@  ·  %@",
               RBLibFormatSize([[entry objectForKey:@"size"] longLongValue]),
               RBLibFormatDate([[entry objectForKey:@"ts"] longLongValue])];
    } else {
        NSString *title = [entry objectForKey:@"title"];
        NSString *url = [entry objectForKey:@"url"] ?: @"";
        cell.textLabel.text = [title length] ? title : url;
        NSString *date = RBLibFormatDate([[entry objectForKey:@"ts"] longLongValue]);
        cell.detailTextLabel.text = [date length]
            ? [NSString stringWithFormat:@"%@  ·  %@", url, date] : url;
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
    NSArray *rows = [self currentRows];
    if (indexPath.row < 0 || indexPath.row >= (NSInteger)[rows count]) return;
    NSDictionary *selected = [rows objectAtIndex:(NSUInteger)indexPath.row];
    switch ([self tab]) {
        case RBLibraryTabHistory: {
            NSDictionary *entry = selected;
            [self.history removeObject:entry];
            if (self.historyTotal > 0) self.historyTotal--;
            if (self.onDeleteHistory) self.onDeleteHistory(entry);
            break;
        }
        case RBLibraryTabBookmarks: {
            NSString *url = [selected objectForKey:@"url"] ?: @"";
            [self.bookmarkItems removeObject:selected];
            if (self.onDeleteBookmark) self.onDeleteBookmark(url);
            break;
        }
        case RBLibraryTabDownloads: {
            NSString *name = [selected objectForKey:@"name"] ?: @"";
            [self.downloadItems removeObject:selected];
            if (self.onDeleteDownload) self.onDeleteDownload(name);
            break;
        }
    }
    [self.tableView reloadData];
}

@end
