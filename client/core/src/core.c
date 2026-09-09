#include "surf/core.h"
#include "surf/protocol.h"

#include <stdlib.h>
#include <string.h>

#define SURF_MAX_URL_BYTES ((size_t)8192)
#define SURF_MAX_TITLE_BYTES ((size_t)2048)
#define SURF_MAX_ICON_BYTES ((size_t)8192)
#define SURF_MAX_SHORT_TEXT_BYTES ((size_t)128)
#define SURF_MAX_CONTENT_BYTES ((size_t)(1024 * 1024))
#define SURF_EFFECT_CAPACITY ((size_t)16)

typedef struct surf_owned_string {
    char *data;
    size_t length;
} surf_owned_string_t;

typedef struct surf_owned_tab {
    int64_t id;
    surf_owned_string_t title;
    surf_owned_string_t url;
    surf_owned_string_t icon;
    int active;
} surf_owned_tab_t;

struct surf_core {
    surf_allocator_t allocator;
    size_t max_tabs;
    surf_owned_tab_t *tabs;
    surf_tab_snapshot_t *tab_views;
    size_t tab_count;
    int64_t active_tab_id;
    surf_owned_string_t active_title;
    surf_owned_string_t current_url;
    surf_owned_string_t security;
    surf_owned_string_t editable_kind;
    surf_owned_string_t dialog_kind;
    surf_owned_string_t dialog_text;
    surf_owned_string_t dialog_default_text;
    surf_owned_string_t select_id;
    surf_owned_string_t select_title;
    surf_owned_string_t clipboard_id;
    surf_owned_string_t clipboard_text;
    surf_owned_string_t reader_title;
    surf_owned_string_t reader_url;
    surf_owned_string_t media_title;
    surf_owned_string_t page_error;
    surf_owned_string_t toast_text;
    surf_owned_string_t download_name;
    surf_owned_string_t history_query;
    surf_owned_string_t browser_mode, browser_host, browser_message;
    uint64_t browser_revision;
    int browser_paused, browser_standalone, browser_can_force;
    double editable_rect[4];
    uint64_t revision;
    uint64_t connection_generation;
    uint64_t stale_event_count;
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
    surf_effect_t effects[SURF_EFFECT_CAPACITY];
    size_t effect_head;
    size_t effect_count;
};

static void *surf_default_allocate(void *context, size_t size) {
    (void)context;
    return malloc(size);
}

static void surf_default_deallocate(void *context, void *pointer) {
    (void)context;
    free(pointer);
}

static void *surf_allocate(const surf_core_t *core, size_t size) {
    if (size == 0) {
        return NULL;
    }
    return core->allocator.allocate(core->allocator.context, size);
}

static void surf_deallocate(const surf_core_t *core, void *pointer) {
    if (pointer != NULL) {
        core->allocator.deallocate(core->allocator.context, pointer);
    }
}

static void surf_owned_string_clear(const surf_core_t *core,
                                    surf_owned_string_t *string) {
    surf_deallocate(core, string->data);
    string->data = NULL;
    string->length = 0;
}

static surf_core_result_t surf_owned_string_create(
    const surf_core_t *core, surf_string_view_t source, size_t maximum,
    surf_owned_string_t *out_string) {
    char *copy;
    out_string->data = NULL;
    out_string->length = 0;
    if ((source.data == NULL && source.length != 0) || source.length > maximum) {
        return SURF_CORE_ERROR_LIMIT;
    }
    if (source.length == 0) {
        return SURF_CORE_OK;
    }
    if (source.length == SIZE_MAX) {
        return SURF_CORE_ERROR_LIMIT;
    }
    copy = (char *)surf_allocate(core, source.length + 1);
    if (copy == NULL) {
        return SURF_CORE_ERROR_ALLOCATE;
    }
    memcpy(copy, source.data, source.length);
    copy[source.length] = '\0';
    out_string->data = copy;
    out_string->length = source.length;
    return SURF_CORE_OK;
}

static surf_core_result_t surf_owned_string_replace(
    surf_core_t *core, surf_owned_string_t *destination,
    surf_string_view_t source, size_t maximum) {
    surf_owned_string_t next;
    surf_core_result_t result = surf_owned_string_create(
        core, source, maximum, &next);
    if (result != SURF_CORE_OK) {
        return result;
    }
    surf_owned_string_clear(core, destination);
    *destination = next;
    return SURF_CORE_OK;
}

static surf_core_result_t surf_owned_string_group_replace(
    surf_core_t *core, surf_owned_string_t **destinations,
    const surf_string_view_t *sources, size_t count, size_t maximum) {
    surf_owned_string_t next[3];
    size_t index;
    surf_core_result_t result = SURF_CORE_OK;
    if (count > sizeof(next) / sizeof(next[0])) return SURF_CORE_ERROR_ARGUMENT;
    memset(next, 0, sizeof(next));
    for (index = 0; index < count; index++) {
        result = surf_owned_string_create(core, sources[index], maximum,
                                          &next[index]);
        if (result != SURF_CORE_OK) break;
    }
    if (result != SURF_CORE_OK) {
        size_t clear_index;
        for (clear_index = 0; clear_index < count; clear_index++)
            surf_owned_string_clear(core, &next[clear_index]);
        return result;
    }
    for (index = 0; index < count; index++) {
        surf_owned_string_clear(core, destinations[index]);
        *destinations[index] = next[index];
    }
    return SURF_CORE_OK;
}

static surf_string_view_t surf_owned_string_view(
    const surf_owned_string_t *string) {
    return surf_string_view(string->data, string->length);
}

static int surf_string_starts_with(surf_string_view_t string,
                                   const char *prefix) {
    size_t prefix_length = strlen(prefix);
    return string.length >= prefix_length && string.data != NULL &&
           memcmp(string.data, prefix, prefix_length) == 0;
}

