#import "RBProtocol.h"

@implementation RBFrame
@end

@implementation RBFrameMetadata
+ (RBFrameMetadata *)metadataFromFrame:(RBFrame *)frame {
    RBFrameMetadata *m = [[RBFrameMetadata alloc] init];
    m.auSequence = frame.seq;
    m.sourceSequence = frame.sourceSeq;
    m.encoderGeneration = frame.encoderGeneration;
    m.interactionID = frame.interactionID;
    m.sourceReceiveNS = frame.sourceReceiveNS;
    m.encodeCompleteNS = frame.encodeCompleteNS;
    m.socketWriteNS = frame.socketWriteNS;
    m.inputReceiveNS = frame.inputReceiveNS;
    m.cdpAcceptedNS = frame.cdpAcceptedNS;
    m.profile = frame.profile;
    return m;
}
@end

static unsigned short RBReadBE16(const unsigned char *p) {
    return (unsigned short)(((unsigned short)p[0] << 8) | (unsigned short)p[1]);
}

static unsigned int RBReadBE32(const unsigned char *p) {
    return ((unsigned int)p[0] << 24) | ((unsigned int)p[1] << 16) | ((unsigned int)p[2] << 8) | (unsigned int)p[3];
}

static unsigned long long RBReadBE64(const unsigned char *p) {
    return ((unsigned long long)RBReadBE32(p) << 32) | RBReadBE32(p + 4);
}

@implementation RBProtocol

+ (RBFrame *)frameFromData:(NSData *)data error:(NSString **)error {
    static const NSUInteger RBFrameHeaderBytes = 84;
    if ([data length] < RBFrameHeaderBytes) {
        if (error) *error = @"short frame";
        return nil;
    }

    const unsigned char *b = (const unsigned char *)[data bytes];
    if (b[0] != 'R' || b[1] != 'B' || b[2] != 'R' || b[3] != '1') {
        if (error) *error = @"bad magic";
        return nil;
    }

    unsigned short headerLen = RBReadBE16(b + 6);
    unsigned int payloadLen = RBReadBE32(b + 20);
    if (headerLen != RBFrameHeaderBytes || (NSUInteger)headerLen > [data length]) {
        if (error) *error = @"bad header length";
        return nil;
    }
    if ((NSUInteger)headerLen + (NSUInteger)payloadLen != [data length]) {
        if (error) *error = @"bad payload length";
        return nil;
    }

    RBFrame *frame = [[RBFrame alloc] init];
    frame.type = b[4];
    frame.flags = b[5];
    frame.seq = RBReadBE32(b + 8);
    frame.sourceSeq = RBReadBE32(b + 12);
    frame.width = RBReadBE16(b + 16);
    frame.height = RBReadBE16(b + 18);
    frame.interactionID = RBReadBE64(b + 24);
    frame.sourceReceiveNS = RBReadBE64(b + 32);
    frame.encodeCompleteNS = RBReadBE64(b + 40);
    frame.socketWriteNS = RBReadBE64(b + 48);
    frame.encoderGeneration = RBReadBE32(b + 56);
    frame.inputReceiveNS = RBReadBE64(b + 64);
    frame.cdpAcceptedNS = RBReadBE64(b + 72);
    frame.profile = b[80];
    frame.payload = [data subdataWithRange:NSMakeRange(headerLen, payloadLen)];
    return frame;
}

@end
