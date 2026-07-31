#import <UIKit/UIKit.h>

@class RBQRScannerController;

@protocol RBQRScannerDelegate <NSObject>
- (void)qrScanner:(RBQRScannerController *)scanner didScanValue:(NSString *)value;
- (void)qrScannerDidCancel:(RBQRScannerController *)scanner;
@end

@interface RBQRScannerController : UIViewController
@property(nonatomic, assign) id<RBQRScannerDelegate> delegate;
@end
