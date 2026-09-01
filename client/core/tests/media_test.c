#include "surf/media.h"

#include "test.h"

static void test_fresh_stream_waits_for_idr(void) {
    surf_media_policy_t policy;
    surf_media_admission_t admission;
    surf_media_policy_init(&policy);
    SURF_CHECK(surf_media_policy_admit(&policy, 1u, 10u, 0, &admission) ==
               SURF_MEDIA_OK);
    SURF_CHECK(admission.action == SURF_MEDIA_ACTION_DROP_REQUEST_KEYFRAME);
    SURF_CHECK(admission.generation_changed == 0);
    SURF_CHECK(admission.sequence_gap == 0);
    SURF_CHECK(surf_media_policy_admit(&policy, 1u, 11u, 1, &admission) ==
               SURF_MEDIA_OK);
    SURF_CHECK(admission.action == SURF_MEDIA_ACTION_RESET_AND_DECODE);
    SURF_CHECK(surf_media_policy_admit(&policy, 1u, 12u, 0, &admission) ==
               SURF_MEDIA_OK);
    SURF_CHECK(admission.action == SURF_MEDIA_ACTION_DECODE);
}

static void test_gap_and_generation_require_recovery_idr(void) {
    surf_media_policy_t policy;
    surf_media_admission_t admission;
    surf_media_policy_init(&policy);
    SURF_CHECK(surf_media_policy_admit(&policy, 7u, UINT32_MAX, 1, &admission) ==
               SURF_MEDIA_OK);
    SURF_CHECK(admission.action == SURF_MEDIA_ACTION_RESET_AND_DECODE);
    SURF_CHECK(surf_media_policy_admit(&policy, 7u, 0u, 0, &admission) ==
               SURF_MEDIA_OK);
    SURF_CHECK(admission.action == SURF_MEDIA_ACTION_DECODE);
    SURF_CHECK(admission.sequence_gap == 0);

    SURF_CHECK(surf_media_policy_admit(&policy, 7u, 2u, 0, &admission) ==
               SURF_MEDIA_OK);
    SURF_CHECK(admission.sequence_gap == 1);
    SURF_CHECK(admission.action == SURF_MEDIA_ACTION_DROP_REQUEST_KEYFRAME);
    SURF_CHECK(surf_media_policy_admit(&policy, 7u, 3u, 1, &admission) ==
               SURF_MEDIA_OK);
    SURF_CHECK(admission.action == SURF_MEDIA_ACTION_RESET_AND_DECODE);

    SURF_CHECK(surf_media_policy_admit(&policy, 8u, 1u, 0, &admission) ==
               SURF_MEDIA_OK);
    SURF_CHECK(admission.generation_changed == 1);
    SURF_CHECK(admission.sequence_gap == 1);
    SURF_CHECK(admission.action == SURF_MEDIA_ACTION_DROP_REQUEST_KEYFRAME);
    SURF_CHECK(surf_media_policy_admit(&policy, 8u, 2u, 1, &admission) ==
               SURF_MEDIA_OK);
    SURF_CHECK(admission.action == SURF_MEDIA_ACTION_RESET_AND_DECODE);
}

static void test_reset_and_arguments(void) {
    surf_media_policy_t policy;
    surf_media_admission_t admission;
    surf_media_policy_init(&policy);
    SURF_CHECK(surf_media_policy_admit(NULL, 1u, 1u, 1, &admission) ==
               SURF_MEDIA_ERROR_ARGUMENT);
    SURF_CHECK(surf_media_policy_admit(&policy, 1u, 1u, 1, NULL) ==
               SURF_MEDIA_ERROR_ARGUMENT);
    SURF_CHECK(surf_media_policy_admit(&policy, 1u, 1u, 1, &admission) ==
               SURF_MEDIA_OK);
    surf_media_policy_reset(&policy);
    SURF_CHECK(surf_media_policy_admit(&policy, 1u, 99u, 0, &admission) ==
               SURF_MEDIA_OK);
    SURF_CHECK(admission.action == SURF_MEDIA_ACTION_DROP_REQUEST_KEYFRAME);
}

int main(void) {
    test_fresh_stream_waits_for_idr();
    test_gap_and_generation_require_recovery_idr();
    test_reset_and_arguments();
    puts("surf_media: all tests passed");
    return EXIT_SUCCESS;
}
