#include "surf/h264.h"

#include <stdint.h>
#include <string.h>

static int surf_next_start_code(const uint8_t *bytes, size_t length,
                                size_t *position, size_t *nal_start) {
    size_t index;
    for (index = *position; index + 3 <= length; index++) {
        if (bytes[index] != 0 || bytes[index + 1] != 0) {
            continue;
        }
        if (bytes[index + 2] == 1) {
            *position = index;
            *nal_start = index + 3;
            return 0;
        }
        if (bytes[index + 2] == 0 && index + 4 <= length &&
            bytes[index + 3] == 1) {
            *position = index;
            *nal_start = index + 4;
            return 0;
        }
    }
    return -1;
}

typedef void (*surf_nal_callback_t)(const uint8_t *nal, size_t length,
                                    void *context);

static int surf_for_each_nal(const uint8_t *access_unit, size_t length,
                             surf_nal_callback_t callback, void *context) {
    size_t position = 0;
    size_t start = 0;
    if (surf_next_start_code(access_unit, length, &position, &start) != 0) {
        return -1;
    }
    while (start < length) {
        size_t next_position = start;
        size_t next_start = 0;
        size_t end;
        if (surf_next_start_code(access_unit, length, &next_position,
                                 &next_start) == 0) {
            end = next_position;
        } else {
            end = length;
        }
        while (end > start && access_unit[end - 1] == 0 && next_start != 0) {
            end--;
        }
        if (end > start) {
            callback(access_unit + start, end - start, context);
        }
        if (next_start == 0) {
            break;
        }
        position = next_position;
        start = next_start;
    }
    return 0;
}

static void surf_scan_nal(const uint8_t *nal, size_t length, void *context) {
    surf_h264_au_info_t *info = (surf_h264_au_info_t *)context;
    uint8_t type;
    if (length == 0) {
        return;
    }
    type = (uint8_t)(nal[0] & 0x1fU);
    switch (type) {
    case 7:
        info->sps = nal;
        info->sps_length = length;
        break;
    case 8:
        info->pps = nal;
        info->pps_length = length;
        break;
    case 5:
        info->has_idr = 1;
        info->has_slice = 1;
        break;
    case 1:
        info->has_slice = 1;
        break;
    default:
        break;
    }
    if (type != 9 && type != 7 && type != 8 &&
        length <= SIZE_MAX - 4 && info->avcc_length <= SIZE_MAX - 4 - length) {
        info->avcc_length += 4 + length;
    }
}

int surf_h264_scan_annexb(const uint8_t *access_unit, size_t length,
                          surf_h264_au_info_t *out_info) {
    if (out_info == NULL) {
        return -1;
    }
    memset(out_info, 0, sizeof(*out_info));
    if (access_unit == NULL || length < 4) {
        return -1;
    }
    return surf_for_each_nal(access_unit, length, surf_scan_nal, out_info);
}

size_t surf_h264_build_avcc_config(const uint8_t *sps, size_t sps_length,
                                   const uint8_t *pps, size_t pps_length,
                                   uint8_t *output, size_t capacity) {
    size_t required;
    uint8_t *cursor;
    if (sps == NULL || sps_length < 4 || pps == NULL || pps_length < 1 ||
        output == NULL || sps_length > UINT16_MAX || pps_length > UINT16_MAX) {
        return 0;
    }
    if (sps_length > SIZE_MAX - pps_length - 11) {
        return 0;
    }
    required = 11 + sps_length + pps_length;
    if (capacity < required) {
        return 0;
    }
    cursor = output;
    *cursor++ = 0x01;
    *cursor++ = sps[1];
    *cursor++ = sps[2];
    *cursor++ = sps[3];
    *cursor++ = 0xff;
    *cursor++ = 0xe1;
    *cursor++ = (uint8_t)(sps_length >> 8);
    *cursor++ = (uint8_t)(sps_length & 0xffU);
    memcpy(cursor, sps, sps_length);
    cursor += sps_length;
    *cursor++ = 0x01;
    *cursor++ = (uint8_t)(pps_length >> 8);
    *cursor++ = (uint8_t)(pps_length & 0xffU);
    memcpy(cursor, pps, pps_length);
    cursor += pps_length;
    return (size_t)(cursor - output);
}

typedef struct surf_avcc_context {
    uint8_t *output;
    size_t capacity;
    size_t used;
    int overflow;
} surf_avcc_context_t;

static void surf_write_avcc_nal(const uint8_t *nal, size_t length,
                                void *context) {
    surf_avcc_context_t *state = (surf_avcc_context_t *)context;
    uint8_t type;
    if (length == 0 || state->overflow) {
        return;
    }
    type = (uint8_t)(nal[0] & 0x1fU);
    if (type == 9 || type == 7 || type == 8) {
        return;
    }
    if (length > UINT32_MAX || length > SIZE_MAX - 4 ||
        state->used > state->capacity ||
        4 + length > state->capacity - state->used) {
        state->overflow = 1;
        return;
    }
    state->output[state->used++] = (uint8_t)(length >> 24);
    state->output[state->used++] = (uint8_t)(length >> 16);
    state->output[state->used++] = (uint8_t)(length >> 8);
    state->output[state->used++] = (uint8_t)(length & 0xffU);
    memcpy(state->output + state->used, nal, length);
    state->used += length;
}

size_t surf_h264_annexb_to_avcc(const uint8_t *access_unit, size_t length,
                                uint8_t *output, size_t capacity) {
    surf_avcc_context_t context;
    if (access_unit == NULL || output == NULL || length < 4) {
        return 0;
    }
    context.output = output;
    context.capacity = capacity;
    context.used = 0;
    context.overflow = 0;
    if (surf_for_each_nal(access_unit, length, surf_write_avcc_nal,
                          &context) != 0 || context.overflow) {
        return 0;
    }
    return context.used;
}
