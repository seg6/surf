#import "RBFindBar.h"
#import "RBTheme.h"

@interface RBFindBar () <UITextFieldDelegate>
@property(nonatomic, strong) RBGradientBar *background;
@property(nonatomic, strong) UITextField *field;
@property(nonatomic, strong) UIButton *prevButton;
@property(nonatomic, strong) UIButton *nextButton;
@property(nonatomic, strong) UIButton *doneButton;
@property(nonatomic, strong) UILabel *stateLabel;
@end

@implementation RBFindBar

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.background = [[RBGradientBar alloc] initWithFrame:self.bounds];
        self.background.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
        self.background.userInteractionEnabled = NO;
        [self addSubview:self.background];

        self.field = [[UITextField alloc] initWithFrame:CGRectZero];
        self.field.delegate = self;
        self.field.borderStyle = UITextBorderStyleRoundedRect;
        self.field.font = [RBTheme fontOfSize:14.0 bold:NO];
        self.field.placeholder = @"Find on page";
        self.field.autocorrectionType = UITextAutocorrectionTypeNo;
        self.field.autocapitalizationType = UITextAutocapitalizationTypeNone;
        self.field.returnKeyType = UIReturnKeySearch;
        self.field.clearButtonMode = UITextFieldViewModeWhileEditing;
        [self addSubview:self.field];

        self.prevButton = [RBTheme barButtonWithIcon:RBIconChevronUp target:self action:@selector(prevTapped:)];
        self.nextButton = [RBTheme barButtonWithIcon:RBIconChevronDown target:self action:@selector(nextTapped:)];
        [self addSubview:self.prevButton];
        [self addSubview:self.nextButton];

        self.stateLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.stateLabel.backgroundColor = [UIColor clearColor];
        self.stateLabel.font = [RBTheme fontOfSize:12.0 bold:NO];
        self.stateLabel.textColor = [UIColor colorWithRed:0.55 green:0.10 blue:0.10 alpha:1.0];
        [self addSubview:self.stateLabel];

        self.doneButton = [UIButton buttonWithType:UIButtonTypeCustom];
        [self.doneButton setTitle:@"Done" forState:UIControlStateNormal];
        [self.doneButton setTitleColor:[RBTheme iconColor] forState:UIControlStateNormal];
        [self.doneButton setTitleColor:[[RBTheme iconColor] colorWithAlphaComponent:0.4] forState:UIControlStateHighlighted];
        self.doneButton.titleLabel.font = [RBTheme fontOfSize:14.0 bold:YES];
        [self.doneButton addTarget:self action:@selector(doneTapped:) forControlEvents:UIControlEventTouchUpInside];
        [self addSubview:self.doneButton];
    }
    return self;
}

- (void)layoutSubviews {
    [super layoutSubviews];
    CGFloat w = self.bounds.size.width;
    CGFloat h = self.bounds.size.height;
    CGFloat y = (h - 31.0) / 2.0;
    self.field.frame = CGRectMake(10.0, y, MIN(320.0, w * 0.42), 31.0);
    CGFloat x = CGRectGetMaxX(self.field.frame) + 4.0;
    self.prevButton.frame = CGRectMake(x, 0.0, 44.0, h);
    self.nextButton.frame = CGRectMake(x + 44.0, 0.0, 44.0, h);
    self.stateLabel.frame = CGRectMake(x + 96.0, 0.0, 120.0, h);
    self.doneButton.frame = CGRectMake(w - 70.0, 0.0, 60.0, h);
}

- (void)focusField {
    [self.field becomeFirstResponder];
    [self.field selectAll:nil];
}

- (void)setFound:(BOOL)found {
    self.stateLabel.text = found ? @"" : @"Not found";
}

- (void)searchDirection:(NSInteger)direction {
    NSString *query = [self.field.text stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
    if (![query length]) return;
    self.stateLabel.text = @"";
    [self.delegate findBar:self search:query direction:direction];
}

- (void)prevTapped:(id)sender { [self searchDirection:-1]; }
- (void)nextTapped:(id)sender { [self searchDirection:1]; }

- (void)doneTapped:(id)sender {
    [self.field resignFirstResponder];
    [self.delegate findBarDone:self];
}

- (BOOL)textFieldShouldReturn:(UITextField *)textField {
    [self searchDirection:1];
    [textField resignFirstResponder];
    return NO;
}

@end
