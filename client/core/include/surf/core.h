#ifndef SURF_CORE_H
#define SURF_CORE_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define SURF_CORE_ABI_VERSION UINT32_C(1)
#define SURF_CORE_DEFAULT_MAX_TABS ((size_t)64)
#define SURF_CORE_HARD_MAX_TABS ((size_t)256)

typedef struct surf_core surf_core_t;

typedef struct surf_string_view {
    const char *data;
    size_t length;
} surf_string_view_t;

typedef void *(*surf_allocate_fn)(void *context, size_t size);
typedef void (*surf_deallocate_fn)(void *context, void *pointer);

typedef struct surf_allocator {
    void *context;
    surf_allocate_fn allocate;
    surf_deallocate_fn deallocate;
} surf_allocator_t;

typedef struct surf_core_config {
    uint32_t abi_version;
    size_t max_tabs;
    surf_allocator_t allocator;
} surf_core_config_t;

typedef enum surf_core_result {
    SURF_CORE_OK = 0,
    SURF_CORE_ERROR_ARGUMENT,
    SURF_CORE_ERROR_ABI,
    SURF_CORE_ERROR_LIMIT,
    SURF_CORE_ERROR_ALLOCATE,
    SURF_CORE_ERROR_STATE
} surf_core_result_t;

typedef struct surf_browser_mode_snapshot {
    surf_string_view_t state, host, message;
    uint64_t revision;
    int paused, standalone, can_force;
} surf_browser_mode_snapshot_t;

surf_core_result_t surf_core_browser_mode(const surf_core_t *core,
    surf_browser_mode_snapshot_t *out_snapshot);

typedef struct surf_tab_event {
    int64_t id;
    surf_string_view_t title;
    surf_string_view_t url;
    surf_string_view_t icon;
    int active;
} surf_tab_event_t;

typedef enum surf_event_kind {
    SURF_EVENT_RESET = 1,
    SURF_EVENT_TABS,
    SURF_EVENT_URL,
    SURF_EVENT_HISTORY_STATE,
    SURF_EVENT_LOADING,
    SURF_EVENT_EDITABLE,
    SURF_EVENT_FULLSCREEN,
    SURF_EVENT_SECURITY,
    SURF_EVENT_STARRED,
    SURF_EVENT_PAGE_FRAME,
    SURF_EVENT_FRAME_PRESENTED,
    SURF_EVENT_KEYBOARD_VISIBILITY
} surf_event_kind_t;

typedef struct surf_event {
    surf_event_kind_t kind;
    union {
        struct {
            const surf_tab_event_t *items;
            size_t count;
        } tabs;
        struct {
            surf_string_view_t url;
            surf_string_view_t security;
            int starred;
        } url;
        struct {
            int can_go_back;
            int can_go_forward;
        } history;
        struct {
            int on;
        } boolean;
        struct {
            int on;
            int show_keyboard;
            surf_string_view_t kind;
            int has_rect;
            double rect[4];
        } editable;
        struct {
            surf_string_view_t state;
        } security;
        struct {
            uint32_t source_sequence;
        } page_frame;
        struct {
            uint32_t source_sequence;
        } frame_presented;
    } data;
} surf_event_t;

typedef enum surf_effect_kind {
    SURF_EFFECT_REQUEST_LIBRARY = 1,
    SURF_EFFECT_SHOW_KEYBOARD,
    SURF_EFFECT_HIDE_KEYBOARD,
    SURF_EFFECT_CLEAR_PAGE_PRESENTATION
} surf_effect_kind_t;

typedef struct surf_effect {
    surf_effect_kind_t kind;
} surf_effect_t;

typedef struct surf_tab_snapshot {
    int64_t id;
    surf_string_view_t title;
    surf_string_view_t url;
    surf_string_view_t icon;
    int active;
} surf_tab_snapshot_t;

/* Snapshot views are owned by `surf_core_t` and remain valid until the next
 * successful dispatch or destruction of that core. Platform hosts that cross
 * threads must copy them into their own immutable representation. */
typedef struct surf_snapshot {
    uint64_t revision;
    const surf_tab_snapshot_t *tabs;
    size_t tab_count;
    int64_t active_tab_id;
    surf_string_view_t active_title;
    surf_string_view_t current_url;
    surf_string_view_t security;
    surf_string_view_t editable_kind;
    double editable_rect[4];
    uint32_t awaited_source_sequence;
    int has_active_tab;
    int show_start_page;
    int loading;
    int can_go_back;
    int can_go_forward;
    int starred;
    int fullscreen;
    int editable;
    int editable_has_rect;
    int keyboard_visible;
    int awaiting_page_frame;
} surf_snapshot_t;

