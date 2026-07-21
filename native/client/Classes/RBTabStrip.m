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
        self.titleLabel.font = [RBTheme fontOfSize:12.0 bold:NO];
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
    self.closeButton.frame = CGRectMake(x, 0.0, 26.0, h);
    x += 26.0;
    BOOL hasIcon = self.iconView.image != nil;
    self.iconView.frame = CGRectMake(x, (h - 14.0) / 2.0, hasIcon ? 14.0 : 0.0, 14.0);
    if (hasIcon) x += 19.0;
    self.titleLabel.frame = CGRectMake(x, 0.0, MAX(10.0, self.bounds.size.width - x - 10.0), h);
    self.titleLabel.textColor = self.active ? [UIColor colorWithWhite:0.12 alpha:1.0]
                                            : [UIColor colorWithWhite:0.92 alpha:1.0];
    self.titleLabel.shadowColor = self.active ? nil : [UIColor colorWithWhite:0.0 alpha:0.4];
    self.titleLabel.shadowOffset = CGSizeMake(0.0, -1.0);
    [self setNeedsDisplay];
}

// Trapezoid tab shape with rounded top corners; active tabs are light and
// join the page, inactive tabs sit darker in the strip.
- (void)drawRect:(CGRect)rect {
    CGContextRef ctx = UIGraphicsGetCurrentContext();
    CGFloat w = self.bounds.size.width;
    CGFloat h = self.bounds.size.height;
    CGFloat slant = 6.0, r = 5.0;

    CGMutablePathRef path = CGPathCreateMutable();
    CGPathMoveToPoint(path, NULL, 0.0, h);
    CGPathAddLineToPoint(path, NULL, slant - r * 0.3, r);
    CGPathAddQuadCurveToPoint(path, NULL, slant, 0.0, slant + r, 0.0);
    CGPathAddLineToPoint(path, NULL, w - slant - r, 0.0);
    CGPathAddQuadCurveToPoint(path, NULL, w - slant, 0.0, w - slant + r * 0.3, r);
    CGPathAddLineToPoint(path, NULL, w, h);
    CGPathCloseSubpath(path);

    CGContextSaveGState(ctx);
    CGContextAddPath(ctx, path);
    CGContextClip(ctx);
    CGColorSpaceRef space = CGColorSpaceCreateDeviceRGB();
    CGFloat lightParts[8] = {0.93, 0.94, 0.96, 1.0, 0.85, 0.87, 0.90, 1.0};
    CGFloat darkParts[8] = {0.60, 0.63, 0.68, 1.0, 0.49, 0.52, 0.57, 1.0};
    CGGradientRef grad = CGGradientCreateWithColorComponents(space, self.active ? lightParts : darkParts, NULL, 2);
    CGContextDrawLinearGradient(ctx, grad, CGPointMake(0.0, 0.0), CGPointMake(0.0, h), 0);
    CGGradientRelease(grad);
    CGColorSpaceRelease(space);
    CGContextRestoreGState(ctx);

    CGContextAddPath(ctx, path);
    CGContextSetStrokeColorWithColor(ctx, [[UIColor colorWithRed:0.28 green:0.30 blue:0.34 alpha:1.0] CGColor]);
    CGContextSetLineWidth(ctx, 1.0);
    CGContextStrokePath(ctx);
    CGPathRelease(path);
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
    CGFloat plusW = 40.0;
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
    for (UIView *sub in [self.scroller.subviews copy]) {
        if ([sub isKindOfClass:[RBTabCell class]]) [sub removeFromSuperview];
    }
    NSUInteger count = [self.tabs count];
    if (!count) return;
    CGFloat h = self.scroller.bounds.size.height;
    CGFloat available = self.scroller.bounds.size.width - 8.0;
    CGFloat cellW = MIN(220.0, MAX(110.0, available / count));
    CGFloat x = 4.0;
    for (NSUInteger i = 0; i < count; i++) {
        NSDictionary *tab = [self.tabs objectAtIndex:i];
        if (![tab isKindOfClass:[NSDictionary class]]) continue;
        RBTabCell *cell = [[RBTabCell alloc] initWithFrame:CGRectMake(x, 2.0, cellW - 2.0, h - 2.0)];
        cell.strip = self;
        cell.tabID = [[tab objectForKey:@"id"] integerValue];
        cell.active = [[tab objectForKey:@"active"] boolValue];
        NSString *title = [tab objectForKey:@"title"];
        if (![title length]) title = [tab objectForKey:@"url"];
        if (![title length]) title = @"Untitled";
        cell.titleLabel.text = title;
        NSString *iconPath = [tab objectForKey:@"icon"];
        if ([iconPath isKindOfClass:[NSString class]] && [iconPath length]) {
            UIImage *cached = [self.iconCache objectForKey:iconPath];
            if (cached) cell.iconView.image = cached;
            else [self fetchIcon:iconPath];
        }
        [self.scroller addSubview:cell];
        x += cellW;
    }
    self.scroller.contentSize = CGSizeMake(x + 4.0, h);
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
