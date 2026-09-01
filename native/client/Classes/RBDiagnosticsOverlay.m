#import "RBDiagnosticsOverlay.h"
#import "RBTheme.h"

#import <QuartzCore/QuartzCore.h>

static const NSUInteger kRBSignalSamples = 60;
static const CGFloat kRBInspectorHeaderHeight = 46.0;
static const CGFloat kRBInspectorContentHeight = 374.0;
static const CGFloat kRBInspectorLandscapeContentHeight = 190.0;

static UIColor *RBInspectorPaperColor(void) {
    return [[RBTheme surfaceColor] colorWithAlphaComponent:[RBTheme isDarkMode] ? 0.94 : 0.84];
}

static UIColor *RBInspectorRuleColor(void) {
    return [RBTheme mistColor];
}

static UIColor *RBInspectorMutedColor(void) {
    return [RBTheme secondaryTextColor];
}

@interface RBDiagnosticsSignalView : UIView {
    CGFloat _samples[kRBSignalSamples];
    NSUInteger _count;
    NSUInteger _cursor;
}
@property(nonatomic, strong) UIColor *lineColor;
@property(nonatomic, assign) CGFloat guideValue;
- (void)addSample:(CGFloat)value;
@end

@implementation RBDiagnosticsSignalView

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [UIColor clearColor];
        self.opaque = NO;
        self.contentMode = UIViewContentModeRedraw;
    }
    return self;
}

- (void)addSample:(CGFloat)value {
    _samples[_cursor] = MAX(0.0, value);
    _cursor = (_cursor + 1) % kRBSignalSamples;
    if (_count < kRBSignalSamples) _count++;
    [self setNeedsDisplay];
}

- (void)drawRect:(CGRect)rect {
    CGContextRef context = UIGraphicsGetCurrentContext();
    if (!context) return;

    CGRect plot = CGRectInset(rect, 1.0, 2.0);
    UIColor *rule = [RBInspectorRuleColor() colorWithAlphaComponent:0.72];
    CGContextSetStrokeColorWithColor(context, rule.CGColor);
    CGContextSetLineWidth(context, 1.0 / MAX(1.0, [UIScreen mainScreen].scale));
    for (NSUInteger i = 0; i < 3; i++) {
        CGFloat y = floorf(CGRectGetMinY(plot) + CGRectGetHeight(plot) * i / 2.0) + 0.5;
        CGContextMoveToPoint(context, CGRectGetMinX(plot), y);
        CGContextAddLineToPoint(context, CGRectGetMaxX(plot), y);
    }
    CGContextStrokePath(context);

    CGFloat maximum = MAX(180.0, self.guideValue);
    for (NSUInteger i = 0; i < _count; i++) {
        NSUInteger index = (_cursor + kRBSignalSamples - _count + i) % kRBSignalSamples;
        maximum = MAX(maximum, _samples[index] * 1.12);
    }

    if (self.guideValue > 0.0) {
        CGFloat guideY = CGRectGetMaxY(plot) -
            MIN(1.0, self.guideValue / maximum) * CGRectGetHeight(plot);
        CGFloat dash[] = { 3.0, 3.0 };
        CGContextSaveGState(context);
        CGContextSetStrokeColorWithColor(context,
            [[UIColor colorWithRed:0.82 green:0.52 blue:0.16 alpha:1.0]
                colorWithAlphaComponent:0.48].CGColor);
        CGContextSetLineDash(context, 0.0, dash, 2);
        CGContextMoveToPoint(context, CGRectGetMinX(plot), guideY);
        CGContextAddLineToPoint(context, CGRectGetMaxX(plot), guideY);
        CGContextStrokePath(context);
        CGContextRestoreGState(context);
    }

    if (_count < 2) return;
    CGContextSetStrokeColorWithColor(context,
        (self.lineColor ?: [RBTheme accentColor]).CGColor);
    CGContextSetLineWidth(context, 1.75);
    CGContextSetLineCap(context, kCGLineCapRound);
    CGContextSetLineJoin(context, kCGLineJoinRound);
    CGPoint latest = CGPointZero;
    for (NSUInteger i = 0; i < _count; i++) {
        NSUInteger index = (_cursor + kRBSignalSamples - _count + i) % kRBSignalSamples;
        CGFloat x = CGRectGetMaxX(plot) - CGRectGetWidth(plot) *
            (_count - 1 - i) / (CGFloat)(kRBSignalSamples - 1);
        CGFloat ratio = MIN(1.0, _samples[index] / maximum);
        CGFloat y = CGRectGetMaxY(plot) - ratio * CGRectGetHeight(plot);
        latest = CGPointMake(x, y);
        if (i == 0) CGContextMoveToPoint(context, x, y);
        else CGContextAddLineToPoint(context, x, y);
    }
    CGContextStrokePath(context);

    CGContextSetFillColorWithColor(context,
        (self.lineColor ?: [RBTheme accentColor]).CGColor);
    CGContextFillEllipseInRect(context, CGRectMake(latest.x - 2.5, latest.y - 2.5, 5.0, 5.0));
}

