#import "RBTabStrip.h"

#import "RBSecureHTTPClient.h"
#import "RBTheme.h"

#import <QuartzCore/QuartzCore.h>

@class RBTabStrip;

static UIColor *RBTabRowTopColor(void) {
    return [RBTheme usesClassicAppearance]
        ? [UIColor colorWithRed:0.70 green:0.73 blue:0.79 alpha:1.0]
        : [UIColor colorWithWhite:0.91 alpha:1.0];
}

static UIColor *RBTabRowBottomColor(void) {
    return [RBTheme usesClassicAppearance]
        ? [UIColor colorWithRed:0.59 green:0.63 blue:0.70 alpha:1.0]
        : [UIColor colorWithWhite:0.88 alpha:1.0];
}

static UIColor *RBTabBorderColor(void) {
    return [RBTheme usesClassicAppearance]
        ? [UIColor colorWithRed:0.42 green:0.46 blue:0.53 alpha:1.0]
        : [RBTheme separatorColor];
}

static NSArray *RBTabFillColors(BOOL active, BOOL highlighted) {
    UIColor *top = nil;
    UIColor *bottom = nil;
    if ([RBTheme usesClassicAppearance]) {
        top = active ? [UIColor colorWithRed:0.95 green:0.96 blue:0.98 alpha:1.0]
                     : [UIColor colorWithRed:0.79 green:0.81 blue:0.85 alpha:1.0];
        bottom = active ? [RBTheme pageBackgroundColor]
                        : [UIColor colorWithRed:0.68 green:0.72 blue:0.78 alpha:1.0];
    } else {
        top = active ? [RBTheme pageBackgroundColor] : [UIColor colorWithWhite:0.94 alpha:1.0];
        bottom = active ? [RBTheme pageBackgroundColor] : [UIColor colorWithWhite:0.88 alpha:1.0];
    }
    if (highlighted) {
        top = [top colorWithAlphaComponent:0.72];
        bottom = [bottom colorWithAlphaComponent:0.72];
    }
    return @[(id)[top CGColor], (id)[bottom CGColor]];
}

@interface RBTabAuxButton : UIButton
@property(nonatomic, strong) CAGradientLayer *fillLayer;
@property(nonatomic, strong) CALayer *leftBorder;
@end

@interface RBTabCell : UIControl
@property(nonatomic, assign) RBTabStrip *strip;
@property(nonatomic, assign) NSInteger tabID;
@property(nonatomic, assign) BOOL active;
@property(nonatomic, strong) CAGradientLayer *fillLayer;
@property(nonatomic, strong) CALayer *topBorder;
@property(nonatomic, strong) CALayer *leftBorder;
@property(nonatomic, strong) CALayer *rightBorder;
@property(nonatomic, strong) CALayer *bottomBorder;
@property(nonatomic, strong) UIButton *closeButton;
@property(nonatomic, strong) UIImageView *faviconView;
@property(nonatomic, copy) NSString *iconPath;
@property(nonatomic, strong) UILabel *titleLabel;
@end

@interface RBTabStrip ()
@property(nonatomic, strong) UIView *tabContainer;
@property(nonatomic, strong) UIButton *overflowButton;
@property(nonatomic, strong) UIButton *addTabButton;
@property(nonatomic, strong) NSArray *tabs;
@property(nonatomic, strong) NSArray *hiddenTabs;
@property(nonatomic, strong) NSMutableDictionary *cells;
@property(nonatomic, strong) NSURL *baseURL;
@property(nonatomic, copy) NSString *fingerprint;
@property(nonatomic, strong) NSMutableDictionary *iconCache;
@property(nonatomic, strong) NSMutableSet *iconFetches;
@property(nonatomic, strong) UIActionSheet *overflowSheet;
@end

@implementation RBTabAuxButton

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.fillLayer = [CAGradientLayer layer];
        self.fillLayer.colors = RBTabFillColors(NO, NO);
        [self.layer insertSublayer:self.fillLayer atIndex:0];
        self.leftBorder = [CALayer layer];
        self.leftBorder.backgroundColor = [RBTabBorderColor() CGColor];
        [self.layer addSublayer:self.leftBorder];
    }
    return self;
}

- (void)setHighlighted:(BOOL)highlighted {
    [super setHighlighted:highlighted];
    self.fillLayer.colors = RBTabFillColors(NO, highlighted);
}

- (void)layoutSubviews {
    [super layoutSubviews];
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    self.fillLayer.frame = self.bounds;
    self.leftBorder.frame = CGRectMake(0.0, 0.0, 1.0, self.bounds.size.height);
    [CATransaction commit];
}

@end

