#include "surf/diagnostics.h"

#include <limits.h>
#include <string.h>

#define SURF_CLOCK_REPLY_TIMEOUT_NS UINT64_C(10000000000)
#define SURF_CLOCK_STARTUP_INTERVAL_NS UINT64_C(1000000000)
#define SURF_CLOCK_STEADY_INTERVAL_NS UINT64_C(30000000000)

static uint64_t surf_delta(uint64_t current, uint64_t previous) {
    return current >= previous ? current - previous : current;
}

static uint64_t surf_add_saturated(uint64_t left, uint64_t right) {
    return UINT64_MAX - left < right ? UINT64_MAX : left + right;
}

static int64_t surf_signed_difference(uint64_t left, uint64_t right) {
    uint64_t magnitude;
    if (left >= right) {
        magnitude = left - right;
        return magnitude > (uint64_t)INT64_MAX ? INT64_MAX : (int64_t)magnitude;
    }
    magnitude = right - left;
    return magnitude > (uint64_t)INT64_MAX ? INT64_MIN : -(int64_t)magnitude;
}

static int64_t surf_signed_average(int64_t left, int64_t right) {
    return left / 2 + right / 2 + (left % 2 + right % 2) / 2;
}

void surf_clock_sync_init(surf_clock_sync_t *sync) {
    surf_clock_sync_reset(sync);
}

void surf_clock_sync_reset(surf_clock_sync_t *sync) {
    if (sync != NULL) {
        memset(sync, 0, sizeof(*sync));
    }
}

int surf_clock_sync_probe(surf_clock_sync_t *sync, uint64_t now_ns,
                          uint64_t *out_client_send_ns) {
    uint64_t interval;
    if (sync == NULL || out_client_send_ns == NULL) {
        return -1;
    }
    if (sync->awaiting_reply != 0u) {
        if (now_ns >= sync->pending_client_send_ns &&
            now_ns - sync->pending_client_send_ns <=
                SURF_CLOCK_REPLY_TIMEOUT_NS) {
            return 0;
        }
        sync->awaiting_reply = 0u;
    }
    interval = sync->sample_count < 4u ? SURF_CLOCK_STARTUP_INTERVAL_NS
                                      : SURF_CLOCK_STEADY_INTERVAL_NS;
    if (sync->last_probe_ns != 0u && now_ns >= sync->last_probe_ns &&
        now_ns - sync->last_probe_ns < interval) {
        return 0;
    }
    sync->last_probe_ns = now_ns;
    sync->pending_client_send_ns = now_ns;
    sync->awaiting_reply = 1u;
    *out_client_send_ns = now_ns;
    return 1;
}

int surf_clock_sync_consume(surf_clock_sync_t *sync, uint64_t client_send_ns,
                            uint64_t backend_receive_ns,
                            uint64_t backend_send_ns,
                            uint64_t client_receive_ns) {
    uint64_t elapsed;
    uint64_t backend_elapsed;
    uint64_t rtt;
    int64_t offset;
    uint8_t index;
    uint8_t best;
    if (sync == NULL || sync->awaiting_reply == 0u ||
        client_send_ns != sync->pending_client_send_ns ||
        client_receive_ns < client_send_ns ||
        backend_send_ns < backend_receive_ns) {
        return 0;
    }
    sync->awaiting_reply = 0u;
    elapsed = client_receive_ns - client_send_ns;
    backend_elapsed = backend_send_ns - backend_receive_ns;
    rtt = elapsed >= backend_elapsed ? elapsed - backend_elapsed : 0u;
    offset = surf_signed_average(
        surf_signed_difference(backend_receive_ns, client_send_ns),
        surf_signed_difference(backend_send_ns, client_receive_ns));

    index = sync->next_sample;
    sync->sample_rtt_ns[index] = rtt;
    sync->sample_offset_ns[index] = offset;
    if (sync->sample_count < SURF_CLOCK_SAMPLE_CAPACITY) {
        sync->sample_count = (uint8_t)(sync->sample_count + 1u);
    }
    sync->next_sample =
        (uint8_t)((index + 1u) % SURF_CLOCK_SAMPLE_CAPACITY);

    best = 0u;
    for (index = 1u; index < sync->sample_count; index++) {
        if (sync->sample_rtt_ns[index] < sync->sample_rtt_ns[best]) {
            best = index;
        }
    }
    sync->best_rtt_ns = sync->sample_rtt_ns[best];
    sync->server_minus_client_ns = sync->sample_offset_ns[best];
    sync->synchronized = sync->sample_count >= 2u ? 1u : 0u;
    return 1;
}