@end

@interface RBDiagnosticsReadout : UIView
@property(nonatomic, strong) UILabel *titleLabel;
@property(nonatomic, strong) UILabel *valueLabel;
- (id)initWithTitle:(NSString *)title;
- (void)applyAppearance;
@end

@implementation RBDiagnosticsReadout

- (id)initWithTitle:(NSString *)title {
    self = [super initWithFrame:CGRectZero];
    if (self) {
        self.backgroundColor = [UIColor clearColor];
        self.titleLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.titleLabel.backgroundColor = [UIColor clearColor];
        self.titleLabel.text = title;
        self.titleLabel.textColor = RBInspectorMutedColor();
        self.titleLabel.font = [RBTheme fontOfSize:10.0 bold:NO];
        [self addSubview:self.titleLabel];

        self.valueLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.valueLabel.backgroundColor = [UIColor clearColor];
        self.valueLabel.textColor = [RBTheme primaryTextColor];
        self.valueLabel.font = [RBTheme monospacedFontOfSize:19.0 bold:YES];
        self.valueLabel.adjustsFontSizeToFitWidth = YES;
        self.valueLabel.minimumFontSize = 13.0;
        [self addSubview:self.valueLabel];
        self.isAccessibilityElement = YES;
    }
    return self;
}

- (void)applyAppearance {
    self.titleLabel.textColor = RBInspectorMutedColor();
    self.valueLabel.textColor = [RBTheme primaryTextColor];
}

- (void)layoutSubviews {
    [super layoutSubviews];
    self.titleLabel.frame = CGRectMake(0.0, 3.0, self.bounds.size.width, 15.0);
    self.valueLabel.frame = CGRectMake(0.0, 18.0, self.bounds.size.width, 31.0);
}

@end

@interface RBDiagnosticsPipelineRow : UIView
@property(nonatomic, strong) UILabel *titleLabel;
@property(nonatomic, strong) UILabel *valueLabel;
@property(nonatomic, strong) UILabel *detailLabel;
@property(nonatomic, strong) UIView *separator;
- (id)initWithTitle:(NSString *)title;
- (void)applyAppearance;
@end

@implementation RBDiagnosticsPipelineRow

- (id)initWithTitle:(NSString *)title {
    self = [super initWithFrame:CGRectZero];
    if (self) {
        self.backgroundColor = [UIColor clearColor];
        self.titleLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.titleLabel.backgroundColor = [UIColor clearColor];
        self.titleLabel.text = title;
        self.titleLabel.textColor = [RBTheme primaryTextColor];
        self.titleLabel.font = [RBTheme fontOfSize:12.0 bold:YES];
        [self addSubview:self.titleLabel];

        self.valueLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.valueLabel.backgroundColor = [UIColor clearColor];
        self.valueLabel.textColor = [RBTheme primaryTextColor];
        self.valueLabel.font = [RBTheme monospacedFontOfSize:12.0 bold:YES];
        self.valueLabel.textAlignment = NSTextAlignmentRight;
        self.valueLabel.adjustsFontSizeToFitWidth = YES;
        self.valueLabel.minimumFontSize = 9.0;
        [self addSubview:self.valueLabel];

        self.detailLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.detailLabel.backgroundColor = [UIColor clearColor];
        self.detailLabel.textColor = RBInspectorMutedColor();
        self.detailLabel.font = [RBTheme monospacedFontOfSize:9.5 bold:NO];
        self.detailLabel.adjustsFontSizeToFitWidth = YES;
        self.detailLabel.minimumFontSize = 7.0;
        [self addSubview:self.detailLabel];

        self.separator = [[UIView alloc] initWithFrame:CGRectZero];
        self.separator.backgroundColor = RBInspectorRuleColor();
        [self addSubview:self.separator];
        self.isAccessibilityElement = YES;
    }
    return self;
}

