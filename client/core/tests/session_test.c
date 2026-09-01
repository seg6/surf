#include "surf/session.h"

#include "test.h"

static void test_default_backoff(void) {
    static const uint32_t expected[] = {250u, 500u, 1000u, 2000u, 4000u};
    surf_reconnect_policy_t policy;
    uint8_t attempt = 0;
    uint32_t delay = 0;
    size_t index;
    surf_reconnect_policy_init(&policy);
    SURF_CHECK(policy.maximum_attempts == 5u);
    for (index = 0; index < sizeof(expected) / sizeof(expected[0]); index++) {
        SURF_CHECK(surf_reconnect_policy_failure(&policy, 1, 0, &attempt, &delay) ==
                   SURF_RECONNECT_RETRY);
        SURF_CHECK(attempt == (uint8_t)(index + 1u));
        SURF_CHECK(delay == expected[index]);
    }
    SURF_CHECK(surf_reconnect_policy_failure(&policy, 1, 0, &attempt, &delay) ==
               SURF_RECONNECT_STOP);
}

static void test_permanent_failure_and_reset(void) {
    surf_reconnect_policy_t policy;
    uint8_t attempt = 0;
    uint32_t delay = 0;
    surf_reconnect_policy_init(&policy);
    SURF_CHECK(surf_reconnect_policy_failure(&policy, 0, 0, &attempt, &delay) ==
               SURF_RECONNECT_STOP);
    SURF_CHECK(policy.attempts == 0u);
    SURF_CHECK(surf_reconnect_policy_failure(&policy, 1, 0, &attempt, &delay) ==
               SURF_RECONNECT_RETRY);
    surf_reconnect_policy_reset(&policy);
    SURF_CHECK(policy.attempts == 0u);
    SURF_CHECK(surf_reconnect_policy_failure(&policy, 1, 0, &attempt, &delay) ==
               SURF_RECONNECT_RETRY);
    SURF_CHECK(attempt == 1u && delay == 250u);
}

static void test_arguments(void) {
    surf_reconnect_policy_t policy;
    uint8_t attempt = 0;
    uint32_t delay = 0;
    surf_reconnect_policy_init(&policy);
    SURF_CHECK(surf_reconnect_policy_failure(NULL, 1, 0, &attempt, &delay) ==
               SURF_RECONNECT_ERROR_ARGUMENT);
    SURF_CHECK(surf_reconnect_policy_failure(&policy, 1, 0, NULL, &delay) ==
               SURF_RECONNECT_ERROR_ARGUMENT);
    SURF_CHECK(surf_reconnect_policy_failure(&policy, 1, 0, &attempt, NULL) ==
               SURF_RECONNECT_ERROR_ARGUMENT);
}

static void test_stable_session_starts_a_new_retry_budget(void) {
    surf_reconnect_policy_t policy;
    uint8_t attempt = 0;
    uint32_t delay = 0;
    surf_reconnect_policy_init(&policy);
    SURF_CHECK(surf_reconnect_policy_failure(&policy, 1, 0, &attempt, &delay) ==
               SURF_RECONNECT_RETRY);
    SURF_CHECK(surf_reconnect_policy_failure(&policy, 1, 0, &attempt, &delay) ==
               SURF_RECONNECT_RETRY);
    SURF_CHECK(attempt == 2u);
    SURF_CHECK(surf_reconnect_policy_failure(
                   &policy, 1, SURF_RECONNECT_STABLE_AFTER_MS, &attempt, &delay) ==
               SURF_RECONNECT_RETRY);
    SURF_CHECK(attempt == 1u && delay == 250u);
}

int main(void) {
    test_default_backoff();
    test_permanent_failure_and_reset();
    test_arguments();
    test_stable_session_starts_a_new_retry_budget();
    puts("surf_session: all tests passed");
    return EXIT_SUCCESS;
}
