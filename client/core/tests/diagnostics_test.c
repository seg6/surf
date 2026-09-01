#include "surf/diagnostics.h"
#include "test.h"

#include <string.h>

static int test_clock_sync(void) {
    surf_clock_sync_t sync;
    uint64_t c0;
    uint64_t translated;

    surf_clock_sync_init(&sync);
    SURF_CHECK(surf_clock_sync_probe(&sync, UINT64_C(1000000000), &c0) == 1);
    SURF_CHECK(c0 == UINT64_C(1000000000));
    SURF_CHECK(surf_clock_sync_probe(&sync, UINT64_C(1005000000), &c0) == 0);
    SURF_CHECK(surf_clock_sync_consume(
                   &sync, UINT64_C(1000000000), UINT64_C(6005000000),
                   UINT64_C(6006000000), UINT64_C(1011000000)) == 1);
    SURF_CHECK(sync.best_rtt_ns == UINT64_C(10000000));
    SURF_CHECK(sync.synchronized == 0u);
    SURF_CHECK(surf_clock_sync_server_to_client(
                   &sync, UINT64_C(7000000000), &translated) == 0);

    SURF_CHECK(surf_clock_sync_probe(&sync, UINT64_C(2011000000), &c0) == 1);
    SURF_CHECK(surf_clock_sync_consume(
                   &sync, c0, UINT64_C(7015000000), UINT64_C(7016000000),
                   UINT64_C(2023000000)) == 1);
    SURF_CHECK(sync.synchronized == 1u);
    SURF_CHECK(surf_clock_sync_server_to_client(
                   &sync, UINT64_C(8000000000), &translated) == 1);
    SURF_CHECK(translated == UINT64_C(3000000000));
    return 0;
}

static int test_pipeline_window(void) {
    surf_diagnostics_t diagnostics;
    surf_diagnostics_sample_t sample;
    surf_diagnostics_report_t report;

    memset(&sample, 0, sizeof(sample));
    surf_diagnostics_init(&diagnostics);
    sample.now_ns = UINT64_C(1000000000);
    sample.connected = 1;
    SURF_CHECK(surf_diagnostics_update(&diagnostics, &sample, &report) == 0);

    sample.now_ns = UINT64_C(3100000000);
    sample.video_packets = 126;
    sample.decoded_frames = 126;
    sample.presented_frames = 125;
    sample.decode_us = 3000;
    sample.upload_us = 80;
    sample.frame_age_us = 18000;
    sample.rtt_us = 4000;
    sample.maximum_presentation_gap_us = 19000;
    SURF_CHECK(surf_diagnostics_update(&diagnostics, &sample, &report) == 1);
    SURF_CHECK(report.video_fps == 60.0);
    SURF_CHECK(report.presentation_fps > 59.5 &&
               report.presentation_fps < 59.6);
    SURF_CHECK(report.health == SURF_DIAGNOSTICS_SMOOTH);
    SURF_CHECK(report.reason == SURF_DIAGNOSTICS_REASON_NONE);

    sample.now_ns = UINT64_C(5200000000);
    sample.video_packets += 126;
    sample.decoded_frames += 120;
    sample.presented_frames += 119;
    sample.output_replaced += 7;
    sample.maximum_presentation_gap_us = 180000;
    SURF_CHECK(surf_diagnostics_update(&diagnostics, &sample, &report) == 1);
    SURF_CHECK(report.dropped_frames == 7u);
    SURF_CHECK(report.health == SURF_DIAGNOSTICS_UNSTABLE);
    SURF_CHECK(report.reason == SURF_DIAGNOSTICS_REASON_FRAME_DROPS);

    sample.now_ns = UINT64_C(7300000000);
    sample.connected = 0;
    SURF_CHECK(surf_diagnostics_update(&diagnostics, &sample, &report) == 0);
    sample.now_ns = UINT64_C(9400000000);
    SURF_CHECK(surf_diagnostics_update(&diagnostics, &sample, &report) == 1);
    SURF_CHECK(report.health == SURF_DIAGNOSTICS_OFFLINE);
    return 0;
}

int main(void) {
    SURF_CHECK(test_clock_sync() == 0);
    SURF_CHECK(test_pipeline_window() == 0);
    return 0;
}
