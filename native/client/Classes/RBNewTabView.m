#import "RBNewTabView.h"
#import "RBTheme.h"

#import <QuartzCore/QuartzCore.h>

@interface RBNewTabView ()
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

        self.titleLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.titleLabel.backgroundColor = [UIColor clearColor];
        self.titleLabel.text = @"Surf";
        self.titleLabel.textAlignment = NSTextAlignmentCenter;
        self.titleLabel.textColor = [RBTheme primaryTextColor];
        self.titleLabel.font = [RBTheme fontOfSize:34.0 bold:YES];
        [self addSubview:self.titleLabel];

        self.searchButton = [UIButton buttonWithType:UIButtonTypeCustom];
        self.searchButton.backgroundColor = [UIColor whiteColor];
        self.searchButton.layer.cornerRadius = 8.0;
        self.searchButton.layer.borderWidth = 1.0;
        self.searchButton.layer.borderColor = [[RBTheme separatorColor] CGColor];
        self.searchButton.contentHorizontalAlignment = UIControlContentHorizontalAlignmentLeft;
        self.searchButton.contentEdgeInsets = UIEdgeInsetsMake(0.0, 16.0, 0.0, 16.0);
        [self.searchButton setTitle:@"Search or enter address" forState:UIControlStateNormal];
        [self.searchButton setTitleColor:[RBTheme secondaryTextColor] forState:UIControlStateNormal];
        self.searchButton.titleLabel.font = [RBTheme fontOfSize:16.0 bold:NO];
        [self.searchButton addTarget:self action:@selector(searchTapped:) forControlEvents:UIControlEventTouchUpInside];
        [self addSubview:self.searchButton];

        self.favoritesLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.favoritesLabel.backgroundColor = [UIColor clearColor];
        self.favoritesLabel.text = @"FAVORITES";
        self.favoritesLabel.textColor = [RBTheme secondaryTextColor];
        self.favoritesLabel.font = [RBTheme fontOfSize:11.0 bold:YES];
        [self addSubview:self.favoritesLabel];

        self.favoritesView = [[UIView alloc] initWithFrame:CGRectZero];
        self.favoritesView.backgroundColor = [UIColor clearColor];
        [self addSubview:self.favoritesView];

        self.libraryButton = [UIButton buttonWithType:UIButtonTypeCustom];
        [self.libraryButton setTitle:@"Open Library" forState:UIControlStateNormal];
        [self.libraryButton setTitleColor:[RBTheme accentColor] forState:UIControlStateNormal];
        self.libraryButton.titleLabel.font = [RBTheme fontOfSize:14.0 bold:NO];
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
        UIButton *button = [UIButton buttonWithType:UIButtonTypeCustom];
        button.tag = (NSInteger)i;
        button.backgroundColor = [UIColor whiteColor];
        button.layer.cornerRadius = 7.0;
        button.layer.borderWidth = 1.0;
        button.layer.borderColor = [[RBTheme separatorColor] CGColor];
        [button setTitle:title forState:UIControlStateNormal];
        [button setTitleColor:[RBTheme primaryTextColor] forState:UIControlStateNormal];
        button.titleLabel.font = [RBTheme fontOfSize:13.0 bold:NO];
        button.titleLabel.lineBreakMode = NSLineBreakByTruncatingTail;
        button.contentEdgeInsets = UIEdgeInsetsMake(0.0, 10.0, 0.0, 10.0);
        [button addTarget:self action:@selector(favoriteTapped:) forControlEvents:UIControlEventTouchUpInside];
        [self.favoritesView addSubview:button];
    }
    self.favoritesLabel.hidden = count == 0;
    [self setNeedsLayout];
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat w = self.bounds.size.width;
    CGFloat top = MAX(42.0, floorf(self.bounds.size.height * 0.12));
    self.titleLabel.frame = CGRectMake(20.0, top, w - 40.0, 42.0);
    CGFloat searchW = MIN(540.0, w - 80.0);
    self.searchButton.frame = CGRectMake(floorf((w - searchW) / 2.0), top + 66.0, searchW, 48.0);
    CGFloat favoritesTop = top + 152.0;
    self.favoritesLabel.frame = CGRectMake(floorf((w - searchW) / 2.0), favoritesTop, searchW, 20.0);
    self.favoritesView.frame = CGRectMake(floorf((w - searchW) / 2.0), favoritesTop + 26.0, searchW, 112.0);
    NSArray *buttons = self.favoritesView.subviews;
    CGFloat gap = 8.0;
    CGFloat cellW = floorf((searchW - gap * 2.0) / 3.0);
    for (NSUInteger i = 0; i < [buttons count]; i++) {
        UIView *button = [buttons objectAtIndex:i];
        button.frame = CGRectMake((i % 3) * (cellW + gap), (i / 3) * 52.0, cellW, 44.0);
    }
    self.libraryButton.frame = CGRectMake((w - 180.0) / 2.0, favoritesTop + 146.0, 180.0, 40.0);
}

- (void)searchTapped:(id)sender { [self.delegate newTabViewWantsOmnibox:self]; }
- (void)libraryTapped:(id)sender { [self.delegate newTabViewWantsLibrary:self]; }

- (void)favoriteTapped:(UIButton *)sender {
    if (sender.tag < 0 || sender.tag >= (NSInteger)[self.favorites count]) return;
    NSString *url = [[self.favorites objectAtIndex:(NSUInteger)sender.tag] objectForKey:@"url"];
    if ([url length]) [self.delegate newTabView:self openURL:url];
}

@end
