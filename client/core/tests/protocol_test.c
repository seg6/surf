#include "surf/protocol.h"

#include "test.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#ifndef SURF_PROTOCOL_EVENT_FIXTURES
#error SURF_PROTOCOL_EVENT_FIXTURES must name protocol/fixtures/events.json
#endif

static char *read_file(const char *path, size_t *out_length) {
    FILE *file = fopen(path, "rb");
    long length;
    char *data;
    SURF_CHECK(file != NULL);
    SURF_CHECK(fseek(file, 0, SEEK_END) == 0);
    length = ftell(file);
    SURF_CHECK(length >= 0);
    SURF_CHECK(fseek(file, 0, SEEK_SET) == 0);
    data = (char *)malloc((size_t)length + 1);
    SURF_CHECK(data != NULL);
    SURF_CHECK(fread(data, 1, (size_t)length, file) == (size_t)length);
    SURF_CHECK(fclose(file) == 0);
    data[length] = '\0';
    *out_length = (size_t)length;
    return data;
}

static surf_protocol_workspace_t *create_workspace(void **out_memory) {
    size_t size = surf_protocol_workspace_size(NULL);
    surf_protocol_workspace_t *workspace = NULL;
    void *memory;
    SURF_CHECK(size > 0);
    memory = malloc(size);
    SURF_CHECK(memory != NULL);
    SURF_CHECK(surf_protocol_workspace_init(&workspace, memory, size, NULL) ==
               SURF_PROTOCOL_OK);
    *out_memory = memory;
    return workspace;
}

static size_t decode_fixture_array(surf_protocol_workspace_t *workspace,
                                   const char *data, size_t length) {
    size_t position = 0;
    size_t count = 0;
    int in_string = 0;
    int escaped = 0;
    int depth = 0;
    size_t start = 0;
    while (position < length) {
        char value = data[position];
        if (in_string) {
            if (escaped) escaped = 0;
            else if (value == '\\') escaped = 1;
            else if (value == '"') in_string = 0;
        } else if (value == '"') {
            in_string = 1;
        } else if (value == '{') {
            if (depth == 0) start = position;
            depth++;
        } else if (value == '}') {
            surf_protocol_event_t event;
            SURF_CHECK(depth > 0);
            depth--;
            if (depth == 0) {
                surf_protocol_result_t result = surf_protocol_decode_event(
                    workspace, data + start, position - start + 1, &event);
                if (result != SURF_PROTOCOL_OK) {
                    fprintf(stderr, "event %lu failed: %s\n",
                            (unsigned long)count,
                            surf_protocol_result_string(result));
                    exit(EXIT_FAILURE);
                }
                SURF_CHECK(event.kind ==
                           (surf_protocol_event_kind_t)(count + 1));
                count++;
            }
        }
        position++;
    }
    SURF_CHECK(!in_string && depth == 0);
    return count;
}

static void test_event_fixtures(void) {
    size_t length;
    char *data = read_file(SURF_PROTOCOL_EVENT_FIXTURES, &length);
    void *memory;
    surf_protocol_workspace_t *workspace = create_workspace(&memory);
    SURF_CHECK(decode_fixture_array(workspace, data, length) == 33);
    free(memory);
    free(data);
}

