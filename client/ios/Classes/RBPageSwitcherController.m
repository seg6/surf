#import "RBPageSwitcherController.h"

#import "RBTheme.h"
#import "RBSecureHTTPClient.h"

#import <QuartzCore/QuartzCore.h>

@interface RBPageCard : UIView
@property(nonatomic, assign) NSInteger tabID;
@property(nonatomic, strong) UIView *headerView;
@property(nonatomic, strong) UILabel *pageTitleLabel;
@property(nonatomic, strong) UIImageView *previewView;
@property(nonatomic, strong) UILabel *placeholderLabel;
@property(nonatomic, strong) UIImageView *titleFaviconView;
@property(nonatomic, strong) UIImageView *placeholderFaviconView;
@property(nonatomic, strong) UIButton *closeButton;
@property(nonatomic, strong) UIView *closeDot;
@property(nonatomic, copy) NSString *iconPath;
@end

@implementation RBPageCard
@end

@interface RBPageSwitcherController () <UIGestureRecognizerDelegate>
@property(nonatomic, strong) NSArray *tabs;
@property(nonatomic, strong) NSDictionary *thumbnails;
@property(nonatomic, strong) RBGradientBar *titleBar;
@property(nonatomic, strong) UILabel *titleLabel;
@property(nonatomic, strong) UIScrollView *scrollView;
@property(nonatomic, strong) UIPageControl *pageControl;
@property(nonatomic, strong) RBGradientBar *bottomBar;
@property(nonatomic, strong) UIButton *addPageButton;
@property(nonatomic, strong) UIButton *doneButton;
@property(nonatomic, strong) NSMutableArray *cards;
@property(nonatomic, strong) NSURL *baseURL;
@property(nonatomic, copy) NSString *fingerprint;
@property(nonatomic, strong) NSMutableDictionary *iconCache;
@property(nonatomic, strong) NSMutableSet *iconFetches;
@property(nonatomic, assign) NSInteger selectedIndex;
@property(nonatomic, assign) CGFloat lastLayoutWidth;
@end

@implementation RBPageSwitcherController

- (id)initWithTabs:(NSArray *)tabs thumbnails:(NSDictionary *)thumbnails baseURL:(NSURL *)baseURL fingerprint:(NSString *)fingerprint {
    self = [super initWithNibName:nil bundle:nil];
    if (self) {
        self.tabs = tabs ?: @[];
        self.thumbnails = thumbnails ?: @{};
        self.baseURL = baseURL;
        self.fingerprint = fingerprint;
        self.iconCache = [NSMutableDictionary dictionary];
        self.iconFetches = [NSMutableSet set];
        self.selectedIndex = [self activeIndexInTabs:self.tabs];
        self.modalTransitionStyle = UIModalTransitionStyleFlipHorizontal;
        self.modalPresentationStyle = UIModalPresentationFullScreen;
    }
    return self;
}

