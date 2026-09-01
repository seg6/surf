#include "surf/core.h"
#include "surf/protocol.h"

#include "test.h"

#include <stdint.h>
#include <stdlib.h>
#include <string.h>

static void dispatch_ok(surf_core_t *core, const surf_event_t *event) {
    surf_core_result_t result = surf_core_dispatch(core, event);
    if (result != SURF_CORE_OK) {
        fprintf(stderr, "dispatch failed: %s\n", surf_core_result_string(result));
        exit(EXIT_FAILURE);
    }
}

static surf_snapshot_t snapshot(surf_core_t *core) {
    surf_snapshot_t value;
    SURF_CHECK(surf_core_snapshot(core, &value) == SURF_CORE_OK);
    return value;
}

static int view_is(surf_string_view_t view, const char *expected) {
    return surf_string_equal(view, surf_string_from_cstr(expected));
}

static surf_core_t *create_core(void) {
    surf_core_t *core = NULL;
    SURF_CHECK(surf_core_create(NULL, &core) == SURF_CORE_OK);
    SURF_CHECK(core != NULL);
    return core;
}

static void test_initial_state(void) {
    surf_core_t *core = create_core();
    surf_snapshot_t state = snapshot(core);
    SURF_CHECK(state.revision == 1);
    SURF_CHECK(state.tab_count == 0);
    SURF_CHECK(!state.has_active_tab);
    SURF_CHECK(state.show_start_page);
    surf_core_destroy(core);
}

static void test_tabs_and_titles(void) {
    surf_core_t *core = create_core();
    char active_title[] = "https://example.test/path";
    surf_tab_event_t tabs[2];
    surf_event_t event;
    surf_snapshot_t state;

    memset(tabs, 0, sizeof(tabs));
    tabs[0].id = 10;
    tabs[0].title = surf_string_from_cstr("Inactive");
    tabs[0].url = surf_string_from_cstr("https://inactive.test/");
    tabs[1].id = 11;
    tabs[1].title = surf_string_from_cstr(active_title);
    tabs[1].url = surf_string_from_cstr("https://example.test/path");
    tabs[1].active = 1;
    memset(&event, 0, sizeof(event));
    event.kind = SURF_EVENT_TABS;
    event.data.tabs.items = tabs;
    event.data.tabs.count = 2;
    dispatch_ok(core, &event);

    active_title[0] = 'X';
    state = snapshot(core);
    SURF_CHECK(state.tab_count == 2);
    SURF_CHECK(state.has_active_tab && state.active_tab_id == 11);
    SURF_CHECK(view_is(state.active_title, "example.test"));
    SURF_CHECK(view_is(state.tabs[1].title, "https://example.test/path"));

    tabs[1].title = surf_string_from_cstr("A real page title");
    dispatch_ok(core, &event);
    state = snapshot(core);
    SURF_CHECK(view_is(state.active_title, "A real page title"));

    tabs[1].title = surf_string_from_cstr("about:blank#surf-new");
    tabs[1].url = surf_string_from_cstr("about:blank#surf-new");
    dispatch_ok(core, &event);
    state = snapshot(core);
    SURF_CHECK(view_is(state.active_title, "New Tab"));
    surf_core_destroy(core);
}

static void test_active_switch_clears_transients(void) {
    surf_core_t *core = create_core();
    surf_tab_event_t tabs[2];
    surf_event_t event;
    surf_snapshot_t state;
    surf_effect_t effect;

    memset(tabs, 0, sizeof(tabs));
    tabs[0].id = 1;
    tabs[0].title = surf_string_from_cstr("One");
    tabs[0].url = surf_string_from_cstr("https://one.test");
    tabs[0].active = 1;
    tabs[1].id = 2;
    tabs[1].title = surf_string_from_cstr("Two");
    tabs[1].url = surf_string_from_cstr("https://two.test");
    memset(&event, 0, sizeof(event));
    event.kind = SURF_EVENT_TABS;
    event.data.tabs.items = tabs;
    event.data.tabs.count = 2;
    dispatch_ok(core, &event);

    memset(&event, 0, sizeof(event));
    event.kind = SURF_EVENT_LOADING;
    event.data.boolean.on = 1;
    dispatch_ok(core, &event);
    event.kind = SURF_EVENT_KEYBOARD_VISIBILITY;
    dispatch_ok(core, &event);
    event.kind = SURF_EVENT_EDITABLE;
    event.data.editable.on = 1;
    event.data.editable.show_keyboard = 1;
    event.data.editable.kind = surf_string_from_cstr("text");
    dispatch_ok(core, &event);
    SURF_CHECK(surf_core_next_effect(core, &effect));
    SURF_CHECK(effect.kind == SURF_EFFECT_SHOW_KEYBOARD);

    tabs[0].active = 0;
    tabs[1].active = 1;
    memset(&event, 0, sizeof(event));
    event.kind = SURF_EVENT_TABS;
    event.data.tabs.items = tabs;
    event.data.tabs.count = 2;
    dispatch_ok(core, &event);
    state = snapshot(core);
    SURF_CHECK(state.active_tab_id == 2);
    SURF_CHECK(!state.loading);
    SURF_CHECK(!state.editable);
    SURF_CHECK(!state.editable_has_rect && state.editable_rect[0] == 0.0);
    SURF_CHECK(!state.awaiting_page_frame);
    SURF_CHECK(surf_core_next_effect(core, &effect));
    SURF_CHECK(effect.kind == SURF_EFFECT_CLEAR_PAGE_PRESENTATION);
    SURF_CHECK(surf_core_next_effect(core, &effect));
    SURF_CHECK(effect.kind == SURF_EFFECT_HIDE_KEYBOARD);
    SURF_CHECK(!surf_core_next_effect(core, &effect));
    surf_core_destroy(core);
}

