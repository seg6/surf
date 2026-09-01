#include "surf/media.h"

#include <string.h>

void surf_media_policy_init(surf_media_policy_t *policy) {
    if (policy == NULL) {
        return;
    }
    memset(policy, 0, sizeof(*policy));
    policy->waiting_for_idr = 1u;
    policy->reset_pending = 1u;
}

void surf_media_policy_reset(surf_media_policy_t *policy) {
    surf_media_policy_init(policy);
}

surf_media_result_t surf_media_policy_admit(surf_media_policy_t *policy,
                                            uint32_t generation,
                                            uint32_t sequence, int is_idr,
                                            surf_media_admission_t *out_admission) {
    int generation_changed;
    int sequence_gap;
    if (policy == NULL || out_admission == NULL) {
        return SURF_MEDIA_ERROR_ARGUMENT;
    }

    generation_changed = policy->has_generation != 0u &&
                         policy->generation != generation;
    sequence_gap = policy->has_sequence != 0u &&
                   sequence != policy->last_sequence + UINT32_C(1);
    if (generation_changed || sequence_gap) {
        policy->waiting_for_idr = 1u;
        policy->reset_pending = 1u;
    }
    policy->generation = generation;
    policy->last_sequence = sequence;
    policy->has_generation = 1u;
    policy->has_sequence = 1u;

    out_admission->generation_changed = generation_changed;
    out_admission->sequence_gap = sequence_gap;
    if (policy->waiting_for_idr != 0u && is_idr == 0) {
        out_admission->action = SURF_MEDIA_ACTION_DROP_REQUEST_KEYFRAME;
        return SURF_MEDIA_OK;
    }
    if (policy->reset_pending != 0u) {
        policy->waiting_for_idr = 0u;
        policy->reset_pending = 0u;
        out_admission->action = SURF_MEDIA_ACTION_RESET_AND_DECODE;
        return SURF_MEDIA_OK;
    }
    out_admission->action = SURF_MEDIA_ACTION_DECODE;
    return SURF_MEDIA_OK;
}

size_t surf_media_policy_sizeof(void) { return sizeof(surf_media_policy_t); }

size_t surf_media_admission_sizeof(void) {
    return sizeof(surf_media_admission_t);
}