static void test_strict_event_rejection(void) {
    void *memory;
    surf_protocol_workspace_t *workspace = create_workspace(&memory);
    surf_protocol_event_t event;
    const char *unknown = "{\"t\":\"loading\",\"on\":true,\"extra\":1}";
    const char *duplicate = "{\"t\":\"loading\",\"on\":true,\"on\":false}";
    const char *wrong_type = "{\"t\":\"loading\",\"on\":1}";
    const char *trailing = "{\"t\":\"loading\",\"on\":true} false";
    const char *partial_rect =
        "{\"t\":\"editable\",\"on\":true,\"rect\":[0,1]}";
    const char *truncated = "{\"t\":\"loading\",\"on\":";
    const char *leading_zero = "{\"t\":\"pageframe\",\"sourceSeq\":01}";
    const char *leading_plus = "{\"t\":\"pageframe\",\"sourceSeq\":+1}";
    const char *missing_fraction =
        "{\"t\":\"media-state\",\"available\":true,\"count\":1,"
        "\"paused\":false,\"muted\":false,\"volume\":1.,"
        "\"currentTime\":0,\"duration\":1}";
    const char invalid_utf8[] =
        "{\"t\":\"toast\",\"text\":\"\xc0\xaf\"}";
    SURF_CHECK(surf_protocol_decode_event(workspace, unknown, strlen(unknown),
                                          &event) ==
               SURF_PROTOCOL_ERROR_FIELD);
    SURF_CHECK(surf_protocol_decode_event(workspace, duplicate,
                                          strlen(duplicate), &event) ==
               SURF_PROTOCOL_ERROR_FIELD);
    SURF_CHECK(surf_protocol_decode_event(workspace, wrong_type,
                                          strlen(wrong_type), &event) ==
               SURF_PROTOCOL_ERROR_TYPE);
    SURF_CHECK(surf_protocol_decode_event(workspace, trailing,
                                          strlen(trailing), &event) ==
               SURF_PROTOCOL_ERROR_JSON);
    SURF_CHECK(surf_protocol_decode_event(workspace, partial_rect,
                                          strlen(partial_rect), &event) ==
               SURF_PROTOCOL_ERROR_FIELD);
    SURF_CHECK(surf_protocol_decode_event(workspace, truncated,
                                          strlen(truncated), &event) ==
               SURF_PROTOCOL_ERROR_JSON);
    SURF_CHECK(surf_protocol_decode_event(workspace, leading_zero,
                                          strlen(leading_zero), &event) ==
               SURF_PROTOCOL_ERROR_JSON);
    SURF_CHECK(surf_protocol_decode_event(workspace, leading_plus,
                                          strlen(leading_plus), &event) ==
               SURF_PROTOCOL_ERROR_JSON);
    SURF_CHECK(surf_protocol_decode_event(workspace, missing_fraction,
                                          strlen(missing_fraction), &event) ==
               SURF_PROTOCOL_ERROR_JSON);
    SURF_CHECK(surf_protocol_decode_event(workspace, invalid_utf8,
                                          sizeof(invalid_utf8) - 1, &event) ==
               SURF_PROTOCOL_ERROR_JSON);
    free(memory);
}

static void test_workspace_limits(void) {
    surf_protocol_workspace_config_t config;
    surf_protocol_workspace_t *workspace = NULL;
    surf_protocol_event_t event;
    size_t size;
    void *memory;
    const char *tabs =
        "{\"t\":\"tabs\",\"tabs\":["
        "{\"id\":1,\"title\":\"one\",\"url\":\"https://one\",\"active\":true},"
        "{\"id\":2,\"title\":\"two\",\"url\":\"https://two\",\"active\":false}]}";

    surf_protocol_workspace_config_init(&config);
    config.item_capacity = 1;
    size = surf_protocol_workspace_size(&config);
    SURF_CHECK(size != 0);
    memory = malloc(size);
    SURF_CHECK(memory != NULL);
    SURF_CHECK(surf_protocol_workspace_init(&workspace, memory, size, &config) ==
               SURF_PROTOCOL_OK);
    SURF_CHECK(surf_protocol_decode_event(workspace, tabs, strlen(tabs),
                                          &event) ==
               SURF_PROTOCOL_ERROR_LIMIT);
    free(memory);

    surf_protocol_workspace_config_init(&config);
    config.token_capacity = 2;
    size = surf_protocol_workspace_size(&config);
    memory = malloc(size);
    SURF_CHECK(memory != NULL);
    SURF_CHECK(surf_protocol_workspace_init(&workspace, memory, size, &config) ==
               SURF_PROTOCOL_OK);
    SURF_CHECK(surf_protocol_decode_event(workspace, tabs, strlen(tabs),
                                          &event) ==
               SURF_PROTOCOL_ERROR_LIMIT);
    free(memory);

    surf_protocol_workspace_config_init(&config);
    config.string_capacity = 3;
    size = surf_protocol_workspace_size(&config);
    memory = malloc(size);
    SURF_CHECK(memory != NULL);
    SURF_CHECK(surf_protocol_workspace_init(&workspace, memory, size, &config) ==
               SURF_PROTOCOL_OK);
    SURF_CHECK(surf_protocol_decode_event(workspace, tabs, strlen(tabs),
                                          &event) ==
               SURF_PROTOCOL_ERROR_LIMIT);
    free(memory);
}

