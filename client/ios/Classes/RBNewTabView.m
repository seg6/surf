#import "RBNewTabView.h"
#import "RBTheme.h"

#import <QuartzCore/QuartzCore.h>

@interface RBHorizonView : UIView
- (void)applyAppearance;
@end

@implementation RBHorizonView
+ (Class)layerClass { return [CAGradientLayer class]; }
- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        CAGradientLayer *layer = (CAGradientLayer *)self.layer;
        layer.startPoint = CGPointMake(0.0, 0.5);
        layer.endPoint = CGPointMake(1.0, 0.5);
        layer.cornerRadius = 1.0;
        [self applyAppearance];
    }
    return self;
}

- (void)applyAppearance {
    CAGradientLayer *layer = (CAGradientLayer *)self.layer;
    layer.colors = @[(id)[[RBTheme accentColor] colorWithAlphaComponent:0.08].CGColor,
                     (id)[[RBTheme seaGlassColor] colorWithAlphaComponent:0.82].CGColor,
                     (id)[[RBTheme accentColor] colorWithAlphaComponent:0.08].CGColor];
}
@end

@interface RBFavoriteButton : UIControl
@property(nonatomic, strong) UILabel *monogramLabel;
@property(nonatomic, strong) UILabel *nameLabel;
- (void)setName:(NSString *)name index:(NSUInteger)index;
- (void)applyAppearance;
@end

@implementation RBFavoriteButton

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [RBTheme surfaceColor];
        self.layer.cornerRadius = 9.0;
        self.layer.borderWidth = 1.0;
        self.layer.borderColor = [[RBTheme mistColor] CGColor];
        self.monogramLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.monogramLabel.textAlignment = NSTextAlignmentCenter;
        self.monogramLabel.textColor = [UIColor whiteColor];
        self.monogramLabel.font = [RBTheme displayFontOfSize:13.0];
        self.monogramLabel.layer.cornerRadius = 13.0;
        self.monogramLabel.layer.masksToBounds = YES;
        [self addSubview:self.monogramLabel];
        self.nameLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.nameLabel.backgroundColor = [UIColor clearColor];
        self.nameLabel.textColor = [RBTheme primaryTextColor];
        self.nameLabel.font = [RBTheme fontOfSize:13.0 bold:NO];
        self.nameLabel.lineBreakMode = NSLineBreakByTruncatingTail;
        [self addSubview:self.nameLabel];
    }
    return self;
}