int surf_clock_sync_server_to_client(const surf_clock_sync_t *sync,
                                     uint64_t server_ns,
                                     uint64_t *out_client_ns) {
    uint64_t magnitude;
    if (sync == NULL || out_client_ns == NULL || sync->synchronized == 0u) {
        return 0;
    }
    if (sync->server_minus_client_ns >= 0) {
        magnitude = (uint64_t)sync->server_minus_client_ns;
        *out_client_ns = server_ns >= magnitude ? server_ns - magnitude : 0u;
    } else {
        magnitude = sync->server_minus_client_ns == INT64_MIN
                        ? (uint64_t)INT64_MAX + 1u
                        : (uint64_t)(-sync->server_minus_client_ns);
        *out_client_ns = UINT64_MAX - server_ns < magnitude
                             ? UINT64_MAX
                             : server_ns + magnitude;
    }
    return 1;
}

size_t surf_clock_sync_sizeof(void) { return sizeof(surf_clock_sync_t); }

void surf_diagnostics_init(surf_diagnostics_t *diagnostics) {
    surf_diagnostics_reset(diagnostics);
}

void surf_diagnostics_reset(surf_diagnostics_t *diagnostics) {
    if (diagnostics != NULL) {
        memset(diagnostics, 0, sizeof(*diagnostics));
    }
}

static void surf_diagnostics_copy_instantaneous(
    const surf_diagnostics_sample_t *sample,
    surf_diagnostics_report_t *report) {
    report->backend_capture_to_encode_us = sample->backend_capture_to_encode_us;
    report->backend_encode_to_write_us = sample->backend_encode_to_write_us;
    report->network_us = sample->network_us;
    report->decode_us = sample->decode_us;
    report->upload_us = sample->upload_us;
    report->frame_age_us = sample->frame_age_us;
    report->rtt_us = sample->rtt_us;
    report->clock_uncertainty_us = sample->clock_uncertainty_us;
    report->maximum_presentation_gap_us = sample->maximum_presentation_gap_us;
    report->encoded_video_depth = sample->encoded_video_depth;
    report->decoded_video_depth = sample->decoded_video_depth;
    report->audio_depth = sample->audio_depth;
    report->timing_synchronized = sample->timing_synchronized;
}

