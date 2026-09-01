#import <UIKit/UIKit.h>

// One tappable row in a popover list.
@interface RBListItem : NSObject
@property(nonatomic, copy) NSString *title;
@property(nonatomic, copy) NSString *subtitle;
@property(nonatomic, strong) id payload;
+ (RBListItem *)itemWithTitle:(NSString *)title subtitle:(NSString *)subtitle payload:(id)payload;
@end

// Generic table for UIPopoverController: sections of RBListItems, one
// callback. Used for the gear menu, history & bookmarks, and downloads.
@interface RBListPopover : UITableViewController
@property(nonatomic, copy) void (^onSelect)(RBListItem *item);

// sections: array of {@"title": NSString (may be empty), @"items": [RBListItem]}
- (id)initWithSections:(NSArray *)sections;
- (CGSize)preferredSize;
@end
