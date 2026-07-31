#import "RBActionActivity.h"

@interface RBActionActivity ()
@property(nonatomic, copy) NSString *rbType;
@property(nonatomic, copy) NSString *rbTitle;
@property(nonatomic, strong) UIImage *rbImage;
@property(nonatomic, copy) void (^handler)(void);
@end

@implementation RBActionActivity

- (id)initWithType:(NSString *)type
             title:(NSString *)title
             image:(UIImage *)image
           handler:(void (^)(void))handler {
    self = [super init];
    if (self) {
        self.rbType = type;
        self.rbTitle = title;
        self.rbImage = image;
        self.handler = handler;
    }
    return self;
}

- (NSString *)activityType { return self.rbType; }
- (NSString *)activityTitle { return self.rbTitle; }
- (UIImage *)activityImage { return self.rbImage; }
- (BOOL)canPerformWithActivityItems:(NSArray *)activityItems { return YES; }
- (void)prepareWithActivityItems:(NSArray *)activityItems {}

- (void)performActivity {
    if (self.handler) self.handler();
    [self activityDidFinish:YES];
}

@end
