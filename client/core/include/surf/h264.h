#ifndef SURF_H264_H
#define SURF_H264_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct surf_h264_au_info {
    const uint8_t *sps; /* Borrowed from the scanned access unit. */
    size_t sps_length;
    const uint8_t *pps;
    size_t pps_length;
    size_t avcc_length;
    int has_idr;
    int has_slice;
} surf_h264_au_info_t;

/* Scans one Annex-B access unit. Returns 0 on success and -1 for invalid
 * arguments or an access unit containing no start code. */
int surf_h264_scan_annexb(const uint8_t *access_unit, size_t length,
                          surf_h264_au_info_t *out_info);

/* Builds an AVCDecoderConfigurationRecord from raw SPS/PPS NAL payloads.
 * Returns bytes written or zero when inputs/output capacity are invalid. */
size_t surf_h264_build_avcc_config(const uint8_t *sps, size_t sps_length,
                                   const uint8_t *pps, size_t pps_length,
                                   uint8_t *output, size_t capacity);

/* Converts an Annex-B access unit to four-byte-length-prefixed AVCC while
 * dropping AUD/SPS/PPS NALs. Returns bytes written or zero on failure. */
size_t surf_h264_annexb_to_avcc(const uint8_t *access_unit, size_t length,
                                uint8_t *output, size_t capacity);

#ifdef __cplusplus
}
#endif

#endif