- (NSInteger)activeIndexInTabs:(NSArray *)tabs {
    for (NSUInteger i = 0; i < [tabs count]; i++) {
        NSDictionary *tab = [tabs objectAtIndex:i];
        if ([[tab objectForKey:@"active"] boolValue]) return (NSInteger)i;
    }
    return [tabs count] ? 0 : NSNotFound;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    self.view.backgroundColor = [RBTheme foamColor];

    self.titleBar = [[RBGradientBar alloc] initWithFrame:CGRectZero];
    [self.titleBar setTopColor:[RBTheme deepTideColor]
                   bottomColor:[RBTheme deepTideColor]
                     lineColor:[RBTheme seaGlassColor]];
    [self.view addSubview:self.titleBar];
    self.titleLabel = [[UILabel alloc] initWithFrame:CGRectZero];
    self.titleLabel.backgroundColor = [UIColor clearColor];
    self.titleLabel.text = @"Tabs";
    self.titleLabel.textAlignment = NSTextAlignmentCenter;
    self.titleLabel.font = [RBTheme displayFontOfSize:18.0];
    self.titleLabel.textColor = [UIColor whiteColor];
    [self.titleBar addSubview:self.titleLabel];

    self.scrollView = [[UIScrollView alloc] initWithFrame:CGRectZero];
    self.scrollView.backgroundColor = [UIColor clearColor];
    self.scrollView.pagingEnabled = YES;
    self.scrollView.bounces = NO;
    self.scrollView.alwaysBounceHorizontal = NO;
    self.scrollView.alwaysBounceVertical = NO;
    self.scrollView.showsHorizontalScrollIndicator = NO;
    self.scrollView.delegate = self;
    [self.view addSubview:self.scrollView];

    self.pageControl = [[UIPageControl alloc] initWithFrame:CGRectZero];
    self.pageControl.userInteractionEnabled = NO;
    if ([self.pageControl respondsToSelector:@selector(setCurrentPageIndicatorTintColor:)]) {
        self.pageControl.currentPageIndicatorTintColor = [RBTheme accentColor];
        self.pageControl.pageIndicatorTintColor = [RBTheme mistColor];
    }
    [self.view addSubview:self.pageControl];

    self.bottomBar = [[RBGradientBar alloc] initWithFrame:CGRectZero];
    [self.view addSubview:self.bottomBar];
    self.addPageButton = [UIButton buttonWithType:UIButtonTypeCustom];
    [self.addPageButton setTitle:@"New Tab" forState:UIControlStateNormal];
    [self.addPageButton setImage:[RBTheme icon:RBIconPlus size:17.0 color:[RBTheme accentColor]]
                        forState:UIControlStateNormal];
    self.addPageButton.titleEdgeInsets = UIEdgeInsetsMake(0.0, 7.0, 0.0, 0.0);
    [self.addPageButton setTitleColor:[RBTheme iconColor] forState:UIControlStateNormal];
    self.addPageButton.titleLabel.font = [RBTheme displayFontOfSize:15.0];
    self.addPageButton.accessibilityLabel = @"New Tab";
    [self.addPageButton addTarget:self action:@selector(newTapped:) forControlEvents:UIControlEventTouchUpInside];
    [self.bottomBar addSubview:self.addPageButton];
    self.doneButton = [UIButton buttonWithType:UIButtonTypeCustom];
    [self.doneButton setTitle:@"Done" forState:UIControlStateNormal];
    [self.doneButton setTitleColor:[RBTheme iconColor] forState:UIControlStateNormal];
    self.doneButton.titleLabel.font = [RBTheme displayFontOfSize:15.0];
    self.doneButton.accessibilityLabel = @"Done";
    [self.doneButton addTarget:self action:@selector(doneTapped:) forControlEvents:UIControlEventTouchUpInside];
    [self.bottomBar addSubview:self.doneButton];

    self.cards = [NSMutableArray array];
    [self rebuildCards];
}

- (void)updateTabs:(NSArray *)tabs thumbnails:(NSDictionary *)thumbnails {
    NSInteger selectedID = [self selectedTabID];
    self.tabs = tabs ?: @[];
    self.thumbnails = thumbnails ?: @{};
    self.selectedIndex = [self indexOfTabID:selectedID];
    if (self.selectedIndex == NSNotFound) self.selectedIndex = [self activeIndexInTabs:self.tabs];
    if ([self isViewLoaded]) {
        [self rebuildCards];
        [self.view setNeedsLayout];
    }
}

- (void)didReceiveMemoryWarning {
    [super didReceiveMemoryWarning];
    self.thumbnails = @{};
    [self.iconCache removeAllObjects];
    if ([self isViewLoaded]) {
        [self rebuildCards];
        [self.view setNeedsLayout];
    }
}

- (NSInteger)indexOfTabID:(NSInteger)tabID {
    for (NSUInteger i = 0; i < [self.tabs count]; i++) {
        if ([[[self.tabs objectAtIndex:i] objectForKey:@"id"] integerValue] == tabID) return (NSInteger)i;
    }
    return NSNotFound;
}

- (NSInteger)selectedTabID {
    if (self.selectedIndex == NSNotFound || self.selectedIndex < 0 ||
        self.selectedIndex >= (NSInteger)[self.tabs count]) return NSNotFound;
    return [[[self.tabs objectAtIndex:(NSUInteger)self.selectedIndex] objectForKey:@"id"] integerValue];
}

- (NSString *)titleForTab:(NSDictionary *)tab {
    NSString *url = [tab objectForKey:@"url"];
    if ([url hasPrefix:@"about:blank#surf-new"]) return @"New Tab";
    NSString *title = [tab objectForKey:@"title"];
    if (![title length]) title = url;
    if ([title hasPrefix:@"about:blank#surf-new"]) title = @"New Tab";
    return [title length] ? title : @"Untitled";
}