- (void)applyAppearance {
    self.titleLabel.textColor = [RBTheme primaryTextColor];
    self.valueLabel.textColor = [RBTheme primaryTextColor];
    self.detailLabel.textColor = RBInspectorMutedColor();
    self.separator.backgroundColor = RBInspectorRuleColor();
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat width = self.bounds.size.width;
    CGFloat height = self.bounds.size.height;
    if (height < 36.0) {
        self.titleLabel.frame = CGRectMake(0.0, 0.0, width * 0.42, 14.0);
        self.valueLabel.frame = CGRectMake(width * 0.40, 0.0, width * 0.60, 14.0);
        self.detailLabel.frame = CGRectMake(0.0, 13.0, width, MAX(6.0, height - 14.0));
    } else {
        CGFloat groupY = MAX(5.0, floorf((height - 38.0) / 2.0));
        self.titleLabel.frame = CGRectMake(0.0, groupY, width * 0.44, 18.0);
        self.valueLabel.frame = CGRectMake(width * 0.42, groupY, width * 0.58, 18.0);
        self.detailLabel.frame = CGRectMake(0.0, groupY + 19.0, width, 16.0);
    }
    self.separator.frame = CGRectMake(0.0, self.bounds.size.height - 1.0, width, 1.0);
}

@end

@interface RBDiagnosticsOverlay ()
@property(nonatomic, strong) UIView *surfaceView;
@property(nonatomic, strong) UIView *healthRail;
@property(nonatomic, strong) UILabel *compactStatusLabel;
@property(nonatomic, strong) UILabel *compactMetricsLabel;
@property(nonatomic, strong) UILabel *compactVersionLabel;
@property(nonatomic, strong) UIImageView *compactInstrumentIcon;
@property(nonatomic, strong) UILabel *headingLabel;
@property(nonatomic, strong) UILabel *headerVersionLabel;
@property(nonatomic, strong) UIView *headerStatusDot;
@property(nonatomic, strong) UILabel *headerStatusLabel;
@property(nonatomic, strong) UIButton *collapseButton;
@property(nonatomic, strong) UIButton *closeButton;
@property(nonatomic, strong) UIView *headerRule;
@property(nonatomic, strong) UIView *contentView;
@property(nonatomic, strong) NSArray *readouts;
@property(nonatomic, strong) NSArray *readoutRules;
@property(nonatomic, strong) UILabel *signalTitleLabel;
@property(nonatomic, strong) UILabel *signalRangeLabel;
@property(nonatomic, strong) RBDiagnosticsSignalView *signalView;
@property(nonatomic, strong) UILabel *pipelineTitleLabel;
@property(nonatomic, strong) NSArray *pipelineRows;
@property(nonatomic, strong) UILabel *footerLabel;
@property(nonatomic, strong) RBDiagnosticsSnapshot *snapshot;
@end

@implementation RBDiagnosticsOverlay