static surf_string_view_t surf_url_hostname(surf_string_view_t url) {
    size_t index;
    size_t authority_start = 0;
    size_t authority_end;
    size_t host_start;
    size_t host_end;
    size_t last_at = SIZE_MAX;
    if (url.data == NULL || url.length == 0) {
        return surf_string_view(NULL, 0);
    }
    for (index = 0; index + 2 < url.length; index++) {
        if (url.data[index] == ':' && url.data[index + 1] == '/' &&
            url.data[index + 2] == '/') {
            authority_start = index + 3;
            break;
        }
    }
    if (authority_start == 0 || authority_start >= url.length) {
        return surf_string_view(NULL, 0);
    }
    authority_end = url.length;
    for (index = authority_start; index < url.length; index++) {
        char character = url.data[index];
        if (character == '/' || character == '?' || character == '#') {
            authority_end = index;
            break;
        }
        if (character == '@') {
            last_at = index;
        }
    }
    host_start = last_at == SIZE_MAX ? authority_start : last_at + 1;
    if (host_start >= authority_end) {
        return surf_string_view(NULL, 0);
    }
    if (url.data[host_start] == '[') {
        host_start++;
        host_end = host_start;
        while (host_end < authority_end && url.data[host_end] != ']') {
            host_end++;
        }
        if (host_end == authority_end) {
            return surf_string_view(NULL, 0);
        }
    } else {
        host_end = authority_end;
        for (index = host_start; index < authority_end; index++) {
            if (url.data[index] == ':') {
                host_end = index;
                break;
            }
        }
    }
    if (host_end <= host_start) {
        return surf_string_view(NULL, 0);
    }
    return surf_string_view(url.data + host_start, host_end - host_start);
}

static void surf_owned_tabs_clear(const surf_core_t *core,
                                  surf_owned_tab_t *tabs, size_t count,
                                  surf_tab_snapshot_t *views) {
    size_t index;
    if (tabs != NULL) {
        for (index = 0; index < count; index++) {
            surf_owned_string_clear(core, &tabs[index].title);
            surf_owned_string_clear(core, &tabs[index].url);
            surf_owned_string_clear(core, &tabs[index].icon);
        }
    }
    surf_deallocate(core, tabs);
    surf_deallocate(core, views);
}

static void surf_refresh_tab_views(surf_core_t *core) {
    size_t index;
    for (index = 0; index < core->tab_count; index++) {
        core->tab_views[index].id = core->tabs[index].id;
        core->tab_views[index].title =
            surf_owned_string_view(&core->tabs[index].title);
        core->tab_views[index].url =
            surf_owned_string_view(&core->tabs[index].url);
        core->tab_views[index].icon =
            surf_owned_string_view(&core->tabs[index].icon);
        core->tab_views[index].active = core->tabs[index].active;
    }
}

static surf_core_result_t surf_queue_effect(surf_core_t *core,
                                            surf_effect_kind_t kind) {
    surf_effect_t retained[SURF_EFFECT_CAPACITY];
    size_t retained_count = 0;
    size_t offset;
    size_t index;
    for (offset = 0; offset < core->effect_count; offset++) {
        surf_effect_t existing =
            core->effects[(core->effect_head + offset) % SURF_EFFECT_CAPACITY];
        int both_keyboard =
            (kind == SURF_EFFECT_SHOW_KEYBOARD ||
             kind == SURF_EFFECT_HIDE_KEYBOARD) &&
            (existing.kind == SURF_EFFECT_SHOW_KEYBOARD ||
             existing.kind == SURF_EFFECT_HIDE_KEYBOARD);
        if (both_keyboard) {
            continue;
        }
        if (existing.kind == kind) {
            return SURF_CORE_OK;
        }
        retained[retained_count++] = existing;
    }
    memcpy(core->effects, retained, retained_count * sizeof(retained[0]));
    core->effect_head = 0;
    core->effect_count = retained_count;
    if (core->effect_count >= SURF_EFFECT_CAPACITY) {
        return SURF_CORE_ERROR_LIMIT;
    }
    index = (core->effect_head + core->effect_count) % SURF_EFFECT_CAPACITY;
    core->effects[index].kind = kind;
    core->effect_count++;
    return SURF_CORE_OK;
}

static void surf_clear_semantic_transients(surf_core_t *core) {
    surf_owned_string_clear(core, &core->dialog_kind);
    surf_owned_string_clear(core, &core->dialog_text);
    surf_owned_string_clear(core, &core->dialog_default_text);
    surf_owned_string_clear(core, &core->select_id);
    surf_owned_string_clear(core, &core->select_title);
    surf_owned_string_clear(core, &core->clipboard_id);
    surf_owned_string_clear(core, &core->reader_title);
    surf_owned_string_clear(core, &core->reader_url);
    surf_owned_string_clear(core, &core->media_title);
    surf_owned_string_clear(core, &core->page_error);
    core->dialog_active = 0;
    core->select_active = 0;
    core->select_multiple = 0;
    core->select_option_count = 0;
    core->file_chooser_active = 0;
    core->file_chooser_multiple = 0;
    core->find_known = 0;
    core->find_found = 0;
    core->suggestion_count = 0;
    core->clipboard_pending = 0;
    core->clipboard_sync_request = 0;
    core->reader_available = 0;
    core->media_available = 0;
    core->media_count = 0;
    core->media_paused = 0;
    core->media_muted = 0;
    core->media_volume = 0.0;
    core->media_current_time = 0.0;
    core->media_duration = 0.0;
}

static void surf_clear_page_transients(surf_core_t *core) {
    core->loading = 0;
    core->editable = 0;
    core->editable_has_rect = 0;
    memset(core->editable_rect, 0, sizeof(core->editable_rect));
    core->awaiting_page_frame = 0;
    core->awaited_source_sequence = 0;
    surf_owned_string_clear(core, &core->editable_kind);
    surf_clear_semantic_transients(core);
}

static void surf_clear_browser_state(surf_core_t *core) {
    surf_owned_tabs_clear(core, core->tabs, core->tab_count, core->tab_views);
    core->tabs = NULL;
    core->tab_views = NULL;
    core->tab_count = 0;
    core->active_tab_id = 0;
    core->has_active_tab = 0;
    surf_owned_string_clear(core, &core->active_title);
    surf_owned_string_clear(core, &core->current_url);
    surf_owned_string_clear(core, &core->security);
    surf_clear_page_transients(core);
    core->show_start_page = 1;
    core->can_go_back = 0;
    core->can_go_forward = 0;
    core->starred = 0;
    core->fullscreen = 0;
    core->keyboard_visible = 0;
    core->effect_head = 0;
    core->effect_count = 0;
    surf_owned_string_clear(core, &core->toast_text);
    surf_owned_string_clear(core, &core->download_name);
    surf_owned_string_clear(core, &core->history_query);
    surf_owned_string_clear(core, &core->browser_mode);
    surf_owned_string_clear(core, &core->browser_host);
    surf_owned_string_clear(core, &core->browser_message);
    core->browser_revision = 0;
    core->browser_paused = core->browser_standalone = core->browser_can_force = 0;
    surf_owned_string_clear(core, &core->clipboard_text);
    core->history_count = 0;
    core->bookmark_count = 0;
    core->download_count = 0;
    core->history_offset = 0;
    core->history_total = 0;
    core->download_percent = 0;
    core->clipboard_sync_enabled = 0;
    core->clipboard_known = 0;
    core->video_generation = 0;
}