- (void)rebuildCards {
    for (UIView *card in self.cards) [card removeFromSuperview];
    [self.cards removeAllObjects];
    for (NSDictionary *tab in self.tabs) {
        NSInteger tabID = [[tab objectForKey:@"id"] integerValue];
        RBPageCard *card = [[RBPageCard alloc] initWithFrame:CGRectZero];
        card.tabID = tabID;
        card.backgroundColor = [RBTheme surfaceColor];
        card.layer.cornerRadius = 12.0;
        card.layer.borderWidth = 1.0;
        card.layer.borderColor = [[RBTheme mistColor] CGColor];
        card.layer.shadowColor = [[RBTheme deepTideColor] CGColor];
        card.layer.shadowOpacity = 0.16;
        card.layer.shadowRadius = 9.0;
        card.layer.shadowOffset = CGSizeMake(0.0, 4.0);

        UIView *header = [[UIView alloc] initWithFrame:CGRectZero];
        header.backgroundColor = [RBTheme surfaceColor];
        [card addSubview:header];
        card.headerView = header;

        UILabel *title = [[UILabel alloc] initWithFrame:CGRectZero];
        title.backgroundColor = [UIColor clearColor];
        title.text = [self titleForTab:tab];
        title.textAlignment = NSTextAlignmentCenter;
        title.lineBreakMode = NSLineBreakByTruncatingTail;
        title.font = [RBTheme displayFontOfSize:13.0];
        title.textColor = [RBTheme primaryTextColor];
        [card addSubview:title];
        card.pageTitleLabel = title;

        NSString *iconPath = [tab objectForKey:@"icon"];
        card.iconPath = [iconPath isKindOfClass:[NSString class]] ? iconPath : nil;
        UIImageView *titleFavicon = [[UIImageView alloc] initWithFrame:CGRectZero];
        titleFavicon.backgroundColor = [UIColor clearColor];
        titleFavicon.contentMode = UIViewContentModeScaleAspectFit;
        titleFavicon.image = [self.iconCache objectForKey:card.iconPath];
        [card addSubview:titleFavicon];
        card.titleFaviconView = titleFavicon;
        if (!titleFavicon.image) [self fetchIcon:card.iconPath];

        UIImageView *preview = [[UIImageView alloc] initWithFrame:CGRectZero];
        preview.backgroundColor = [RBTheme pageBackgroundColor];
        preview.contentMode = UIViewContentModeScaleAspectFill;
        preview.clipsToBounds = YES;
        preview.image = [self.thumbnails objectForKey:[NSNumber numberWithInteger:tabID]];
        [card addSubview:preview];
        card.previewView = preview;

        if (!preview.image) {
            UIImageView *favicon = [[UIImageView alloc] initWithFrame:CGRectZero];
            favicon.backgroundColor = [UIColor clearColor];
            favicon.contentMode = UIViewContentModeScaleAspectFit;
            favicon.image = [self.iconCache objectForKey:card.iconPath];
            if (!favicon.image) {
                [self fetchIcon:card.iconPath];
            }
            [card addSubview:favicon];
            card.placeholderFaviconView = favicon;

            UILabel *placeholder = [[UILabel alloc] initWithFrame:CGRectZero];
            placeholder.backgroundColor = [UIColor clearColor];
            placeholder.text = [self titleForTab:tab];
            placeholder.textAlignment = NSTextAlignmentCenter;
            placeholder.numberOfLines = 3;
            placeholder.font = [RBTheme fontOfSize:16.0 bold:YES];
            placeholder.textColor = [RBTheme secondaryTextColor];
            [card addSubview:placeholder];
            card.placeholderLabel = placeholder;
        }

        UIButton *close = [UIButton buttonWithType:UIButtonTypeCustom];
        close.tag = tabID;
        close.backgroundColor = [UIColor clearColor];
        close.accessibilityLabel = @"Close Tab";
        [close addTarget:self action:@selector(closeTapped:) forControlEvents:UIControlEventTouchUpInside];
        UIView *closeDot = [[UIView alloc] initWithFrame:CGRectZero];
        closeDot.backgroundColor = [RBTheme mistColor];
        closeDot.layer.borderWidth = 0.0;
        closeDot.layer.cornerRadius = 9.0;
        closeDot.userInteractionEnabled = NO;
        UIImageView *closeGlyph = [[UIImageView alloc] initWithImage:
            [RBTheme icon:RBIconClose size:11.0 color:[RBTheme slateColor]]];
        closeGlyph.tag = 9107;
        closeGlyph.userInteractionEnabled = NO;
        [closeDot addSubview:closeGlyph];
        [close addSubview:closeDot];
        [card addSubview:close];
        card.closeButton = close;
        card.closeDot = closeDot;

        UITapGestureRecognizer *tap = [[UITapGestureRecognizer alloc] initWithTarget:self action:@selector(cardTapped:)];
        tap.delegate = self;
        [card addGestureRecognizer:tap];
        [self.scrollView addSubview:card];
        [self.cards addObject:card];
    }
    self.pageControl.numberOfPages = [self.tabs count];
    self.pageControl.currentPage = self.selectedIndex == NSNotFound ? 0 : self.selectedIndex;
}

