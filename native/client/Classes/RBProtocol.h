#import <Foundation/Foundation.h>

@interface RBFrame : NSObject
@property(nonatomic, assign) unsigned char type;
@property(nonatomic, assign) unsigned char flags; // type 3: bit0 = AU contains an IDR
@property(nonatomic, assign) unsigned int seq;
@property(nonatomic, assign) unsigned short width;
@property(nonatomic, assign) unsigned short height;
@property(nonatomic, strong) NSData *payload;
@end

@interface RBProtocol : NSObject
+ (RBFrame *)frameFromData:(NSData *)data error:(NSString **)error;
@end