static surf_core_result_t surf_dispatch_tabs(surf_core_t *core,
                                             const surf_event_t *event) {
    surf_owned_tab_t *tabs = NULL;
    surf_tab_snapshot_t *views = NULL;
    surf_owned_string_t active_title;
    size_t index;
    size_t active_count = 0;
    int64_t active_id = 0;
    int active_changed;
    int had_active_page;
    surf_core_result_t result;

    memset(&active_title, 0, sizeof(active_title));
    if (event->data.tabs.count > core->max_tabs ||
        (event->data.tabs.count != 0 && event->data.tabs.items == NULL)) {
        return SURF_CORE_ERROR_LIMIT;
    }
    if (event->data.tabs.count != 0) {
        if (event->data.tabs.count > SIZE_MAX / sizeof(*tabs) ||
            event->data.tabs.count > SIZE_MAX / sizeof(*views)) {
            return SURF_CORE_ERROR_LIMIT;
        }
        tabs = (surf_owned_tab_t *)surf_allocate(
            core, event->data.tabs.count * sizeof(*tabs));
        views = (surf_tab_snapshot_t *)surf_allocate(
            core, event->data.tabs.count * sizeof(*views));
        if (tabs == NULL || views == NULL) {
            surf_deallocate(core, tabs);
            surf_deallocate(core, views);
            return SURF_CORE_ERROR_ALLOCATE;
        }
        memset(tabs, 0, event->data.tabs.count * sizeof(*tabs));
        memset(views, 0, event->data.tabs.count * sizeof(*views));
    }

    for (index = 0; index < event->data.tabs.count; index++) {
        const surf_tab_event_t *source = &event->data.tabs.items[index];
        size_t previous;
        for (previous = 0; previous < index; previous++) {
            if (tabs[previous].id == source->id) {
                surf_owned_string_clear(core, &active_title);
                surf_owned_tabs_clear(core, tabs, event->data.tabs.count,
                                      views);
                return SURF_CORE_ERROR_STATE;
            }
        }
        tabs[index].id = source->id;
        tabs[index].active = source->active != 0;
        result = surf_owned_string_create(core, source->title,
                                          SURF_MAX_TITLE_BYTES,
                                          &tabs[index].title);
        if (result == SURF_CORE_OK) {
            result = surf_owned_string_create(core, source->url,
                                              SURF_MAX_URL_BYTES,
                                              &tabs[index].url);
        }
        if (result == SURF_CORE_OK) {
            result = surf_owned_string_create(core, source->icon,
                                              SURF_MAX_ICON_BYTES,
                                              &tabs[index].icon);
        }
        if (result != SURF_CORE_OK) {
            surf_owned_tabs_clear(core, tabs, event->data.tabs.count, views);
            return result;
        }
        if (tabs[index].active) {
            surf_string_view_t title = source->title;
            active_count++;
            if (active_count > 1) {
                surf_owned_string_clear(core, &active_title);
                surf_owned_tabs_clear(core, tabs, event->data.tabs.count,
                                      views);
                return SURF_CORE_ERROR_STATE;
            }
            active_id = tabs[index].id;
            if (surf_string_starts_with(source->url,
                                        "about:blank#surf-new") ||
                surf_string_starts_with(title, "about:blank#surf-new")) {
                title = surf_string_from_cstr("New Tab");
            } else if (title.length == 0 ||
                       surf_string_equal(title, source->url)) {
                title = surf_url_hostname(source->url);
                if (title.length == 0) {
                    title = surf_string_from_cstr("New Tab");
                }
            }
            result = surf_owned_string_create(core, title,
                                              SURF_MAX_TITLE_BYTES,
                                              &active_title);
            if (result != SURF_CORE_OK) {
                surf_owned_tabs_clear(core, tabs, event->data.tabs.count,
                                      views);
                return result;
            }
        }
    }
    active_changed = core->has_active_tab != (active_count == 1) ||
                     (active_count == 1 && core->active_tab_id != active_id);
    had_active_page = core->has_active_tab;
    surf_owned_tabs_clear(core, core->tabs, core->tab_count, core->tab_views);
    core->tabs = tabs;
    core->tab_views = views;
    core->tab_count = event->data.tabs.count;
    core->active_tab_id = active_id;
    core->has_active_tab = active_count == 1;
    surf_owned_string_clear(core, &core->active_title);
    core->active_title = active_title;
    surf_refresh_tab_views(core);
    if (active_changed) {
        surf_clear_page_transients(core);
        if (had_active_page) {
            result = surf_queue_effect(core,
                                       SURF_EFFECT_CLEAR_PAGE_PRESENTATION);
            if (result != SURF_CORE_OK) {
                return result;
            }
        }
        if (core->keyboard_visible) {
            result = surf_queue_effect(core, SURF_EFFECT_HIDE_KEYBOARD);
            if (result != SURF_CORE_OK) {
                return result;
            }
        }
    }
    return SURF_CORE_OK;
}

