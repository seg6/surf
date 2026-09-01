#import <Foundation/Foundation.h>

@interface RBFrame : NSObject
@property(nonatomic, assign) unsigned char type;
@property(nonatomic, assign) unsigned char flags; // type 3: bit0 = AU contains an IDR
@property(nonatomic, assign) unsigned int seq;
@property(nonatomic, assign) unsigned int sourceSeq;
@property(nonatomic, assign) unsigned short width;
@property(nonatomic, assign) unsigned short height;
@property(nonatomic, assign) unsigned long long interactionID;
@property(nonatomic, assign) unsigned long long sourceReceiveNS;
@property(nonatomic, assign) unsigned long long encodeCompleteNS;
@property(nonatomic, assign) unsigned long long socketWriteNS;
@property(nonatomic, assign) unsigned int encoderGeneration;
@property(nonatomic, assign) unsigned long long inputReceiveNS;
@property(nonatomic, assign) unsigned long long cdpAcceptedNS;
@property(nonatomic, assign) unsigned char profile;
@property(nonatomic, strong) NSData *payload;
@end

@interface RBFrameMetadata : NSObject
@property(nonatomic, assign) unsigned int auSequence;
@property(nonatomic, assign) unsigned int sourceSequence;
@property(nonatomic, assign) unsigned int encoderGeneration;
@property(nonatomic, assign) unsigned long long interactionID;
@property(nonatomic, assign) unsigned long long sourceReceiveNS;
@property(nonatomic, assign) unsigned long long encodeCompleteNS;
@property(nonatomic, assign) unsigned long long socketWriteNS;
@property(nonatomic, assign) unsigned long long inputReceiveNS;
@property(nonatomic, assign) unsigned long long cdpAcceptedNS;
@property(nonatomic, assign) unsigned char profile;
+ (RBFrameMetadata *)metadataFromFrame:(RBFrame *)frame;
@end

@interface RBProtocol : NSObject
+ (RBFrame *)frameFromData:(NSData *)data error:(NSString **)error;
@end
