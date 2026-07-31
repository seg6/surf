#import <UIKit/UIKit.h>

@class RBAddServerController;

@protocol RBAddServerControllerDelegate <NSObject>
- (void)addServerController:(RBAddServerController *)controller
          didSubmitEndpoint:(NSString *)endpoint;
@end

@interface RBAddServerController : UITableViewController
@property(nonatomic, assign) id<RBAddServerControllerDelegate> delegate;
@property(nonatomic, readonly) NSString *endpoint;
- (id)initWithEndpoint:(NSString *)endpoint;
- (void)setBusy:(BOOL)busy message:(NSString *)message;
- (void)showError:(NSString *)message;
@end
