#ifndef SURF_FRAME_H
#define SURF_FRAME_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define SURF_FRAME_HEADER_BYTES ((size_t)84)
#define SURF_FRAME_MAGIC_BYTES ((size_t)4)
#define SURF_FRAME_FLAG_IDR ((uint8_t)1)

typedef enum surf_frame_type {
    SURF_FRAME_TYPE_VIDEO = 3,
    SURF_FRAME_TYPE_AUDIO = 4
} surf_frame_type_t;

typedef enum surf_frame_result {
    SURF_FRAME_OK = 0,
    SURF_FRAME_ERROR_ARGUMENT,
    SURF_FRAME_ERROR_SHORT,
    SURF_FRAME_ERROR_MAGIC,
    SURF_FRAME_ERROR_HEADER_LENGTH,
    SURF_FRAME_ERROR_PAYLOAD_LENGTH
} surf_frame_result_t;

/* Borrowed view over one validated RBR1 binary message. `payload` points into
 * the caller-owned input buffer and remains valid only as long as that buffer. */
typedef struct surf_frame_view {
    uint8_t type;
    uint8_t flags;
    uint32_t sequence;
    uint32_t source_sequence;
    uint16_t width;
    uint16_t height;
    uint64_t interaction_id;
    uint64_t source_receive_ns;
    uint64_t encode_complete_ns;
    uint64_t socket_write_ns;
    uint32_t encoder_generation;
    uint64_t input_receive_ns;
    uint64_t cdp_accepted_ns;
    uint8_t profile;
    const uint8_t *payload;
    size_t payload_length;
} surf_frame_view_t;

surf_frame_result_t surf_frame_parse(const uint8_t *data, size_t length,
                                     surf_frame_view_t *out_frame);
const char *surf_frame_result_string(surf_frame_result_t result);

#ifdef __cplusplus
}
#endif

#endif