static surf_core_result_t surf_dispatch_url(surf_core_t *core,
                                            const surf_event_t *event) {
    surf_owned_string_t next_url;
    surf_owned_string_t next_security;
    int new_tab;
    int page_changed;
    surf_core_result_t result;
    memset(&next_url, 0, sizeof(next_url));
    memset(&next_security, 0, sizeof(next_security));
    result = surf_owned_string_create(core, event->data.url.url,
                                      SURF_MAX_URL_BYTES, &next_url);
    if (result != SURF_CORE_OK) {
        return result;
    }
    result = surf_owned_string_create(core, event->data.url.security,
                                      SURF_MAX_SHORT_TEXT_BYTES,
                                      &next_security);
    if (result != SURF_CORE_OK) {
        surf_owned_string_clear(core, &next_url);
        return result;
    }
    page_changed = core->current_url.length != 0 &&
                   !surf_string_equal(surf_owned_string_view(&core->current_url),
                                      event->data.url.url);
    if (page_changed) {
        surf_clear_semantic_transients(core);
        result = surf_queue_effect(core,
                                   SURF_EFFECT_CLEAR_PAGE_PRESENTATION);
        if (result != SURF_CORE_OK) {
            surf_owned_string_clear(core, &next_url);
            surf_owned_string_clear(core, &next_security);
            return result;
        }
    }
    surf_owned_string_clear(core, &core->current_url);
    surf_owned_string_clear(core, &core->security);
    core->current_url = next_url;
    core->security = next_security;
    core->starred = event->data.url.starred != 0;
    new_tab = surf_string_starts_with(event->data.url.url,
                                      "about:blank#surf-new");
    core->show_start_page = new_tab;
    if (new_tab) {
        core->loading = 0;
        core->awaiting_page_frame = 0;
        core->awaited_source_sequence = 0;
        return surf_queue_effect(core, SURF_EFFECT_REQUEST_LIBRARY);
    }
    return SURF_CORE_OK;
}

static surf_core_result_t surf_dispatch_editable(surf_core_t *core,
                                                 const surf_event_t *event) {
    surf_core_result_t result;
    if (!event->data.editable.on) {
        core->editable = 0;
        core->editable_has_rect = 0;
        surf_owned_string_clear(core, &core->editable_kind);
        if (core->keyboard_visible) {
            return surf_queue_effect(core, SURF_EFFECT_HIDE_KEYBOARD);
        }
        return SURF_CORE_OK;
    }
    result = surf_owned_string_replace(core, &core->editable_kind,
                                       event->data.editable.kind,
                                       SURF_MAX_SHORT_TEXT_BYTES);
    if (result != SURF_CORE_OK) {
        return result;
    }
    core->editable = 1;
    core->editable_has_rect = event->data.editable.has_rect != 0;
    if (core->editable_has_rect) {
        memcpy(core->editable_rect, event->data.editable.rect,
               sizeof(core->editable_rect));
    } else {
        memset(core->editable_rect, 0, sizeof(core->editable_rect));
    }
    if (event->data.editable.show_keyboard || core->keyboard_visible) {
        return surf_queue_effect(core, SURF_EFFECT_SHOW_KEYBOARD);
    }
    return SURF_CORE_OK;
}

surf_string_view_t surf_string_view(const char *data, size_t length) {
    surf_string_view_t view;
    view.data = data;
    view.length = length;
    return view;
}

surf_string_view_t surf_string_from_cstr(const char *string) {
    if (string == NULL) {
        return surf_string_view(NULL, 0);
    }
    return surf_string_view(string, strlen(string));
}

int surf_string_equal(surf_string_view_t left, surf_string_view_t right) {
    if (left.length != right.length) {
        return 0;
    }
    if (left.length == 0) {
        return 1;
    }
    return left.data != NULL && right.data != NULL &&
           memcmp(left.data, right.data, left.length) == 0;
}

void surf_core_config_init(surf_core_config_t *config) {
    if (config == NULL) {
        return;
    }
    memset(config, 0, sizeof(*config));
    config->abi_version = SURF_CORE_ABI_VERSION;
    config->max_tabs = SURF_CORE_DEFAULT_MAX_TABS;
}

surf_core_result_t surf_core_create(const surf_core_config_t *config,
                                    surf_core_t **out_core) {
    surf_core_config_t resolved;
    surf_allocator_t allocator;
    surf_core_t *core;
    if (out_core == NULL) {
        return SURF_CORE_ERROR_ARGUMENT;
    }
    *out_core = NULL;
    surf_core_config_init(&resolved);
    if (config != NULL) {
        resolved = *config;
    }
    if (resolved.abi_version != SURF_CORE_ABI_VERSION) {
        return SURF_CORE_ERROR_ABI;
    }
    if (resolved.max_tabs == 0) {
        resolved.max_tabs = SURF_CORE_DEFAULT_MAX_TABS;
    }
    if (resolved.max_tabs > SURF_CORE_HARD_MAX_TABS) {
        return SURF_CORE_ERROR_LIMIT;
    }
    allocator = resolved.allocator;
    if (allocator.allocate == NULL && allocator.deallocate == NULL) {
        allocator.allocate = surf_default_allocate;
        allocator.deallocate = surf_default_deallocate;
    } else if (allocator.allocate == NULL || allocator.deallocate == NULL) {
        return SURF_CORE_ERROR_ARGUMENT;
    }
    core = (surf_core_t *)allocator.allocate(allocator.context, sizeof(*core));
    if (core == NULL) {
        return SURF_CORE_ERROR_ALLOCATE;
    }
    memset(core, 0, sizeof(*core));
    core->allocator = allocator;
    core->max_tabs = resolved.max_tabs;
    core->show_start_page = 1;
    core->revision = 1;
    core->connection_generation = 1;
    *out_core = core;
    return SURF_CORE_OK;
}

void surf_core_destroy(surf_core_t *core) {
    surf_allocator_t allocator;
    if (core == NULL) {
        return;
    }
    allocator = core->allocator;
    surf_clear_browser_state(core);
    memset(core, 0, sizeof(*core));
    allocator.deallocate(allocator.context, core);
}

