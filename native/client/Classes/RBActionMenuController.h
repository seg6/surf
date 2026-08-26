#import <UIKit/UIKit.h>

#import "RBTheme.h"

@interface RBActionMenuItem : NSObject
@property(nonatomic, copy) NSString *title;
@property(nonatomic, copy) NSString *action;
@property(nonatomic, assign) RBIcon icon;
@property(nonatomic, assign, getter=isEnabled) BOOL enabled;
+ (RBActionMenuItem *)itemWithTitle:(NSString *)title action:(NSString *)action icon:(RBIcon)icon;
@end

// Surf commands are navigation, not sharing. This dedicated surface prevents
// system share targets such as AirDrop from appearing behind the More button.
@interface RBActionMenuController : UIViewController
@property(nonatomic, copy) void (^onSelect)(NSString *action);
@property(nonatomic, copy) void (^onDismiss)(void);

- (id)initWithItems:(NSArray *)items phoneLayout:(BOOL)phoneLayout;
- (CGSize)preferredSize;
- (void)showAnimated:(BOOL)animated;
- (void)dismissAnimated:(BOOL)animated completion:(void (^)(void))completion;
@end
