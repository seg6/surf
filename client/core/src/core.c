#include "surf/core.h"

#include <stdlib.h>
#include <string.h>

#define SURF_MAX_URL_BYTES ((size_t)8192)
#define SURF_MAX_TITLE_BYTES ((size_t)2048)
#define SURF_MAX_ICON_BYTES ((size_t)8192)
#define SURF_MAX_SHORT_TEXT_BYTES ((size_t)128)
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
    double editable_rect[4];
    uint64_t revision;
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

static void surf_clear_page_transients(surf_core_t *core) {
    core->loading = 0;
    core->editable = 0;
    core->editable_has_rect = 0;
    memset(core->editable_rect, 0, sizeof(core->editable_rect));
    core->awaiting_page_frame = 0;
    core->awaited_source_sequence = 0;
    surf_owned_string_clear(core, &core->editable_kind);
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
    *out_core = core;
    return SURF_CORE_OK;
}

void surf_core_destroy(surf_core_t *core) {
    surf_allocator_t allocator;
    if (core == NULL) {
        return;
    }
    allocator = core->allocator;
    surf_owned_tabs_clear(core, core->tabs, core->tab_count, core->tab_views);
    surf_owned_string_clear(core, &core->active_title);
    surf_owned_string_clear(core, &core->current_url);
    surf_owned_string_clear(core, &core->security);
    surf_owned_string_clear(core, &core->editable_kind);
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

int surf_core_next_effect(surf_core_t *core, surf_effect_t *out_effect) {
    if (core == NULL || out_effect == NULL || core->effect_count == 0) {
        return 0;
    }
    *out_effect = core->effects[core->effect_head];
    core->effect_head = (core->effect_head + 1) % SURF_EFFECT_CAPACITY;
    core->effect_count--;
    return 1;
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