- (UILabel *)labelWithSize:(CGFloat)size color:(UIColor *)color bold:(BOOL)bold {
    UILabel *label = [[UILabel alloc] initWithFrame:CGRectZero];
    label.backgroundColor = [UIColor clearColor];
    label.textColor = color;
    label.font = [RBTheme fontOfSize:size bold:bold];
    return label;
}

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [UIColor clearColor];
        self.opaque = NO;
        self.userInteractionEnabled = YES;
        self.accessibilityViewIsModal = NO;
        self.layer.shadowColor = [UIColor blackColor].CGColor;
        self.layer.shadowOpacity = 0.16;
        self.layer.shadowRadius = 7.0;
        self.layer.shadowOffset = CGSizeMake(0.0, 2.0);
        _displayMode = RBDiagnosticsOverlayCompact;

        self.surfaceView = [[UIView alloc] initWithFrame:CGRectZero];
        self.surfaceView.backgroundColor = RBInspectorPaperColor();
        self.surfaceView.layer.borderWidth = 1.0 / MAX(1.0, [UIScreen mainScreen].scale);
        self.surfaceView.layer.borderColor = RBInspectorRuleColor().CGColor;
        self.surfaceView.layer.masksToBounds = YES;
        [self addSubview:self.surfaceView];

        self.healthRail = [[UIView alloc] initWithFrame:CGRectZero];
        [self.surfaceView addSubview:self.healthRail];

        self.compactStatusLabel = [self labelWithSize:12.0
            color:[RBTheme primaryTextColor] bold:YES];
        [self.surfaceView addSubview:self.compactStatusLabel];
        self.compactMetricsLabel = [self labelWithSize:10.0
            color:RBInspectorMutedColor() bold:NO];
        self.compactMetricsLabel.font = [RBTheme monospacedFontOfSize:10.0 bold:NO];
        self.compactMetricsLabel.textAlignment = NSTextAlignmentRight;
        self.compactMetricsLabel.adjustsFontSizeToFitWidth = YES;
        self.compactMetricsLabel.minimumFontSize = 8.0;
        [self.surfaceView addSubview:self.compactMetricsLabel];
        self.compactVersionLabel = [self labelWithSize:9.0
            color:RBInspectorMutedColor() bold:NO];
        self.compactVersionLabel.font = [RBTheme monospacedFontOfSize:9.0 bold:NO];
        self.compactVersionLabel.textAlignment = NSTextAlignmentRight;
        self.compactVersionLabel.adjustsFontSizeToFitWidth = YES;
        self.compactVersionLabel.minimumFontSize = 7.0;
        [self.surfaceView addSubview:self.compactVersionLabel];
        self.compactInstrumentIcon = [[UIImageView alloc] initWithImage:
            [RBTheme icon:RBIconGauge size:15.0 color:[RBTheme accentColor]]];
        self.compactInstrumentIcon.contentMode = UIViewContentModeCenter;
        [self.surfaceView addSubview:self.compactInstrumentIcon];
        UITapGestureRecognizer *tap = [[UITapGestureRecognizer alloc]
            initWithTarget:self action:@selector(surfaceTapped:)];
        tap.cancelsTouchesInView = NO;
        [self addGestureRecognizer:tap];

        self.headingLabel = [self labelWithSize:16.0
            color:[RBTheme primaryTextColor] bold:YES];
        self.headingLabel.text = @"Performance";
        [self.surfaceView addSubview:self.headingLabel];
        self.headerVersionLabel = [self labelWithSize:9.0
            color:RBInspectorMutedColor() bold:NO];
        self.headerVersionLabel.font = [RBTheme monospacedFontOfSize:9.0 bold:NO];
        self.headerVersionLabel.adjustsFontSizeToFitWidth = YES;
        self.headerVersionLabel.minimumFontSize = 7.0;
        [self.surfaceView addSubview:self.headerVersionLabel];
        self.headerStatusDot = [[UIView alloc] initWithFrame:CGRectZero];
        self.headerStatusDot.layer.cornerRadius = 4.0;
        [self.surfaceView addSubview:self.headerStatusDot];
        self.headerStatusLabel = [self labelWithSize:11.0
            color:[RBTheme secondaryTextColor] bold:YES];
        self.headerStatusLabel.adjustsFontSizeToFitWidth = YES;
        self.headerStatusLabel.minimumFontSize = 9.0;
        [self.surfaceView addSubview:self.headerStatusLabel];

        self.collapseButton = [UIButton buttonWithType:UIButtonTypeCustom];
        [self.collapseButton setImage:[RBTheme icon:RBIconChevronDown size:16.0
                                                       color:[RBTheme secondaryTextColor]]
                              forState:UIControlStateNormal];
        [self.collapseButton setImage:[RBTheme icon:RBIconChevronDown size:16.0
                                                       color:[RBTheme deepTideColor]]
                              forState:UIControlStateHighlighted];
        self.collapseButton.accessibilityLabel = @"Collapse Performance Monitor";
        [self.collapseButton addTarget:self action:@selector(collapseTapped:)
                      forControlEvents:UIControlEventTouchUpInside];
        [self.surfaceView addSubview:self.collapseButton];
        self.closeButton = [UIButton buttonWithType:UIButtonTypeCustom];
        [self.closeButton setImage:[RBTheme icon:RBIconClose size:16.0
                                                    color:[RBTheme secondaryTextColor]]
                           forState:UIControlStateNormal];
        [self.closeButton setImage:[RBTheme icon:RBIconClose size:16.0
                                                    color:[RBTheme deepTideColor]]
                           forState:UIControlStateHighlighted];
        self.closeButton.accessibilityLabel = @"Hide Performance Monitor";
        [self.closeButton addTarget:self action:@selector(closeTapped:)
                   forControlEvents:UIControlEventTouchUpInside];
        [self.surfaceView addSubview:self.closeButton];
        self.headerRule = [[UIView alloc] initWithFrame:CGRectZero];
        self.headerRule.backgroundColor = RBInspectorRuleColor();
        [self.surfaceView addSubview:self.headerRule];

        self.contentView = [[UIView alloc] initWithFrame:CGRectZero];
        self.contentView.backgroundColor = [UIColor clearColor];
        [self.surfaceView addSubview:self.contentView];

        self.readouts = @[
            [[RBDiagnosticsReadout alloc] initWithTitle:@"Round trip"],
            [[RBDiagnosticsReadout alloc] initWithTitle:@"Response"],
            [[RBDiagnosticsReadout alloc] initWithTitle:@"Video"]
        ];
        for (UIView *readout in self.readouts) [self.contentView addSubview:readout];
        NSMutableArray *readoutRules = [NSMutableArray array];
        for (NSUInteger i = 0; i < 2; i++) {
            UIView *rule = [[UIView alloc] initWithFrame:CGRectZero];
            rule.backgroundColor = RBInspectorRuleColor();
            [self.contentView addSubview:rule];
            [readoutRules addObject:rule];
        }
        self.readoutRules = readoutRules;

        self.signalTitleLabel = [self labelWithSize:11.0
            color:[RBTheme primaryTextColor] bold:YES];
        self.signalTitleLabel.text = @"Round-trip history";
        [self.contentView addSubview:self.signalTitleLabel];
        self.signalRangeLabel = [self labelWithSize:9.0
            color:RBInspectorMutedColor() bold:NO];
        self.signalRangeLabel.text = @"recent · 120 ms guide";
        self.signalRangeLabel.textAlignment = NSTextAlignmentRight;
        [self.contentView addSubview:self.signalRangeLabel];
        self.signalView = [[RBDiagnosticsSignalView alloc] initWithFrame:CGRectZero];
        self.signalView.lineColor = [RBTheme accentColor];
        self.signalView.guideValue = 120.0;
        [self.contentView addSubview:self.signalView];

        self.pipelineTitleLabel = [self labelWithSize:11.0
            color:[RBTheme primaryTextColor] bold:YES];
        self.pipelineTitleLabel.text = @"Pipeline";
        [self.contentView addSubview:self.pipelineTitleLabel];
        self.pipelineRows = @[
            [[RBDiagnosticsPipelineRow alloc] initWithTitle:@"Video"],
            [[RBDiagnosticsPipelineRow alloc] initWithTitle:@"Renderer"],
            [[RBDiagnosticsPipelineRow alloc] initWithTitle:@"Continuity"],
            [[RBDiagnosticsPipelineRow alloc] initWithTitle:@"Audio"]
        ];
        for (UIView *row in self.pipelineRows) [self.contentView addSubview:row];

        self.footerLabel = [self labelWithSize:9.0 color:RBInspectorMutedColor() bold:NO];
        self.footerLabel.font = [RBTheme monospacedFontOfSize:9.0 bold:NO];
        self.footerLabel.numberOfLines = 2;
        self.footerLabel.adjustsFontSizeToFitWidth = YES;
        self.footerLabel.minimumFontSize = 7.0;
        [self.contentView addSubview:self.footerLabel];
        [self updateVisibility];
    }
    return self;
}