surf_core_result_t surf_core_dispatch(surf_core_t *core,
                                      const surf_event_t *event) {
    surf_core_result_t result = SURF_CORE_OK;
    if (core == NULL || event == NULL) {
        return SURF_CORE_ERROR_ARGUMENT;
    }
    switch (event->kind) {
    case SURF_EVENT_RESET:
        surf_clear_page_transients(core);
        core->can_go_back = 0;
        core->can_go_forward = 0;
        core->fullscreen = 0;
        core->keyboard_visible = 0;
        core->effect_head = 0;
        core->effect_count = 0;
        break;
    case SURF_EVENT_TABS:
        result = surf_dispatch_tabs(core, event);
        break;
    case SURF_EVENT_URL:
        result = surf_dispatch_url(core, event);
        break;
    case SURF_EVENT_HISTORY_STATE:
        core->can_go_back = event->data.history.can_go_back != 0;
        core->can_go_forward = event->data.history.can_go_forward != 0;
        break;
    case SURF_EVENT_LOADING:
        core->loading = event->data.boolean.on != 0;
        break;
    case SURF_EVENT_EDITABLE:
        result = surf_dispatch_editable(core, event);
        break;
    case SURF_EVENT_FULLSCREEN:
        core->fullscreen = event->data.boolean.on != 0;
        break;
    case SURF_EVENT_SECURITY:
        result = surf_owned_string_replace(core, &core->security,
                                           event->data.security.state,
                                           SURF_MAX_SHORT_TEXT_BYTES);
        break;
    case SURF_EVENT_STARRED:
        core->starred = event->data.boolean.on != 0;
        break;
    case SURF_EVENT_PAGE_FRAME:
        core->awaited_source_sequence =
            event->data.page_frame.source_sequence;
        core->awaiting_page_frame = 1;
        break;
    case SURF_EVENT_FRAME_PRESENTED:
        if (core->awaiting_page_frame &&
            event->data.frame_presented.source_sequence >=
                core->awaited_source_sequence) {
            core->awaiting_page_frame = 0;
            core->show_start_page = 0;
        }
        break;
    case SURF_EVENT_KEYBOARD_VISIBILITY:
        core->keyboard_visible = event->data.boolean.on != 0;
        break;
    default:
        result = SURF_CORE_ERROR_ARGUMENT;
        break;
    }
    if (result == SURF_CORE_OK) {
        core->revision++;
    }
    return result;
}

surf_core_result_t surf_core_begin_connection(surf_core_t *core,
                                              uint64_t generation) {
    if (core == NULL || generation == 0) return SURF_CORE_ERROR_ARGUMENT;
    if (generation <= core->connection_generation) return SURF_CORE_ERROR_STATE;
    surf_clear_browser_state(core);
    core->connection_generation = generation;
    core->revision++;
    return SURF_CORE_OK;
}

surf_core_result_t surf_core_dispatch_scoped(surf_core_t *core,
                                             uint64_t generation,
                                             const surf_event_t *event) {
    if (core == NULL || event == NULL || generation == 0)
        return SURF_CORE_ERROR_ARGUMENT;
    if (generation != core->connection_generation) {
        core->stale_event_count++;
        return SURF_CORE_OK;
    }
    return surf_core_dispatch(core, event);
}

static surf_core_result_t surf_dispatch_protocol_strings(
    surf_core_t *core, surf_owned_string_t **destinations,
    const surf_string_view_t *sources, size_t count, size_t maximum) {
    surf_core_result_t result = surf_owned_string_group_replace(
        core, destinations, sources, count, maximum);
    if (result == SURF_CORE_OK) core->revision++;
    return result;
}

