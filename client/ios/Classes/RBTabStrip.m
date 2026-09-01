#import "RBTabStrip.h"

#import "RBSecureHTTPClient.h"
#import "RBTheme.h"

#import <QuartzCore/QuartzCore.h>

@class RBTabStrip;

static UIColor *RBTabBorderColor(void) {
    return [RBTheme barLineColor];
}

static NSArray *RBTabFillColors(BOOL active, BOOL highlighted) {
    UIColor *activeFill = [[RBTheme accentColor]
        colorWithAlphaComponent:[RBTheme isDarkMode] ? 0.24 : 0.14];
    UIColor *top = active ? activeFill : [UIColor clearColor];
    UIColor *bottom = top;
    if (highlighted && !active) {
        top = [[RBTheme separatorColor] colorWithAlphaComponent:0.44];
        bottom = top;
    } else if (highlighted) {
        top = [top colorWithAlphaComponent:0.76];
        bottom = [bottom colorWithAlphaComponent:0.76];
    }
    return @[(id)[top CGColor], (id)[bottom CGColor]];
}

@interface RBTabCell : UIControl
@property(nonatomic, assign) RBTabStrip *strip;
@property(nonatomic, assign) NSInteger tabID;
@property(nonatomic, assign) BOOL active;
@property(nonatomic, strong) CAGradientLayer *fillLayer;
@property(nonatomic, strong) CALayer *divider;
@property(nonatomic, strong) UIButton *closeButton;
@property(nonatomic, strong) UIImageView *faviconView;
@property(nonatomic, copy) NSString *iconPath;
@property(nonatomic, strong) UILabel *titleLabel;
- (void)applyAppearance;
@end

@interface RBTabStrip ()
@property(nonatomic, strong) UIScrollView *tabScroller;
@property(nonatomic, strong) UIButton *addTabButton;
@property(nonatomic, strong) NSArray *tabs;
@property(nonatomic, strong) NSMutableDictionary *cells;
@property(nonatomic, strong) NSURL *baseURL;
@property(nonatomic, copy) NSString *fingerprint;
@property(nonatomic, strong) NSMutableDictionary *iconCache;
@property(nonatomic, strong) NSMutableSet *iconFetches;
@property(nonatomic, assign) NSInteger activeTabID;
@property(nonatomic, assign) NSUInteger lastTabCount;
@property(nonatomic, assign) CGFloat lastLayoutWidth;
@property(nonatomic, assign) BOOL needsActiveReveal;
@end

@implementation RBTabCell

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [UIColor clearColor];
        self.opaque = NO;
        self.layer.cornerRadius = 8.0;
        self.layer.masksToBounds = YES;

        self.fillLayer = [CAGradientLayer layer];
        [self.layer insertSublayer:self.fillLayer atIndex:0];
        self.divider = [CALayer layer];
        self.divider.backgroundColor = [[RBTabBorderColor() colorWithAlphaComponent:0.55] CGColor];
        [self.layer addSublayer:self.divider];
        [self addTarget:self action:@selector(tapped:) forControlEvents:UIControlEventTouchUpInside];

        self.closeButton = [UIButton buttonWithType:UIButtonTypeCustom];
        [self.closeButton setImage:[RBTheme icon:RBIconClose size:10.5 color:[RBTheme secondaryTextColor]]
                          forState:UIControlStateNormal];
        [self.closeButton setImage:[RBTheme icon:RBIconClose size:10.5 color:[RBTheme primaryTextColor]]
                          forState:UIControlStateHighlighted];
        self.closeButton.accessibilityLabel = @"Close Tab";
        [self.closeButton addTarget:self action:@selector(closeTapped:) forControlEvents:UIControlEventTouchUpInside];
        [self addSubview:self.closeButton];

        self.faviconView = [[UIImageView alloc] initWithFrame:CGRectZero];
        self.faviconView.backgroundColor = [UIColor clearColor];
        self.faviconView.contentMode = UIViewContentModeScaleAspectFit;
        self.faviconView.userInteractionEnabled = NO;
        [self addSubview:self.faviconView];

        self.titleLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.titleLabel.backgroundColor = [UIColor clearColor];
        self.titleLabel.font = [RBTheme fontOfSize:12.0 bold:NO];
        self.titleLabel.textAlignment = NSTextAlignmentLeft;
        self.titleLabel.lineBreakMode = NSLineBreakByTruncatingTail;
        self.titleLabel.userInteractionEnabled = NO;
        [self addSubview:self.titleLabel];
    }
    return self;
}