- (void)setName:(NSString *)name index:(NSUInteger)index {
    self.nameLabel.text = name;
    NSString *trimmed = [name stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
    self.monogramLabel.text = [trimmed length] ? [[trimmed substringToIndex:1] uppercaseString] : @"•";
    NSArray *colors = @[[RBTheme accentColor], [RBTheme seaGlassColor],
                        [UIColor colorWithRed:0.29 green:0.43 blue:0.62 alpha:1.0]];
    self.monogramLabel.backgroundColor = [colors objectAtIndex:index % [colors count]];
    self.accessibilityLabel = name;
    self.isAccessibilityElement = YES;
}

- (void)setHighlighted:(BOOL)highlighted {
    [super setHighlighted:highlighted];
    self.backgroundColor = highlighted ? [[RBTheme accentColor] colorWithAlphaComponent:0.08]
                                       : [RBTheme surfaceColor];
    self.transform = highlighted ? CGAffineTransformMakeScale(0.985, 0.985) : CGAffineTransformIdentity;
}

- (void)applyAppearance {
    self.backgroundColor = [RBTheme surfaceColor];
    self.layer.borderColor = [[RBTheme mistColor] CGColor];
    self.nameLabel.textColor = [RBTheme primaryTextColor];
}

- (void)layoutSubviews {
    [super layoutSubviews];
    self.monogramLabel.frame = CGRectMake(10.0, floorf((self.bounds.size.height - 26.0) / 2.0), 26.0, 26.0);
    self.nameLabel.frame = CGRectMake(44.0, 0.0, MAX(10.0, self.bounds.size.width - 54.0), self.bounds.size.height);
}

@end

@interface RBNewTabView ()
@property(nonatomic, strong) UIImageView *markView;
@property(nonatomic, strong) RBHorizonView *horizonView;
@property(nonatomic, strong) UILabel *titleLabel;
@property(nonatomic, strong) UIButton *searchButton;
@property(nonatomic, strong) UILabel *favoritesLabel;
@property(nonatomic, strong) UIView *favoritesView;
@property(nonatomic, strong) UIButton *libraryButton;
@property(nonatomic, strong) NSArray *favorites;
@end

@implementation RBNewTabView

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [RBTheme pageBackgroundColor];
        self.horizonView = [[RBHorizonView alloc] initWithFrame:CGRectZero];
        [self addSubview:self.horizonView];
        self.markView = [[UIImageView alloc] initWithImage:[UIImage imageNamed:@"brand-mark.png"]];
        self.markView.contentMode = UIViewContentModeScaleAspectFit;
        self.markView.layer.shadowColor = [[RBTheme deepTideColor] CGColor];
        self.markView.layer.shadowOpacity = 0.14;
        self.markView.layer.shadowRadius = 8.0;
        self.markView.layer.shadowOffset = CGSizeMake(0.0, 3.0);
        [self addSubview:self.markView];

        self.titleLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.titleLabel.backgroundColor = [UIColor clearColor];
        self.titleLabel.text = @"Surf";
        self.titleLabel.textAlignment = NSTextAlignmentCenter;
        self.titleLabel.textColor = [RBTheme primaryTextColor];
        self.titleLabel.font = [RBTheme displayFontOfSize:30.0];
        [self addSubview:self.titleLabel];

        self.searchButton = [UIButton buttonWithType:UIButtonTypeCustom];
        self.searchButton.backgroundColor = [RBTheme surfaceColor];
        self.searchButton.layer.cornerRadius = 10.0;
        self.searchButton.layer.borderWidth = 1.0;
        self.searchButton.layer.borderColor = [[RBTheme mistColor] CGColor];
        self.searchButton.contentHorizontalAlignment = UIControlContentHorizontalAlignmentLeft;
        self.searchButton.contentEdgeInsets = UIEdgeInsetsMake(0.0, 14.0, 0.0, 14.0);
        self.searchButton.titleEdgeInsets = UIEdgeInsetsMake(0.0, 9.0, 0.0, 0.0);
        [self.searchButton setImage:[RBTheme icon:RBIconSearch size:18.0 color:[RBTheme accentColor]]
                           forState:UIControlStateNormal];
        [self.searchButton setTitle:@"Search or enter address" forState:UIControlStateNormal];
        [self.searchButton setTitleColor:[RBTheme secondaryTextColor] forState:UIControlStateNormal];
        self.searchButton.titleLabel.font = [RBTheme fontOfSize:15.0 bold:NO];
        [self.searchButton addTarget:self action:@selector(searchTapped:) forControlEvents:UIControlEventTouchUpInside];
        [self addSubview:self.searchButton];

        self.favoritesLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.favoritesLabel.backgroundColor = [UIColor clearColor];
        self.favoritesLabel.text = @"Favorites";
        self.favoritesLabel.textColor = [RBTheme primaryTextColor];
        self.favoritesLabel.font = [RBTheme displayFontOfSize:14.0];
        [self addSubview:self.favoritesLabel];

        self.favoritesView = [[UIView alloc] initWithFrame:CGRectZero];
        self.favoritesView.backgroundColor = [UIColor clearColor];
        [self addSubview:self.favoritesView];

        self.libraryButton = [UIButton buttonWithType:UIButtonTypeCustom];
        [self.libraryButton setTitle:@"Open Library" forState:UIControlStateNormal];
        [self.libraryButton setImage:[RBTheme icon:RBIconBook size:17.0 color:[RBTheme accentColor]]
                            forState:UIControlStateNormal];
        self.libraryButton.titleEdgeInsets = UIEdgeInsetsMake(0.0, 7.0, 0.0, 0.0);
        [RBTheme styleSecondaryButton:self.libraryButton];
        [self.libraryButton addTarget:self action:@selector(libraryTapped:) forControlEvents:UIControlEventTouchUpInside];
        [self addSubview:self.libraryButton];
    }
    return self;
}

- (void)setFavorites:(NSArray *)favorites {
    _favorites = [favorites isKindOfClass:[NSArray class]] ? [favorites copy] : @[];
    for (UIView *view in self.favoritesView.subviews) [view removeFromSuperview];
    NSUInteger count = MIN((NSUInteger)6, [_favorites count]);
    for (NSUInteger i = 0; i < count; i++) {
        NSDictionary *entry = [_favorites objectAtIndex:i];
        NSString *url = [entry objectForKey:@"url"] ?: @"";
        NSString *title = [entry objectForKey:@"title"];
        if (![title length]) title = [[NSURL URLWithString:url] host] ?: url;
        RBFavoriteButton *button = [[RBFavoriteButton alloc] initWithFrame:CGRectZero];
        button.tag = (NSInteger)i;
        [button setName:title index:i];
        [button addTarget:self action:@selector(favoriteTapped:) forControlEvents:UIControlEventTouchUpInside];
        [self.favoritesView addSubview:button];
    }
    self.favoritesLabel.hidden = count == 0;
    [self setNeedsLayout];
}

