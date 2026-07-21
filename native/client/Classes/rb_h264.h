/* rb_h264: pure C helpers for the H.264 lane — Annex-B NAL scanning, avcC
 * box construction, and Annex-B → 4-byte-length-prefixed (AVCC) conversion.
 * Platform-independent by design: compiles into the iOS app unchanged and
 * into a desktop ASan test binary (rb_h264_test.c). */
#ifndef RB_H264_H
#define RB_H264_H

#include <stddef.h>
#include <stdint.h>

typedef struct {
    const uint8_t *sps; /* points into the scanned AU; NULL if absent */
    size_t sps_len;
    const uint8_t *pps;
    size_t pps_len;
    int has_idr;  /* any NAL type 5 */
    int has_slice; /* any VCL NAL (type 1 or 5) */
} rb_au_info;

/* Scans one Annex-B access unit. Returns 0 on success, -1 on malformed
 * input (no start codes found). Pointers in info reference au's memory. */
int rb_au_scan(const uint8_t *au, size_t len, rb_au_info *info);

/* Builds an avcC (AVCDecoderConfigurationRecord) from raw SPS/PPS NAL
 * payloads. Returns the number of bytes written, or 0 if cap is too small
 * or inputs are invalid (sps_len < 4). Layout:
 *   01 | profile | compat | level | FF | E1 | spsLen16 | sps | 01 | ppsLen16 | pps */
size_t rb_avcc_build(const uint8_t *sps, size_t sps_len,
                     const uint8_t *pps, size_t pps_len,
                     uint8_t *out, size_t cap);

/* Converts an Annex-B AU into one contiguous AVCC buffer (4-byte big-endian
 * length before each NAL), dropping AUD (9), SPS (7) and PPS (8) NALs —
 * those live in the format description. Returns bytes written, or 0 if cap
 * is too small or no NALs survive. */
size_t rb_au_to_avcc(const uint8_t *au, size_t len, uint8_t *out, size_t cap);

#endif
