#ifndef SURF_INPUT_H
#define SURF_INPUT_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef enum surf_input_result {
    SURF_INPUT_ERROR_OVERFLOW = -2,
    SURF_INPUT_ERROR_ARGUMENT = -1,
    SURF_INPUT_OK = 0
} surf_input_result_t;

/*
 * Host-independent input ordering. `sequence` is scoped to the currently
 * presented video surface; interaction IDs and timestamps remain monotonic
 * across surface changes so media diagnostics can correlate an input with the
 * first frame it caused.
 */
typedef struct surf_input_state {
    uint64_t sequence;
    uint64_t interaction_id;
    uint64_t last_timestamp_ns;
    uint32_t surface_generation;
    uint32_t reserved;
} surf_input_state_t;

typedef struct surf_input_causal {
    uint64_t interaction_id;
    uint64_t client_ns;
} surf_input_causal_t;

/* Coordinates and deltas are fractions of the exact presented surface. */
typedef struct surf_input_sample {
    uint64_t sequence;
    uint64_t interaction_id;
    uint64_t client_ns;
    uint64_t event_ns;
    uint32_t surface_generation;
    uint32_t reserved;
    double x;
    double y;
    double delta_x;
    double delta_y;
} surf_input_sample_t;

void surf_input_state_init(surf_input_state_t *state);
void surf_input_set_surface(surf_input_state_t *state, uint32_t generation);

surf_input_result_t surf_input_next_causal(
    surf_input_state_t *state, uint64_t timestamp_ns,
    surf_input_causal_t *out_causal);

surf_input_result_t surf_input_pointer_sample(
    surf_input_state_t *state, double local_x, double local_y,
    double surface_width, double surface_height, uint64_t timestamp_ns,
    surf_input_sample_t *out_sample);

surf_input_result_t surf_input_wheel_sample(
    surf_input_state_t *state, double local_x, double local_y,
    double delta_x, double delta_y, double surface_width,
    double surface_height, uint64_t timestamp_ns,
    surf_input_sample_t *out_sample);

size_t surf_input_state_sizeof(void);
size_t surf_input_causal_sizeof(void);
size_t surf_input_sample_sizeof(void);

#ifdef __cplusplus
}
#endif

#endif
