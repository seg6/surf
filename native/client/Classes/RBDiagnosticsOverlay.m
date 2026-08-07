#import "RBDiagnosticsOverlay.h"
#import "RBTheme.h"

#import <QuartzCore/QuartzCore.h>

static const NSUInteger kRBChartSamples = 60;

@interface RBDiagnosticsChart : UIView {
    CGFloat _samples[kRBChartSamples];
    NSUInteger _count;
    NSUInteger _cursor;
}
@property(nonatomic, strong) UIColor *lineColor;
@property(nonatomic, assign) CGFloat target;
- (void)addSample:(CGFloat)value;
@end

@implementation RBDiagnosticsChart

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
    _cursor = (_cursor + 1) % kRBChartSamples;
    if (_count < kRBChartSamples) _count++;
    [self setNeedsDisplay];
}

- (void)drawRect:(CGRect)rect {
    CGContextRef context = UIGraphicsGetCurrentContext();
    if (!context) return;
    CGContextSetLineWidth(context, 1.0);
    CGContextSetStrokeColorWithColor(context, [UIColor colorWithWhite:1.0 alpha:0.10].CGColor);
    for (NSUInteger row = 1; row < 3; row++) {
        CGFloat y = CGRectGetMinY(rect) + CGRectGetHeight(rect) * row / 3.0;
        CGContextMoveToPoint(context, CGRectGetMinX(rect), y);
        CGContextAddLineToPoint(context, CGRectGetMaxX(rect), y);
    }
    CGContextStrokePath(context);
    if (_count < 2) return;

    CGFloat maximum = MAX(1.0, self.target);
    for (NSUInteger i = 0; i < _count; i++) {
        NSUInteger index = (_cursor + kRBChartSamples - _count + i) % kRBChartSamples;
        maximum = MAX(maximum, _samples[index] * 1.10);
    }
    CGContextSetStrokeColorWithColor(context, (self.lineColor ?: [UIColor whiteColor]).CGColor);
    CGContextSetLineWidth(context, 1.6);
    CGContextSetLineJoin(context, kCGLineJoinRound);
    for (NSUInteger i = 0; i < _count; i++) {
        NSUInteger index = (_cursor + kRBChartSamples - _count + i) % kRBChartSamples;
        CGFloat x = CGRectGetMinX(rect) + CGRectGetWidth(rect) * i / (CGFloat)(kRBChartSamples - 1);
        CGFloat ratio = MIN(1.0, _samples[index] / maximum);
        CGFloat y = CGRectGetMaxY(rect) - ratio * (CGRectGetHeight(rect) - 2.0) - 1.0;
        if (i == 0) CGContextMoveToPoint(context, x, y);
        else CGContextAddLineToPoint(context, x, y);
    }
    CGContextStrokePath(context);
}

@end

