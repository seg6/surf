#import <UIKit/UIKit.h>

// The Library: one full-screen surface for everything the browser remembers —
// History | Bookmarks | Downloads behind a segmented control. Replaces the
// old per-feature popovers. Data arrives via the consume/set methods; the
// root view controller owns the wire.
@interface RBLibraryController : UITableViewController

// History: server-paged. onRequestHistoryPage sends {"t":"history",q,offset}.
@property(nonatomic, copy) void (^onRequestHistoryPage)(NSString *query, NSInteger offset);
@property(nonatomic, copy) void (^onDeleteHistory)(NSDictionary *entry); // histdel
@property(nonatomic, copy) void (^onClearHistory)(void);                 // clear history
// Bookmarks
@property(nonatomic, copy) void (^onDeleteBookmark)(NSString *url);      // bmdel
// Downloads
@property(nonatomic, copy) void (^onOpenDownload)(NSString *name);       // fetch + Open In
@property(nonatomic, copy) void (^onDeleteDownload)(NSString *name);     // dldel
// Any row navigation (history/bookmark tap)
@property(nonatomic, copy) void (^onPick)(NSString *url);
// Segment switched to a tab whose data hasn't loaded: root refreshes it.
@property(nonatomic, copy) void (^onNeedsData)(NSString *kind); // "bookmarks"|"downloads"

- (void)consumeHistoryReply:(NSDictionary *)message;   // {"t":"history"}
- (void)setBookmarks:(NSArray *)bookmarks;             // from {"t":"hist"}
- (void)setDownloads:(NSArray *)items;                 // from {"t":"downloads"}
- (void)updateDownloadProgress:(NSString *)name pct:(NSInteger)pct; // dlprogress
@end
