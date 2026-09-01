#include "surf/frame.h"

#include <string.h>

static uint16_t surf_read_be16(const uint8_t *value) {
    return (uint16_t)(((uint16_t)value[0] << 8) | (uint16_t)value[1]);
}

static uint32_t surf_read_be32(const uint8_t *value) {
    return ((uint32_t)value[0] << 24) |
           ((uint32_t)value[1] << 16) |
           ((uint32_t)value[2] << 8) |
           (uint32_t)value[3];
}

static uint64_t surf_read_be64(const uint8_t *value) {
    return ((uint64_t)surf_read_be32(value) << 32) |
           (uint64_t)surf_read_be32(value + 4);
}

surf_frame_result_t surf_frame_parse(const uint8_t *data, size_t length,
                                     surf_frame_view_t *out_frame) {
    uint16_t header_length;
    uint32_t encoded_payload_length;
    surf_frame_view_t frame;

    if (data == NULL || out_frame == NULL) {
        return SURF_FRAME_ERROR_ARGUMENT;
    }
    memset(out_frame, 0, sizeof(*out_frame));
    if (length < SURF_FRAME_HEADER_BYTES) {
        return SURF_FRAME_ERROR_SHORT;
    }
    if (memcmp(data, "RBR1", SURF_FRAME_MAGIC_BYTES) != 0) {
        return SURF_FRAME_ERROR_MAGIC;
    }

    header_length = surf_read_be16(data + 6);
    if ((size_t)header_length != SURF_FRAME_HEADER_BYTES ||
        (size_t)header_length > length) {
        return SURF_FRAME_ERROR_HEADER_LENGTH;
    }
    encoded_payload_length = surf_read_be32(data + 20);
    if ((size_t)encoded_payload_length != length - (size_t)header_length) {
        return SURF_FRAME_ERROR_PAYLOAD_LENGTH;
    }

    memset(&frame, 0, sizeof(frame));
    frame.type = data[4];
    frame.flags = data[5];
    frame.sequence = surf_read_be32(data + 8);
    frame.source_sequence = surf_read_be32(data + 12);
    frame.width = surf_read_be16(data + 16);
    frame.height = surf_read_be16(data + 18);
    frame.interaction_id = surf_read_be64(data + 24);
    frame.source_receive_ns = surf_read_be64(data + 32);
    frame.encode_complete_ns = surf_read_be64(data + 40);
    frame.socket_write_ns = surf_read_be64(data + 48);
    frame.encoder_generation = surf_read_be32(data + 56);
    frame.input_receive_ns = surf_read_be64(data + 64);
    frame.cdp_accepted_ns = surf_read_be64(data + 72);
    frame.profile = data[80];
    frame.payload = data + header_length;
    frame.payload_length = (size_t)encoded_payload_length;
    *out_frame = frame;
    return SURF_FRAME_OK;
}

const char *surf_frame_result_string(surf_frame_result_t result) {
    switch (result) {
    case SURF_FRAME_OK:
        return "ok";
    case SURF_FRAME_ERROR_ARGUMENT:
        return "invalid argument";
    case SURF_FRAME_ERROR_SHORT:
        return "short frame";
    case SURF_FRAME_ERROR_MAGIC:
        return "bad magic";
    case SURF_FRAME_ERROR_HEADER_LENGTH:
        return "bad header length";
    case SURF_FRAME_ERROR_PAYLOAD_LENGTH:
        return "bad payload length";
    default:
        return "unknown frame error";
    }
}

size_t surf_frame_sizeof_view(void) {
    return sizeof(surf_frame_view_t);
}
