#ifndef SURF_PROTOCOL_H
#define SURF_PROTOCOL_H

#include "surf/core.h"

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define SURF_PROTOCOL_DEFAULT_TOKENS ((size_t)32768)
#define SURF_PROTOCOL_DEFAULT_ITEMS ((size_t)4096)
#define SURF_PROTOCOL_DEFAULT_STRINGS ((size_t)(2 * 1024 * 1024))
#define SURF_PROTOCOL_MAX_DEPTH ((size_t)32)

typedef enum surf_protocol_result {
    SURF_PROTOCOL_OK = 0,
    SURF_PROTOCOL_ERROR_ARGUMENT,
    SURF_PROTOCOL_ERROR_JSON,
    SURF_PROTOCOL_ERROR_KIND,
    SURF_PROTOCOL_ERROR_FIELD,
    SURF_PROTOCOL_ERROR_TYPE,
    SURF_PROTOCOL_ERROR_LIMIT,
    SURF_PROTOCOL_ERROR_BUFFER
} surf_protocol_result_t;

typedef struct surf_protocol_workspace_config {
    size_t token_capacity;
    size_t item_capacity;
    size_t string_capacity;
} surf_protocol_workspace_config_t;

typedef struct surf_protocol_workspace surf_protocol_workspace_t;

void surf_protocol_workspace_config_init(
    surf_protocol_workspace_config_t *config);
size_t surf_protocol_workspace_size(
    const surf_protocol_workspace_config_t *config);
surf_protocol_result_t surf_protocol_workspace_init(
    surf_protocol_workspace_t **out_workspace, void *memory, size_t memory_size,
    const surf_protocol_workspace_config_t *config);

/* Event views borrow the workspace arena and remain valid until its next
 * decode. `memory` must have ordinary malloc alignment. A smaller custom
 * configuration is useful on constrained hosts that enforce lower limits. */

typedef struct surf_protocol_causal {
    uint64_t interaction_id;
    uint64_t client_ns;
} surf_protocol_causal_t;

typedef struct surf_protocol_touch_point {
    int32_t id;
    double x;
    double y;
    double radius_x;
    double radius_y;
    double force;
} surf_protocol_touch_point_t;

typedef struct surf_protocol_library_entry {
    surf_string_view_t url;
    surf_string_view_t title;
    int64_t timestamp;
} surf_protocol_library_entry_t;

typedef struct surf_protocol_download_item {
    surf_string_view_t name;
    int64_t size;
    int64_t timestamp;
} surf_protocol_download_item_t;

typedef struct surf_protocol_select_option {
    surf_string_view_t label;
    int disabled;
    int selected;
} surf_protocol_select_option_t;

typedef enum surf_protocol_event_kind {
    SURF_PROTOCOL_EVENT_HELLO = 1,
    SURF_PROTOCOL_EVENT_TABS,
    SURF_PROTOCOL_EVENT_VIDEO_CONFIG,
    SURF_PROTOCOL_EVENT_AUDIO_CONFIG,
    SURF_PROTOCOL_EVENT_LOADING,
    SURF_PROTOCOL_EVENT_FULLSCREEN,
    SURF_PROTOCOL_EVENT_FOUND,
    SURF_PROTOCOL_EVENT_STARRED,
    SURF_PROTOCOL_EVENT_TOAST,
    SURF_PROTOCOL_EVENT_CLOCK,
    SURF_PROTOCOL_EVENT_URL,
    SURF_PROTOCOL_EVENT_HISTORY_STATE,
    SURF_PROTOCOL_EVENT_DOWNLOAD,
    SURF_PROTOCOL_EVENT_DOWNLOAD_PROGRESS,
    SURF_PROTOCOL_EVENT_SUGGEST,
    SURF_PROTOCOL_EVENT_LIBRARY,
    SURF_PROTOCOL_EVENT_HISTORY,
    SURF_PROTOCOL_EVENT_DOWNLOADS,
    SURF_PROTOCOL_EVENT_DIALOG,
    SURF_PROTOCOL_EVENT_DIALOG_DONE,
    SURF_PROTOCOL_EVENT_FILE_CHOOSER,
    SURF_PROTOCOL_EVENT_SECURITY,
    SURF_PROTOCOL_EVENT_PAGE_ERROR,
    SURF_PROTOCOL_EVENT_READER,
    SURF_PROTOCOL_EVENT_EDITABLE,
    SURF_PROTOCOL_EVENT_SELECT,
    SURF_PROTOCOL_EVENT_MEDIA_STATE,
    SURF_PROTOCOL_EVENT_PAGE_FRAME,
    SURF_PROTOCOL_EVENT_CLIPBOARD,
    SURF_PROTOCOL_EVENT_CLIPBOARD_SYNC,
    SURF_PROTOCOL_EVENT_LOG_REQUEST,
    SURF_PROTOCOL_EVENT_LOG_CLEAR,
    SURF_PROTOCOL_EVENT_BROWSER_MODE
} surf_protocol_event_kind_t;

