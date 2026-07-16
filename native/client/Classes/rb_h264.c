#include "rb_h264.h"

#include <string.h>

/* Finds the next start code at or after *pos. On success returns 0 and sets
 * *nal_start (first byte after the start code) and *pos (offset of the start
 * code itself). Returns -1 when no further start code exists. */
static int next_start_code(const uint8_t *b, size_t len, size_t *pos, size_t *nal_start) {
    for (size_t i = *pos; i + 3 <= len; i++) {
        if (b[i] != 0 || b[i + 1] != 0) continue;
        if (b[i + 2] == 1) {
            *pos = i;
            *nal_start = i + 3;
            return 0;
        }
        if (b[i + 2] == 0 && i + 4 <= len && b[i + 3] == 1) {
            *pos = i;
            *nal_start = i + 4;
            return 0;
        }
    }
    return -1;
}

/* Invokes cb for every NAL (payload start + length, start code excluded). */
typedef void (*nal_cb)(const uint8_t *nal, size_t len, void *ctx);

static int for_each_nal(const uint8_t *au, size_t len, nal_cb cb, void *ctx) {
    size_t pos = 0, start = 0;
    if (next_start_code(au, len, &pos, &start) != 0) return -1;
    while (start < len) {
        size_t next_pos = start, next_start = 0;
        size_t end;
        if (next_start_code(au, len, &next_pos, &next_start) == 0) {
            end = next_pos;
        } else {
            end = len;
        }
        /* Trim trailing zero padding before the next start code. */
        while (end > start && au[end - 1] == 0 && next_start != 0) end--;
        if (end > start) cb(au + start, end - start, ctx);
        if (next_start == 0) break;
        pos = next_pos;
        start = next_start;
    }
    return 0;
}

static void scan_cb(const uint8_t *nal, size_t len, void *ctx) {
    rb_au_info *info = (rb_au_info *)ctx;
    if (len == 0) return;
    uint8_t type = nal[0] & 0x1F;
    switch (type) {
    case 7:
        info->sps = nal;
        info->sps_len = len;
        break;
    case 8:
        info->pps = nal;
        info->pps_len = len;
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
}

int rb_au_scan(const uint8_t *au, size_t len, rb_au_info *info) {
    memset(info, 0, sizeof(*info));
    if (!au || len < 4) return -1;
    return for_each_nal(au, len, scan_cb, info);
}

size_t rb_avcc_build(const uint8_t *sps, size_t sps_len,
                     const uint8_t *pps, size_t pps_len,
                     uint8_t *out, size_t cap) {
    if (!sps || sps_len < 4 || !pps || pps_len < 1 || sps_len > 0xFFFF || pps_len > 0xFFFF) return 0;
    size_t need = 5 + 1 + 2 + sps_len + 1 + 2 + pps_len;
    if (cap < need) return 0;
    uint8_t *p = out;
    *p++ = 0x01;
    *p++ = sps[1]; /* profile_idc */
    *p++ = sps[2]; /* profile_compatibility */
    *p++ = sps[3]; /* level_idc */
    *p++ = 0xFF;   /* 6 reserved bits + lengthSizeMinusOne=3 (4-byte lengths) */
    *p++ = 0xE1;   /* 3 reserved bits + numOfSPS=1 */
    *p++ = (uint8_t)(sps_len >> 8);
    *p++ = (uint8_t)(sps_len & 0xFF);
    memcpy(p, sps, sps_len);
    p += sps_len;
    *p++ = 0x01; /* numOfPPS */
    *p++ = (uint8_t)(pps_len >> 8);
    *p++ = (uint8_t)(pps_len & 0xFF);
    memcpy(p, pps, pps_len);
    p += pps_len;
    return (size_t)(p - out);
}

typedef struct {
    uint8_t *out;
    size_t cap;
    size_t used;
    int overflow;
} avcc_ctx;

static void avcc_cb(const uint8_t *nal, size_t len, void *vctx) {
    avcc_ctx *ctx = (avcc_ctx *)vctx;
    if (len == 0 || ctx->overflow) return;
    uint8_t type = nal[0] & 0x1F;
    if (type == 9 || type == 7 || type == 8) return; /* AUD/SPS/PPS dropped */
    if (ctx->used + 4 + len > ctx->cap) {
        ctx->overflow = 1;
        return;
    }
    ctx->out[ctx->used++] = (uint8_t)(len >> 24);
    ctx->out[ctx->used++] = (uint8_t)(len >> 16);
    ctx->out[ctx->used++] = (uint8_t)(len >> 8);
    ctx->out[ctx->used++] = (uint8_t)(len & 0xFF);
    memcpy(ctx->out + ctx->used, nal, len);
    ctx->used += len;
}

size_t rb_au_to_avcc(const uint8_t *au, size_t len, uint8_t *out, size_t cap) {
    if (!au || !out || len < 4) return 0;
    avcc_ctx ctx = {out, cap, 0, 0};
    if (for_each_nal(au, len, avcc_cb, &ctx) != 0) return 0;
    if (ctx.overflow) return 0;
    return ctx.used;
}
