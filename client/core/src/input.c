#include "surf/input.h"

#include <math.h>
#include <string.h>

static double clamp(double value, double minimum, double maximum) {
    if (value < minimum) {
        return minimum;
    }
    if (value > maximum) {
        return maximum;
    }
    return value;
}

static surf_input_result_t next_values(
    const surf_input_state_t *state, uint64_t timestamp_ns,
    uint64_t *out_sequence, uint64_t *out_interaction_id,
    uint64_t *out_timestamp_ns) {
    uint64_t normalized_timestamp;
    if (state == NULL || out_sequence == NULL || out_interaction_id == NULL ||
        out_timestamp_ns == NULL || timestamp_ns == 0 ||
        state->surface_generation == 0) {
        return SURF_INPUT_ERROR_ARGUMENT;
    }
    if (state->sequence == UINT64_MAX || state->interaction_id == UINT64_MAX) {
        return SURF_INPUT_ERROR_OVERFLOW;
    }
    normalized_timestamp = timestamp_ns;
    if (normalized_timestamp <= state->last_timestamp_ns) {
        if (state->last_timestamp_ns == UINT64_MAX) {
            return SURF_INPUT_ERROR_OVERFLOW;
        }
        normalized_timestamp = state->last_timestamp_ns + 1u;
    }
    *out_sequence = state->sequence + 1u;
    *out_interaction_id = state->interaction_id + 1u;
    *out_timestamp_ns = normalized_timestamp;
    return SURF_INPUT_OK;
}

static surf_input_result_t normalized_sample(
    surf_input_state_t *state, double local_x, double local_y,
    double delta_x, double delta_y, double surface_width,
    double surface_height, uint64_t timestamp_ns,
    surf_input_sample_t *out_sample) {
    surf_input_sample_t sample;
    surf_input_result_t result;
    uint64_t sequence;
    uint64_t interaction_id;
    uint64_t normalized_timestamp;
    if (state == NULL || out_sample == NULL || !isfinite(local_x) ||
        !isfinite(local_y) || !isfinite(delta_x) || !isfinite(delta_y) ||
        !isfinite(surface_width) || !isfinite(surface_height) ||
        surface_width <= 0.0 || surface_height <= 0.0) {
        return SURF_INPUT_ERROR_ARGUMENT;
    }
    result = next_values(state, timestamp_ns, &sequence, &interaction_id,
                         &normalized_timestamp);
    if (result != SURF_INPUT_OK) {
        return result;
    }
    memset(&sample, 0, sizeof(sample));
    sample.sequence = sequence;
    sample.interaction_id = interaction_id;
    sample.client_ns = normalized_timestamp;
    sample.event_ns = normalized_timestamp;
    sample.surface_generation = state->surface_generation;
    sample.x = clamp(local_x / surface_width, 0.0, 1.0);
    sample.y = clamp(local_y / surface_height, 0.0, 1.0);
    sample.delta_x = clamp(delta_x / surface_width, -4.0, 4.0);
    sample.delta_y = clamp(delta_y / surface_height, -4.0, 4.0);

    state->sequence = sequence;
    state->interaction_id = interaction_id;
    state->last_timestamp_ns = normalized_timestamp;
    *out_sample = sample;
    return SURF_INPUT_OK;
}

void surf_input_state_init(surf_input_state_t *state) {
    if (state != NULL) {
        memset(state, 0, sizeof(*state));
    }
}

void surf_input_set_surface(surf_input_state_t *state, uint32_t generation) {
    if (state == NULL || state->surface_generation == generation) {
        return;
    }
    state->surface_generation = generation;
    state->sequence = 0;
}

surf_input_result_t surf_input_next_causal(
    surf_input_state_t *state, uint64_t timestamp_ns,
    surf_input_causal_t *out_causal) {
    uint64_t normalized_timestamp;
    if (state == NULL || out_causal == NULL || timestamp_ns == 0) {
        return SURF_INPUT_ERROR_ARGUMENT;
    }
    if (state->interaction_id == UINT64_MAX) {
        return SURF_INPUT_ERROR_OVERFLOW;
    }
    normalized_timestamp = timestamp_ns;
    if (normalized_timestamp <= state->last_timestamp_ns) {
        if (state->last_timestamp_ns == UINT64_MAX) {
            return SURF_INPUT_ERROR_OVERFLOW;
        }
        normalized_timestamp = state->last_timestamp_ns + 1u;
    }
    out_causal->interaction_id = state->interaction_id + 1u;
    out_causal->client_ns = normalized_timestamp;
    state->interaction_id = out_causal->interaction_id;
    state->last_timestamp_ns = normalized_timestamp;
    return SURF_INPUT_OK;
}

surf_input_result_t surf_input_pointer_sample(
    surf_input_state_t *state, double local_x, double local_y,
    double surface_width, double surface_height, uint64_t timestamp_ns,
    surf_input_sample_t *out_sample) {
    return normalized_sample(state, local_x, local_y, 0.0, 0.0,
                             surface_width, surface_height, timestamp_ns,
                             out_sample);
}

surf_input_result_t surf_input_wheel_sample(
    surf_input_state_t *state, double local_x, double local_y,
    double delta_x, double delta_y, double surface_width,
    double surface_height, uint64_t timestamp_ns,
    surf_input_sample_t *out_sample) {
    return normalized_sample(state, local_x, local_y, delta_x, delta_y,
                             surface_width, surface_height, timestamp_ns,
                             out_sample);
}

size_t surf_input_state_sizeof(void) {
    return sizeof(surf_input_state_t);
}

size_t surf_input_causal_sizeof(void) {
    return sizeof(surf_input_causal_t);
}

size_t surf_input_sample_sizeof(void) {
    return sizeof(surf_input_sample_t);
}