@interface RBDiagnosticsOverlay ()
@property(nonatomic, strong) UILabel *statusLabel;
@property(nonatomic, strong) UILabel *fpsValue;
@property(nonatomic, strong) UILabel *latencyValue;
@property(nonatomic, strong) UILabel *rttValue;
@property(nonatomic, strong) UILabel *fpsCaption;
@property(nonatomic, strong) UILabel *latencyCaption;
@property(nonatomic, strong) UILabel *rttCaption;
@property(nonatomic, strong) NSArray *groupLabels;
@property(nonatomic, strong) NSArray *metricCaptions;
@property(nonatomic, strong) NSArray *metricValues;
@property(nonatomic, strong) NSArray *gridLines;
@property(nonatomic, strong) RBDiagnosticsChart *fpsChart;
@property(nonatomic, strong) RBDiagnosticsChart *latencyChart;
@property(nonatomic, strong) RBDiagnosticsChart *rttChart;
@property(nonatomic, assign) CFTimeInterval lastSampleAt;
@property(nonatomic, assign) unsigned long long lastAUs;
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
        self.backgroundColor = [UIColor colorWithWhite:0.055 alpha:0.94];
        self.layer.cornerRadius = 10.0;
        self.layer.borderWidth = 1.0;
        self.layer.borderColor = [UIColor colorWithWhite:1.0 alpha:0.14].CGColor;
        self.layer.shadowColor = [UIColor blackColor].CGColor;
        self.layer.shadowOpacity = 0.35;
        self.layer.shadowRadius = 8.0;
        self.layer.shadowOffset = CGSizeMake(0.0, 3.0);
        self.userInteractionEnabled = NO;

        self.statusLabel = [self labelWithSize:11.0 color:[UIColor colorWithWhite:0.72 alpha:1.0] bold:NO];
        self.statusLabel.textAlignment = NSTextAlignmentRight;
        self.statusLabel.adjustsFontSizeToFitWidth = YES;
        self.statusLabel.minimumFontSize = 8.0;
        [self addSubview:self.statusLabel];

        UIColor *primary = [UIColor colorWithWhite:0.98 alpha:1.0];
        UIColor *secondary = [UIColor colorWithWhite:0.58 alpha:1.0];
        self.fpsValue = [self labelWithSize:25.0 color:primary bold:YES];
        self.latencyValue = [self labelWithSize:25.0 color:primary bold:YES];
        self.rttValue = [self labelWithSize:25.0 color:primary bold:YES];
        self.fpsCaption = [self labelWithSize:10.0 color:secondary bold:YES];
        self.latencyCaption = [self labelWithSize:10.0 color:secondary bold:YES];
        self.rttCaption = [self labelWithSize:10.0 color:secondary bold:YES];
        self.fpsCaption.text = @"UNIQUE IMAGE FPS";
        self.latencyCaption.text = @"INPUT → SCREEN";
        self.rttCaption.text = @"NETWORK RTT";
        for (UIView *view in @[self.fpsValue, self.latencyValue, self.rttValue,
                               self.fpsCaption, self.latencyCaption, self.rttCaption]) {
            [self addSubview:view];
        }

        self.fpsChart = [[RBDiagnosticsChart alloc] initWithFrame:CGRectZero];
        self.fpsChart.lineColor = [UIColor colorWithRed:0.30 green:0.82 blue:0.52 alpha:1.0];
        self.fpsChart.target = 60.0;
        self.latencyChart = [[RBDiagnosticsChart alloc] initWithFrame:CGRectZero];
        self.latencyChart.lineColor = [UIColor colorWithRed:0.98 green:0.69 blue:0.25 alpha:1.0];
        self.latencyChart.target = 100.0;
        self.rttChart = [[RBDiagnosticsChart alloc] initWithFrame:CGRectZero];
        self.rttChart.lineColor = [UIColor colorWithRed:0.34 green:0.68 blue:0.96 alpha:1.0];
        self.rttChart.target = 100.0;
        [self addSubview:self.fpsChart];
        [self addSubview:self.latencyChart];
        [self addSubview:self.rttChart];

        NSMutableArray *groups = [NSMutableArray array];
        for (NSString *title in @[@"STREAM", @"VIDEO", @"DECODE", @"AUDIO"]) {
            UILabel *label = [self labelWithSize:9.0 color:[UIColor colorWithWhite:0.48 alpha:1.0] bold:YES];
            label.text = title;
            label.textAlignment = NSTextAlignmentRight;
            [self addSubview:label];
            [groups addObject:label];
        }
        self.groupLabels = groups;

        NSArray *metricNames = @[
            @"AU RATE", @"FRAME AGE", @"5S MAX GAP",
            @"QUEUE", @"AU LOSS", @"OVERWRITTEN",
            @"SUBMIT", @"CALLBACK", @"ERRORS / DROPS",
            @"QUEUE", @"DROPS", @"UNDERRUNS / RESTARTS"
        ];
        NSMutableArray *captions = [NSMutableArray array];
        NSMutableArray *values = [NSMutableArray array];
        for (NSString *name in metricNames) {
            UILabel *caption = [self labelWithSize:8.0 color:[UIColor colorWithWhite:0.48 alpha:1.0] bold:YES];
            caption.text = name;
            UILabel *value = [self labelWithSize:12.0 color:[UIColor colorWithWhite:0.90 alpha:1.0] bold:NO];
            caption.textAlignment = NSTextAlignmentCenter;
            caption.adjustsFontSizeToFitWidth = YES;
            caption.minimumFontSize = 6.5;
            value.textAlignment = NSTextAlignmentCenter;
            value.font = [UIFont fontWithName:@"Courier-Bold" size:11.0] ?: [RBTheme fontOfSize:11.0 bold:YES];
            value.adjustsFontSizeToFitWidth = YES;
            value.minimumFontSize = 8.0;
            [self addSubview:caption];
            [self addSubview:value];
            [captions addObject:caption];
            [values addObject:value];
        }
        self.metricCaptions = captions;
        self.metricValues = values;

        NSMutableArray *lines = [NSMutableArray array];
        for (NSUInteger i = 0; i < 7; i++) {
            UIView *line = [[UIView alloc] initWithFrame:CGRectZero];
            line.backgroundColor = [UIColor colorWithWhite:1.0 alpha:0.09];
            [self addSubview:line];
            [lines addObject:line];
        }
        self.gridLines = lines;
    }
    return self;
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat width = self.bounds.size.width;
    CGFloat inset = 14.0;
    CGFloat column = (width - inset * 2.0 - 16.0) / 3.0;
    self.statusLabel.frame = CGRectMake(inset, 8.0, width - inset * 2.0, 16.0);
    NSArray *values = @[self.fpsValue, self.latencyValue, self.rttValue];
    NSArray *captions = @[self.fpsCaption, self.latencyCaption, self.rttCaption];
    NSArray *charts = @[self.fpsChart, self.latencyChart, self.rttChart];
    for (NSUInteger i = 0; i < 3; i++) {
        CGFloat x = inset + i * (column + 8.0);
        ((UIView *)values[i]).frame = CGRectMake(x, 28.0, column, 31.0);
        ((UIView *)captions[i]).frame = CGRectMake(x, 58.0, column, 14.0);
        ((UIView *)charts[i]).frame = CGRectMake(x, 77.0, column, 45.0);
    }
    CGFloat rowsTop = 130.0;
    CGFloat tableBottom = self.bounds.size.height - 8.0;
    CGFloat rowHeight = (tableBottom - rowsTop) / 4.0;
    CGFloat groupWidth = 62.0;
    CGFloat metricWidth = (width - inset * 2.0 - groupWidth) / 3.0;
    for (NSUInteger row = 0; row < 4; row++) {
        CGFloat y = rowsTop + row * rowHeight;
        UILabel *group = [self.groupLabels objectAtIndex:row];
        group.frame = CGRectMake(inset, y, groupWidth - 9.0, rowHeight);
        for (NSUInteger columnIndex = 0; columnIndex < 3; columnIndex++) {
            NSUInteger index = row * 3 + columnIndex;
            CGFloat x = inset + groupWidth + columnIndex * metricWidth;
            UILabel *caption = [self.metricCaptions objectAtIndex:index];
            UILabel *value = [self.metricValues objectAtIndex:index];
            caption.frame = CGRectMake(x + 3.0, y + 2.0, metricWidth - 6.0, 10.0);
            value.frame = CGRectMake(x + 3.0, y + 13.0, metricWidth - 6.0, rowHeight - 14.0);
        }
    }
    for (NSUInteger row = 0; row < 3; row++) {
        UIView *line = [self.gridLines objectAtIndex:row];
        line.frame = CGRectMake(inset, rowsTop + (row + 1) * rowHeight,
                                width - inset * 2.0, 1.0);
    }
    for (NSUInteger columnIndex = 0; columnIndex < 4; columnIndex++) {
        UIView *line = [self.gridLines objectAtIndex:3 + columnIndex];
        CGFloat x = inset + groupWidth + columnIndex * metricWidth;
        line.frame = CGRectMake(x, rowsTop, 1.0, tableBottom - rowsTop);
    }
}

