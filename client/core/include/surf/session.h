#ifndef SURF_SESSION_H
#define SURF_SESSION_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define SURF_RECONNECT_DEFAULT_MAXIMUM ((uint8_t)5)
#define SURF_RECONNECT_DEFAULT_BASE_DELAY_MS ((uint32_t)250)
#define SURF_RECONNECT_DEFAULT_MAX_DELAY_MS ((uint32_t)4000)
#define SURF_RECONNECT_STABLE_AFTER_MS ((uint64_t)30000)

typedef struct surf_reconnect_policy {
    uint8_t attempts;
    uint8_t maximum_attempts;
    uint16_t reserved;
    uint32_t base_delay_ms;
    uint32_t maximum_delay_ms;
} surf_reconnect_policy_t;

typedef enum surf_reconnect_result {
    SURF_RECONNECT_ERROR_ARGUMENT = -1,
    SURF_RECONNECT_STOP = 0,
    SURF_RECONNECT_RETRY = 1
} surf_reconnect_result_t;

void surf_reconnect_policy_init(surf_reconnect_policy_t *policy);
void surf_reconnect_policy_reset(surf_reconnect_policy_t *policy);
surf_reconnect_result_t surf_reconnect_policy_failure(
    surf_reconnect_policy_t *policy, int retryable, uint64_t connected_ms,
    uint8_t *out_attempt, uint32_t *out_delay_ms);
size_t surf_reconnect_policy_sizeof(void);

#ifdef __cplusplus
}
#endif

#endif
