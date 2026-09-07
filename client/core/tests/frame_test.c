#include "surf/frame.h"

#include "test.h"

#include <stdint.h>
#include <string.h>

static void put_be16(uint8_t *output, uint16_t value) {
    output[0] = (uint8_t)(value >> 8);
    output[1] = (uint8_t)value;
}

static void put_be32(uint8_t *output, uint32_t value) {
    output[0] = (uint8_t)(value >> 24);
    output[1] = (uint8_t)(value >> 16);
    output[2] = (uint8_t)(value >> 8);
    output[3] = (uint8_t)value;
}

static void put_be64(uint8_t *output, uint64_t value) {
    put_be32(output, (uint32_t)(value >> 32));
    put_be32(output + 4, (uint32_t)value);
}

static size_t make_frame(uint8_t *output, size_t capacity) {
    static const uint8_t payload[] = {0x00, 0x00, 0x00, 0x01, 0x65, 0xaa};
    size_t length = SURF_FRAME_HEADER_BYTES + sizeof(payload);
    SURF_CHECK(capacity >= length);
    memset(output, 0, length);
    memcpy(output, "RBR1", 4);
    output[4] = SURF_FRAME_TYPE_VIDEO;
    output[5] = SURF_FRAME_FLAG_IDR;
    put_be16(output + 6, (uint16_t)SURF_FRAME_HEADER_BYTES);
    put_be32(output + 8, 0x01020304U);
    put_be32(output + 12, 0xa1a2a3a4U);
    put_be16(output + 16, 1920);
    put_be16(output + 18, 1080);
    put_be32(output + 20, (uint32_t)sizeof(payload));
    put_be64(output + 24, UINT64_C(0x0102030405060708));
    put_be64(output + 32, UINT64_C(11));
    put_be64(output + 40, UINT64_C(22));
    put_be64(output + 48, UINT64_C(33));
    put_be32(output + 56, 44);
    put_be64(output + 64, UINT64_C(55));
    put_be64(output + 72, UINT64_C(66));
    output[80] = 7;
    memcpy(output + SURF_FRAME_HEADER_BYTES, payload, sizeof(payload));
    return length;
}

static void test_valid_frame(void) {
    uint8_t bytes[128];
    size_t length = make_frame(bytes, sizeof(bytes));
    surf_frame_view_t frame;

    SURF_CHECK(surf_frame_parse(bytes, length, &frame) == SURF_FRAME_OK);
    SURF_CHECK(frame.type == SURF_FRAME_TYPE_VIDEO);
    SURF_CHECK(frame.flags == SURF_FRAME_FLAG_IDR);
    SURF_CHECK(frame.sequence == UINT32_C(0x01020304));
    SURF_CHECK(frame.source_sequence == UINT32_C(0xa1a2a3a4));
    SURF_CHECK(frame.width == 1920);
    SURF_CHECK(frame.height == 1080);
    SURF_CHECK(frame.interaction_id == UINT64_C(0x0102030405060708));
    SURF_CHECK(frame.source_receive_ns == 11);
    SURF_CHECK(frame.encode_complete_ns == 22);
    SURF_CHECK(frame.socket_write_ns == 33);
    SURF_CHECK(frame.encoder_generation == 44);
    SURF_CHECK(frame.input_receive_ns == 55);
    SURF_CHECK(frame.cdp_accepted_ns == 66);
    SURF_CHECK(frame.profile == 7);
    SURF_CHECK(frame.payload == bytes + SURF_FRAME_HEADER_BYTES);
    SURF_CHECK(frame.payload_length == 6);
    SURF_CHECK(frame.payload[4] == 0x65);
}

static void test_zero_payload(void) {
    uint8_t bytes[SURF_FRAME_HEADER_BYTES];
    surf_frame_view_t frame;
    memset(bytes, 0, sizeof(bytes));
    memcpy(bytes, "RBR1", 4);
    bytes[4] = SURF_FRAME_TYPE_AUDIO;
    put_be16(bytes + 6, (uint16_t)SURF_FRAME_HEADER_BYTES);
    SURF_CHECK(surf_frame_parse(bytes, sizeof(bytes), &frame) == SURF_FRAME_OK);
    SURF_CHECK(frame.payload_length == 0);
    SURF_CHECK(frame.payload == bytes + SURF_FRAME_HEADER_BYTES);
}

static void test_invalid_frames(void) {
    uint8_t bytes[128];
    uint8_t original[128];
    size_t length = make_frame(bytes, sizeof(bytes));
    surf_frame_view_t frame;
    memcpy(original, bytes, length);

    SURF_CHECK(surf_frame_parse(NULL, length, &frame) ==
               SURF_FRAME_ERROR_ARGUMENT);
    SURF_CHECK(surf_frame_parse(bytes, length, NULL) ==
               SURF_FRAME_ERROR_ARGUMENT);
    SURF_CHECK(surf_frame_parse(bytes, SURF_FRAME_HEADER_BYTES - 1, &frame) ==
               SURF_FRAME_ERROR_SHORT);

    bytes[0] = 'X';
    SURF_CHECK(surf_frame_parse(bytes, length, &frame) ==
               SURF_FRAME_ERROR_MAGIC);
    memcpy(bytes, original, length);

    put_be16(bytes + 6, 83);
    SURF_CHECK(surf_frame_parse(bytes, length, &frame) ==
               SURF_FRAME_ERROR_HEADER_LENGTH);
    memcpy(bytes, original, length);

    put_be16(bytes + 6, 0xffffU);
    SURF_CHECK(surf_frame_parse(bytes, length, &frame) ==
               SURF_FRAME_ERROR_HEADER_LENGTH);
    memcpy(bytes, original, length);

    put_be32(bytes + 20, 5);
    SURF_CHECK(surf_frame_parse(bytes, length, &frame) ==
               SURF_FRAME_ERROR_PAYLOAD_LENGTH);
    memcpy(bytes, original, length);

    put_be32(bytes + 20, UINT32_MAX);
    SURF_CHECK(surf_frame_parse(bytes, length, &frame) ==
               SURF_FRAME_ERROR_PAYLOAD_LENGTH);
}

static void test_result_strings(void) {
    SURF_CHECK(strcmp(surf_frame_result_string(SURF_FRAME_OK), "ok") == 0);
    SURF_CHECK(strcmp(surf_frame_result_string((surf_frame_result_t)999),
                      "unknown frame error") == 0);
}

int main(void) {
    test_valid_frame();
    test_zero_payload();
    test_invalid_frames();
    test_result_strings();
    puts("surf_frame: all tests passed");
    return EXIT_SUCCESS;
}