static void test_url_history_and_page_frame(void) {
    surf_core_t *core = create_core();
    surf_event_t event;
    surf_snapshot_t state;
    surf_effect_t effect;

    memset(&event, 0, sizeof(event));
    event.kind = SURF_EVENT_URL;
    event.data.url.url = surf_string_from_cstr("about:blank#surf-new");
    event.data.url.security = surf_string_from_cstr("local");
    event.data.url.starred = 1;
    dispatch_ok(core, &event);
    state = snapshot(core);
    SURF_CHECK(state.show_start_page && !state.loading && state.starred);
    SURF_CHECK(view_is(state.security, "local"));
    SURF_CHECK(surf_core_next_effect(core, &effect));
    SURF_CHECK(effect.kind == SURF_EFFECT_REQUEST_LIBRARY);

    event.data.url.url = surf_string_from_cstr("https://example.test/");
    event.data.url.security = surf_string_from_cstr("secure");
    dispatch_ok(core, &event);
    SURF_CHECK(surf_core_next_effect(core, &effect));
    SURF_CHECK(effect.kind == SURF_EFFECT_CLEAR_PAGE_PRESENTATION);

    memset(&event, 0, sizeof(event));
    event.kind = SURF_EVENT_HISTORY_STATE;
    event.data.history.can_go_back = 1;
    dispatch_ok(core, &event);
    state = snapshot(core);
    SURF_CHECK(state.can_go_back && !state.can_go_forward);

    memset(&event, 0, sizeof(event));
    event.kind = SURF_EVENT_PAGE_FRAME;
    event.data.page_frame.source_sequence = 100;
    dispatch_ok(core, &event);
    state = snapshot(core);
    SURF_CHECK(state.awaiting_page_frame);
    event.kind = SURF_EVENT_FRAME_PRESENTED;
    event.data.frame_presented.source_sequence = 99;
    dispatch_ok(core, &event);
    SURF_CHECK(snapshot(core).awaiting_page_frame);
    event.data.frame_presented.source_sequence = 100;
    dispatch_ok(core, &event);
    state = snapshot(core);
    SURF_CHECK(!state.awaiting_page_frame && !state.show_start_page);
    surf_core_destroy(core);
}

static void test_invalid_tabs_are_atomic(void) {
    surf_core_t *core = create_core();
    surf_tab_event_t tabs[2];
    surf_event_t event;
    surf_snapshot_t before;
    surf_snapshot_t after;
    memset(tabs, 0, sizeof(tabs));
    tabs[0].id = 1;
    tabs[0].title = surf_string_from_cstr("One");
    tabs[0].url = surf_string_from_cstr("https://one.test");
    tabs[0].active = 1;
    memset(&event, 0, sizeof(event));
    event.kind = SURF_EVENT_TABS;
    event.data.tabs.items = tabs;
    event.data.tabs.count = 1;
    dispatch_ok(core, &event);
    before = snapshot(core);

    tabs[1] = tabs[0];
    tabs[1].id = 2;
    event.data.tabs.count = 2;
    SURF_CHECK(surf_core_dispatch(core, &event) == SURF_CORE_ERROR_STATE);
    after = snapshot(core);
    SURF_CHECK(after.revision == before.revision);
    SURF_CHECK(after.tab_count == 1 && after.active_tab_id == 1);
    SURF_CHECK(view_is(after.active_title, "One"));

    tabs[1].active = 0;
    tabs[1].id = 1;
    SURF_CHECK(surf_core_dispatch(core, &event) == SURF_CORE_ERROR_STATE);
    after = snapshot(core);
    SURF_CHECK(after.revision == before.revision);
    SURF_CHECK(after.tab_count == 1 && after.active_tab_id == 1);
    surf_core_destroy(core);
}

