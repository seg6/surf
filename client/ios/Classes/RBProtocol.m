#import "RBProtocol.h"

#include "surf/frame.h"

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

@implementation RBProtocol

+ (RBFrame *)frameFromData:(NSData *)data error:(NSString **)error {
    surf_frame_view_t parsed;
    surf_frame_result_t result = surf_frame_parse(
        (const uint8_t *)[data bytes], [data length], &parsed);
    if (result != SURF_FRAME_OK) {
        if (error) {
            *error = [NSString stringWithUTF8String:surf_frame_result_string(result)];
        }
        return nil;
    }

    RBFrame *frame = [[RBFrame alloc] init];
    frame.type = parsed.type;
    frame.flags = parsed.flags;
    frame.seq = parsed.sequence;
    frame.sourceSeq = parsed.source_sequence;
    frame.width = parsed.width;
    frame.height = parsed.height;
    frame.interactionID = parsed.interaction_id;
    frame.sourceReceiveNS = parsed.source_receive_ns;
    frame.encodeCompleteNS = parsed.encode_complete_ns;
    frame.socketWriteNS = parsed.socket_write_ns;
    frame.encoderGeneration = parsed.encoder_generation;
    frame.inputReceiveNS = parsed.input_receive_ns;
    frame.cdpAcceptedNS = parsed.cdp_accepted_ns;
    frame.profile = parsed.profile;
    frame.payload = [data subdataWithRange:NSMakeRange(
        SURF_FRAME_HEADER_BYTES, parsed.payload_length)];
    return frame;
}

@end