- (void)applyAppearance {
    self.backgroundColor = [RBTheme pageBackgroundColor];
    [self.horizonView applyAppearance];
    self.markView.layer.shadowColor = [[RBTheme deepTideColor] CGColor];
    self.titleLabel.textColor = [RBTheme primaryTextColor];
    self.searchButton.backgroundColor = [RBTheme surfaceColor];
    self.searchButton.layer.borderColor = [[RBTheme mistColor] CGColor];
    [self.searchButton setImage:[RBTheme icon:RBIconSearch size:18.0 color:[RBTheme accentColor]]
                       forState:UIControlStateNormal];
    [self.searchButton setTitleColor:[RBTheme secondaryTextColor] forState:UIControlStateNormal];
    self.favoritesLabel.textColor = [RBTheme primaryTextColor];
    for (RBFavoriteButton *button in self.favoritesView.subviews) [button applyAppearance];
    [self.libraryButton setImage:[RBTheme icon:RBIconBook size:17.0 color:[RBTheme accentColor]]
                        forState:UIControlStateNormal];
    [RBTheme styleSecondaryButton:self.libraryButton];
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat w = self.bounds.size.width;
    CGFloat h = self.bounds.size.height;
    BOOL compact = w < 520.0;
    CGFloat contentW = MIN(compact ? w - 28.0 : 560.0, w - 28.0);
    CGFloat x = floorf((w - contentW) / 2.0);
    CGFloat top = compact ? MAX(12.0, floorf(h * 0.035)) : MAX(34.0, floorf(h * 0.08));
    CGFloat mark = compact ? 50.0 : 62.0;
    self.horizonView.frame = CGRectMake(floorf((w - MIN(360.0, w - 48.0)) / 2.0),
                                         top + mark / 2.0 - 1.0, MIN(360.0, w - 48.0), 2.0);
    self.markView.frame = CGRectMake(floorf((w - mark) / 2.0), top, mark, mark);
    self.titleLabel.frame = CGRectMake(x, top + mark + 3.0, contentW, compact ? 34.0 : 40.0);
    CGFloat searchTop = top + mark + (compact ? 42.0 : 52.0);
    self.searchButton.frame = CGRectMake(x, searchTop, contentW, compact ? 44.0 : 48.0);
    CGFloat favoritesTop = searchTop + (compact ? 57.0 : 66.0);
    self.favoritesLabel.frame = CGRectMake(x + 2.0, favoritesTop, contentW - 4.0, 22.0);
    NSUInteger columns = compact ? 2 : 3;
    CGFloat rowH = compact ? 46.0 : 50.0;
    CGFloat gap = 8.0;
    NSUInteger rows = ([self.favoritesView.subviews count] + columns - 1) / columns;
    CGFloat favoritesH = rows ? rows * rowH + (rows - 1) * gap : 0.0;
    self.favoritesView.frame = CGRectMake(x, favoritesTop + 27.0, contentW, favoritesH);
    CGFloat cellW = floorf((contentW - gap * (columns - 1)) / columns);
    for (NSUInteger i = 0; i < [self.favoritesView.subviews count]; i++) {
        CGFloat cellX = (i % columns) * (cellW + gap);
        CGFloat cellY = (i / columns) * (rowH + gap);
        ((UIView *)[self.favoritesView.subviews objectAtIndex:i]).frame = CGRectMake(cellX, cellY, cellW, rowH);
    }
    CGFloat libraryY = favoritesTop + 27.0 + favoritesH + (compact ? 8.0 : 14.0);
    self.libraryButton.frame = CGRectMake(floorf((w - 176.0) / 2.0),
                                          MIN(libraryY, h - 44.0), 176.0, 38.0);
}

- (void)searchTapped:(id)sender { [self.delegate newTabViewWantsOmnibox:self]; }
- (void)libraryTapped:(id)sender { [self.delegate newTabViewWantsLibrary:self]; }

- (void)favoriteTapped:(UIControl *)sender {
    if (sender.tag < 0 || sender.tag >= (NSInteger)[self.favorites count]) return;
    NSString *url = [[self.favorites objectAtIndex:(NSUInteger)sender.tag] objectForKey:@"url"];
    if ([url length]) [self.delegate newTabView:self openURL:url];
}

@end