typedef struct failing_allocator {
    size_t successful_allocations;
    size_t fail_after;
} failing_allocator_t;

static void *test_allocate(void *context, size_t size) {
    failing_allocator_t *state = (failing_allocator_t *)context;
    if (state->successful_allocations >= state->fail_after) {
        return NULL;
    }
    state->successful_allocations++;
    return malloc(size);
}

static void test_deallocate(void *context, void *pointer) {
    (void)context;
    free(pointer);
}

static void test_allocator_failure(void) {
    failing_allocator_t allocator_state;
    surf_core_config_t config;
    surf_core_t *core = NULL;
    surf_tab_event_t tab;
    surf_event_t event;
    surf_snapshot_t state;

    allocator_state.successful_allocations = 0;
    allocator_state.fail_after = 1;
    surf_core_config_init(&config);
    config.allocator.context = &allocator_state;
    config.allocator.allocate = test_allocate;
    config.allocator.deallocate = test_deallocate;
    SURF_CHECK(surf_core_create(&config, &core) == SURF_CORE_OK);
    memset(&tab, 0, sizeof(tab));
    tab.id = 1;
    tab.title = surf_string_from_cstr("One");
    tab.url = surf_string_from_cstr("https://one.test");
    tab.active = 1;
    memset(&event, 0, sizeof(event));
    event.kind = SURF_EVENT_TABS;
    event.data.tabs.items = &tab;
    event.data.tabs.count = 1;
    SURF_CHECK(surf_core_dispatch(core, &event) ==
               SURF_CORE_ERROR_ALLOCATE);
    state = snapshot(core);
    SURF_CHECK(state.tab_count == 0 && state.revision == 1);
    surf_core_destroy(core);
}

static void test_configuration(void) {
    surf_core_config_t config;
    surf_core_t *core = NULL;
    surf_core_config_init(&config);
    config.abi_version++;
    SURF_CHECK(surf_core_create(&config, &core) == SURF_CORE_ERROR_ABI);
    config.abi_version = SURF_CORE_ABI_VERSION;
    config.max_tabs = SURF_CORE_HARD_MAX_TABS + 1;
    SURF_CHECK(surf_core_create(&config, &core) == SURF_CORE_ERROR_LIMIT);
}

static void test_connection_generations_reject_stale_events(void) {
    surf_core_t *core = create_core();
    surf_event_t event;
    surf_snapshot_t before;
    surf_snapshot_t after;

    memset(&event, 0, sizeof(event));
    event.kind = SURF_EVENT_LOADING;
    event.data.boolean.on = 1;
    SURF_CHECK(surf_core_dispatch_scoped(core, 1, &event) == SURF_CORE_OK);
    SURF_CHECK(snapshot(core).loading);

    SURF_CHECK(surf_core_begin_connection(core, 2) == SURF_CORE_OK);
    before = snapshot(core);
    SURF_CHECK(!before.loading && before.tab_count == 0);
    SURF_CHECK(surf_core_connection_generation(core) == 2);

    SURF_CHECK(surf_core_dispatch_scoped(core, 1, &event) == SURF_CORE_OK);
    after = snapshot(core);
    SURF_CHECK(after.revision == before.revision);
    SURF_CHECK(!after.loading);
    SURF_CHECK(surf_core_stale_event_count(core) == 1);

    SURF_CHECK(surf_core_dispatch_scoped(core, 2, &event) == SURF_CORE_OK);
    SURF_CHECK(snapshot(core).loading);
    SURF_CHECK(surf_core_begin_connection(core, 2) == SURF_CORE_ERROR_STATE);
    SURF_CHECK(surf_core_begin_connection(core, 0) ==
               SURF_CORE_ERROR_ARGUMENT);
    surf_core_destroy(core);
}

