#ifndef SURF_MEDIA_H
#define SURF_MEDIA_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct surf_media_policy {
    uint32_t generation;
    uint32_t last_sequence;
    uint8_t has_generation;
    uint8_t has_sequence;
    uint8_t waiting_for_idr;
    uint8_t reset_pending;
} surf_media_policy_t;

typedef enum surf_media_action {
    SURF_MEDIA_ACTION_DROP_REQUEST_KEYFRAME = 0,
    SURF_MEDIA_ACTION_DECODE = 1,
    SURF_MEDIA_ACTION_RESET_AND_DECODE = 2
} surf_media_action_t;

typedef struct surf_media_admission {
    surf_media_action_t action;
    int generation_changed;
    int sequence_gap;
} surf_media_admission_t;

typedef enum surf_media_result {
    SURF_MEDIA_ERROR_ARGUMENT = -1,
    SURF_MEDIA_OK = 0
} surf_media_result_t;

void surf_media_policy_init(surf_media_policy_t *policy);
void surf_media_policy_reset(surf_media_policy_t *policy);
surf_media_result_t surf_media_policy_admit(surf_media_policy_t *policy,
                                            uint32_t generation,
                                            uint32_t sequence, int is_idr,
                                            surf_media_admission_t *out_admission);
size_t surf_media_policy_sizeof(void);
size_t surf_media_admission_sizeof(void);

#ifdef __cplusplus
}
#endif

#endif