- (UIColor *)colorForHealth:(RBDiagnosticsHealth)health {
    if (health == RBDiagnosticsHealthSmooth) {
        return [UIColor colorWithRed:0.12 green:0.62 blue:0.45 alpha:1.0];
    }
    if (health == RBDiagnosticsHealthDelayed) {
        return [UIColor colorWithRed:0.82 green:0.51 blue:0.13 alpha:1.0];
    }
    if (health == RBDiagnosticsHealthUnstable) {
        return [UIColor colorWithRed:0.78 green:0.25 blue:0.25 alpha:1.0];
    }
    return [UIColor colorWithRed:0.45 green:0.52 blue:0.55 alpha:1.0];
}

- (void)setDisplayMode:(RBDiagnosticsOverlayMode)displayMode {
    if (_displayMode == displayMode) return;
    _displayMode = displayMode;
    [self updateVisibility];
    [self setNeedsLayout];
    if ([self.delegate respondsToSelector:@selector(diagnosticsOverlayDidChangeMode:)]) {
        [self.delegate diagnosticsOverlayDidChangeMode:self];
    }
}

- (void)applyAppearance {
    self.surfaceView.backgroundColor = RBInspectorPaperColor();
    self.surfaceView.layer.borderColor = RBInspectorRuleColor().CGColor;
    self.compactStatusLabel.textColor = [RBTheme primaryTextColor];
    self.compactMetricsLabel.textColor = RBInspectorMutedColor();
    self.compactVersionLabel.textColor = RBInspectorMutedColor();
    self.compactInstrumentIcon.image = [RBTheme icon:RBIconGauge size:15.0
                                                    color:[RBTheme accentColor]];
    self.headingLabel.textColor = [RBTheme primaryTextColor];
    self.headerVersionLabel.textColor = RBInspectorMutedColor();
    self.headerStatusLabel.textColor = [RBTheme secondaryTextColor];
    [self.collapseButton setImage:[RBTheme icon:RBIconChevronDown size:16.0
                                                   color:[RBTheme secondaryTextColor]]
                          forState:UIControlStateNormal];
    [self.collapseButton setImage:[RBTheme icon:RBIconChevronDown size:16.0
                                                   color:[RBTheme primaryTextColor]]
                          forState:UIControlStateHighlighted];
    [self.closeButton setImage:[RBTheme icon:RBIconClose size:16.0
                                                color:[RBTheme secondaryTextColor]]
                       forState:UIControlStateNormal];
    [self.closeButton setImage:[RBTheme icon:RBIconClose size:16.0
                                                color:[RBTheme primaryTextColor]]
                       forState:UIControlStateHighlighted];
    self.headerRule.backgroundColor = RBInspectorRuleColor();
    for (RBDiagnosticsReadout *readout in self.readouts) [readout applyAppearance];
    for (UIView *rule in self.readoutRules) rule.backgroundColor = RBInspectorRuleColor();
    self.signalTitleLabel.textColor = [RBTheme primaryTextColor];
    self.signalRangeLabel.textColor = RBInspectorMutedColor();
    self.signalView.lineColor = [RBTheme accentColor];
    [self.signalView setNeedsDisplay];
    self.pipelineTitleLabel.textColor = [RBTheme primaryTextColor];
    for (RBDiagnosticsPipelineRow *row in self.pipelineRows) [row applyAppearance];
    self.footerLabel.textColor = RBInspectorMutedColor();
}

