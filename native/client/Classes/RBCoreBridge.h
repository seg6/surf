#import <Foundation/Foundation.h>

typedef NS_ENUM(NSInteger, RBCoreEffect) {
    RBCoreEffectRequestLibrary = 1,
    RBCoreEffectShowKeyboard,
    RBCoreEffectHideKeyboard
};

// Objective-C ownership adapter for Surf's portable C99 browser model. String
// and collection properties are immutable copies and may be retained freely by
// UIKit. All methods are main-thread only.
@interface RBCoreBridge : NSObject
@property(nonatomic, copy, readonly) NSArray *tabs;
@property(nonatomic, copy, readonly) NSString *activeTitle;
@property(nonatomic, copy, readonly) NSString *currentURL;
@property(nonatomic, copy, readonly) NSString *security;
@property(nonatomic, copy, readonly) NSString *editableKind;
@property(nonatomic, copy, readonly) NSArray *editableRect;
@property(nonatomic, assign, readonly) unsigned long long revision;
@property(nonatomic, assign, readonly) long long activeTabID;
@property(nonatomic, assign, readonly) unsigned int awaitedSourceSequence;
@property(nonatomic, assign, readonly) BOOL hasActiveTab;
@property(nonatomic, assign, readonly) BOOL showStartPage;
@property(nonatomic, assign, readonly) BOOL loading;
@property(nonatomic, assign, readonly) BOOL canGoBack;
@property(nonatomic, assign, readonly) BOOL canGoForward;
@property(nonatomic, assign, readonly) BOOL starred;
@property(nonatomic, assign, readonly) BOOL fullscreen;
@property(nonatomic, assign, readonly) BOOL editable;
@property(nonatomic, assign, readonly) BOOL editableHasRect;
@property(nonatomic, assign, readonly) BOOL keyboardVisible;
@property(nonatomic, assign, readonly) BOOL awaitingPageFrame;

// Returns YES when the message type belongs to the migrated core surface.
- (BOOL)consumeControlMessage:(NSDictionary *)message error:(NSError **)error;
// Preferred socket path. The C99 codec validates the original wire bytes before
// Foundation or UIKit may act on them; `message` is retained only as a legacy
// rendering adapter while surfaces migrate to typed snapshots.
- (BOOL)consumeControlData:(NSData *)data message:(NSDictionary *)message
                     error:(NSError **)error;
- (void)notePresentedSourceSequence:(unsigned int)sourceSequence;
- (void)noteKeyboardVisible:(BOOL)visible;
- (void)reset;
- (BOOL)consumeEffect:(RBCoreEffect)effect;
@end