typedef struct surf_protocol_event {
    surf_protocol_event_kind_t kind;
    union {
        struct {
            surf_string_view_t state, host, message;
            uint64_t revision;
            int standalone, can_force;
        } browser_mode;
        struct { int32_t width, height; } viewport;
        struct { const surf_tab_event_t *items; size_t count; } tabs;
        struct {
            surf_string_view_t state, reason, profile;
            int32_t width, height;
            uint32_t generation;
        } video;
        struct { int ok; int32_t rate, channels; } audio;
        struct { int on; } boolean;
        struct { surf_string_view_t text; } text;
        struct { uint64_t c0, s1, s2; } clock;
        struct {
            surf_string_view_t url, security;
            int starred;
        } url;
        struct { int back, forward; } history_state;
        struct { surf_string_view_t name; } name;
        struct { surf_string_view_t name; int32_t percent; } progress;
        struct {
            const surf_protocol_library_entry_t *items;
            size_t count;
        } entries;
        struct {
            const surf_protocol_library_entry_t *history;
            size_t history_count;
            const surf_protocol_library_entry_t *bookmarks;
            size_t bookmark_count;
            int starred;
        } library;
        struct {
            surf_string_view_t query;
            const surf_protocol_library_entry_t *items;
            size_t count;
            int32_t offset, total;
        } history;
        struct {
            const surf_protocol_download_item_t *items;
            size_t count;
        } downloads;
        struct {
            surf_string_view_t kind, text, default_text;
        } dialog;
        struct { int multiple; } chooser;
        struct { surf_string_view_t state; } security;
        struct {
            int ok;
            surf_string_view_t title, html, url;
        } reader;
        struct {
            int on, show_keyboard, has_rect;
            surf_string_view_t kind;
            double rect[4];
        } editable;
        struct {
            surf_string_view_t id, title;
            int multiple, has_rect;
            const surf_protocol_select_option_t *options;
            size_t option_count;
            double rect[4];
        } select;
        struct {
            int available, paused, muted;
            int32_t count;
            double volume, current_time, duration;
            surf_string_view_t title;
        } media;
        struct { uint32_t source_sequence; } page_frame;
        struct { surf_string_view_t id, text; int sync; } clipboard;
        struct { int enabled, known; surf_string_view_t text; } clipboard_sync;
    } data;
} surf_protocol_event_t;

surf_protocol_result_t surf_protocol_decode_event(
    surf_protocol_workspace_t *workspace, const char *json, size_t length,
    surf_protocol_event_t *out_event);