static void test_unescaped_strings_and_typed_arrays(void) {
    void *memory;
    surf_protocol_workspace_t *workspace = create_workspace(&memory);
    surf_protocol_event_t event;
    const char *json =
        "{\"t\":\"tabs\",\"tabs\":[{\"id\":7,\"title\":\"h\\u00e9\\nllo\","
        "\"url\":\"https://example.test\",\"active\":true}]}";
    SURF_CHECK(surf_protocol_decode_event(workspace, json, strlen(json),
                                          &event) == SURF_PROTOCOL_OK);
    SURF_CHECK(event.kind == SURF_PROTOCOL_EVENT_TABS);
    SURF_CHECK(event.data.tabs.count == 1);
    SURF_CHECK(event.data.tabs.items[0].id == 7);
    SURF_CHECK(surf_string_equal(
        event.data.tabs.items[0].title,
        surf_string_view("h\xc3\xa9\nllo", strlen("h\xc3\xa9\nllo"))));
    free(memory);
}

static void initialize_command(surf_protocol_command_t *command,
                               surf_protocol_command_kind_t kind) {
    static const surf_protocol_touch_point_t point = {
        1, 0.5, 0.4, 0.01, 0.02, 0.5
    };
    static const int32_t indices[] = {0, 2};
    memset(command, 0, sizeof(*command));
    command->kind = kind;
    command->causal.interaction_id = 1;
    command->causal.client_ns = 2;
    command->data.size.width = 1180;
    command->data.size.height = 680;
    switch (kind) {
    case SURF_PROTOCOL_COMMAND_CLOCK:
        command->data.clock.client_send_ns = 123;
        break;
    case SURF_PROTOCOL_COMMAND_TAB:
        command->data.tab.action = surf_string_from_cstr("select");
        command->data.tab.id = 7;
        break;
    case SURF_PROTOCOL_COMMAND_NAVIGATE:
    case SURF_PROTOCOL_COMMAND_OPEN_NEW:
    case SURF_PROTOCOL_COMMAND_BOOKMARK_DELETE:
        command->data.url.url = surf_string_from_cstr("https://example.com");
        break;
    case SURF_PROTOCOL_COMMAND_HISTORY_DELETE:
        command->data.history_delete.url =
            surf_string_from_cstr("https://example.com/old");
        command->data.history_delete.timestamp = 42;
        break;
    case SURF_PROTOCOL_COMMAND_AUDIO:
    case SURF_PROTOCOL_COMMAND_MOBILE:
    case SURF_PROTOCOL_COMMAND_DARK:
    case SURF_PROTOCOL_COMMAND_FULLSCREEN:
        command->data.toggle.on = 1;
        break;
    case SURF_PROTOCOL_COMMAND_TOUCH:
        command->data.touch.phase = surf_string_from_cstr("move");
        command->data.touch.sequence = 3;
        command->data.touch.surface = 7;
        command->data.touch.timestamp_ns = 99;
        command->data.touch.points = &point;
        command->data.touch.point_count = 1;
        break;
    case SURF_PROTOCOL_COMMAND_POINTER:
        command->data.pointer.phase = surf_string_from_cstr("down");
        command->data.pointer.button = surf_string_from_cstr("left");
        command->data.pointer.sequence = 4;
        command->data.pointer.surface = 7;
        command->data.pointer.timestamp_ns = 100;
        command->data.pointer.x = 0.5;
        command->data.pointer.y = 0.4;
        command->data.pointer.buttons = 1;
        command->data.pointer.clicks = 1;
        break;
    case SURF_PROTOCOL_COMMAND_WHEEL:
        command->data.wheel.sequence = 5;
        command->data.wheel.surface = 7;
        command->data.wheel.timestamp_ns = 101;
        command->data.wheel.x = 0.5;
        command->data.wheel.y = 0.4;
        command->data.wheel.delta_y = 0.1;
        break;
    case SURF_PROTOCOL_COMMAND_KEY:
        command->data.key.down = 1;
        command->data.key.key = surf_string_from_cstr("a");
        command->data.key.code = surf_string_from_cstr("KeyA");
        command->data.key.key_code = 65;
        command->data.key.text = surf_string_from_cstr("a");
        break;
    case SURF_PROTOCOL_COMMAND_PASTE:
    case SURF_PROTOCOL_COMMAND_CLIPBOARD_CHANGE:
        command->data.text.text = surf_string_from_cstr("copied");
        break;
    case SURF_PROTOCOL_COMMAND_COMPOSE:
        command->data.compose.phase = surf_string_from_cstr("update");
        command->data.compose.text = surf_string_from_cstr("h\xc3\xa9");
        command->data.compose.end = 2;
        break;
    case SURF_PROTOCOL_COMMAND_SUGGEST:
    case SURF_PROTOCOL_COMMAND_HISTORY:
        command->data.query.query = surf_string_from_cstr("surf");
        command->data.query.offset = 20;
        break;
    case SURF_PROTOCOL_COMMAND_FIND:
        command->data.find.query = surf_string_from_cstr("needle");
        command->data.find.direction = 1;
        break;
    case SURF_PROTOCOL_COMMAND_DOWNLOAD_DELETE:
        command->data.name.name = surf_string_from_cstr("archive.zip");
        break;
    case SURF_PROTOCOL_COMMAND_CLEAR:
        command->data.clear.what = surf_string_from_cstr("history");
        break;
    case SURF_PROTOCOL_COMMAND_DIALOG_REPLY:
        command->data.dialog_reply.accept = 1;
        command->data.dialog_reply.text = surf_string_from_cstr("answer");
        break;
    case SURF_PROTOCOL_COMMAND_SELECT_REPLY:
        command->data.select_reply.id = surf_string_from_cstr("request-1");
        command->data.select_reply.indices = indices;
        command->data.select_reply.index_count = 2;
        break;
    case SURF_PROTOCOL_COMMAND_MEDIA_STATS:
        command->data.media_stats.fps = 60.0;
        command->data.media_stats.presented_fps = 60.0;
        command->data.media_stats.decode_fps = 60.0;
        command->data.media_stats.au_rate = 60.0;
        command->data.media_stats.renderer = surf_string_from_cstr("gl");
        command->data.media_stats.renderer_fps = 60.0;
        command->data.media_stats.renderer_ms = 1.0;
        command->data.media_stats.callback_ms = 0.2;
        command->data.media_stats.gap_ms = 16.7;
        command->data.media_stats.frame_age_ms = 5.0;
        command->data.media_stats.window_ms = 1000.0;
        break;
    case SURF_PROTOCOL_COMMAND_MEDIA_VOLUME:
        command->data.volume.value = 0.75;
        break;
    case SURF_PROTOCOL_COMMAND_BROWSER_RESUME:
        command->data.browser_resume.revision = 7;
        break;
    case SURF_PROTOCOL_COMMAND_CLIPBOARD_RESULT:
        command->data.clipboard_result.id = surf_string_from_cstr("clip-1");
        command->data.clipboard_result.ok = 1;
        break;
    case SURF_PROTOCOL_COMMAND_LOG_RECORD:
        command->data.log_record.json =
            surf_string_from_cstr("{\"level\":\"info\"}");
        break;
    default:
        break;
    }
}