surf_core_result_t surf_core_dispatch_protocol(
    surf_core_t *core, uint64_t generation,
    const struct surf_protocol_event *opaque_event) {
    const surf_protocol_event_t *event =
        (const surf_protocol_event_t *)opaque_event;
    surf_event_t mapped;
    surf_owned_string_t *destinations[3];
    surf_string_view_t sources[3];
    surf_core_result_t result;
    if (core == NULL || event == NULL || generation == 0)
        return SURF_CORE_ERROR_ARGUMENT;
    if (generation != core->connection_generation) {
        core->stale_event_count++;
        return SURF_CORE_OK;
    }
    memset(&mapped, 0, sizeof(mapped));
    switch (event->kind) {
    case SURF_PROTOCOL_EVENT_TABS:
        mapped.kind = SURF_EVENT_TABS;
        mapped.data.tabs.items = event->data.tabs.items;
        mapped.data.tabs.count = event->data.tabs.count;
        return surf_core_dispatch_scoped(core, generation, &mapped);
    case SURF_PROTOCOL_EVENT_BROWSER_MODE:
        if (event->data.browser_mode.revision < core->browser_revision) return SURF_CORE_OK;
        destinations[0] = &core->browser_mode;
        destinations[1] = &core->browser_host;
        destinations[2] = &core->browser_message;
        sources[0] = event->data.browser_mode.state;
        sources[1] = event->data.browser_mode.host;
        sources[2] = event->data.browser_mode.message;
        result = surf_owned_string_group_replace(core, destinations, sources, 3, SURF_MAX_TITLE_BYTES);
        if (result != SURF_CORE_OK) return result;
        core->browser_revision = event->data.browser_mode.revision;
        core->browser_paused = !(sources[0].length == 9 && memcmp(sources[0].data, "streaming", 9) == 0);
        core->browser_standalone = event->data.browser_mode.standalone;
        core->browser_can_force = event->data.browser_mode.can_force;
        if (core->browser_paused) {
            surf_clear_page_transients(core);
            core->fullscreen = core->keyboard_visible = 0;
            core->video_generation = 0;
            result = surf_queue_effect(core, SURF_EFFECT_CLEAR_PAGE_PRESENTATION);
            if (result != SURF_CORE_OK) return result;
            result = surf_queue_effect(core, SURF_EFFECT_HIDE_KEYBOARD);
        }
        core->revision++;
        return result;
    case SURF_PROTOCOL_EVENT_URL:
        mapped.kind = SURF_EVENT_URL;
        mapped.data.url.url = event->data.url.url;
        mapped.data.url.security = event->data.url.security;
        mapped.data.url.starred = event->data.url.starred;
        return surf_core_dispatch_scoped(core, generation, &mapped);
    case SURF_PROTOCOL_EVENT_HISTORY_STATE:
        mapped.kind = SURF_EVENT_HISTORY_STATE;
        mapped.data.history.can_go_back = event->data.history_state.back;
        mapped.data.history.can_go_forward = event->data.history_state.forward;
        return surf_core_dispatch_scoped(core, generation, &mapped);
    case SURF_PROTOCOL_EVENT_LOADING:
    case SURF_PROTOCOL_EVENT_FULLSCREEN:
    case SURF_PROTOCOL_EVENT_STARRED:
        mapped.kind = event->kind == SURF_PROTOCOL_EVENT_LOADING
            ? SURF_EVENT_LOADING
            : (event->kind == SURF_PROTOCOL_EVENT_FULLSCREEN
                ? SURF_EVENT_FULLSCREEN : SURF_EVENT_STARRED);
        mapped.data.boolean.on = event->data.boolean.on;
        return surf_core_dispatch_scoped(core, generation, &mapped);
    case SURF_PROTOCOL_EVENT_EDITABLE:
        mapped.kind = SURF_EVENT_EDITABLE;
        mapped.data.editable.on = event->data.editable.on;
        mapped.data.editable.show_keyboard =
            event->data.editable.show_keyboard;
        mapped.data.editable.kind = event->data.editable.kind;
        mapped.data.editable.has_rect = event->data.editable.has_rect;
        memcpy(mapped.data.editable.rect, event->data.editable.rect,
               sizeof(mapped.data.editable.rect));
        return surf_core_dispatch_scoped(core, generation, &mapped);
    case SURF_PROTOCOL_EVENT_SECURITY:
        mapped.kind = SURF_EVENT_SECURITY;
        mapped.data.security.state = event->data.security.state;
        return surf_core_dispatch_scoped(core, generation, &mapped);
    case SURF_PROTOCOL_EVENT_PAGE_FRAME:
        mapped.kind = SURF_EVENT_PAGE_FRAME;
        mapped.data.page_frame.source_sequence =
            event->data.page_frame.source_sequence;
        return surf_core_dispatch_scoped(core, generation, &mapped);
    case SURF_PROTOCOL_EVENT_FOUND:
        core->find_known = 1;
        core->find_found = event->data.boolean.on != 0;
        core->revision++;
        return SURF_CORE_OK;
    case SURF_PROTOCOL_EVENT_VIDEO_CONFIG:
        core->video_generation = event->data.video.generation;
        core->revision++;
        return SURF_CORE_OK;
    case SURF_PROTOCOL_EVENT_SUGGEST:
        core->suggestion_count = event->data.entries.count;
        core->revision++;
        return SURF_CORE_OK;
    case SURF_PROTOCOL_EVENT_LIBRARY:
        core->history_count = event->data.library.history_count;
        core->bookmark_count = event->data.library.bookmark_count;
        core->starred = event->data.library.starred != 0;
        core->revision++;
        return SURF_CORE_OK;
    case SURF_PROTOCOL_EVENT_HISTORY:
        destinations[0] = &core->history_query;
        sources[0] = event->data.history.query;
        result = surf_owned_string_group_replace(core, destinations, sources,
                                                 1, SURF_MAX_TITLE_BYTES);
        if (result != SURF_CORE_OK) return result;
        core->history_count = event->data.history.count;
        core->history_offset = event->data.history.offset;
        core->history_total = event->data.history.total;
        core->revision++;
        return SURF_CORE_OK;
    case SURF_PROTOCOL_EVENT_DOWNLOADS:
        core->download_count = event->data.downloads.count;
        core->revision++;
        return SURF_CORE_OK;
    case SURF_PROTOCOL_EVENT_DIALOG:
        destinations[0] = &core->dialog_kind;
        destinations[1] = &core->dialog_text;
        destinations[2] = &core->dialog_default_text;
        sources[0] = event->data.dialog.kind;
        sources[1] = event->data.dialog.text;
        sources[2] = event->data.dialog.default_text;
        result = surf_dispatch_protocol_strings(
            core, destinations, sources, 3, SURF_MAX_CONTENT_BYTES);
        if (result == SURF_CORE_OK) core->dialog_active = 1;
        return result;
    case SURF_PROTOCOL_EVENT_DIALOG_DONE:
        surf_owned_string_clear(core, &core->dialog_kind);
        surf_owned_string_clear(core, &core->dialog_text);
        surf_owned_string_clear(core, &core->dialog_default_text);
        core->dialog_active = 0;
        core->revision++;
        return SURF_CORE_OK;
    case SURF_PROTOCOL_EVENT_FILE_CHOOSER:
        core->file_chooser_active = 1;
        core->file_chooser_multiple = event->data.chooser.multiple != 0;
        core->revision++;
        return SURF_CORE_OK;
    case SURF_PROTOCOL_EVENT_SELECT:
        destinations[0] = &core->select_id;
        destinations[1] = &core->select_title;
        sources[0] = event->data.select.id;
        sources[1] = event->data.select.title;
        result = surf_dispatch_protocol_strings(
            core, destinations, sources, 2, SURF_MAX_TITLE_BYTES);
        if (result == SURF_CORE_OK) {
            core->select_active = 1;
            core->select_multiple = event->data.select.multiple != 0;
            core->select_option_count = event->data.select.option_count;
        }
        return result;
    case SURF_PROTOCOL_EVENT_CLIPBOARD:
        destinations[0] = &core->clipboard_id;
        destinations[1] = &core->clipboard_text;
        sources[0] = event->data.clipboard.id;
        sources[1] = event->data.clipboard.text;
        result = surf_dispatch_protocol_strings(
            core, destinations, sources, 2, SURF_MAX_CONTENT_BYTES);
        if (result == SURF_CORE_OK) {
            core->clipboard_pending = 1;
            core->clipboard_sync_request = event->data.clipboard.sync != 0;
        }
        return result;
    case SURF_PROTOCOL_EVENT_CLIPBOARD_SYNC:
        destinations[0] = &core->clipboard_text;
        sources[0] = event->data.clipboard_sync.text;
        result = surf_dispatch_protocol_strings(
            core, destinations, sources, 1, SURF_MAX_CONTENT_BYTES);
        if (result == SURF_CORE_OK) {
            core->clipboard_pending = 0;
            core->clipboard_sync_enabled =
                event->data.clipboard_sync.enabled != 0;
            core->clipboard_known = event->data.clipboard_sync.known != 0;
        }
        return result;
    case SURF_PROTOCOL_EVENT_READER:
        destinations[0] = &core->reader_title;
        destinations[1] = &core->reader_url;
        sources[0] = event->data.reader.title;
        sources[1] = event->data.reader.url;
        result = surf_dispatch_protocol_strings(
            core, destinations, sources, 2, SURF_MAX_URL_BYTES);
        if (result == SURF_CORE_OK)
            core->reader_available = event->data.reader.ok != 0;
        return result;
    case SURF_PROTOCOL_EVENT_MEDIA_STATE:
        destinations[0] = &core->media_title;
        sources[0] = event->data.media.title;
        result = surf_dispatch_protocol_strings(
            core, destinations, sources, 1, SURF_MAX_TITLE_BYTES);
        if (result == SURF_CORE_OK) {
            core->media_available = event->data.media.available != 0;
            core->media_count = event->data.media.count;
            core->media_paused = event->data.media.paused != 0;
            core->media_muted = event->data.media.muted != 0;
            core->media_volume = event->data.media.volume;
            core->media_current_time = event->data.media.current_time;
            core->media_duration = event->data.media.duration;
        }
        return result;
    case SURF_PROTOCOL_EVENT_PAGE_ERROR:
        destinations[0] = &core->current_url;
        destinations[1] = &core->security;
        destinations[2] = &core->page_error;
        sources[0] = event->data.url.url;
        sources[1] = event->data.url.security;
        sources[2] = event->data.url.url;
        result = surf_owned_string_group_replace(
            core, destinations, sources, 3, SURF_MAX_URL_BYTES);
        if (result != SURF_CORE_OK) return result;
        core->starred = event->data.url.starred != 0;
        core->show_start_page = 0;
        core->revision++;
        return SURF_CORE_OK;
    case SURF_PROTOCOL_EVENT_TOAST:
        destinations[0] = &core->toast_text;
        sources[0] = event->data.text.text;
        return surf_dispatch_protocol_strings(
            core, destinations, sources, 1, SURF_MAX_TITLE_BYTES);
    case SURF_PROTOCOL_EVENT_DOWNLOAD:
        destinations[0] = &core->download_name;
        sources[0] = event->data.name.name;
        return surf_dispatch_protocol_strings(
            core, destinations, sources, 1, SURF_MAX_TITLE_BYTES);
    case SURF_PROTOCOL_EVENT_DOWNLOAD_PROGRESS:
        destinations[0] = &core->download_name;
        sources[0] = event->data.progress.name;
        result = surf_dispatch_protocol_strings(
            core, destinations, sources, 1, SURF_MAX_TITLE_BYTES);
        if (result == SURF_CORE_OK)
            core->download_percent = event->data.progress.percent;
        return result;
    case SURF_PROTOCOL_EVENT_HELLO:
    case SURF_PROTOCOL_EVENT_AUDIO_CONFIG:
    case SURF_PROTOCOL_EVENT_CLOCK:
    case SURF_PROTOCOL_EVENT_LOG_REQUEST:
    case SURF_PROTOCOL_EVENT_LOG_CLEAR:
        return SURF_CORE_OK;
    default:
        return SURF_CORE_ERROR_ARGUMENT;
    }
}