- (void)setActive:(BOOL)active {
    _active = active;
    self.titleLabel.textColor = active ? [RBTheme primaryTextColor]
                                       : [[RBTheme secondaryTextColor] colorWithAlphaComponent:0.66];
    self.titleLabel.font = active ? [RBTheme displayFontOfSize:12.0] : [RBTheme fontOfSize:12.0 bold:NO];
    self.closeButton.alpha = active ? 1.0 : 0.46;
    self.faviconView.alpha = active ? 1.0 : 0.70;
    self.fillLayer.colors = RBTabFillColors(active, self.highlighted);
    self.layer.borderWidth = active ? 1.0 : 0.0;
    self.layer.borderColor = [[[RBTheme accentColor] colorWithAlphaComponent:0.72] CGColor];
    self.divider.hidden = active;
    [self setNeedsLayout];
}

- (void)setHighlighted:(BOOL)highlighted {
    [super setHighlighted:highlighted];
    self.fillLayer.colors = RBTabFillColors(self.active, highlighted);
}

- (void)applyAppearance {
    self.divider.backgroundColor = [[RBTabBorderColor() colorWithAlphaComponent:0.55] CGColor];
    [self.closeButton setImage:[RBTheme icon:RBIconClose size:10.5 color:[RBTheme secondaryTextColor]]
                      forState:UIControlStateNormal];
    [self.closeButton setImage:[RBTheme icon:RBIconClose size:10.5 color:[RBTheme primaryTextColor]]
                      forState:UIControlStateHighlighted];
    self.active = self.active;
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat w = self.bounds.size.width;
    CGFloat h = self.bounds.size.height;
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    self.fillLayer.frame = self.bounds;
    self.divider.frame = CGRectMake(MAX(0.0, w - 1.0), 7.0, 1.0, MAX(0.0, h - 14.0));
    [CATransaction commit];
    CGFloat closeWidth = 27.0;
    self.closeButton.frame = CGRectMake(MAX(0.0, w - closeWidth), 0.0, closeWidth, h);
    BOOL hasIcon = self.faviconView.image != nil;
    self.faviconView.hidden = !hasIcon;
    self.faviconView.frame = CGRectMake(7.0, floorf((h - 14.0) / 2.0), 14.0, 14.0);
    CGFloat titleX = hasIcon ? 26.0 : 9.0;
    self.titleLabel.frame = CGRectMake(titleX, 0.0, MAX(8.0, w - closeWidth - titleX - 4.0), h);
}

- (void)tapped:(id)sender { [self.strip performSelector:@selector(cellTapped:) withObject:self]; }
- (void)closeTapped:(id)sender { [self.strip performSelector:@selector(cellClosed:) withObject:self]; }

@end

@implementation RBTabStrip

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [UIColor clearColor];
        self.opaque = NO;

        self.tabScroller = [[UIScrollView alloc] initWithFrame:CGRectZero];
        self.tabScroller.backgroundColor = [UIColor clearColor];
        self.tabScroller.showsHorizontalScrollIndicator = NO;
        self.tabScroller.showsVerticalScrollIndicator = NO;
        self.tabScroller.alwaysBounceHorizontal = NO;
        self.tabScroller.directionalLockEnabled = YES;
        [self addSubview:self.tabScroller];
        self.addTabButton = [RBTheme barButtonWithIcon:RBIconPlus target:self action:@selector(newTapped:)];
        self.addTabButton.accessibilityLabel = @"New Tab";
        [self addSubview:self.addTabButton];
        self.cells = [NSMutableDictionary dictionary];
        self.iconCache = [NSMutableDictionary dictionary];
        self.iconFetches = [NSMutableSet set];
    }
    return self;
}