- (void)viewDidLayoutSubviews {
    [super viewDidLayoutSubviews];
    CGFloat w = self.view.bounds.size.width;
    CGFloat h = self.view.bounds.size.height;
    CGFloat titleH = 44.0;
    CGFloat bottomH = 44.0;
    CGFloat dotsH = 25.0;
    self.titleBar.frame = CGRectMake(0.0, 0.0, w, titleH);
    self.titleLabel.frame = self.titleBar.bounds;
    self.bottomBar.frame = CGRectMake(0.0, h - bottomH, w, bottomH);
    self.addPageButton.frame = CGRectMake(0.0, 0.0, w / 2.0, bottomH);
    self.doneButton.frame = CGRectMake(w / 2.0, 0.0, w / 2.0, bottomH);
    self.pageControl.frame = CGRectMake(0.0, h - bottomH - dotsH, w, dotsH);
    self.scrollView.frame = CGRectMake(0.0, titleH, w, MAX(1.0, h - titleH - bottomH - dotsH));

    CGFloat cardW = MIN(320.0, MAX(220.0, w - 42.0));
    CGFloat cardH = MAX(160.0, self.scrollView.bounds.size.height - 24.0);
    for (NSUInteger i = 0; i < [self.cards count]; i++) {
        RBPageCard *card = [self.cards objectAtIndex:i];
        card.frame = CGRectMake(w * i + floorf((w - cardW) / 2.0), 10.0, cardW, cardH);
        UILabel *title = card.pageTitleLabel;
        UIImageView *preview = card.previewView;
        UILabel *placeholder = card.placeholderLabel;
        card.headerView.frame = CGRectMake(0.0, 0.0, cardW, 28.0);
        BOOL hasIcon = card.titleFaviconView.image != nil;
        card.titleFaviconView.hidden = !hasIcon;
        CGFloat titleX = hasIcon ? 54.0 : 34.0;
        title.frame = CGRectMake(titleX, 0.0, MAX(1.0, cardW - titleX - 7.0), 28.0);
        title.textAlignment = NSTextAlignmentLeft;
        card.titleFaviconView.frame = CGRectMake(33.0, 6.0, 16.0, 16.0);
        preview.frame = CGRectMake(0.0, 28.0, cardW, cardH - 28.0);
        if (placeholder) {
            CGFloat centerY = CGRectGetMidY(preview.frame);
            BOOL hasPlaceholderIcon = card.placeholderFaviconView.image != nil;
            card.placeholderFaviconView.hidden = !hasPlaceholderIcon;
            card.placeholderFaviconView.frame = CGRectMake(floorf((cardW - 48.0) / 2.0), centerY - 62.0, 48.0, 48.0);
            placeholder.frame = CGRectMake(24.0, hasPlaceholderIcon ? centerY - 4.0 : centerY - 38.0, cardW - 48.0, 78.0);
        }
        card.closeButton.frame = CGRectMake(0.0, 0.0, 32.0, 28.0);
        card.closeDot.frame = CGRectMake(7.0, 5.0, 18.0, 18.0);
        UIView *closeGlyph = [card.closeDot viewWithTag:9107];
        closeGlyph.frame = card.closeDot.bounds;
    }
    self.scrollView.contentSize = CGSizeMake(w * [self.tabs count], self.scrollView.bounds.size.height);
    if (self.selectedIndex != NSNotFound && [self.tabs count]) {
        self.scrollView.contentOffset = CGPointMake(w * self.selectedIndex, 0.0);
    }
    self.lastLayoutWidth = w;
}

