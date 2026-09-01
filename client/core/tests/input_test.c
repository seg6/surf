#include "surf/input.h"

#include "test.h"

#include <math.h>
#include <stdint.h>
#include <string.h>

static void test_pointer_normalization_and_order(void) {
    surf_input_state_t state;
    surf_input_sample_t first;
    surf_input_sample_t second;
    surf_input_state_init(&state);
    surf_input_set_surface(&state, 7u);
    SURF_CHECK(surf_input_pointer_sample(&state, 50.0, 25.0, 100.0, 100.0,
                                         100u, &first) == SURF_INPUT_OK);
    SURF_CHECK(first.sequence == 1u);
    SURF_CHECK(first.interaction_id == 1u);
    SURF_CHECK(first.surface_generation == 7u);
    SURF_CHECK(first.x == 0.5 && first.y == 0.25);
    SURF_CHECK(first.delta_x == 0.0 && first.delta_y == 0.0);
    SURF_CHECK(surf_input_pointer_sample(&state, -10.0, 400.0, 100.0, 100.0,
                                         99u, &second) == SURF_INPUT_OK);
    SURF_CHECK(second.sequence == 2u && second.interaction_id == 2u);
    SURF_CHECK(second.event_ns == 101u && second.client_ns == 101u);
    SURF_CHECK(second.x == 0.0 && second.y == 1.0);
}

static void test_surface_change_resets_only_surface_sequence(void) {
    surf_input_state_t state;
    surf_input_sample_t sample;
    surf_input_causal_t causal;
    surf_input_state_init(&state);
    surf_input_set_surface(&state, 1u);
    SURF_CHECK(surf_input_pointer_sample(&state, 1.0, 1.0, 2.0, 2.0, 5u,
                                         &sample) == SURF_INPUT_OK);
    surf_input_set_surface(&state, 2u);
    SURF_CHECK(surf_input_pointer_sample(&state, 1.0, 1.0, 2.0, 2.0, 6u,
                                         &sample) == SURF_INPUT_OK);
    SURF_CHECK(sample.sequence == 1u && sample.interaction_id == 2u);
    SURF_CHECK(sample.surface_generation == 2u);
    SURF_CHECK(surf_input_next_causal(&state, 5u, &causal) == SURF_INPUT_OK);
    SURF_CHECK(causal.interaction_id == 3u && causal.client_ns == 7u);
    surf_input_set_surface(&state, 0u);
    SURF_CHECK(surf_input_pointer_sample(&state, 0.0, 0.0, 2.0, 2.0, 8u,
                                         &sample) == SURF_INPUT_ERROR_ARGUMENT);
}

static void test_wheel_normalization_and_bounds(void) {
    surf_input_state_t state;
    surf_input_sample_t sample;
    surf_input_state_init(&state);
    surf_input_set_surface(&state, 3u);
    SURF_CHECK(surf_input_wheel_sample(&state, 20.0, 60.0, -10.0, 30.0,
                                       200.0, 100.0, 20u,
                                       &sample) == SURF_INPUT_OK);
    SURF_CHECK(sample.x == 0.1 && sample.y == 0.6);
    SURF_CHECK(sample.delta_x == -0.05 && sample.delta_y == 0.3);
    SURF_CHECK(surf_input_wheel_sample(&state, 0.0, 0.0, 10000.0, -10000.0,
                                       10.0, 10.0, 21u,
                                       &sample) == SURF_INPUT_OK);
    SURF_CHECK(sample.delta_x == 4.0 && sample.delta_y == -4.0);
}

static void test_invalid_arguments_are_atomic(void) {
    surf_input_state_t state;
    surf_input_state_t before;
    surf_input_sample_t sample;
    surf_input_causal_t causal;
    surf_input_state_init(&state);
    surf_input_set_surface(&state, 4u);
    before = state;
    SURF_CHECK(surf_input_pointer_sample(&state, NAN, 0.0, 1.0, 1.0, 1u,
                                         &sample) == SURF_INPUT_ERROR_ARGUMENT);
    SURF_CHECK(memcmp(&state, &before, sizeof(state)) == 0);
    SURF_CHECK(surf_input_wheel_sample(&state, 0.0, 0.0, 0.0, INFINITY,
                                       1.0, 1.0, 1u,
                                       &sample) == SURF_INPUT_ERROR_ARGUMENT);
    SURF_CHECK(memcmp(&state, &before, sizeof(state)) == 0);
    SURF_CHECK(surf_input_next_causal(&state, 0u, &causal) ==
               SURF_INPUT_ERROR_ARGUMENT);
    SURF_CHECK(memcmp(&state, &before, sizeof(state)) == 0);
}

static void test_overflow_is_rejected(void) {
    surf_input_state_t state;
    surf_input_sample_t sample;
    surf_input_causal_t causal;
    surf_input_state_init(&state);
    surf_input_set_surface(&state, 1u);
    state.sequence = UINT64_MAX;
    SURF_CHECK(surf_input_pointer_sample(&state, 0.0, 0.0, 1.0, 1.0, 1u,
                                         &sample) == SURF_INPUT_ERROR_OVERFLOW);
    state.sequence = 0u;
    state.interaction_id = UINT64_MAX;
    SURF_CHECK(surf_input_next_causal(&state, 1u, &causal) ==
               SURF_INPUT_ERROR_OVERFLOW);
}

int main(void) {
    test_pointer_normalization_and_order();
    test_surface_change_resets_only_surface_sequence();
    test_wheel_normalization_and_bounds();
    test_invalid_arguments_are_atomic();
    test_overflow_is_rejected();
    puts("surf_input: all tests passed");
    return EXIT_SUCCESS;
}