- (void)setTabs:(NSArray *)tabs baseURL:(NSURL *)baseURL fingerprint:(NSString *)fingerprint {
    self.tabs = tabs ?: @[];
    self.baseURL = baseURL;
    self.fingerprint = fingerprint;
    NSInteger nextActiveTabID = -1;
    for (NSDictionary *tab in self.tabs) {
        if ([[tab objectForKey:@"active"] boolValue]) {
            nextActiveTabID = [[tab objectForKey:@"id"] integerValue];
            break;
        }
    }
    self.needsActiveReveal = self.activeTabID != nextActiveTabID ||
                             self.lastTabCount != [self.tabs count];
    self.activeTabID = nextActiveTabID;
    self.lastTabCount = [self.tabs count];
    [self setNeedsLayout];
}

- (void)purgeIconCache {
    [self.iconCache removeAllObjects];
    for (RBTabCell *cell in [self.cells allValues]) cell.faviconView.image = nil;
}

- (void)applyAppearance {
    [RBTheme styleBarButton:self.addTabButton icon:RBIconPlus];
    for (RBTabCell *cell in [self.cells allValues]) [cell applyAppearance];
}

- (void)fetchIcon:(NSString *)iconPath {
    if (![iconPath length] || !self.baseURL || [self.iconFetches containsObject:iconPath]) return;
    NSURL *url = [NSURL URLWithString:iconPath relativeToURL:self.baseURL];
    if (!url) return;
    [self.iconFetches addObject:iconPath];
    __weak RBTabStrip *weakSelf = self;
    NSString *fingerprint = self.fingerprint;
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        NSURLRequest *request = [NSURLRequest requestWithURL:url cachePolicy:NSURLRequestReturnCacheDataElseLoad timeoutInterval:15.0];
        RBSecureHTTPClient *client = [RBSecureHTTPClient clientForEndpoint:[self.baseURL absoluteString] fingerprint:fingerprint];
        NSHTTPURLResponse *response = nil; NSError *error = nil;
        NSData *data = [client sendRequest:request response:&response error:&error];
        UIImage *image = !error && [response statusCode] == 200 ? [UIImage imageWithData:data] : nil;
        dispatch_async(dispatch_get_main_queue(), ^{
            RBTabStrip *strongSelf = weakSelf;
            if (!strongSelf) return;
            [strongSelf.iconFetches removeObject:iconPath];
            if (!image) return;
            [strongSelf.iconCache setObject:image forKey:iconPath];
            for (RBTabCell *cell in [strongSelf.cells allValues]) {
                if ([cell.iconPath isEqualToString:iconPath]) cell.faviconView.image = image;
                if ([cell.iconPath isEqualToString:iconPath]) [cell setNeedsLayout];
            }
        });
    });
}