static void test_protocol_semantic_state_and_tab_cleanup(void) {
    surf_core_t *core = create_core();
    surf_protocol_event_t protocol;
    surf_semantic_snapshot_t semantic;
    surf_tab_event_t tab;
    surf_event_t event;

    memset(&protocol, 0, sizeof(protocol));
    protocol.kind = SURF_PROTOCOL_EVENT_DIALOG;
    protocol.data.dialog.kind = surf_string_from_cstr("prompt");
    protocol.data.dialog.text = surf_string_from_cstr("Your name?");
    protocol.data.dialog.default_text = surf_string_from_cstr("Surf");
    SURF_CHECK(surf_core_dispatch_protocol(core, 1, &protocol) ==
               SURF_CORE_OK);

    memset(&protocol, 0, sizeof(protocol));
    protocol.kind = SURF_PROTOCOL_EVENT_SELECT;
    protocol.data.select.id = surf_string_from_cstr("select-7");
    protocol.data.select.title = surf_string_from_cstr("Choose");
    protocol.data.select.multiple = 1;
    protocol.data.select.option_count = 3;
    SURF_CHECK(surf_core_dispatch_protocol(core, 1, &protocol) ==
               SURF_CORE_OK);

    memset(&protocol, 0, sizeof(protocol));
    protocol.kind = SURF_PROTOCOL_EVENT_MEDIA_STATE;
    protocol.data.media.available = 1;
    protocol.data.media.count = 2;
    protocol.data.media.paused = 0;
    protocol.data.media.volume = 0.75;
    protocol.data.media.duration = 42.0;
    protocol.data.media.title = surf_string_from_cstr("Player");
    SURF_CHECK(surf_core_dispatch_protocol(core, 1, &protocol) ==
               SURF_CORE_OK);

    memset(&protocol, 0, sizeof(protocol));
    protocol.kind = SURF_PROTOCOL_EVENT_CLIPBOARD_SYNC;
    protocol.data.clipboard_sync.enabled = 1;
    protocol.data.clipboard_sync.known = 1;
    protocol.data.clipboard_sync.text = surf_string_from_cstr("shared text");
    SURF_CHECK(surf_core_dispatch_protocol(core, 1, &protocol) ==
               SURF_CORE_OK);

    SURF_CHECK(surf_core_semantic_snapshot(core, &semantic) == SURF_CORE_OK);
    SURF_CHECK(semantic.dialog_active && semantic.select_active);
    SURF_CHECK(semantic.select_multiple && semantic.select_option_count == 3);
    SURF_CHECK(view_is(semantic.dialog_text, "Your name?"));
    SURF_CHECK(view_is(semantic.select_id, "select-7"));
    SURF_CHECK(semantic.media_available && semantic.media_count == 2);
    SURF_CHECK(semantic.media_volume == 0.75);
    SURF_CHECK(surf_core_complete_semantic(
                   core, 1, SURF_SEMANTIC_COMPLETE_SELECT) == SURF_CORE_OK);
    SURF_CHECK(surf_core_semantic_snapshot(core, &semantic) == SURF_CORE_OK);
    SURF_CHECK(!semantic.select_active && semantic.dialog_active);

    memset(&tab, 0, sizeof(tab));
    tab.id = 1;
    tab.title = surf_string_from_cstr("One");
    tab.url = surf_string_from_cstr("https://one.test");
    tab.active = 1;
    memset(&event, 0, sizeof(event));
    event.kind = SURF_EVENT_TABS;
    event.data.tabs.items = &tab;
    event.data.tabs.count = 1;
    dispatch_ok(core, &event);
    tab.id = 2;
    dispatch_ok(core, &event);

    SURF_CHECK(surf_core_semantic_snapshot(core, &semantic) == SURF_CORE_OK);
    SURF_CHECK(!semantic.dialog_active && !semantic.select_active);
    SURF_CHECK(!semantic.media_available);
    SURF_CHECK(semantic.dialog_text.length == 0);
    SURF_CHECK(semantic.clipboard_sync_enabled && semantic.clipboard_known);
    SURF_CHECK(view_is(semantic.clipboard_text, "shared text"));
    surf_core_destroy(core);
}

static void test_presented_frames_are_surface_scoped(void) {
    surf_core_t *core = create_core();
    surf_protocol_event_t protocol;
    surf_event_t page_frame;
    uint64_t stale_before;

    memset(&protocol, 0, sizeof(protocol));
    protocol.kind = SURF_PROTOCOL_EVENT_VIDEO_CONFIG;
    protocol.data.video.generation = 7;
    SURF_CHECK(surf_core_dispatch_protocol(core, 1, &protocol) ==
               SURF_CORE_OK);
    memset(&page_frame, 0, sizeof(page_frame));
    page_frame.kind = SURF_EVENT_PAGE_FRAME;
    page_frame.data.page_frame.source_sequence = 100;
    dispatch_ok(core, &page_frame);
    SURF_CHECK(snapshot(core).awaiting_page_frame);

    stale_before = surf_core_stale_event_count(core);
    SURF_CHECK(surf_core_present_frame(core, 1, 6, 100) == SURF_CORE_OK);
    SURF_CHECK(snapshot(core).awaiting_page_frame);
    SURF_CHECK(surf_core_stale_event_count(core) == stale_before + 1);
    SURF_CHECK(surf_core_present_frame(core, 1, 7, 100) == SURF_CORE_OK);
    SURF_CHECK(!snapshot(core).awaiting_page_frame);
    surf_core_destroy(core);
}

