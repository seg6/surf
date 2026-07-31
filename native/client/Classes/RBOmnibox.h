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

// Unified URL/search field with a loading fill behind the text and a
// reload/stop control at the right edge. The optional bookmark control is
// hidden in Surf's device chrome because Library already has a dedicated
// button.
@interface RBOmnibox : UIView
@property(nonatomic, assign) id<RBOmniboxDelegate> delegate;
@property(nonatomic, readonly) BOOL editing;
@property(nonatomic, assign) BOOL showsBookmarkButton;

- (void)setURLText:(NSString *)url;
- (NSString *)currentText;
- (void)setLoading:(BOOL)loading;
- (void)setStarred:(BOOL)starred;
// TLS indicator (M2.5): "secure" shows a padlock, "insecure" a struck one,
// anything else hides it.
- (void)setSecurityState:(NSString *)state;
- (void)focus;
- (void)dismissKeyboard;
@end