static void surf_diagnostics_classify(surf_diagnostics_report_t *report,
                                      int connected) {
    uint32_t video_depth =
        UINT32_MAX - report->encoded_video_depth < report->decoded_video_depth
            ? UINT32_MAX
            : report->encoded_video_depth + report->decoded_video_depth;
    if (connected == 0) {
        report->health = SURF_DIAGNOSTICS_OFFLINE;
        report->reason = SURF_DIAGNOSTICS_REASON_OFFLINE;
    } else if (report->decode_errors > 0u) {
        report->health = SURF_DIAGNOSTICS_UNSTABLE;
        report->reason = SURF_DIAGNOSTICS_REASON_DECODE_ERROR;
    } else if (video_depth >= 3u) {
        report->health = SURF_DIAGNOSTICS_UNSTABLE;
        report->reason = SURF_DIAGNOSTICS_REASON_VIDEO_BACKLOG;
    } else if (report->rtt_us >= UINT64_C(250000) ||
               (report->timing_synchronized != 0 &&
                report->network_us >= UINT64_C(250000))) {
        report->health = SURF_DIAGNOSTICS_UNSTABLE;
        report->reason = SURF_DIAGNOSTICS_REASON_NETWORK;
    } else if (report->frame_age_us >= UINT64_C(350000)) {
        report->health = SURF_DIAGNOSTICS_UNSTABLE;
        report->reason = SURF_DIAGNOSTICS_REASON_FRAME_AGE;
    } else if (report->decode_us >= UINT64_C(25000)) {
        report->health = SURF_DIAGNOSTICS_UNSTABLE;
        report->reason = SURF_DIAGNOSTICS_REASON_DECODE_TIME;
    } else if (report->audio_underruns >= 2u) {
        report->health = SURF_DIAGNOSTICS_UNSTABLE;
        report->reason = SURF_DIAGNOSTICS_REASON_AUDIO_UNDERRUN;
    } else if (report->dropped_frames >= 6u) {
        report->health = SURF_DIAGNOSTICS_UNSTABLE;
        report->reason = SURF_DIAGNOSTICS_REASON_FRAME_DROPS;
    } else if (report->maximum_presentation_gap_us >= UINT64_C(350000)) {
        report->health = SURF_DIAGNOSTICS_UNSTABLE;
        report->reason = SURF_DIAGNOSTICS_REASON_PRESENTATION_GAP;
    } else if (report->rtt_us >= UINT64_C(120000) ||
               (report->timing_synchronized != 0 &&
                report->network_us >= UINT64_C(120000))) {
        report->health = SURF_DIAGNOSTICS_DELAYED;
        report->reason = SURF_DIAGNOSTICS_REASON_NETWORK;
    } else if (report->frame_age_us >= UINT64_C(200000)) {
        report->health = SURF_DIAGNOSTICS_DELAYED;
        report->reason = SURF_DIAGNOSTICS_REASON_FRAME_AGE;
    } else if (report->decode_us >= UINT64_C(12000)) {
        report->health = SURF_DIAGNOSTICS_DELAYED;
        report->reason = SURF_DIAGNOSTICS_REASON_DECODE_TIME;
    } else if (report->audio_underruns > 0u) {
        report->health = SURF_DIAGNOSTICS_DELAYED;
        report->reason = SURF_DIAGNOSTICS_REASON_AUDIO_UNDERRUN;
    } else if (report->dropped_frames > 0u || report->sequence_gaps > 0u) {
        report->health = SURF_DIAGNOSTICS_DELAYED;
        report->reason = SURF_DIAGNOSTICS_REASON_FRAME_DROPS;
    } else if (report->maximum_presentation_gap_us >= UINT64_C(150000)) {
        report->health = SURF_DIAGNOSTICS_DELAYED;
        report->reason = SURF_DIAGNOSTICS_REASON_PRESENTATION_GAP;
    } else if (video_depth >= 2u) {
        report->health = SURF_DIAGNOSTICS_DELAYED;
        report->reason = SURF_DIAGNOSTICS_REASON_VIDEO_BACKLOG;
    } else {
        report->health = SURF_DIAGNOSTICS_SMOOTH;
        report->reason = SURF_DIAGNOSTICS_REASON_NONE;
    }
}