typedef enum surf_protocol_command_kind {
    SURF_PROTOCOL_COMMAND_SIZE = 1,
    SURF_PROTOCOL_COMMAND_CLOCK,
    SURF_PROTOCOL_COMMAND_TAB,
    SURF_PROTOCOL_COMMAND_NAVIGATE,
    SURF_PROTOCOL_COMMAND_OPEN_NEW,
    SURF_PROTOCOL_COMMAND_HISTORY_DELETE,
    SURF_PROTOCOL_COMMAND_BOOKMARK_DELETE,
    SURF_PROTOCOL_COMMAND_AUDIO,
    SURF_PROTOCOL_COMMAND_MOBILE,
    SURF_PROTOCOL_COMMAND_DARK,
    SURF_PROTOCOL_COMMAND_FULLSCREEN,
    SURF_PROTOCOL_COMMAND_TOUCH,
    SURF_PROTOCOL_COMMAND_POINTER,
    SURF_PROTOCOL_COMMAND_WHEEL,
    SURF_PROTOCOL_COMMAND_KEY,
    SURF_PROTOCOL_COMMAND_PASTE,
    SURF_PROTOCOL_COMMAND_COMPOSE,
    SURF_PROTOCOL_COMMAND_SUGGEST,
    SURF_PROTOCOL_COMMAND_HISTORY,
    SURF_PROTOCOL_COMMAND_FIND,
    SURF_PROTOCOL_COMMAND_DOWNLOAD_DELETE,
    SURF_PROTOCOL_COMMAND_CLEAR,
    SURF_PROTOCOL_COMMAND_DIALOG_REPLY,
    SURF_PROTOCOL_COMMAND_SELECT_REPLY,
    SURF_PROTOCOL_COMMAND_MEDIA_STATS,
    SURF_PROTOCOL_COMMAND_MEDIA_VOLUME,
    SURF_PROTOCOL_COMMAND_CLIPBOARD_RESULT,
    SURF_PROTOCOL_COMMAND_CLIPBOARD_CHANGE,
    SURF_PROTOCOL_COMMAND_LOG_RECORD,
    SURF_PROTOCOL_COMMAND_LOG_CLEARED,
    SURF_PROTOCOL_COMMAND_BACK,
    SURF_PROTOCOL_COMMAND_FORWARD,
    SURF_PROTOCOL_COMMAND_RELOAD,
    SURF_PROTOCOL_COMMAND_STOP,
    SURF_PROTOCOL_COMMAND_VIDEO_RETRY,
    SURF_PROTOCOL_COMMAND_REQUEST_KEYFRAME,
    SURF_PROTOCOL_COMMAND_LIBRARY,
    SURF_PROTOCOL_COMMAND_BOOKMARK,
    SURF_PROTOCOL_COMMAND_DOWNLOADS,
    SURF_PROTOCOL_COMMAND_READER,
    SURF_PROTOCOL_COMMAND_MEDIA_PLAY_PAUSE,
    SURF_PROTOCOL_COMMAND_MEDIA_MUTE,
    SURF_PROTOCOL_COMMAND_MEDIA_QUERY,
    SURF_PROTOCOL_COMMAND_BROWSER_WATCH,
    SURF_PROTOCOL_COMMAND_BROWSER_RESUME
} surf_protocol_command_kind_t;

typedef struct surf_protocol_command {
    surf_protocol_command_kind_t kind;
    surf_protocol_causal_t causal;
    union {
        struct { uint64_t revision; int force; } browser_resume;
        struct { int32_t width, height; } size;
        struct { uint64_t client_send_ns; } clock;
        struct { surf_string_view_t action; int32_t id; } tab;
        struct { surf_string_view_t url; } url;
        struct { surf_string_view_t url; int64_t timestamp; } history_delete;
        struct { int on; } toggle;
        struct {
            surf_string_view_t phase;
            uint64_t sequence, timestamp_ns;
            uint32_t surface;
            const surf_protocol_touch_point_t *points;
            size_t point_count;
        } touch;
        struct {
            surf_string_view_t phase, button;
            uint64_t sequence, timestamp_ns;
            uint32_t surface;
            double x, y;
            int32_t buttons, modifiers, clicks;
        } pointer;
        struct {
            uint64_t sequence, timestamp_ns;
            uint32_t surface;
            double x, y, delta_x, delta_y;
            int32_t buttons, modifiers;
        } wheel;
        struct {
            int down;
            surf_string_view_t key, code, text;
            int32_t key_code, modifiers;
        } key;
        struct { surf_string_view_t text; } text;
        struct {
            surf_string_view_t phase, text;
            int32_t start, end;
        } compose;
        struct { surf_string_view_t query; int32_t offset; } query;
        struct { surf_string_view_t query; int32_t direction; } find;
        struct { surf_string_view_t name; } name;
        struct { surf_string_view_t what; } clear;
        struct { int accept; surf_string_view_t text; } dialog_reply;
        struct {
            surf_string_view_t id;
            int cancel;
            const int32_t *indices;
            size_t index_count;
        } select_reply;
        struct {
            double fps, presented_fps, decode_fps, au_rate;
            surf_string_view_t renderer;
            double renderer_fps, renderer_ms;
            int32_t renderer_backpressure, renderer_recoveries;
            int32_t renderer_failures;
            double callback_ms, gap_ms, frame_age_ms, window_ms, drop_percent;
            int32_t queue_depth, decode_errors;
            int memory_warn;
        } media_stats;
        struct { double value; } volume;
        struct { surf_string_view_t id; int ok; } clipboard_result;
        struct { surf_string_view_t json; } log_record;
    } data;
} surf_protocol_command_t;

surf_protocol_result_t surf_protocol_encode_command(
    const surf_protocol_command_t *command, char *buffer, size_t capacity,
    size_t *out_length);
/* `out_length` always receives the required byte count. A non-NUL-terminated
 * exact-capacity buffer is valid; pass one extra byte when a C string is
 * desired. SURF_PROTOCOL_ERROR_BUFFER means the supplied capacity was short. */

const char *surf_protocol_result_string(surf_protocol_result_t result);

#ifdef __cplusplus
}
#endif

#endif