surf_core_result_t surf_core_dispatch_protocol_json(
    surf_core_t *core, uint64_t generation,
    struct surf_protocol_workspace *workspace, const char *json,
    size_t length) {
    surf_protocol_event_t event;
    surf_protocol_result_t result;
    if (core == NULL || workspace == NULL || json == NULL || length == 0)
        return SURF_CORE_ERROR_ARGUMENT;
    result = surf_protocol_decode_event(workspace, json, length, &event);
    if (result != SURF_PROTOCOL_OK) return SURF_CORE_ERROR_ARGUMENT;
    return surf_core_dispatch_protocol(core, generation, &event);
}

surf_core_result_t surf_core_browser_mode(const surf_core_t *core,
    surf_browser_mode_snapshot_t *out_snapshot) {
    if (core == NULL || out_snapshot == NULL) return SURF_CORE_ERROR_ARGUMENT;
    out_snapshot->state = surf_owned_string_view(&core->browser_mode);
    out_snapshot->host = surf_owned_string_view(&core->browser_host);
    out_snapshot->message = surf_owned_string_view(&core->browser_message);
    out_snapshot->revision = core->browser_revision;
    out_snapshot->paused = core->browser_paused;
    out_snapshot->standalone = core->browser_standalone;
    out_snapshot->can_force = core->browser_can_force;
    return SURF_CORE_OK;
}

surf_core_result_t surf_core_present_frame(
    surf_core_t *core, uint64_t connection_generation,
    uint32_t video_generation, uint32_t source_sequence) {
    surf_event_t event;
    if (core == NULL || connection_generation == 0)
        return SURF_CORE_ERROR_ARGUMENT;
    if (core->browser_paused || connection_generation != core->connection_generation ||
        (core->video_generation != 0 && video_generation != 0 &&
         video_generation != core->video_generation)) {
        core->stale_event_count++;
        return SURF_CORE_OK;
    }
    memset(&event, 0, sizeof(event));
    event.kind = SURF_EVENT_FRAME_PRESENTED;
    event.data.frame_presented.source_sequence = source_sequence;
    return surf_core_dispatch_scoped(core, connection_generation, &event);
}

surf_core_result_t surf_core_complete_semantic(
    surf_core_t *core, uint64_t generation,
    surf_semantic_completion_t completion) {
    if (core == NULL || generation == 0) return SURF_CORE_ERROR_ARGUMENT;
    if (generation != core->connection_generation) {
        core->stale_event_count++;
        return SURF_CORE_OK;
    }
    switch (completion) {
    case SURF_SEMANTIC_COMPLETE_DIALOG:
        surf_owned_string_clear(core, &core->dialog_kind);
        surf_owned_string_clear(core, &core->dialog_text);
        surf_owned_string_clear(core, &core->dialog_default_text);
        core->dialog_active = 0;
        break;
    case SURF_SEMANTIC_COMPLETE_SELECT:
        surf_owned_string_clear(core, &core->select_id);
        surf_owned_string_clear(core, &core->select_title);
        core->select_active = 0;
        core->select_multiple = 0;
        core->select_option_count = 0;
        break;
    case SURF_SEMANTIC_COMPLETE_FILE_CHOOSER:
        core->file_chooser_active = 0;
        core->file_chooser_multiple = 0;
        break;
    case SURF_SEMANTIC_COMPLETE_CLIPBOARD:
        surf_owned_string_clear(core, &core->clipboard_id);
        core->clipboard_pending = 0;
        core->clipboard_sync_request = 0;
        break;
    case SURF_SEMANTIC_COMPLETE_TOAST:
        surf_owned_string_clear(core, &core->toast_text);
        break;
    default:
        return SURF_CORE_ERROR_ARGUMENT;
    }
    core->revision++;
    return SURF_CORE_OK;
}

