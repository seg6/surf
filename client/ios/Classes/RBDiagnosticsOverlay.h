#import <UIKit/UIKit.h>
#import "RBDiagnostics.h"

typedef enum {
    RBDiagnosticsOverlayCompact,
    RBDiagnosticsOverlayExpanded
} RBDiagnosticsOverlayMode;

@class RBDiagnosticsOverlay;

@protocol RBDiagnosticsOverlayDelegate <NSObject>
- (void)diagnosticsOverlayDidChangeMode:(RBDiagnosticsOverlay *)overlay;
- (void)diagnosticsOverlayDidRequestClose:(RBDiagnosticsOverlay *)overlay;
@end

// A compact health instrument that expands to its content height without
// scrolling or changing the browser viewport.
@interface RBDiagnosticsOverlay : UIView
@property(nonatomic, assign) id<RBDiagnosticsOverlayDelegate> delegate;
@property(nonatomic, assign) RBDiagnosticsOverlayMode displayMode;
- (CGFloat)preferredExpandedHeightForWidth:(CGFloat)width;
- (void)updateWithSnapshot:(RBDiagnosticsSnapshot *)snapshot;
- (void)applyAppearance;
@end