static uint32_t next_random(uint32_t *state) {
    *state = *state * UINT32_C(1664525) + UINT32_C(1013904223);
    return *state;
}

static void test_randomized_transition_invariants(void) {
    surf_core_t *core = create_core();
    uint32_t random = UINT32_C(0x51f15e);
    size_t iteration;
    for (iteration = 0; iteration < 10000; iteration++) {
        surf_event_t event;
        surf_snapshot_t state;
        uint32_t choice = next_random(&random) % 8u;
        memset(&event, 0, sizeof(event));
        if (choice == 0) {
            event.kind = SURF_EVENT_LOADING;
            event.data.boolean.on = (int)(next_random(&random) & 1u);
        } else if (choice == 1) {
            event.kind = SURF_EVENT_EDITABLE;
            event.data.editable.on = (int)(next_random(&random) & 1u);
            event.data.editable.show_keyboard =
                (int)(next_random(&random) & 1u);
            event.data.editable.kind = surf_string_from_cstr("text");
            event.data.editable.has_rect = event.data.editable.on;
            event.data.editable.rect[2] = 0.5;
            event.data.editable.rect[3] = 0.25;
        } else if (choice == 2) {
            event.kind = SURF_EVENT_KEYBOARD_VISIBILITY;
            event.data.boolean.on = (int)(next_random(&random) & 1u);
        } else if (choice == 3) {
            event.kind = SURF_EVENT_PAGE_FRAME;
            event.data.page_frame.source_sequence = next_random(&random);
        } else if (choice == 4) {
            event.kind = SURF_EVENT_FRAME_PRESENTED;
            event.data.frame_presented.source_sequence = next_random(&random);
        } else if (choice == 5) {
            event.kind = SURF_EVENT_URL;
            event.data.url.url = (next_random(&random) & 1u)
                ? surf_string_from_cstr("about:blank#surf-new")
                : surf_string_from_cstr("https://surf.test");
            event.data.url.security = surf_string_from_cstr("secure");
        } else if (choice == 6) {
            event.kind = SURF_EVENT_RESET;
        } else {
            event.kind = SURF_EVENT_FULLSCREEN;
            event.data.boolean.on = (int)(next_random(&random) & 1u);
        }
        dispatch_ok(core, &event);
        state = snapshot(core);
        SURF_CHECK(state.tab_count <= SURF_CORE_HARD_MAX_TABS);
        SURF_CHECK(!state.editable || state.editable_kind.length != 0);
        SURF_CHECK(state.editable || !state.editable_has_rect);
        if (event.kind == SURF_EVENT_PAGE_FRAME) {
            SURF_CHECK(state.awaiting_page_frame);
            SURF_CHECK(state.awaited_source_sequence ==
                       event.data.page_frame.source_sequence);
        }
    }
    surf_core_destroy(core);
}

static void test_abi_layout(void) {
    SURF_CHECK(surf_core_abi_version() == SURF_CORE_ABI_VERSION);
    SURF_CHECK(surf_core_sizeof_string_view() == sizeof(surf_string_view_t));
    SURF_CHECK(surf_core_sizeof_config() == sizeof(surf_core_config_t));
    SURF_CHECK(surf_core_sizeof_event() == sizeof(surf_event_t));
    SURF_CHECK(surf_core_sizeof_snapshot() == sizeof(surf_snapshot_t));
    SURF_CHECK(surf_core_sizeof_semantic_snapshot() ==
               sizeof(surf_semantic_snapshot_t));
}

int main(void) {
    test_initial_state();
    test_tabs_and_titles();
    test_active_switch_clears_transients();
    test_url_history_and_page_frame();
    test_invalid_tabs_are_atomic();
    test_allocator_failure();
    test_configuration();
    test_connection_generations_reject_stale_events();
    test_protocol_semantic_state_and_tab_cleanup();
    test_presented_frames_are_surface_scoped();
    test_randomized_transition_invariants();
    test_abi_layout();
    puts("surf_core: all tests passed");
    return EXIT_SUCCESS;
}
