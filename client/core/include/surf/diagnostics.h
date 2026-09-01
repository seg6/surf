#ifndef SURF_DIAGNOSTICS_H
#define SURF_DIAGNOSTICS_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define SURF_CLOCK_SAMPLE_CAPACITY ((uint8_t)8)
#define SURF_DIAGNOSTICS_WINDOW_NS UINT64_C(2000000000)

typedef struct surf_clock_sync {
    uint64_t pending_client_send_ns;
    uint64_t last_probe_ns;
    uint64_t best_rtt_ns;
    int64_t server_minus_client_ns;
    uint64_t sample_rtt_ns[8];
    int64_t sample_offset_ns[8];
    uint8_t sample_count;
    uint8_t next_sample;
    uint8_t awaiting_reply;
    uint8_t synchronized;
} surf_clock_sync_t;

void surf_clock_sync_init(surf_clock_sync_t *sync);
void surf_clock_sync_reset(surf_clock_sync_t *sync);
int surf_clock_sync_probe(surf_clock_sync_t *sync, uint64_t now_ns,
                          uint64_t *out_client_send_ns);
int surf_clock_sync_consume(surf_clock_sync_t *sync, uint64_t client_send_ns,
                            uint64_t backend_receive_ns,
                            uint64_t backend_send_ns,
                            uint64_t client_receive_ns);
int surf_clock_sync_server_to_client(const surf_clock_sync_t *sync,
                                     uint64_t server_ns,
                                     uint64_t *out_client_ns);
size_t surf_clock_sync_sizeof(void);

typedef enum surf_diagnostics_health {
    SURF_DIAGNOSTICS_OFFLINE = 0,
    SURF_DIAGNOSTICS_SMOOTH,
    SURF_DIAGNOSTICS_DELAYED,
    SURF_DIAGNOSTICS_UNSTABLE
} surf_diagnostics_health_t;

typedef enum surf_diagnostics_reason {
    SURF_DIAGNOSTICS_REASON_NONE = 0,
    SURF_DIAGNOSTICS_REASON_OFFLINE,
    SURF_DIAGNOSTICS_REASON_DECODE_ERROR,
    SURF_DIAGNOSTICS_REASON_VIDEO_BACKLOG,
    SURF_DIAGNOSTICS_REASON_NETWORK,
    SURF_DIAGNOSTICS_REASON_FRAME_AGE,
    SURF_DIAGNOSTICS_REASON_DECODE_TIME,
    SURF_DIAGNOSTICS_REASON_AUDIO_UNDERRUN,
    SURF_DIAGNOSTICS_REASON_FRAME_DROPS,
    SURF_DIAGNOSTICS_REASON_PRESENTATION_GAP
} surf_diagnostics_reason_t;

typedef struct surf_diagnostics_sample {
    uint64_t now_ns;
    uint64_t video_packets;
    uint64_t decoded_frames;
    uint64_t presented_frames;
    uint64_t ingress_replaced;
    uint64_t output_replaced;
    uint64_t presentation_replaced;
    uint64_t sequence_gaps;
    uint64_t decode_errors;
    uint64_t audio_underruns;
    uint64_t backend_capture_to_encode_us;
    uint64_t backend_encode_to_write_us;
    uint64_t network_us;
    uint64_t decode_us;
    uint64_t upload_us;
    uint64_t frame_age_us;
    uint64_t rtt_us;
    uint64_t clock_uncertainty_us;
    uint64_t maximum_presentation_gap_us;
    uint32_t encoded_video_depth;
    uint32_t decoded_video_depth;
    uint32_t audio_depth;
    int timing_synchronized;
    int connected;
} surf_diagnostics_sample_t;

typedef struct surf_diagnostics_report {
    double window_ms;
    double video_fps;
    double decode_fps;
    double presentation_fps;
    double drop_percent;
    uint64_t dropped_frames;
    uint64_t sequence_gaps;
    uint64_t decode_errors;
    uint64_t audio_underruns;
    uint64_t backend_capture_to_encode_us;
    uint64_t backend_encode_to_write_us;
    uint64_t network_us;
    uint64_t decode_us;
    uint64_t upload_us;
    uint64_t frame_age_us;
    uint64_t rtt_us;
    uint64_t clock_uncertainty_us;
    uint64_t maximum_presentation_gap_us;
    uint32_t encoded_video_depth;
    uint32_t decoded_video_depth;
    uint32_t audio_depth;
    int timing_synchronized;
    surf_diagnostics_health_t health;
    surf_diagnostics_reason_t reason;
} surf_diagnostics_report_t;

typedef struct surf_diagnostics {
    surf_diagnostics_sample_t baseline;
    uint64_t maximum_presentation_gap_us;
    uint8_t initialized;
    uint8_t reserved[7];
} surf_diagnostics_t;

void surf_diagnostics_init(surf_diagnostics_t *diagnostics);
void surf_diagnostics_reset(surf_diagnostics_t *diagnostics);
int surf_diagnostics_update(surf_diagnostics_t *diagnostics,
                            const surf_diagnostics_sample_t *sample,
                            surf_diagnostics_report_t *out_report);
const char *surf_diagnostics_health_string(surf_diagnostics_health_t health);
const char *surf_diagnostics_reason_string(surf_diagnostics_reason_t reason);
size_t surf_diagnostics_sizeof(void);
size_t surf_diagnostics_sample_sizeof(void);
size_t surf_diagnostics_report_sizeof(void);

#ifdef __cplusplus
}
#endif

#endif