- (void)updateVisibility {
    BOOL compact = self.displayMode == RBDiagnosticsOverlayCompact;
    self.compactStatusLabel.hidden = !compact;
    self.compactMetricsLabel.hidden = !compact;
    self.compactVersionLabel.hidden = !compact;
    self.compactInstrumentIcon.hidden = !compact;
    self.headingLabel.hidden = compact;
    self.headerVersionLabel.hidden = compact;
    self.headerStatusDot.hidden = compact;
    self.headerStatusLabel.hidden = compact;
    self.collapseButton.hidden = compact;
    self.closeButton.hidden = compact;
    self.headerRule.hidden = compact;
    self.contentView.hidden = compact;
    self.surfaceView.layer.cornerRadius = compact ? 6.0 : 8.0;
    self.layer.shadowOpacity = compact ? 0.12 : 0.16;
    self.accessibilityLabel = compact ? @"Performance Monitor" : @"Performance Details";
}

- (void)surfaceTapped:(UITapGestureRecognizer *)tap {
    if (tap.state == UIGestureRecognizerStateEnded &&
        self.displayMode == RBDiagnosticsOverlayCompact) {
        self.displayMode = RBDiagnosticsOverlayExpanded;
    }
}

- (void)collapseTapped:(id)sender { self.displayMode = RBDiagnosticsOverlayCompact; }

- (void)closeTapped:(id)sender {
    if ([self.delegate respondsToSelector:@selector(diagnosticsOverlayDidRequestClose:)]) {
        [self.delegate diagnosticsOverlayDidRequestClose:self];
    }
}

- (CGFloat)preferredExpandedHeightForWidth:(CGFloat)width {
    CGFloat usableWidth = MAX(1.0, width - 4.0 - 24.0);
    CGFloat contentHeight = usableWidth >= 390.0 ?
        kRBInspectorLandscapeContentHeight : kRBInspectorContentHeight;
    return kRBInspectorHeaderHeight + contentHeight;
}

