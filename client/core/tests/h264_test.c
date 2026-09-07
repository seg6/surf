#include "surf/h264.h"

#include "test.h"

#include <stdint.h>
#include <string.h>

static size_t put_nal(uint8_t *output, int four_byte_start, uint8_t header,
                      const uint8_t *payload, size_t payload_length) {
    size_t used = 0;
    if (four_byte_start) {
        output[used++] = 0;
    }
    output[used++] = 0;
    output[used++] = 0;
    output[used++] = 1;
    output[used++] = header;
    memcpy(output + used, payload, payload_length);
    return used + payload_length;
}

static void test_access_unit(void) {
    uint8_t access_unit[512];
    size_t length = 0;
    static const uint8_t aud[] = {0x10};
    static const uint8_t sps[] = {0x64, 0x00, 0x1f, 0xac, 0xd9};
    static const uint8_t pps[] = {0xeb, 0xec, 0xb2};
    static const uint8_t sei[] = {0x05, 0x02, 0xaa, 0xbb, 0x80};
    static const uint8_t idr[] = {0x88, 0x84, 0x21, 0xff, 0x00, 0x77};
    surf_h264_au_info_t info;
    uint8_t config[256];
    uint8_t converted[512];
    size_t config_length;
    size_t converted_length;
    size_t first_length;
    size_t second_offset;
    size_t second_length;

    length += put_nal(access_unit + length, 1, 0x09, aud, sizeof(aud));
    length += put_nal(access_unit + length, 1, 0x67, sps, sizeof(sps));
    length += put_nal(access_unit + length, 1, 0x68, pps, sizeof(pps));
    length += put_nal(access_unit + length, 0, 0x06, sei, sizeof(sei));
    length += put_nal(access_unit + length, 0, 0x65, idr, sizeof(idr));

    SURF_CHECK(surf_h264_scan_annexb(access_unit, length, &info) == 0);
    SURF_CHECK(info.has_idr && info.has_slice);
    SURF_CHECK(info.sps != NULL && info.sps_length == 1 + sizeof(sps));
    SURF_CHECK(info.pps != NULL && info.pps_length == 1 + sizeof(pps));
    SURF_CHECK(info.avcc_length ==
               4 + (1 + sizeof(sei)) + 4 + (1 + sizeof(idr)));
    SURF_CHECK(memcmp(info.sps + 1, sps, sizeof(sps)) == 0);

    config_length = surf_h264_build_avcc_config(
        info.sps, info.sps_length, info.pps, info.pps_length, config,
        sizeof(config));
    SURF_CHECK(config_length == 11 + info.sps_length + info.pps_length);
    SURF_CHECK(config[0] == 1 && config[1] == 0x64 && config[2] == 0x00 &&
               config[3] == 0x1f && config[4] == 0xff && config[5] == 0xe1);
    SURF_CHECK((size_t)(((size_t)config[6] << 8) | config[7]) ==
               info.sps_length);
    SURF_CHECK(surf_h264_build_avcc_config(
                   info.sps, info.sps_length, info.pps, info.pps_length,
                   config, 8) == 0);

    converted_length = surf_h264_annexb_to_avcc(
        access_unit, length, converted, sizeof(converted));
    SURF_CHECK(converted_length == info.avcc_length);
    first_length = ((size_t)converted[0] << 24) |
                   ((size_t)converted[1] << 16) |
                   ((size_t)converted[2] << 8) | converted[3];
    SURF_CHECK(first_length == 1 + sizeof(sei));
    SURF_CHECK((converted[4] & 0x1fU) == 6);
    second_offset = 4 + first_length;
    second_length = ((size_t)converted[second_offset] << 24) |
                    ((size_t)converted[second_offset + 1] << 16) |
                    ((size_t)converted[second_offset + 2] << 8) |
                    converted[second_offset + 3];
    SURF_CHECK(second_length == 1 + sizeof(idr));
    SURF_CHECK((converted[second_offset + 4] & 0x1fU) == 5);
    SURF_CHECK(memcmp(converted + second_offset + 5, idr, sizeof(idr)) == 0);
    SURF_CHECK(surf_h264_annexb_to_avcc(access_unit, length, converted, 8) ==
               0);
}

static void test_invalid_input(void) {
    uint8_t junk[16] = {9, 9, 9, 9, 9, 9, 9, 9};
    uint8_t output[32];
    surf_h264_au_info_t info;
    SURF_CHECK(surf_h264_scan_annexb(junk, sizeof(junk), &info) == -1);
    SURF_CHECK(surf_h264_scan_annexb(NULL, 0, &info) == -1);
    SURF_CHECK(surf_h264_scan_annexb(junk, sizeof(junk), NULL) == -1);
    SURF_CHECK(surf_h264_annexb_to_avcc(junk, sizeof(junk), output,
                                        sizeof(output)) == 0);
    SURF_CHECK(surf_h264_build_avcc_config(NULL, 0, NULL, 0, output,
                                           sizeof(output)) == 0);
}

static void test_p_frame(void) {
    uint8_t access_unit[64];
    size_t length = 0;
    static const uint8_t aud[] = {0x10};
    static const uint8_t slice[] = {0x9a, 0x00, 0x11};
    uint8_t converted[64];
    surf_h264_au_info_t info;
    size_t converted_length;

    length += put_nal(access_unit + length, 1, 0x09, aud, sizeof(aud));
    length += put_nal(access_unit + length, 0, 0x41, slice, sizeof(slice));
    SURF_CHECK(surf_h264_scan_annexb(access_unit, length, &info) == 0);
    SURF_CHECK(!info.has_idr && info.has_slice);
    SURF_CHECK(info.sps == NULL && info.pps == NULL);
    SURF_CHECK(info.avcc_length == 4 + 1 + sizeof(slice));
    converted_length = surf_h264_annexb_to_avcc(
        access_unit, length, converted, sizeof(converted));
    SURF_CHECK(converted_length == 4 + 1 + sizeof(slice));
    SURF_CHECK((converted[4] & 0x1fU) == 1);
}

int main(void) {
    test_access_unit();
    test_invalid_input();
    test_p_frame();
    puts("surf_h264: all tests passed");
    return EXIT_SUCCESS;
}