- (NSString *)titleForTab:(NSDictionary *)tab {
    NSString *title = [tab objectForKey:@"title"];
    NSString *url = [tab objectForKey:@"url"];
    title = [title stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
    if ([url hasPrefix:@"about:blank"] || [title hasPrefix:@"about:blank"]) return @"New Tab";
    if (![title length] || [title isEqualToString:url]) {
        NSURL *parsedURL = [NSURL URLWithString:url ?: @""];
        title = [parsedURL host];
        if (![title length]) title = url;
    }
    return [title length] ? title : @"New Tab";
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat w = self.bounds.size.width;
    if (self.lastLayoutWidth != w) {
        self.lastLayoutWidth = w;
        self.needsActiveReveal = YES;
    }
    CGFloat verticalInset = 2.0;
    CGFloat buttonW = 36.0;
    CGFloat buttonGap = 5.0;
    CGFloat targetCellWidth = 142.0;
    CGFloat scrollerW = MAX(0.0, w - buttonW - buttonGap);
    self.addTabButton.frame = CGRectMake(scrollerW + buttonGap, 0.0,
                                         buttonW, self.bounds.size.height);
    CGFloat scrollerH = MAX(0.0, self.bounds.size.height - verticalInset * 2.0);
    CGRect scrollerFrame = CGRectMake(0.0, verticalInset, scrollerW, scrollerH);
    CGFloat available = scrollerW;
    NSUInteger tabCount = [self.tabs count];
    CGFloat cellW = targetCellWidth;
    if (tabCount && targetCellWidth * tabCount <= available) {
        cellW = MIN(190.0, floorf(available / tabCount));
    }
    self.tabScroller.frame = scrollerFrame;

    NSMutableSet *used = [NSMutableSet set];
    CGFloat usedTabsWidth = floorf(cellW * tabCount);
    self.tabScroller.contentSize = CGSizeMake(usedTabsWidth, scrollerH);
    RBTabCell *activeCell = nil;
    for (NSUInteger visibleIndex = 0; visibleIndex < tabCount; visibleIndex++) {
        NSDictionary *tab = [self.tabs objectAtIndex:visibleIndex];
        NSNumber *tabKey = [tab objectForKey:@"id"];
        if (!tabKey) continue;
        RBTabCell *cell = [self.cells objectForKey:tabKey];
        if (!cell) {
            cell = [[RBTabCell alloc] initWithFrame:CGRectZero];
            cell.strip = self;
            cell.tabID = [tabKey integerValue];
            [self.cells setObject:cell forKey:tabKey];
            [self.tabScroller addSubview:cell];
        }
        [used addObject:tabKey];
        cell.hidden = NO;
        CGFloat cellX = floor(visibleIndex * cellW);
        CGFloat nextX = floor((visibleIndex + 1) * cellW);
        // Individual tabs provide the only persistent chrome in the rail. The
        // active outline identifies selection without enclosing the full row.
        cell.frame = CGRectMake(cellX + 1.0, 0.0, MAX(1.0, nextX - cellX - 2.0), scrollerH);
        cell.titleLabel.text = [self titleForTab:tab];
        cell.accessibilityLabel = cell.titleLabel.text;
        id iconValue = [tab objectForKey:@"icon"];
        cell.iconPath = [iconValue isKindOfClass:[NSString class]] ? iconValue : nil;
        cell.faviconView.image = [self.iconCache objectForKey:cell.iconPath];
        [cell setNeedsLayout];
        if (!cell.faviconView.image) [self fetchIcon:cell.iconPath];
        cell.active = [[tab objectForKey:@"active"] boolValue];
        if (cell.active) activeCell = cell;
    }
    for (NSNumber *key in [self.cells allKeys]) {
        RBTabCell *cell = [self.cells objectForKey:key];
        if (![used containsObject:key]) cell.hidden = YES;
    }
    if (activeCell) [self.tabScroller bringSubviewToFront:activeCell];
    if (activeCell && self.needsActiveReveal) {
        self.needsActiveReveal = NO;
        CGRect revealRect = CGRectInset(activeCell.frame, -6.0, 0.0);
        [self.tabScroller scrollRectToVisible:revealRect animated:self.window != nil];
    }
}

- (void)cellTapped:(RBTabCell *)cell {
    for (RBTabCell *other in [self.cells allValues]) {
        if (!other.hidden) other.active = other == cell;
    }
    [self.delegate tabStrip:self selectTab:cell.tabID];
}

- (void)cellClosed:(RBTabCell *)cell {
    [self.delegate tabStrip:self closeTab:cell.tabID];
}

- (void)newTapped:(id)sender { [self.delegate tabStripNewTab:self]; }

@end