surf_core_result_t surf_core_snapshot(const surf_core_t *core,
                                      surf_snapshot_t *out_snapshot) {
    if (core == NULL || out_snapshot == NULL) {
        return SURF_CORE_ERROR_ARGUMENT;
    }
    memset(out_snapshot, 0, sizeof(*out_snapshot));
    out_snapshot->revision = core->revision;
    out_snapshot->tabs = core->tab_views;
    out_snapshot->tab_count = core->tab_count;
    out_snapshot->active_tab_id = core->active_tab_id;
    out_snapshot->active_title = surf_owned_string_view(&core->active_title);
    out_snapshot->current_url = surf_owned_string_view(&core->current_url);
    out_snapshot->security = surf_owned_string_view(&core->security);
    out_snapshot->editable_kind =
        surf_owned_string_view(&core->editable_kind);
    memcpy(out_snapshot->editable_rect, core->editable_rect,
           sizeof(out_snapshot->editable_rect));
    out_snapshot->awaited_source_sequence = core->awaited_source_sequence;
    out_snapshot->has_active_tab = core->has_active_tab;
    out_snapshot->show_start_page = core->show_start_page;
    out_snapshot->loading = core->loading;
    out_snapshot->can_go_back = core->can_go_back;
    out_snapshot->can_go_forward = core->can_go_forward;
    out_snapshot->starred = core->starred;
    out_snapshot->fullscreen = core->fullscreen;
    out_snapshot->editable = core->editable;
    out_snapshot->editable_has_rect = core->editable_has_rect;
    out_snapshot->keyboard_visible = core->keyboard_visible;
    out_snapshot->awaiting_page_frame = core->awaiting_page_frame;
    return SURF_CORE_OK;
}

surf_core_result_t surf_core_semantic_snapshot(
    const surf_core_t *core, surf_semantic_snapshot_t *out_snapshot) {
    if (core == NULL || out_snapshot == NULL)
        return SURF_CORE_ERROR_ARGUMENT;
    memset(out_snapshot, 0, sizeof(*out_snapshot));
    out_snapshot->revision = core->revision;
    out_snapshot->dialog_kind = surf_owned_string_view(&core->dialog_kind);
    out_snapshot->dialog_text = surf_owned_string_view(&core->dialog_text);
    out_snapshot->dialog_default_text =
        surf_owned_string_view(&core->dialog_default_text);
    out_snapshot->select_id = surf_owned_string_view(&core->select_id);
    out_snapshot->select_title = surf_owned_string_view(&core->select_title);
    out_snapshot->clipboard_id = surf_owned_string_view(&core->clipboard_id);
    out_snapshot->clipboard_text =
        surf_owned_string_view(&core->clipboard_text);
    out_snapshot->reader_title = surf_owned_string_view(&core->reader_title);
    out_snapshot->reader_url = surf_owned_string_view(&core->reader_url);
    out_snapshot->media_title = surf_owned_string_view(&core->media_title);
    out_snapshot->page_error = surf_owned_string_view(&core->page_error);
    out_snapshot->toast_text = surf_owned_string_view(&core->toast_text);
    out_snapshot->download_name = surf_owned_string_view(&core->download_name);
    out_snapshot->history_query = surf_owned_string_view(&core->history_query);
    out_snapshot->select_option_count = core->select_option_count;
    out_snapshot->suggestion_count = core->suggestion_count;
    out_snapshot->history_count = core->history_count;
    out_snapshot->bookmark_count = core->bookmark_count;
    out_snapshot->download_count = core->download_count;
    out_snapshot->history_offset = core->history_offset;
    out_snapshot->history_total = core->history_total;
    out_snapshot->download_percent = core->download_percent;
    out_snapshot->media_count = core->media_count;
    out_snapshot->media_volume = core->media_volume;
    out_snapshot->media_current_time = core->media_current_time;
    out_snapshot->media_duration = core->media_duration;
    out_snapshot->dialog_active = core->dialog_active;
    out_snapshot->select_active = core->select_active;
    out_snapshot->select_multiple = core->select_multiple;
    out_snapshot->file_chooser_active = core->file_chooser_active;
    out_snapshot->file_chooser_multiple = core->file_chooser_multiple;
    out_snapshot->find_known = core->find_known;
    out_snapshot->find_found = core->find_found;
    out_snapshot->clipboard_pending = core->clipboard_pending;
    out_snapshot->clipboard_sync_request = core->clipboard_sync_request;
    out_snapshot->clipboard_sync_enabled = core->clipboard_sync_enabled;
    out_snapshot->clipboard_known = core->clipboard_known;
    out_snapshot->reader_available = core->reader_available;
    out_snapshot->media_available = core->media_available;
    out_snapshot->media_paused = core->media_paused;
    out_snapshot->media_muted = core->media_muted;
    out_snapshot->video_generation = core->video_generation;
    return SURF_CORE_OK;
}

int surf_core_next_effect(surf_core_t *core, surf_effect_t *out_effect) {
    if (core == NULL || out_effect == NULL || core->effect_count == 0) {
        return 0;
    }
    *out_effect = core->effects[core->effect_head];
    core->effect_head = (core->effect_head + 1) % SURF_EFFECT_CAPACITY;
    core->effect_count--;
    return 1;
}

uint64_t surf_core_connection_generation(const surf_core_t *core) {
    return core == NULL ? 0 : core->connection_generation;
}

uint64_t surf_core_stale_event_count(const surf_core_t *core) {
    return core == NULL ? 0 : core->stale_event_count;
}

const char *surf_core_result_string(surf_core_result_t result) {
    switch (result) {
    case SURF_CORE_OK:
        return "ok";
    case SURF_CORE_ERROR_ARGUMENT:
        return "invalid argument";
    case SURF_CORE_ERROR_ABI:
        return "core ABI mismatch";
    case SURF_CORE_ERROR_LIMIT:
        return "core limit exceeded";
    case SURF_CORE_ERROR_ALLOCATE:
        return "allocation failed";
    case SURF_CORE_ERROR_STATE:
        return "invalid state";
    default:
        return "unknown core error";
    }
}

uint32_t surf_core_abi_version(void) {
    return SURF_CORE_ABI_VERSION;
}

size_t surf_core_sizeof_string_view(void) {
    return sizeof(surf_string_view_t);
}

size_t surf_core_sizeof_config(void) {
    return sizeof(surf_core_config_t);
}

size_t surf_core_sizeof_event(void) {
    return sizeof(surf_event_t);
}

size_t surf_core_sizeof_snapshot(void) {
    return sizeof(surf_snapshot_t);
}

size_t surf_core_sizeof_semantic_snapshot(void) {
    return sizeof(surf_semantic_snapshot_t);
}
