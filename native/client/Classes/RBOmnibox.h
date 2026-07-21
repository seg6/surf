#import <UIKit/UIKit.h>

@class RBOmnibox;

@protocol RBOmniboxDelegate <NSObject>
- (void)omnibox:(RBOmnibox *)omnibox navigateTo:(NSString *)text;
- (void)omnibox:(RBOmnibox *)omnibox textChanged:(NSString *)text;
- (void)omniboxEditingBegan:(RBOmnibox *)omnibox;
- (void)omniboxEditingEnded:(RBOmnibox *)omnibox;
- (void)omniboxStarTapped:(RBOmnibox *)omnibox;
- (void)omniboxReloadOrStopTapped:(RBOmnibox *)omnibox;
@end

// Safari-style unified URL/search field: white rounded field with a blue
// loading fill behind the text, star at the left edge, reload/stop at the
// right edge.
@interface RBOmnibox : UIView
@property(nonatomic, assign) id<RBOmniboxDelegate> delegate;
@property(nonatomic, readonly) BOOL editing;

- (void)setURLText:(NSString *)url;
- (NSString *)currentText;
- (void)setLoading:(BOOL)loading;
- (void)setStarred:(BOOL)starred;
// TLS indicator (M2.5): "secure" shows a padlock, "insecure" a struck one,
// anything else hides it.
- (void)setSecurityState:(NSString *)state;
- (void)dismissKeyboard;
@end