- (void)fetchIcon:(NSString *)iconPath {
    if (![iconPath length] || !self.baseURL || [self.iconFetches containsObject:iconPath]) return;
    NSURL *url = [NSURL URLWithString:iconPath relativeToURL:self.baseURL];
    if (!url) return;
    [self.iconFetches addObject:iconPath];
    NSURLRequest *request = [NSURLRequest requestWithURL:url
                                            cachePolicy:NSURLRequestReturnCacheDataElseLoad
                                        timeoutInterval:15.0];
    __weak RBPageSwitcherController *weakSelf = self;
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        RBSecureHTTPClient *client = [RBSecureHTTPClient clientForEndpoint:[self.baseURL absoluteString] fingerprint:self.fingerprint];
        NSHTTPURLResponse *response = nil; NSError *error = nil;
        NSData *data = [client sendRequest:request response:&response error:&error];
        dispatch_async(dispatch_get_main_queue(), ^{
        RBPageSwitcherController *strongSelf = weakSelf;
        [strongSelf.iconFetches removeObject:iconPath];
        if (!strongSelf || error || [response statusCode] != 200 || ![data length]) return;
        UIImage *image = [UIImage imageWithData:data];
        if (!image) return;
        [strongSelf.iconCache setObject:image forKey:iconPath];
        for (RBPageCard *card in strongSelf.cards) {
            if ([card.iconPath isEqualToString:iconPath]) {
                card.titleFaviconView.image = image;
                card.placeholderFaviconView.image = image;
            }
        }
        [strongSelf.view setNeedsLayout];
        });
    });
}

- (void)scrollViewDidEndDecelerating:(UIScrollView *)scrollView {
    CGFloat w = MAX(1.0, scrollView.bounds.size.width);
    NSInteger page = (NSInteger)floor((scrollView.contentOffset.x + w / 2.0) / w);
    self.selectedIndex = MIN(MAX(0, page), MAX(0, (NSInteger)[self.tabs count] - 1));
    self.pageControl.currentPage = self.selectedIndex;
}

- (BOOL)gestureRecognizer:(UIGestureRecognizer *)gestureRecognizer shouldReceiveTouch:(UITouch *)touch {
    UIView *view = touch.view;
    while (view && view != gestureRecognizer.view) {
        if ([view isKindOfClass:[UIButton class]]) return NO;
        view = view.superview;
    }
    return YES;
}

- (void)cardTapped:(UITapGestureRecognizer *)tap {
    if (tap.state != UIGestureRecognizerStateEnded) return;
    NSInteger index = [self indexOfTabID:[(RBPageCard *)tap.view tabID]];
    if (index != NSNotFound) self.selectedIndex = index;
    [self chooseSelectedTab];
}

- (void)closeTapped:(UIButton *)sender {
    NSInteger tabID = sender.tag;
    NSInteger oldIndex = [self indexOfTabID:tabID];
    if (oldIndex == NSNotFound) return;
    NSMutableArray *remaining = [self.tabs mutableCopy];
    [remaining removeObjectAtIndex:(NSUInteger)oldIndex];
    self.tabs = remaining;
    self.selectedIndex = MIN(oldIndex, (NSInteger)[self.tabs count] - 1);
    if (![self.tabs count]) self.selectedIndex = NSNotFound;
    [self.delegate pageSwitcher:self closeTab:tabID];
    [self rebuildCards];
    [self.view setNeedsLayout];
}

- (void)newTapped:(id)sender {
    [self.delegate pageSwitcherNewTab:self];
}

- (void)doneTapped:(id)sender {
    if (![self.tabs count]) {
        [self.delegate pageSwitcherNewTab:self];
        return;
    }
    [self chooseSelectedTab];
}

- (void)chooseSelectedTab {
    NSInteger tabID = [self selectedTabID];
    if (tabID != NSNotFound) [self.delegate pageSwitcher:self selectTab:tabID];
}

@end
