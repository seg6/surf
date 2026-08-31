#import "RBActionMenuController.h"

#import <QuartzCore/QuartzCore.h>

@implementation RBActionMenuItem

+ (RBActionMenuItem *)itemWithTitle:(NSString *)title action:(NSString *)action icon:(RBIcon)icon {
    RBActionMenuItem *item = [[RBActionMenuItem alloc] init];
    item.title = title;
    item.action = action;
    item.icon = icon;
    item.enabled = YES;
    return item;
}

@end

@interface RBActionTile : UIControl
@property(nonatomic, strong) UIImageView *iconView;
@property(nonatomic, strong) UILabel *titleLabel;
@property(nonatomic, strong) RBActionMenuItem *item;
@property(nonatomic, assign) BOOL compactLayout;
@end

@implementation RBActionTile

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.layer.cornerRadius = 10.0;
        self.iconView = [[UIImageView alloc] initWithFrame:CGRectZero];
        self.iconView.contentMode = UIViewContentModeCenter;
        [self addSubview:self.iconView];
        self.titleLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.titleLabel.backgroundColor = [UIColor clearColor];
        self.titleLabel.textAlignment = NSTextAlignmentCenter;
        self.titleLabel.font = [RBTheme fontOfSize:12.0 bold:YES];
        self.titleLabel.textColor = [RBTheme primaryTextColor];
        self.titleLabel.numberOfLines = 2;
        [self addSubview:self.titleLabel];
    }
    return self;
}

- (void)setItem:(RBActionMenuItem *)item {
    _item = item;
    self.titleLabel.text = item.title;
    UIColor *color = item.enabled ? [RBTheme accentColor] : [[RBTheme slateColor] colorWithAlphaComponent:0.30];
    self.iconView.image = [RBTheme icon:item.icon size:(self.compactLayout ? 22.0 : 26.0)
                                      color:color];
    self.enabled = item.enabled;
    self.accessibilityLabel = item.title;
    self.isAccessibilityElement = YES;
}

- (void)setHighlighted:(BOOL)highlighted {
    [super setHighlighted:highlighted];
    self.backgroundColor = highlighted ? [[RBTheme accentColor] colorWithAlphaComponent:0.10]
                                       : [UIColor clearColor];
    self.transform = highlighted ? CGAffineTransformMakeScale(0.97, 0.97) : CGAffineTransformIdentity;
}

- (void)layoutSubviews {
    [super layoutSubviews];
    if (self.compactLayout) {
        self.iconView.frame = CGRectMake(0.0, 1.0, self.bounds.size.width, 24.0);
        self.titleLabel.frame = CGRectMake(4.0, 25.0, self.bounds.size.width - 8.0,
                                           MAX(20.0, self.bounds.size.height - 27.0));
        return;
    }
    self.iconView.frame = CGRectMake(0.0, 8.0, self.bounds.size.width, 32.0);
    self.titleLabel.frame = CGRectMake(4.0, 43.0, self.bounds.size.width - 8.0,
                                       MAX(18.0, self.bounds.size.height - 47.0));
}

@end

@interface RBActionMenuController ()
@property(nonatomic, strong) NSArray *items;
@property(nonatomic, assign) BOOL phoneLayout;
@property(nonatomic, strong) UIControl *backdrop;
@property(nonatomic, strong) UIView *card;
@property(nonatomic, strong) UIView *handle;
@property(nonatomic, strong) UILabel *heading;
@property(nonatomic, strong) NSArray *tiles;
@end

@implementation RBActionMenuController

- (id)initWithItems:(NSArray *)items phoneLayout:(BOOL)phoneLayout {
    self = [super initWithNibName:nil bundle:nil];
    if (self) {
        self.items = items ?: @[];
        self.phoneLayout = phoneLayout;
        self.contentSizeForViewInPopover = [self preferredSize];
    }
    return self;
}

- (CGSize)preferredSize { return CGSizeMake(330.0, self.phoneLayout ? 226.0 : 116.0); }