- (void)layoutReadoutsAtY:(CGFloat)y
                   height:(CGFloat)height
                    inset:(CGFloat)inset
              usableWidth:(CGFloat)usableWidth {
    CGFloat columnWidth = floorf(usableWidth / 3.0);
    for (NSUInteger i = 0; i < [self.readouts count]; i++) {
        CGFloat columnX = inset + floorf(usableWidth * i / 3.0);
        CGFloat nextX = inset + floorf(usableWidth * (i + 1) / 3.0);
        CGFloat leading = i == 0 ? 0.0 : 9.0;
        CGFloat trailing = i + 1 == [self.readouts count] ? 0.0 : 9.0;
        ((UIView *)[self.readouts objectAtIndex:i]).frame =
            CGRectMake(columnX + leading, y,
                MAX(columnWidth - leading - trailing,
                    nextX - columnX - leading - trailing), height);
    }
    for (NSUInteger i = 0; i < [self.readoutRules count]; i++) {
        CGFloat x = inset + floorf(usableWidth * (i + 1) / 3.0);
        ((UIView *)[self.readoutRules objectAtIndex:i]).frame =
            CGRectMake(x, y + 3.0, 1.0, MAX(1.0, height - 7.0));
    }
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat width = self.bounds.size.width;
    CGFloat height = self.bounds.size.height;
    self.surfaceView.frame = self.bounds;
    self.layer.shadowPath = [UIBezierPath bezierPathWithRoundedRect:self.bounds
                                                      cornerRadius:self.surfaceView.layer.cornerRadius].CGPath;

    if (self.displayMode == RBDiagnosticsOverlayCompact) {
        self.healthRail.frame = CGRectMake(0.0, 0.0, 4.0, height);
        self.compactStatusLabel.frame = CGRectMake(13.0, 0.0, 59.0, height);
        self.compactInstrumentIcon.frame = CGRectMake(width - 31.0, 0.0, 26.0, height);
        CGFloat versionWidth = MIN(88.0, MAX(42.0, width * 0.31));
        CGFloat versionX = width - 36.0 - versionWidth;
        self.compactMetricsLabel.frame = CGRectMake(76.0, 0.0,
            MAX(18.0, versionX - 82.0), height);
        self.compactVersionLabel.frame = CGRectMake(versionX, 0.0, versionWidth, height);
        return;
    }

    CGFloat headerHeight = kRBInspectorHeaderHeight;
    self.healthRail.frame = CGRectMake(0.0, 0.0, 4.0, height);
    self.headingLabel.frame = CGRectMake(14.0, 2.0, 101.0, 25.0);
    self.headerVersionLabel.frame = CGRectMake(14.0, 24.0, 101.0, 16.0);
    self.headerStatusDot.frame = CGRectMake(121.0, 19.0, 8.0, 8.0);
    self.headerStatusLabel.frame = CGRectMake(135.0, 0.0,
        MAX(34.0, width - 231.0), headerHeight);
    self.collapseButton.frame = CGRectMake(width - 88.0, 1.0, 44.0, 44.0);
    self.closeButton.frame = CGRectMake(width - 44.0, 1.0, 44.0, 44.0);
    self.headerRule.frame = CGRectMake(4.0, headerHeight - 1.0, width - 4.0, 1.0);
    self.contentView.frame = CGRectMake(4.0, headerHeight,
        MAX(1.0, width - 4.0), MAX(1.0, height - headerHeight));

    CGFloat contentWidth = self.contentView.bounds.size.width;
    CGFloat contentHeight = self.contentView.bounds.size.height;
    CGFloat inset = 12.0;
    CGFloat usableWidth = MAX(1.0, contentWidth - inset * 2.0);
    BOOL shortLandscape = contentHeight < 285.0 && usableWidth >= 390.0;
    CGFloat readoutY = shortLandscape ? 2.0 : 7.0;
    CGFloat readoutHeight = shortLandscape ? 40.0 : 48.0;
    [self layoutReadoutsAtY:readoutY height:readoutHeight
                      inset:inset usableWidth:usableWidth];

    if (shortLandscape) {
        CGFloat gap = 17.0;
        CGFloat columnWidth = floorf((usableWidth - gap) / 2.0);
        CGFloat rightX = inset + columnWidth + gap;
        CGFloat sectionY = readoutY + readoutHeight + 4.0;
        self.signalTitleLabel.frame = CGRectMake(inset, sectionY, 116.0, 16.0);
        self.signalRangeLabel.frame = CGRectMake(inset + 116.0, sectionY,
            MAX(20.0, columnWidth - 116.0), 16.0);
        self.signalView.frame = CGRectMake(inset, sectionY + 18.0, columnWidth,
            MAX(20.0, contentHeight - sectionY - 20.0));
        self.pipelineTitleLabel.frame = CGRectMake(rightX, sectionY, columnWidth, 16.0);
        CGFloat rowY = sectionY + 18.0;
        CGFloat rowHeight = MAX(20.0, floorf((contentHeight - rowY - 1.0) / 4.0));
        for (NSUInteger i = 0; i < [self.pipelineRows count]; i++) {
            ((UIView *)[self.pipelineRows objectAtIndex:i]).frame =
                CGRectMake(rightX, rowY + i * rowHeight, columnWidth, rowHeight);
        }
        self.footerLabel.hidden = YES;
        self.footerLabel.frame = CGRectZero;
        return;
    }

    self.footerLabel.hidden = NO;
    CGFloat signalHeaderY = readoutY + readoutHeight + 10.0;
    self.signalTitleLabel.frame = CGRectMake(inset, signalHeaderY, 116.0, 18.0);
    self.signalRangeLabel.frame = CGRectMake(inset + 116.0, signalHeaderY,
        MAX(20.0, usableWidth - 116.0), 18.0);
    CGFloat signalY = signalHeaderY + 20.0;
    CGFloat signalHeight = MIN(120.0, MAX(42.0, contentHeight * 0.22));
    self.signalView.frame = CGRectMake(inset, signalY, usableWidth, signalHeight);

    CGFloat pipelineY = signalY + signalHeight + 9.0;
    self.pipelineTitleLabel.frame = CGRectMake(inset, pipelineY, usableWidth, 19.0);
    CGFloat rowY = pipelineY + 20.0;
    CGFloat footerY = MAX(rowY + 84.0, contentHeight - 31.0);
    CGFloat rowHeight = MAX(20.0, floorf((footerY - rowY - 4.0) / 4.0));
    for (NSUInteger i = 0; i < [self.pipelineRows count]; i++) {
        ((UIView *)[self.pipelineRows objectAtIndex:i]).frame =
            CGRectMake(inset, rowY + i * rowHeight, usableWidth, rowHeight);
    }
    self.footerLabel.frame = CGRectMake(inset, contentHeight - 30.0, usableWidth, 27.0);
}

