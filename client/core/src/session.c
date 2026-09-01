#include "surf/session.h"

void surf_reconnect_policy_init(surf_reconnect_policy_t *policy) {
    if (policy == NULL) {
        return;
    }
    policy->attempts = 0;
    policy->maximum_attempts = SURF_RECONNECT_DEFAULT_MAXIMUM;
    policy->reserved = 0;
    policy->base_delay_ms = SURF_RECONNECT_DEFAULT_BASE_DELAY_MS;
    policy->maximum_delay_ms = SURF_RECONNECT_DEFAULT_MAX_DELAY_MS;
}

void surf_reconnect_policy_reset(surf_reconnect_policy_t *policy) {
    if (policy != NULL) {
        policy->attempts = 0;
    }
}

surf_reconnect_result_t surf_reconnect_policy_failure(
    surf_reconnect_policy_t *policy, int retryable, uint64_t connected_ms,
    uint8_t *out_attempt, uint32_t *out_delay_ms) {
    uint32_t delay;
    uint8_t index;
    if (policy == NULL || out_attempt == NULL || out_delay_ms == NULL) {
        return SURF_RECONNECT_ERROR_ARGUMENT;
    }
    if (connected_ms >= SURF_RECONNECT_STABLE_AFTER_MS) {
        policy->attempts = 0;
    }
    if (!retryable || policy->attempts >= policy->maximum_attempts ||
        policy->base_delay_ms == 0 || policy->maximum_delay_ms == 0) {
        return SURF_RECONNECT_STOP;
    }

    policy->attempts = (uint8_t)(policy->attempts + 1u);
    delay = policy->base_delay_ms;
    for (index = 1; index < policy->attempts; index++) {
        if (delay >= policy->maximum_delay_ms ||
            delay > policy->maximum_delay_ms / 2u) {
            delay = policy->maximum_delay_ms;
            break;
        }
        delay *= 2u;
    }
    if (delay > policy->maximum_delay_ms) {
        delay = policy->maximum_delay_ms;
    }
    *out_attempt = policy->attempts;
    *out_delay_ms = delay;
    return SURF_RECONNECT_RETRY;
}

size_t surf_reconnect_policy_sizeof(void) {
    return sizeof(surf_reconnect_policy_t);
}