- (double)number:(NSDictionary *)snapshot key:(NSString *)key {
    return [[snapshot objectForKey:key] doubleValue];
}

- (void)updateWithSnapshot:(NSDictionary *)snapshot {
    CFTimeInterval now = CACurrentMediaTime();
    unsigned long long aus = [[snapshot objectForKey:@"aus"] unsignedLongLongValue];
    double aups = 0.0;
    if (self.lastSampleAt > 0.0 && now > self.lastSampleAt) {
        double dt = now - self.lastSampleAt;
        if (aus >= self.lastAUs) aups = (aus - self.lastAUs) / dt;
    }
    self.lastSampleAt = now;
    self.lastAUs = aus;

    double fps = [self number:snapshot key:@"imageFPS"];
    double latency = [self number:snapshot key:@"latency"];
    double rtt = [self number:snapshot key:@"rtt"];
    [self.fpsChart addSample:fps];
    [self.latencyChart addSample:latency];
    [self.rttChart addSample:rtt];
    self.fpsValue.text = [NSString stringWithFormat:@"%.1f", fps];
    self.latencyValue.text = latency > 0.0 ? [NSString stringWithFormat:@"%.0f ms", latency] : @"—";
    self.rttValue.text = rtt > 0.0 ? [NSString stringWithFormat:@"%.0f ms", rtt] : @"—";
    self.statusLabel.text = [NSString stringWithFormat:@"%@  •  %@  •  %@  •  triple-tap to close",
                             [snapshot objectForKey:@"server"] ?: @"Surf",
                             [snapshot objectForKey:@"state"] ?: @"idle",
                             [snapshot objectForKey:@"version"] ?: @""];

    NSArray *values = @[
        [NSString stringWithFormat:@"%.1f / sec", aups],
        [NSString stringWithFormat:@"%.0f ms", [self number:snapshot key:@"age"]],
        [NSString stringWithFormat:@"%.0f ms", [self number:snapshot key:@"maxGap"]],
        [NSString stringWithFormat:@"%d frames", [[snapshot objectForKey:@"queue"] intValue]],
        [NSString stringWithFormat:@"%llu", [[snapshot objectForKey:@"gaps"] unsignedLongLongValue]],
        [NSString stringWithFormat:@"%llu", [[snapshot objectForKey:@"overwritten"] unsignedLongLongValue]],
        [NSString stringWithFormat:@"%.1f ms", [self number:snapshot key:@"submitMS"]],
        [NSString stringWithFormat:@"%.1f ms", [self number:snapshot key:@"callbackMS"]],
        [NSString stringWithFormat:@"%llu / %llu",
         [[snapshot objectForKey:@"errors"] unsignedLongLongValue],
         [[snapshot objectForKey:@"drops"] unsignedLongLongValue]],
        [NSString stringWithFormat:@"%d buffers", [[snapshot objectForKey:@"audioQueue"] intValue]],
        [NSString stringWithFormat:@"%llu", [[snapshot objectForKey:@"audioDrops"] unsignedLongLongValue]],
        [NSString stringWithFormat:@"%llu / %llu",
         [[snapshot objectForKey:@"audioUnderruns"] unsignedLongLongValue],
         [[snapshot objectForKey:@"audioRestarts"] unsignedLongLongValue]]
    ];
    for (NSUInteger i = 0; i < [values count]; i++) {
        ((UILabel *)[self.metricValues objectAtIndex:i]).text = [values objectAtIndex:i];
    }
}

@end
