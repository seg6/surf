#include "surf/core.h"

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

static void test_abi_layout(void) {
    SURF_CHECK(surf_core_abi_version() == SURF_CORE_ABI_VERSION);
    SURF_CHECK(surf_core_sizeof_string_view() == sizeof(surf_string_view_t));
    SURF_CHECK(surf_core_sizeof_config() == sizeof(surf_core_config_t));
    SURF_CHECK(surf_core_sizeof_event() == sizeof(surf_event_t));
    SURF_CHECK(surf_core_sizeof_snapshot() == sizeof(surf_snapshot_t));
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
    test_abi_layout();
    puts("surf_core: all tests passed");
    return EXIT_SUCCESS;
}