- (void)viewDidLoad {
    [super viewDidLoad];
    self.view.backgroundColor = self.phoneLayout ? [UIColor clearColor] : [RBTheme foamColor];

    self.backdrop = [[UIControl alloc] initWithFrame:CGRectZero];
    self.backdrop.backgroundColor = [UIColor colorWithWhite:0.02 alpha:0.34];
    self.backdrop.hidden = !self.phoneLayout;
    [self.backdrop addTarget:self action:@selector(backdropTapped:) forControlEvents:UIControlEventTouchUpInside];
    [self.view addSubview:self.backdrop];

    self.card = [[UIView alloc] initWithFrame:CGRectZero];
    self.card.backgroundColor = [RBTheme foamColor];
    self.card.layer.cornerRadius = self.phoneLayout ? 16.0 : 0.0;
    self.card.layer.borderWidth = self.phoneLayout ? 1.0 : 0.0;
    self.card.layer.borderColor = [[RBTheme mistColor] CGColor];
    if (self.phoneLayout) {
        self.card.layer.shadowColor = [[RBTheme deepTideColor] CGColor];
        self.card.layer.shadowOpacity = 0.20;
        self.card.layer.shadowRadius = 12.0;
        self.card.layer.shadowOffset = CGSizeMake(0.0, -3.0);
    }
    [self.view addSubview:self.card];

    self.handle = [[UIView alloc] initWithFrame:CGRectZero];
    self.handle.backgroundColor = [RBTheme mistColor];
    self.handle.layer.cornerRadius = 2.0;
    self.handle.hidden = !self.phoneLayout;
    [self.card addSubview:self.handle];

    self.heading = [[UILabel alloc] initWithFrame:CGRectZero];
    self.heading.backgroundColor = [UIColor clearColor];
    self.heading.text = @"Browser tools";
    self.heading.font = [RBTheme displayFontOfSize:14.0];
    self.heading.textColor = [RBTheme primaryTextColor];
    self.heading.textAlignment = NSTextAlignmentCenter;
    self.heading.hidden = !self.phoneLayout;
    [self.card addSubview:self.heading];

    NSMutableArray *tiles = [NSMutableArray array];
    for (RBActionMenuItem *item in self.items) {
        RBActionTile *tile = [[RBActionTile alloc] initWithFrame:CGRectZero];
        tile.compactLayout = !self.phoneLayout;
        tile.item = item;
        [tile addTarget:self action:@selector(tileTapped:) forControlEvents:UIControlEventTouchUpInside];
        [self.card addSubview:tile];
        [tiles addObject:tile];
    }
    self.tiles = tiles;
}

- (void)viewDidLayoutSubviews {
    [super viewDidLayoutSubviews];
    CGFloat w = self.view.bounds.size.width;
    CGFloat h = self.view.bounds.size.height;
    self.backdrop.frame = self.view.bounds;
    CGFloat cardH = MIN(self.phoneLayout ? 236.0 : 116.0, h);
    self.card.frame = self.phoneLayout ? CGRectMake(0.0, h - cardH, w, cardH) : self.view.bounds;
    CGFloat cardW = self.card.bounds.size.width;
    self.handle.frame = CGRectMake(floorf((cardW - 38.0) / 2.0), 8.0, 38.0, 4.0);
    self.heading.frame = self.phoneLayout
        ? CGRectMake(16.0, 17.0, cardW - 32.0, 24.0)
        : CGRectZero;
    CGFloat top = self.phoneLayout ? 45.0 : 4.0;
    CGFloat inset = self.phoneLayout ? 10.0 : 4.0;
    CGFloat gap = 4.0;
    CGFloat tileW = floorf((cardW - inset * 2.0 - gap * 2.0) / 3.0);
    CGFloat bottomInset = self.phoneLayout ? 10.0 : 4.0;
    CGFloat tileH = floorf((cardH - top - bottomInset - gap) / 2.0);
    for (NSUInteger i = 0; i < [self.tiles count]; i++) {
        CGFloat x = inset + (i % 3) * (tileW + gap);
        CGFloat y = top + (i / 3) * (tileH + gap);
        ((UIView *)[self.tiles objectAtIndex:i]).frame = CGRectMake(x, y, tileW, tileH);
    }
}

- (void)showAnimated:(BOOL)animated {
    if (!self.phoneLayout) return;
    [self.view layoutIfNeeded];
    self.backdrop.alpha = 0.0;
    self.card.transform = CGAffineTransformMakeTranslation(0.0, self.card.bounds.size.height);
    NSTimeInterval duration = animated ? 0.20 : 0.0;
    [UIView animateWithDuration:duration animations:^{
        self.backdrop.alpha = 1.0;
        self.card.transform = CGAffineTransformIdentity;
    }];
}

- (void)dismissAnimated:(BOOL)animated completion:(void (^)(void))completion {
    if (!self.phoneLayout) {
        if (completion) completion();
        return;
    }
    NSTimeInterval duration = animated ? 0.16 : 0.0;
    [UIView animateWithDuration:duration animations:^{
        self.backdrop.alpha = 0.0;
        self.card.transform = CGAffineTransformMakeTranslation(0.0, self.card.bounds.size.height);
    } completion:^(BOOL finished) {
        if (completion) completion();
    }];
}

- (void)tileTapped:(RBActionTile *)tile {
    if (!tile.item.enabled) return;
    if (self.onSelect) self.onSelect(tile.item.action);
}

- (void)backdropTapped:(id)sender {
    if (self.onDismiss) self.onDismiss();
}

@end