@implementation RBTabCell

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [UIColor clearColor];
        self.opaque = YES;

        self.fillLayer = [CAGradientLayer layer];
        [self.layer insertSublayer:self.fillLayer atIndex:0];
        self.topBorder = [CALayer layer];
        self.leftBorder = [CALayer layer];
        self.rightBorder = [CALayer layer];
        self.bottomBorder = [CALayer layer];
        for (CALayer *border in @[self.topBorder, self.leftBorder, self.rightBorder, self.bottomBorder]) {
            border.backgroundColor = [RBTabBorderColor() CGColor];
            [self.layer addSublayer:border];
        }
        [self addTarget:self action:@selector(tapped:) forControlEvents:UIControlEventTouchUpInside];

        self.closeButton = [UIButton buttonWithType:UIButtonTypeCustom];
        [self.closeButton setImage:[RBTheme icon:RBIconClose size:10.5 color:[UIColor colorWithWhite:0.34 alpha:1.0]]
                          forState:UIControlStateNormal];
        [self.closeButton setImage:[RBTheme icon:RBIconClose size:10.5 color:[UIColor colorWithWhite:0.16 alpha:1.0]]
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
        self.titleLabel.font = [RBTheme fontOfSize:([RBTheme usesClassicAppearance] ? 11.0 : 12.0) bold:NO];
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
                                       : ([RBTheme usesClassicAppearance]
                                          ? [UIColor colorWithWhite:0.26 alpha:1.0]
                                          : [RBTheme secondaryTextColor]);
    self.closeButton.alpha = active ? 1.0 : 0.72;
    self.fillLayer.colors = RBTabFillColors(active, self.highlighted);
    self.bottomBorder.hidden = active;
    [self setNeedsLayout];
}

- (void)setHighlighted:(BOOL)highlighted {
    [super setHighlighted:highlighted];
    self.fillLayer.colors = RBTabFillColors(self.active, highlighted);
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat w = self.bounds.size.width;
    CGFloat h = self.bounds.size.height;
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    self.fillLayer.frame = self.bounds;
    self.topBorder.frame = CGRectMake(0.0, 0.0, w, 1.0);
    self.leftBorder.frame = CGRectMake(0.0, 0.0, 1.0, h);
    self.rightBorder.frame = CGRectMake(MAX(0.0, w - 1.0), 0.0, 1.0, h);
    self.bottomBorder.frame = CGRectMake(0.0, MAX(0.0, h - 1.0), w, 1.0);
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
        self.backgroundColor = RBTabRowBottomColor();
        RBGradientBar *background = [[RBGradientBar alloc] initWithFrame:self.bounds];
        background.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
        [background setTopColor:RBTabRowTopColor()
                    bottomColor:RBTabRowBottomColor()
                      lineColor:[RBTheme barLineColor]];
        background.userInteractionEnabled = NO;
        [self addSubview:background];

        self.tabContainer = [[UIView alloc] initWithFrame:CGRectZero];
        self.tabContainer.backgroundColor = [UIColor clearColor];
        [self addSubview:self.tabContainer];
        self.overflowButton = [[RBTabAuxButton alloc] initWithFrame:CGRectZero];
        [self.overflowButton setTitle:@"\u00bb" forState:UIControlStateNormal];
        [self.overflowButton setTitleColor:[RBTheme primaryTextColor]
                                  forState:UIControlStateNormal];
        self.overflowButton.titleLabel.font = [RBTheme fontOfSize:16.0 bold:YES];
        self.overflowButton.accessibilityLabel = @"More Tabs";
        [self.overflowButton addTarget:self action:@selector(overflowTapped:) forControlEvents:UIControlEventTouchUpInside];
        [self addSubview:self.overflowButton];
        self.addTabButton = [[RBTabAuxButton alloc] initWithFrame:CGRectZero];
        [self.addTabButton setTitle:@"+" forState:UIControlStateNormal];
        [self.addTabButton setTitle:@"+" forState:UIControlStateHighlighted];
        [self.addTabButton setTitleColor:[UIColor blackColor] forState:UIControlStateNormal];
        [self.addTabButton setTitleColor:[UIColor blackColor] forState:UIControlStateHighlighted];
        self.addTabButton.titleLabel.font = [RBTheme fontOfSize:23.0 bold:YES];
        [self.addTabButton addTarget:self action:@selector(newTapped:) forControlEvents:UIControlEventTouchUpInside];
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
    [self setNeedsLayout];
}

- (void)purgeIconCache {
    [self.iconCache removeAllObjects];
    for (RBTabCell *cell in [self.cells allValues]) cell.faviconView.image = nil;
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
    if ([url hasPrefix:@"about:blank"] || [title hasPrefix:@"about:blank"]) return @"New Page";
    if (![title length] || [title isEqualToString:url]) {
        NSURL *parsedURL = [NSURL URLWithString:url ?: @""];
        title = [parsedURL host];
        if (![title length]) title = url;
    }
    return [title length] ? title : @"New Page";
}

