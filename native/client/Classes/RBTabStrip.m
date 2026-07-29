#import "RBTabStrip.h"
#import "RBTheme.h"

#import <QuartzCore/QuartzCore.h>

@class RBTabCell;

@interface RBTabCell : UIView
@property(nonatomic, assign) NSInteger tabID;
@property(nonatomic, assign) BOOL active;
@property(nonatomic, strong) UILabel *titleLabel;
@property(nonatomic, strong) UIImageView *iconView;
@property(nonatomic, strong) UIButton *closeButton;
@property(nonatomic, assign) RBTabStrip *strip;
@end

@interface RBTabStrip ()
@property(nonatomic, strong) UIScrollView *scroller;
@property(nonatomic, strong) UIButton *addTabButton;
@property(nonatomic, strong) NSMutableDictionary *iconCache; // icon path -> UIImage
@property(nonatomic, strong) NSMutableSet *iconFetches;      // icon paths in flight
@property(nonatomic, strong) NSArray *tabs;
@property(nonatomic, strong) NSURL *baseURL;
- (void)cellTapped:(RBTabCell *)cell;
- (void)cellClosed:(RBTabCell *)cell;
@end

@implementation RBTabCell

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [UIColor clearColor];
        self.opaque = NO;
        self.layer.cornerRadius = 5.0;
        self.layer.borderWidth = 1.0;
        self.layer.masksToBounds = YES;

        self.closeButton = [UIButton buttonWithType:UIButtonTypeCustom];
        [self.closeButton setImage:[RBTheme icon:RBIconClose size:11.0 color:[UIColor colorWithWhite:0.35 alpha:1.0]]
                          forState:UIControlStateNormal];
        [self.closeButton addTarget:self action:@selector(closeTapped:) forControlEvents:UIControlEventTouchUpInside];
        [self addSubview:self.closeButton];

        self.iconView = [[UIImageView alloc] initWithFrame:CGRectZero];
        self.iconView.contentMode = UIViewContentModeScaleAspectFit;
        [self addSubview:self.iconView];

        self.titleLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.titleLabel.backgroundColor = [UIColor clearColor];
        self.titleLabel.font = [RBTheme fontOfSize:11.0 bold:NO];
        self.titleLabel.lineBreakMode = NSLineBreakByTruncatingTail;
        [self addSubview:self.titleLabel];

        UITapGestureRecognizer *tap = [[UITapGestureRecognizer alloc] initWithTarget:self action:@selector(tapped:)];
        [self addGestureRecognizer:tap];
    }
    return self;
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat h = self.bounds.size.height;
    CGFloat x = 8.0;
    self.closeButton.frame = CGRectMake(self.bounds.size.width - 27.0, 0.0, 26.0, h);
    BOOL hasIcon = self.iconView.image != nil;
    self.iconView.frame = CGRectMake(x, (h - 13.0) / 2.0, hasIcon ? 13.0 : 0.0, 13.0);
    if (hasIcon) x += 17.0;
    self.titleLabel.frame = CGRectMake(x, 0.0, MAX(10.0, self.bounds.size.width - x - 28.0), h);
    self.titleLabel.textColor = self.active ? [UIColor colorWithWhite:0.12 alpha:1.0]
                                            : [UIColor colorWithWhite:0.92 alpha:1.0];
    self.titleLabel.shadowColor = nil;
    self.backgroundColor = self.active ? [RBTheme pageBackgroundColor]
                                       : [UIColor colorWithWhite:1.0 alpha:0.08];
    self.layer.borderColor = [(self.active ? [RBTheme pageBackgroundColor]
                                           : [UIColor colorWithWhite:1.0 alpha:0.14]) CGColor];
}

- (void)tapped:(UITapGestureRecognizer *)tap {
    if (tap.state == UIGestureRecognizerStateEnded) [self.strip cellTapped:self];
}

- (void)closeTapped:(id)sender {
    [self.strip cellClosed:self];
}

@end

@implementation RBTabStrip

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [RBTheme stripBottomColor];
        RBGradientBar *bg = [[RBGradientBar alloc] initWithFrame:self.bounds];
        bg.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
        [bg setTopColor:[RBTheme stripTopColor] bottomColor:[RBTheme stripBottomColor] lineColor:[RBTheme barLineColor]];
        bg.userInteractionEnabled = NO;
        [self addSubview:bg];

        self.scroller = [[UIScrollView alloc] initWithFrame:CGRectZero];
        self.scroller.backgroundColor = [UIColor clearColor];
        self.scroller.showsHorizontalScrollIndicator = NO;
        self.scroller.showsVerticalScrollIndicator = NO;
        [self addSubview:self.scroller];

        self.addTabButton = [RBTheme barButtonWithIcon:RBIconPlus target:self action:@selector(newTapped:)];
        [self.addTabButton setImage:[RBTheme icon:RBIconPlus size:16.0 color:[UIColor colorWithWhite:0.95 alpha:1.0]]
                           forState:UIControlStateNormal];
        [self addSubview:self.addTabButton];

        self.iconCache = [NSMutableDictionary dictionary];
        self.iconFetches = [NSMutableSet set];
    }
    return self;
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat w = self.bounds.size.width;
    CGFloat h = self.bounds.size.height;
    CGFloat plusW = 36.0;
    self.addTabButton.frame = CGRectMake(w - plusW, 0.0, plusW, h);
    self.scroller.frame = CGRectMake(0.0, 0.0, w - plusW, h);
    [self rebuildCells];
}

- (void)setTabs:(NSArray *)tabs baseURL:(NSURL *)baseURL {
    _tabs = tabs;
    _baseURL = baseURL;
    [self rebuildCells];
}