static void emit_commands(void) {
    surf_protocol_command_kind_t kind;
    for (kind = SURF_PROTOCOL_COMMAND_SIZE;
         kind <= SURF_PROTOCOL_COMMAND_BROWSER_RESUME;
         kind = (surf_protocol_command_kind_t)(kind + 1)) {
        surf_protocol_command_t command;
        char buffer[8192];
        size_t length = 0;
        initialize_command(&command, kind);
        SURF_CHECK(surf_protocol_encode_command(&command, buffer,
                                                sizeof(buffer), &length) ==
                   SURF_PROTOCOL_OK);
        SURF_CHECK(length < sizeof(buffer));
        puts(buffer);
    }
}

static void test_command_encoding(void) {
    surf_protocol_command_t command;
    char buffer[256];
    size_t length;
    initialize_command(&command, SURF_PROTOCOL_COMMAND_NAVIGATE);
    command.data.url.url = surf_string_from_cstr("https://x.test/\"line\n");
    SURF_CHECK(surf_protocol_encode_command(&command, buffer, sizeof(buffer),
                                            &length) == SURF_PROTOCOL_OK);
    SURF_CHECK(strstr(buffer, "https://x.test/\\\"line\\n") != NULL);
    SURF_CHECK(surf_protocol_encode_command(&command, buffer, 8, &length) ==
               SURF_PROTOCOL_ERROR_BUFFER);
    SURF_CHECK(length > 8);
    initialize_command(&command, SURF_PROTOCOL_COMMAND_BROWSER_RESUME);
    command.data.browser_resume.revision = 0;
    SURF_CHECK(surf_protocol_encode_command(&command, buffer, sizeof(buffer),
                                            &length) == SURF_PROTOCOL_ERROR_FIELD);
}

int main(int argc, char **argv) {
    if (argc == 2 && strcmp(argv[1], "--emit-commands") == 0) {
        emit_commands();
        return EXIT_SUCCESS;
    }
    test_event_fixtures();
    test_strict_event_rejection();
    test_unescaped_strings_and_typed_arrays();
    test_workspace_limits();
    test_command_encoding();
    puts("surf_protocol: all tests passed");
    return EXIT_SUCCESS;
}
