#import "RBProtocol.h"

@implementation RBFrame
@end

static unsigned short RBReadBE16(const unsigned char *p) {
    return (unsigned short)(((unsigned short)p[0] << 8) | (unsigned short)p[1]);
}

static unsigned int RBReadBE32(const unsigned char *p) {
    return ((unsigned int)p[0] << 24) | ((unsigned int)p[1] << 16) | ((unsigned int)p[2] << 8) | (unsigned int)p[3];
}

@implementation RBProtocol

+ (RBFrame *)frameFromData:(NSData *)data error:(NSString **)error {
    if ([data length] < 32) {
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
    if (headerLen < 32 || (NSUInteger)headerLen > [data length]) {
        if (error) *error = @"bad header length";
        return nil;
    }
    if (payloadLen == 0) {
        payloadLen = (unsigned int)([data length] - headerLen);
    }
    if ((NSUInteger)headerLen + (NSUInteger)payloadLen > [data length]) {
        if (error) *error = @"bad payload length";
        return nil;
    }

    RBFrame *frame = [[RBFrame alloc] init];
    frame.type = b[4];
    frame.flags = b[5];
    frame.seq = RBReadBE32(b + 8);
    frame.width = RBReadBE16(b + 16);
    frame.height = RBReadBE16(b + 18);
    frame.payload = [data subdataWithRange:NSMakeRange(headerLen, payloadLen)];
    return frame;
}

@end