- (NSArray *)visibleTabsForCount:(NSUInteger)capacity {
    if ([self.tabs count] <= capacity) {
        self.hiddenTabs = @[];
        return self.tabs;
    }
    NSInteger active = 0;
    for (NSUInteger i = 0; i < [self.tabs count]; i++) {
        if ([[[self.tabs objectAtIndex:i] objectForKey:@"active"] boolValue]) { active = (NSInteger)i; break; }
    }
    NSInteger start = MAX(0, MIN(active - (NSInteger)capacity / 2,
                                 (NSInteger)[self.tabs count] - (NSInteger)capacity));
    NSRange visibleRange = NSMakeRange((NSUInteger)start, capacity);
    NSArray *visible = [self.tabs subarrayWithRange:visibleRange];
    NSMutableArray *hidden = [NSMutableArray arrayWithCapacity:[self.tabs count] - capacity];
    for (NSUInteger i = 0; i < [self.tabs count]; i++) {
        if (!NSLocationInRange(i, visibleRange)) [hidden addObject:[self.tabs objectAtIndex:i]];
    }
    self.hiddenTabs = hidden;
    return visible;
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat w = self.bounds.size.width;
    CGFloat h = self.bounds.size.height;
    CGFloat buttonW = 34.0;
    BOOL needsOverflow = [self.tabs count] > (NSUInteger)MAX(1, floor((w - buttonW) / 108.0));
    CGFloat controlsW = buttonW + (needsOverflow ? buttonW : 0.0);
    CGFloat available = MAX(1.0, w - controlsW);
    NSUInteger capacity = (NSUInteger)MAX(1, floor(available / 108.0));
    NSArray *visibleTabs = [self visibleTabsForCount:capacity];
    needsOverflow = [self.hiddenTabs count] > 0;
    self.overflowButton.hidden = !needsOverflow;
    self.addTabButton.frame = CGRectMake(w - buttonW, 0.0, buttonW, h);
    self.overflowButton.frame = needsOverflow ? CGRectMake(w - buttonW * 2.0, 0.0, buttonW, h) : CGRectZero;
    self.tabContainer.frame = CGRectMake(0.0, 0.0, w - buttonW - (needsOverflow ? buttonW : 0.0), h);

    NSMutableSet *used = [NSMutableSet set];
    CGFloat cellW = [visibleTabs count] ? MIN(190.0, self.tabContainer.bounds.size.width / [visibleTabs count]) : 0.0;
    RBTabCell *activeCell = nil;
    for (NSUInteger visibleIndex = 0; visibleIndex < [visibleTabs count]; visibleIndex++) {
        NSDictionary *tab = [visibleTabs objectAtIndex:visibleIndex];
        NSNumber *tabKey = [tab objectForKey:@"id"];
        if (!tabKey) continue;
        RBTabCell *cell = [self.cells objectForKey:tabKey];
        if (!cell) {
            cell = [[RBTabCell alloc] initWithFrame:CGRectZero];
            cell.strip = self;
            cell.tabID = [tabKey integerValue];
            [self.cells setObject:cell forKey:tabKey];
            [self.tabContainer addSubview:cell];
        }
        [used addObject:tabKey];
        cell.hidden = NO;
        CGFloat cellX = floor(visibleIndex * cellW);
        CGFloat nextX = floor((visibleIndex + 1) * cellW);
        cell.frame = CGRectMake(cellX, 0.0, MAX(70.0, nextX - cellX), h);
        cell.titleLabel.text = [self titleForTab:tab];
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
    if (activeCell) [self.tabContainer bringSubviewToFront:activeCell];
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

- (void)overflowTapped:(UIButton *)sender {
    UIActionSheet *sheet = [[UIActionSheet alloc] initWithTitle:@"Open Tabs" delegate:self
                                              cancelButtonTitle:nil destructiveButtonTitle:nil
                                              otherButtonTitles:nil];
    for (NSDictionary *tab in self.hiddenTabs) [sheet addButtonWithTitle:[self titleForTab:tab]];
    sheet.cancelButtonIndex = [sheet addButtonWithTitle:@"Cancel"];
    self.overflowSheet = sheet;
    [sheet showFromRect:sender.frame inView:self animated:YES];
}

- (void)actionSheet:(UIActionSheet *)actionSheet clickedButtonAtIndex:(NSInteger)buttonIndex {
    if (actionSheet != self.overflowSheet) return;
    if (buttonIndex >= 0 && buttonIndex < (NSInteger)[self.hiddenTabs count]) {
        NSDictionary *tab = [self.hiddenTabs objectAtIndex:(NSUInteger)buttonIndex];
        [self.delegate tabStrip:self selectTab:[[tab objectForKey:@"id"] integerValue]];
    }
    self.overflowSheet = nil;
}

@end
