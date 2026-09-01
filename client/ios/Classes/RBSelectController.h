#import <UIKit/UIKit.h>

@class RBSelectController;

@protocol RBSelectControllerDelegate <NSObject>
- (void)selectController:(RBSelectController *)controller choseIndices:(NSArray *)indices;
- (void)selectControllerDidCancel:(RBSelectController *)controller;
@end

// Native replacement for Chromium's HTML <select> popup. Chromium renders the
// real popup outside the captured tab surface, so Surf presents the options on
// the touch device and returns only their indices to the server.
@interface RBSelectController : UITableViewController
@property(nonatomic, assign) id<RBSelectControllerDelegate> delegate;
@property(nonatomic, copy, readonly) NSString *requestID;
@property(nonatomic, assign, readonly) BOOL multiple;

- (id)initWithRequestID:(NSString *)requestID
                  title:(NSString *)title
                options:(NSArray *)options
               multiple:(BOOL)multiple;
- (CGSize)preferredPopoverSize;
@end
