#import <UIKit/UIKit.h>

// Compact on-device performance dashboard. It accepts cumulative counters and
// derives one-second rates internally so diagnostics do not perturb media
// delivery or presentation.
@interface RBDiagnosticsOverlay : UIView
- (void)updateWithSnapshot:(NSDictionary *)snapshot;
@end
