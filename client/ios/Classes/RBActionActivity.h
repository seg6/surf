#import <UIKit/UIKit.h>

// A small parameterized UIActivity used to place Surf commands in the same
// native activity surface as Safari's actions on every supported OS version.
@interface RBActionActivity : UIActivity
- (id)initWithType:(NSString *)type
             title:(NSString *)title
             image:(UIImage *)image
           handler:(void (^)(void))handler;
@end