int surf_diagnostics_update(surf_diagnostics_t *diagnostics,
                            const surf_diagnostics_sample_t *sample,
                            surf_diagnostics_report_t *out_report) {
    uint64_t elapsed;
    uint64_t video_delta;
    uint64_t ingress_drop_delta;
    uint64_t output_drop_delta;
    uint64_t presentation_drop_delta;
    if (diagnostics == NULL || sample == NULL || out_report == NULL) {
        return -1;
    }
    if (sample->maximum_presentation_gap_us >
        diagnostics->maximum_presentation_gap_us) {
        diagnostics->maximum_presentation_gap_us =
            sample->maximum_presentation_gap_us;
    }
    if (diagnostics->initialized == 0u ||
        sample->now_ns <= diagnostics->baseline.now_ns ||
        sample->connected != diagnostics->baseline.connected) {
        diagnostics->baseline = *sample;
        diagnostics->maximum_presentation_gap_us = 0u;
        diagnostics->initialized = 1u;
        return 0;
    }
    elapsed = sample->now_ns - diagnostics->baseline.now_ns;
    if (elapsed < SURF_DIAGNOSTICS_WINDOW_NS) {
        return 0;
    }

    memset(out_report, 0, sizeof(*out_report));
    video_delta = surf_delta(sample->video_packets,
                             diagnostics->baseline.video_packets);
    out_report->window_ms = (double)elapsed / 1000000.0;
    out_report->video_fps = (double)video_delta * 1000000000.0 /
                            (double)elapsed;
    out_report->decode_fps =
        (double)surf_delta(sample->decoded_frames,
                           diagnostics->baseline.decoded_frames) *
        1000000000.0 / (double)elapsed;
    out_report->presentation_fps =
        (double)surf_delta(sample->presented_frames,
                           diagnostics->baseline.presented_frames) *
        1000000000.0 / (double)elapsed;
    ingress_drop_delta = surf_delta(sample->ingress_replaced,
                                    diagnostics->baseline.ingress_replaced);
    output_drop_delta = surf_delta(sample->output_replaced,
                                   diagnostics->baseline.output_replaced);
    presentation_drop_delta = surf_delta(
        sample->presentation_replaced,
        diagnostics->baseline.presentation_replaced);
    out_report->dropped_frames = surf_add_saturated(
        surf_add_saturated(ingress_drop_delta, output_drop_delta),
        presentation_drop_delta);
    out_report->drop_percent =
        video_delta > 0u
            ? 100.0 * (double)out_report->dropped_frames / (double)video_delta
            : 0.0;
    out_report->sequence_gaps = surf_delta(
        sample->sequence_gaps, diagnostics->baseline.sequence_gaps);
    out_report->decode_errors = surf_delta(
        sample->decode_errors, diagnostics->baseline.decode_errors);
    out_report->audio_underruns = surf_delta(
        sample->audio_underruns, diagnostics->baseline.audio_underruns);
    surf_diagnostics_copy_instantaneous(sample, out_report);
    out_report->maximum_presentation_gap_us =
        diagnostics->maximum_presentation_gap_us;
    surf_diagnostics_classify(out_report, sample->connected);
    diagnostics->baseline = *sample;
    diagnostics->maximum_presentation_gap_us = 0u;
    return 1;
}

const char *surf_diagnostics_health_string(surf_diagnostics_health_t health) {
    switch (health) {
        case SURF_DIAGNOSTICS_OFFLINE:
            return "offline";
        case SURF_DIAGNOSTICS_SMOOTH:
            return "smooth";
        case SURF_DIAGNOSTICS_DELAYED:
            return "delayed";
        case SURF_DIAGNOSTICS_UNSTABLE:
            return "unstable";
        default:
            return "unknown";
    }
}

const char *surf_diagnostics_reason_string(surf_diagnostics_reason_t reason) {
    switch (reason) {
        case SURF_DIAGNOSTICS_REASON_NONE:
            return "none";
        case SURF_DIAGNOSTICS_REASON_OFFLINE:
            return "offline";
        case SURF_DIAGNOSTICS_REASON_DECODE_ERROR:
            return "decode-error";
        case SURF_DIAGNOSTICS_REASON_VIDEO_BACKLOG:
            return "video-backlog";
        case SURF_DIAGNOSTICS_REASON_NETWORK:
            return "network";
        case SURF_DIAGNOSTICS_REASON_FRAME_AGE:
            return "frame-age";
        case SURF_DIAGNOSTICS_REASON_DECODE_TIME:
            return "decode-time";
        case SURF_DIAGNOSTICS_REASON_AUDIO_UNDERRUN:
            return "audio-underrun";
        case SURF_DIAGNOSTICS_REASON_FRAME_DROPS:
            return "frame-drops";
        case SURF_DIAGNOSTICS_REASON_PRESENTATION_GAP:
            return "presentation-gap";
        default:
            return "unknown";
    }
}

size_t surf_diagnostics_sizeof(void) { return sizeof(surf_diagnostics_t); }
size_t surf_diagnostics_sample_sizeof(void) {
    return sizeof(surf_diagnostics_sample_t);
}
size_t surf_diagnostics_report_sizeof(void) {
    return sizeof(surf_diagnostics_report_t);
}