- (NSString *)milliseconds:(double)value {
    return value > 0.0 ? [NSString stringWithFormat:@"%.0f ms", value] : @"—";
}

- (void)updateWithSnapshot:(RBDiagnosticsSnapshot *)snapshot {
    self.snapshot = snapshot;
    UIColor *healthColor = [self colorForHealth:snapshot.health];
    self.healthRail.backgroundColor = healthColor;
    self.headerStatusDot.backgroundColor = healthColor;
    self.headerStatusLabel.textColor = healthColor;
    self.compactStatusLabel.text = snapshot.healthLabel ?: @"Offline";
    self.headerStatusLabel.text = snapshot.healthLabel ?: @"Offline";
    NSString *clientVersion = [NSString stringWithFormat:@"Surf %@", snapshot.version ?: @"—"];
    self.compactVersionLabel.text = clientVersion;
    self.headerVersionLabel.text = clientVersion;
    self.compactMetricsLabel.text = [NSString stringWithFormat:@"%@ · %.0f fps",
        [self milliseconds:snapshot.RTTMS], snapshot.imageFPS];
    self.accessibilityValue = [NSString stringWithFormat:@"%@. %@",
        snapshot.healthLabel ?: @"Offline", snapshot.healthReason ?: @""];

    RBDiagnosticsReadout *roundTrip = [self.readouts objectAtIndex:0];
    RBDiagnosticsReadout *response = [self.readouts objectAtIndex:1];
    RBDiagnosticsReadout *video = [self.readouts objectAtIndex:2];
    roundTrip.valueLabel.text = [self milliseconds:snapshot.RTTMS];
    response.valueLabel.text = [self milliseconds:snapshot.latencyMS];
    video.valueLabel.text = [NSString stringWithFormat:@"%.0f fps", snapshot.imageFPS];
    for (RBDiagnosticsReadout *readout in self.readouts) {
        readout.accessibilityLabel = readout.titleLabel.text;
        readout.accessibilityValue = readout.valueLabel.text;
    }
    [self.signalView addSample:snapshot.RTTMS];

    BOOL systemRenderer = [snapshot.rendererMode isEqualToString:@"system"];
    NSArray *values = @[
        [NSString stringWithFormat:@"%d queued", snapshot.queuedAUs],
        [NSString stringWithFormat:@"%.1f ms", systemRenderer ? snapshot.rendererMS : snapshot.callbackMS],
        [NSString stringWithFormat:@"%llu gaps", snapshot.sequenceGaps],
        [NSString stringWithFormat:@"%d queued", snapshot.audioQueuedBuffers]
    ];
    NSArray *details = @[
        [NSString stringWithFormat:@"%.1f AU/s · %.0f ms old · %.0f ms max gap",
            snapshot.AURate, snapshot.ageMS, snapshot.maxGapMS],
        systemRenderer ?
            [NSString stringWithFormat:@"system · %llu waits · %llu recoveries · %llu failures",
                snapshot.rendererBackpressure, snapshot.rendererRecoveries, snapshot.rendererFailures] :
            [NSString stringWithFormat:@"legacy GL · %.1f submit · %.1f handoff · %llu errors",
                snapshot.submitMS, snapshot.handoffMS, snapshot.decodeErrors],
        [NSString stringWithFormat:@"%llu overwritten · %.0f ms largest pause",
            snapshot.overwrittenFrames, snapshot.maxGapMS],
        [NSString stringWithFormat:@"%llu drops · %llu underruns · %llu restarts",
            snapshot.audioDroppedPCM, snapshot.audioUnderruns, snapshot.audioRestarts]
    ];
    for (NSUInteger i = 0; i < [self.pipelineRows count]; i++) {
        RBDiagnosticsPipelineRow *row = [self.pipelineRows objectAtIndex:i];
        row.valueLabel.text = [values objectAtIndex:i];
        row.detailLabel.text = [details objectAtIndex:i];
        row.accessibilityLabel = row.titleLabel.text;
        row.accessibilityValue = [NSString stringWithFormat:@"%@. %@",
            row.valueLabel.text, row.detailLabel.text];
    }
    self.footerLabel.text = [NSString stringWithFormat:@"%@ · %@ · %@\ncompatibility %@",
        snapshot.server ?: @"Surf", snapshot.state ?: @"idle",
        snapshot.streamState ?: @"video idle", snapshot.compatibilityVersion ?: @"—"];
}

@end
