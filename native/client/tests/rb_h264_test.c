/* Desktop test for rb_h264.c — run under ASan on the Mac:
 *   clang -fsanitize=address,undefined -I../Classes ../Classes/rb_h264.c rb_h264_test.c -o rb_h264_test && ./rb_h264_test
 */
#include "rb_h264.h"

#include <assert.h>
#include <stdio.h>
#include <string.h>

static size_t put_nal(uint8_t *out, int four, uint8_t hdr, const uint8_t *payload, size_t plen) {
    size_t n = 0;
    if (four) out[n++] = 0;
    out[n++] = 0;
    out[n++] = 0;
    out[n++] = 1;
    out[n++] = hdr;
    memcpy(out + n, payload, plen);
    return n + plen;
}

int main(void) {
    uint8_t au[512];
    size_t n = 0;
    const uint8_t aud_p[] = {0x10};
    const uint8_t sps_p[] = {0x64, 0x00, 0x1f, 0xac, 0xd9};  /* profile 0x64 hi, level 0x1f */
    const uint8_t pps_p[] = {0xeb, 0xec, 0xb2};
    const uint8_t sei_p[] = {0x05, 0x02, 0xaa, 0xbb, 0x80};
    const uint8_t idr_p[] = {0x88, 0x84, 0x21, 0xff, 0x00, 0x77}; /* fake slice bytes */

    n += put_nal(au + n, 1, 0x09, aud_p, sizeof aud_p);
    n += put_nal(au + n, 1, 0x67, sps_p, sizeof sps_p);
    n += put_nal(au + n, 1, 0x68, pps_p, sizeof pps_p);
    n += put_nal(au + n, 0, 0x06, sei_p, sizeof sei_p);
    n += put_nal(au + n, 0, 0x65, idr_p, sizeof idr_p);

    /* ---- scan ---- */
    rb_au_info info;
    assert(rb_au_scan(au, n, &info) == 0);
    assert(info.has_idr && info.has_slice);
    assert(info.sps && info.sps_len == 1 + sizeof sps_p && (info.sps[0] & 0x1F) == 7);
    assert(info.pps && info.pps_len == 1 + sizeof pps_p && (info.pps[0] & 0x1F) == 8);
    assert(memcmp(info.sps + 1, sps_p, sizeof sps_p) == 0);

    /* ---- avcC ---- */
    uint8_t avcc[256];
    size_t alen = rb_avcc_build(info.sps, info.sps_len, info.pps, info.pps_len, avcc, sizeof avcc);
    assert(alen == 5 + 1 + 2 + info.sps_len + 1 + 2 + info.pps_len);
    assert(avcc[0] == 0x01);
    assert(avcc[1] == 0x64 && avcc[2] == 0x00 && avcc[3] == 0x1f); /* profile/compat/level from SPS */
    assert(avcc[4] == 0xFF && avcc[5] == 0xE1);
    assert((size_t)((avcc[6] << 8) | avcc[7]) == info.sps_len);
    /* too-small cap */
    assert(rb_avcc_build(info.sps, info.sps_len, info.pps, info.pps_len, avcc, 8) == 0);

    /* ---- Annex-B → AVCC ---- */
    uint8_t conv[512];
    size_t clen = rb_au_to_avcc(au, n, conv, sizeof conv);
    /* survivors: SEI (6) and IDR (5); AUD/SPS/PPS dropped */
    size_t want = 4 + (1 + sizeof sei_p) + 4 + (1 + sizeof idr_p);
    assert(clen == want);
    size_t l1 = ((size_t)conv[0] << 24) | ((size_t)conv[1] << 16) | ((size_t)conv[2] << 8) | conv[3];
    assert(l1 == 1 + sizeof sei_p && (conv[4] & 0x1F) == 6);
    size_t off2 = 4 + l1;
    size_t l2 = ((size_t)conv[off2] << 24) | ((size_t)conv[off2 + 1] << 16) | ((size_t)conv[off2 + 2] << 8) | conv[off2 + 3];
    assert(l2 == 1 + sizeof idr_p && (conv[off2 + 4] & 0x1F) == 5);
    assert(memcmp(conv + off2 + 5, idr_p, sizeof idr_p) == 0);
    /* too-small cap */
    assert(rb_au_to_avcc(au, n, conv, 8) == 0);

    /* ---- garbage ---- */
    uint8_t junk[16] = {9, 9, 9, 9, 9, 9, 9, 9};
    assert(rb_au_scan(junk, sizeof junk, &info) == -1);
    assert(rb_au_to_avcc(junk, sizeof junk, conv, sizeof conv) == 0);
    assert(rb_au_scan(NULL, 0, &info) == -1);

    /* ---- P-frame AU (no SPS/PPS) ---- */
    uint8_t pau[64];
    size_t pn = 0;
    const uint8_t p_p[] = {0x9a, 0x00, 0x11};
    pn += put_nal(pau + pn, 1, 0x09, aud_p, sizeof aud_p);
    pn += put_nal(pau + pn, 0, 0x41, p_p, sizeof p_p);
    assert(rb_au_scan(pau, pn, &info) == 0);
    assert(!info.has_idr && info.has_slice && !info.sps && !info.pps);
    clen = rb_au_to_avcc(pau, pn, conv, sizeof conv);
    assert(clen == 4 + 1 + sizeof p_p && (conv[4] & 0x1F) == 1);

    printf("rb_h264: all tests passed\n");
    return 0;
}