/* Non-visual state for platform-presented browser surfaces. Large collection
 * contents remain typed protocol views/copies in the host; the core owns their
 * current identity, presence, counts, and control values so reconnect/tab
 * transitions cannot leave stale presentation behind. */
typedef struct surf_semantic_snapshot {
    uint64_t revision;
    surf_string_view_t dialog_kind;
    surf_string_view_t dialog_text;
    surf_string_view_t dialog_default_text;
    surf_string_view_t select_id;
    surf_string_view_t select_title;
    surf_string_view_t clipboard_id;
    surf_string_view_t clipboard_text;
    surf_string_view_t reader_title;
    surf_string_view_t reader_url;
    surf_string_view_t media_title;
    surf_string_view_t page_error;
    surf_string_view_t toast_text;
    surf_string_view_t download_name;
    surf_string_view_t history_query;
    size_t select_option_count;
    size_t suggestion_count;
    size_t history_count;
    size_t bookmark_count;
    size_t download_count;
    int32_t history_offset;
    int32_t history_total;
    int32_t download_percent;
    int32_t media_count;
    double media_volume;
    double media_current_time;
    double media_duration;
    int dialog_active;
    int select_active;
    int select_multiple;
    int file_chooser_active;
    int file_chooser_multiple;
    int find_known;
    int find_found;
    int clipboard_pending;
    int clipboard_sync_request;
    int clipboard_sync_enabled;
    int clipboard_known;
    int reader_available;
    int media_available;
    int media_paused;
    int media_muted;
    uint32_t video_generation;
} surf_semantic_snapshot_t;

typedef enum surf_semantic_completion {
    SURF_SEMANTIC_COMPLETE_DIALOG = 1,
    SURF_SEMANTIC_COMPLETE_SELECT,
    SURF_SEMANTIC_COMPLETE_FILE_CHOOSER,
    SURF_SEMANTIC_COMPLETE_CLIPBOARD,
    SURF_SEMANTIC_COMPLETE_TOAST
} surf_semantic_completion_t;

surf_string_view_t surf_string_view(const char *data, size_t length);
surf_string_view_t surf_string_from_cstr(const char *string);
int surf_string_equal(surf_string_view_t left, surf_string_view_t right);

void surf_core_config_init(surf_core_config_t *config);
surf_core_result_t surf_core_create(const surf_core_config_t *config,
                                    surf_core_t **out_core);
void surf_core_destroy(surf_core_t *core);
/* Starts a new authenticated control epoch. Generations must be non-zero and
 * strictly increasing. All prior browser/transient state is discarded. */
surf_core_result_t surf_core_begin_connection(surf_core_t *core,
                                              uint64_t generation);
/* Dispatches only when `generation` is the current control epoch. Stale events
 * are ignored without changing the snapshot revision. */
surf_core_result_t surf_core_dispatch_scoped(surf_core_t *core,
                                             uint64_t generation,
                                             const surf_event_t *event);
struct surf_protocol_event;
struct surf_protocol_workspace;
surf_core_result_t surf_core_dispatch_protocol(
    surf_core_t *core, uint64_t generation,
    const struct surf_protocol_event *event);
surf_core_result_t surf_core_dispatch_protocol_json(
    surf_core_t *core, uint64_t generation,
    struct surf_protocol_workspace *workspace, const char *json,
    size_t length);
surf_core_result_t surf_core_present_frame(
    surf_core_t *core, uint64_t connection_generation,
    uint32_t video_generation, uint32_t source_sequence);
surf_core_result_t surf_core_complete_semantic(
    surf_core_t *core, uint64_t generation,
    surf_semantic_completion_t completion);
surf_core_result_t surf_core_dispatch(surf_core_t *core,
                                      const surf_event_t *event);
surf_core_result_t surf_core_snapshot(const surf_core_t *core,
                                      surf_snapshot_t *out_snapshot);
surf_core_result_t surf_core_semantic_snapshot(
    const surf_core_t *core, surf_semantic_snapshot_t *out_snapshot);
int surf_core_next_effect(surf_core_t *core, surf_effect_t *out_effect);
uint64_t surf_core_connection_generation(const surf_core_t *core);
uint64_t surf_core_stale_event_count(const surf_core_t *core);
const char *surf_core_result_string(surf_core_result_t result);

/* Runtime layout probes used by language bindings to fail fast if their raw
 * declarations drift from the C compiler's ABI. */
uint32_t surf_core_abi_version(void);
size_t surf_core_sizeof_string_view(void);
size_t surf_core_sizeof_config(void);
size_t surf_core_sizeof_event(void);
size_t surf_core_sizeof_snapshot(void);
size_t surf_core_sizeof_semantic_snapshot(void);

#ifdef __cplusplus
}
#endif

#endif