- (void)purgeIconCache {
    [self.iconCache removeAllObjects];
}

- (void)rebuildCells {
	// Reuse cells by tab ID. The old implementation destroyed and recreated
	// every cell on every tabs broadcast and layoutSubviews pass; on the A5,
	// that main-thread churn also delayed video AU delivery/presentation and
	// made opening a tab feel substantially slower than the CDP operation.
	NSMutableDictionary *existing = [NSMutableDictionary dictionary];
	for (UIView *sub in self.scroller.subviews) {
		if (![sub isKindOfClass:[RBTabCell class]]) continue;
		RBTabCell *cell = (RBTabCell *)sub;
		[existing setObject:cell forKey:[NSNumber numberWithInteger:cell.tabID]];
	}
	NSMutableSet *used = [NSMutableSet set];
    NSUInteger count = [self.tabs count];
	if (!count) {
		for (RBTabCell *cell in [existing allValues]) [cell removeFromSuperview];
		self.scroller.contentSize = CGSizeZero;
		return;
	}
    CGFloat h = self.scroller.bounds.size.height;
    CGFloat available = self.scroller.bounds.size.width - 8.0;
    CGFloat cellW = MIN(190.0, MAX(96.0, available / count));
    CGFloat x = 3.0;
    for (NSUInteger i = 0; i < count; i++) {
        NSDictionary *tab = [self.tabs objectAtIndex:i];
        if (![tab isKindOfClass:[NSDictionary class]]) continue;
		NSInteger tabID = [[tab objectForKey:@"id"] integerValue];
		NSNumber *tabKey = [NSNumber numberWithInteger:tabID];
		RBTabCell *cell = [existing objectForKey:tabKey];
		if (!cell) {
			cell = [[RBTabCell alloc] initWithFrame:CGRectZero];
			cell.strip = self;
			cell.tabID = tabID;
			[self.scroller addSubview:cell];
		}
		[used addObject:tabKey];
		cell.frame = CGRectMake(x, 2.0, cellW - 3.0, h - 3.0);
		cell.active = [[tab objectForKey:@"active"] boolValue];
        NSString *title = [tab objectForKey:@"title"];
        NSString *tabURL = [tab objectForKey:@"url"];
        if ([tabURL hasPrefix:@"about:blank#surf-new"]) title = @"New Tab";
        if (![title length]) title = [tab objectForKey:@"url"];
        if (![title length]) title = @"Untitled";
        cell.titleLabel.text = title;
        NSString *iconPath = [tab objectForKey:@"icon"];
		if ([iconPath isKindOfClass:[NSString class]] && [iconPath length]) {
			UIImage *cached = [self.iconCache objectForKey:iconPath];
			if (cached) cell.iconView.image = cached;
			else {
				cell.iconView.image = nil;
				[self fetchIcon:iconPath];
			}
		} else {
			cell.iconView.image = nil;
		}
		[cell setNeedsLayout];
		[cell setNeedsDisplay];
		x += cellW;
	}
	for (NSNumber *tabKey in existing) {
		if (![used containsObject:tabKey]) [[existing objectForKey:tabKey] removeFromSuperview];
	}
    self.scroller.contentSize = CGSizeMake(x + 3.0, h);
    // Keep the active tab on screen.
    for (RBTabCell *cell in self.scroller.subviews) {
        if ([cell isKindOfClass:[RBTabCell class]] && cell.active) {
            [self.scroller scrollRectToVisible:CGRectInset(cell.frame, -20.0, 0.0) animated:NO];
            break;
        }
    }
}

- (void)fetchIcon:(NSString *)iconPath {
    if (!self.baseURL || [self.iconFetches containsObject:iconPath]) return;
    NSURL *url = [NSURL URLWithString:iconPath relativeToURL:self.baseURL];
    if (!url) return;
    [self.iconFetches addObject:iconPath];
    NSURLRequest *request = [NSURLRequest requestWithURL:url cachePolicy:NSURLRequestReturnCacheDataElseLoad timeoutInterval:15.0];
    [NSURLConnection sendAsynchronousRequest:request queue:[NSOperationQueue mainQueue]
                           completionHandler:^(NSURLResponse *response, NSData *data, NSError *error) {
        [self.iconFetches removeObject:iconPath];
        if (error || ![data length]) return;
        UIImage *image = [UIImage imageWithData:data];
        if (!image) return;
        [self.iconCache setObject:image forKey:iconPath];
        for (RBTabCell *cell in self.scroller.subviews) {
            if (![cell isKindOfClass:[RBTabCell class]]) continue;
            for (NSDictionary *tab in self.tabs) {
                if ([[tab objectForKey:@"id"] integerValue] == cell.tabID &&
                    [[tab objectForKey:@"icon"] isEqual:iconPath]) {
                    cell.iconView.image = image;
                    [cell setNeedsLayout];
                }
            }
        }
    }];
}

// Optimistic highlight (web-client parity): mark the tapped cell active
// immediately; the next tabs broadcast corrects it if needed.
- (void)cellTapped:(RBTabCell *)cell {
    for (RBTabCell *other in self.scroller.subviews) {
        if ([other isKindOfClass:[RBTabCell class]]) {
            other.active = other == cell;
            [other setNeedsLayout];
            [other setNeedsDisplay];
        }
    }
    [self.delegate tabStrip:self selectTab:cell.tabID];
}

- (void)cellClosed:(RBTabCell *)cell {
    [self.delegate tabStrip:self closeTab:cell.tabID];
}

- (void)newTapped:(id)sender {
    [self.delegate tabStripNewTab:self];
}

@end
