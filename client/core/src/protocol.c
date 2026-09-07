#include "surf/protocol.h"

#include <errno.h>
#include <inttypes.h>
#include <limits.h>
#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef enum surf_json_type {
    SURF_JSON_OBJECT = 1,
    SURF_JSON_ARRAY,
    SURF_JSON_STRING,
    SURF_JSON_PRIMITIVE
} surf_json_type_t;

typedef struct surf_json_token {
    surf_json_type_t type;
    size_t start;
    size_t end;
    size_t parent;
    size_t size;
} surf_json_token_t;

struct surf_protocol_workspace {
    surf_json_token_t *tokens;
    size_t token_capacity;
    size_t token_count;
    char *strings;
    size_t string_capacity;
    size_t string_used;
    surf_tab_event_t *tabs;
    surf_protocol_library_entry_t *entries;
    surf_protocol_download_item_t *downloads;
    surf_protocol_select_option_t *options;
    size_t item_capacity;
    size_t tab_used;
    size_t entry_used;
    size_t download_used;
    size_t option_used;
};

typedef struct surf_json_parser {
    surf_protocol_workspace_t *workspace;
    const char *json;
    size_t length;
    size_t position;
} surf_json_parser_t;

typedef struct surf_writer {
    char *buffer;
    size_t capacity;
    size_t length;
    int failed;
} surf_writer_t;

static size_t surf_align_size(size_t value, size_t alignment) {
    size_t remainder = value % alignment;
    if (remainder == 0) {
        return value;
    }
    if (value > SIZE_MAX - (alignment - remainder)) {
        return SIZE_MAX;
    }
    return value + (alignment - remainder);
}

static int surf_add_size(size_t *value, size_t count, size_t width,
                         size_t alignment) {
    size_t aligned = surf_align_size(*value, alignment);
    if (aligned == SIZE_MAX || count > (SIZE_MAX - aligned) / width) {
        return 0;
    }
    *value = aligned + count * width;
    return 1;
}

void surf_protocol_workspace_config_init(
    surf_protocol_workspace_config_t *config) {
    if (config == NULL) {
        return;
    }
    config->token_capacity = SURF_PROTOCOL_DEFAULT_TOKENS;
    config->item_capacity = SURF_PROTOCOL_DEFAULT_ITEMS;
    config->string_capacity = SURF_PROTOCOL_DEFAULT_STRINGS;
}

static surf_protocol_workspace_config_t surf_workspace_config(
    const surf_protocol_workspace_config_t *config) {
    surf_protocol_workspace_config_t resolved;
    surf_protocol_workspace_config_init(&resolved);
    if (config != NULL) {
        resolved = *config;
    }
    return resolved;
}

size_t surf_protocol_workspace_size(
    const surf_protocol_workspace_config_t *config) {
    surf_protocol_workspace_config_t resolved = surf_workspace_config(config);
    size_t size = sizeof(surf_protocol_workspace_t);
    if (resolved.token_capacity == 0 || resolved.item_capacity == 0 ||
        resolved.string_capacity == 0 ||
        !surf_add_size(&size, resolved.token_capacity,
                       sizeof(surf_json_token_t), sizeof(void *)) ||
        !surf_add_size(&size, resolved.string_capacity, sizeof(char),
                       sizeof(void *)) ||
        !surf_add_size(&size, resolved.item_capacity, sizeof(surf_tab_event_t),
                       sizeof(void *)) ||
        !surf_add_size(&size, resolved.item_capacity,
                       sizeof(surf_protocol_library_entry_t), sizeof(void *)) ||
        !surf_add_size(&size, resolved.item_capacity,
                       sizeof(surf_protocol_download_item_t), sizeof(void *)) ||
        !surf_add_size(&size, resolved.item_capacity,
                       sizeof(surf_protocol_select_option_t), sizeof(void *))) {
        return 0;
    }
    return size;
}

static void *surf_workspace_slice(unsigned char *base, size_t *offset,
                                  size_t count, size_t width) {
    *offset = surf_align_size(*offset, sizeof(void *));
    {
        void *value = base + *offset;
        *offset += count * width;
        return value;
    }
}

surf_protocol_result_t surf_protocol_workspace_init(
    surf_protocol_workspace_t **out_workspace, void *memory, size_t memory_size,
    const surf_protocol_workspace_config_t *config) {
    surf_protocol_workspace_config_t resolved = surf_workspace_config(config);
    size_t required = surf_protocol_workspace_size(&resolved);
    size_t offset = sizeof(surf_protocol_workspace_t);
    unsigned char *base = (unsigned char *)memory;
    surf_protocol_workspace_t *workspace;
    if (out_workspace == NULL || memory == NULL ||
        (uintptr_t)memory % sizeof(void *) != 0) {
        return SURF_PROTOCOL_ERROR_ARGUMENT;
    }
    *out_workspace = NULL;
    if (required == 0 || memory_size < required) {
        return SURF_PROTOCOL_ERROR_BUFFER;
    }
    memset(memory, 0, required);
    workspace = (surf_protocol_workspace_t *)memory;
    workspace->tokens = (surf_json_token_t *)surf_workspace_slice(
        base, &offset, resolved.token_capacity, sizeof(surf_json_token_t));
    workspace->strings = (char *)surf_workspace_slice(
        base, &offset, resolved.string_capacity, sizeof(char));
    workspace->tabs = (surf_tab_event_t *)surf_workspace_slice(
        base, &offset, resolved.item_capacity, sizeof(surf_tab_event_t));
    workspace->entries = (surf_protocol_library_entry_t *)surf_workspace_slice(
        base, &offset, resolved.item_capacity,
        sizeof(surf_protocol_library_entry_t));
    workspace->downloads = (surf_protocol_download_item_t *)surf_workspace_slice(
        base, &offset, resolved.item_capacity,
        sizeof(surf_protocol_download_item_t));
    workspace->options = (surf_protocol_select_option_t *)surf_workspace_slice(
        base, &offset, resolved.item_capacity,
        sizeof(surf_protocol_select_option_t));
    workspace->token_capacity = resolved.token_capacity;
    workspace->item_capacity = resolved.item_capacity;
    workspace->string_capacity = resolved.string_capacity;
    *out_workspace = workspace;
    return SURF_PROTOCOL_OK;
}

static int surf_json_space(char value) {
    return value == ' ' || value == '\t' || value == '\r' || value == '\n';
}

static void surf_json_skip_space(surf_json_parser_t *parser) {
    while (parser->position < parser->length &&
           surf_json_space(parser->json[parser->position])) {
        parser->position++;
    }
}

static surf_protocol_result_t surf_json_token(surf_json_parser_t *parser,
                                              surf_json_type_t type,
                                              size_t parent,
                                              size_t *out_index) {
    surf_protocol_workspace_t *workspace = parser->workspace;
    surf_json_token_t *token;
    if (workspace->token_count >= workspace->token_capacity) {
        return SURF_PROTOCOL_ERROR_LIMIT;
    }
    *out_index = workspace->token_count++;
    token = &workspace->tokens[*out_index];
    memset(token, 0, sizeof(*token));
    token->type = type;
    token->start = parser->position;
    token->parent = parent;
    return SURF_PROTOCOL_OK;
}

static int surf_json_hex(char value) {
    if (value >= '0' && value <= '9') return value - '0';
    if (value >= 'a' && value <= 'f') return value - 'a' + 10;
    if (value >= 'A' && value <= 'F') return value - 'A' + 10;
    return -1;
}

static surf_protocol_result_t surf_json_parse_value(
    surf_json_parser_t *parser, size_t parent, size_t depth,
    size_t *out_index);

static surf_protocol_result_t surf_json_parse_string(
    surf_json_parser_t *parser, size_t parent, size_t *out_index) {
    surf_protocol_result_t result;
    surf_json_token_t *token;
    size_t index;
    if (parser->position >= parser->length ||
        parser->json[parser->position] != '"') {
        return SURF_PROTOCOL_ERROR_JSON;
    }
    parser->position++;
    result = surf_json_token(parser, SURF_JSON_STRING, parent, &index);
    if (result != SURF_PROTOCOL_OK) return result;
    token = &parser->workspace->tokens[index];
    token->start = parser->position;
    while (parser->position < parser->length) {
        unsigned char value = (unsigned char)parser->json[parser->position++];
        if (value == '"') {
            token->end = parser->position - 1;
            *out_index = index;
            return SURF_PROTOCOL_OK;
        }
        if (value < 0x20) return SURF_PROTOCOL_ERROR_JSON;
        if (value == '\\') {
            char escaped;
            if (parser->position >= parser->length) {
                return SURF_PROTOCOL_ERROR_JSON;
            }
            escaped = parser->json[parser->position++];
            if (escaped == 'u') {
                size_t count;
                for (count = 0; count < 4; count++) {
                    if (parser->position >= parser->length ||
                        surf_json_hex(parser->json[parser->position++]) < 0) {
                        return SURF_PROTOCOL_ERROR_JSON;
                    }
                }
            } else if (strchr("\"\\/bfnrt", escaped) == NULL) {
                return SURF_PROTOCOL_ERROR_JSON;
            }
        }
    }
    return SURF_PROTOCOL_ERROR_JSON;
}

static surf_protocol_result_t surf_json_parse_primitive(
    surf_json_parser_t *parser, size_t parent, size_t *out_index) {
    surf_protocol_result_t result;
    surf_json_token_t *token;
    size_t index;
    size_t start = parser->position;
    result = surf_json_token(parser, SURF_JSON_PRIMITIVE, parent, &index);
    if (result != SURF_PROTOCOL_OK) return result;
    while (parser->position < parser->length) {
        char value = parser->json[parser->position];
        if (surf_json_space(value) || value == ',' || value == ']' ||
            value == '}') {
            break;
        }
        if ((unsigned char)value < 0x20 || value == ':' || value == '[' ||
            value == '{' || value == '"') {
            return SURF_PROTOCOL_ERROR_JSON;
        }
        parser->position++;
    }
    if (parser->position == start) return SURF_PROTOCOL_ERROR_JSON;
    token = &parser->workspace->tokens[index];
    token->start = start;
    token->end = parser->position;
    {
        size_t cursor = start;
        size_t end = parser->position;
        int number = 1;
        if ((end - start == 4 &&
             (memcmp(parser->json + start, "true", 4) == 0 ||
              memcmp(parser->json + start, "null", 4) == 0)) ||
            (end - start == 5 &&
             memcmp(parser->json + start, "false", 5) == 0)) {
            number = 0;
        }
        if (number) {
            if (cursor < end && parser->json[cursor] == '-') cursor++;
            if (cursor >= end) return SURF_PROTOCOL_ERROR_JSON;
            if (parser->json[cursor] == '0') {
                cursor++;
                if (cursor < end && parser->json[cursor] >= '0' &&
                    parser->json[cursor] <= '9') {
                    return SURF_PROTOCOL_ERROR_JSON;
                }
            } else if (parser->json[cursor] >= '1' &&
                       parser->json[cursor] <= '9') {
                do { cursor++; }
                while (cursor < end && parser->json[cursor] >= '0' &&
                       parser->json[cursor] <= '9');
            } else {
                return SURF_PROTOCOL_ERROR_JSON;
            }
            if (cursor < end && parser->json[cursor] == '.') {
                cursor++;
                if (cursor >= end || parser->json[cursor] < '0' ||
                    parser->json[cursor] > '9') {
                    return SURF_PROTOCOL_ERROR_JSON;
                }
                do { cursor++; }
                while (cursor < end && parser->json[cursor] >= '0' &&
                       parser->json[cursor] <= '9');
            }
            if (cursor < end &&
                (parser->json[cursor] == 'e' ||
                 parser->json[cursor] == 'E')) {
                cursor++;
                if (cursor < end &&
                    (parser->json[cursor] == '+' ||
                     parser->json[cursor] == '-')) cursor++;
                if (cursor >= end || parser->json[cursor] < '0' ||
                    parser->json[cursor] > '9') {
                    return SURF_PROTOCOL_ERROR_JSON;
                }
                do { cursor++; }
                while (cursor < end && parser->json[cursor] >= '0' &&
                       parser->json[cursor] <= '9');
            }
            if (cursor != end) return SURF_PROTOCOL_ERROR_JSON;
        }
    }
    *out_index = index;
    return SURF_PROTOCOL_OK;
}

static surf_protocol_result_t surf_json_parse_array(
    surf_json_parser_t *parser, size_t parent, size_t depth,
    size_t *out_index) {
    surf_protocol_result_t result;
    size_t index;
    surf_json_token_t *token;
    parser->position++;
    result = surf_json_token(parser, SURF_JSON_ARRAY, parent, &index);
    if (result != SURF_PROTOCOL_OK) return result;
    token = &parser->workspace->tokens[index];
    token->start = parser->position - 1;
    surf_json_skip_space(parser);
    if (parser->position < parser->length &&
        parser->json[parser->position] == ']') {
        parser->position++;
        token->end = parser->position;
        *out_index = index;
        return SURF_PROTOCOL_OK;
    }
    for (;;) {
        size_t child;
        result = surf_json_parse_value(parser, index, depth + 1, &child);
        if (result != SURF_PROTOCOL_OK) return result;
        (void)child;
        token = &parser->workspace->tokens[index];
        token->size++;
        surf_json_skip_space(parser);
        if (parser->position >= parser->length) return SURF_PROTOCOL_ERROR_JSON;
        if (parser->json[parser->position] == ']') {
            parser->position++;
            token->end = parser->position;
            *out_index = index;
            return SURF_PROTOCOL_OK;
        }
        if (parser->json[parser->position++] != ',') {
            return SURF_PROTOCOL_ERROR_JSON;
        }
        surf_json_skip_space(parser);
    }
}

static surf_protocol_result_t surf_json_parse_object(
    surf_json_parser_t *parser, size_t parent, size_t depth,
    size_t *out_index) {
    surf_protocol_result_t result;
    size_t index;
    surf_json_token_t *token;
    parser->position++;
    result = surf_json_token(parser, SURF_JSON_OBJECT, parent, &index);
    if (result != SURF_PROTOCOL_OK) return result;
    token = &parser->workspace->tokens[index];
    token->start = parser->position - 1;
    surf_json_skip_space(parser);
    if (parser->position < parser->length &&
        parser->json[parser->position] == '}') {
        parser->position++;
        token->end = parser->position;
        *out_index = index;
        return SURF_PROTOCOL_OK;
    }
    for (;;) {
        size_t key, value;
        result = surf_json_parse_string(parser, index, &key);
        if (result != SURF_PROTOCOL_OK) return result;
        surf_json_skip_space(parser);
        if (parser->position >= parser->length ||
            parser->json[parser->position++] != ':') {
            return SURF_PROTOCOL_ERROR_JSON;
        }
        surf_json_skip_space(parser);
        result = surf_json_parse_value(parser, index, depth + 1, &value);
        if (result != SURF_PROTOCOL_OK) return result;
        (void)key;
        (void)value;
        token = &parser->workspace->tokens[index];
        token->size++;
        surf_json_skip_space(parser);
        if (parser->position >= parser->length) return SURF_PROTOCOL_ERROR_JSON;
        if (parser->json[parser->position] == '}') {
            parser->position++;
            token->end = parser->position;
            *out_index = index;
            return SURF_PROTOCOL_OK;
        }
        if (parser->json[parser->position++] != ',') {
            return SURF_PROTOCOL_ERROR_JSON;
        }
        surf_json_skip_space(parser);
    }
}

static surf_protocol_result_t surf_json_parse_value(
    surf_json_parser_t *parser, size_t parent, size_t depth,
    size_t *out_index) {
    if (depth > SURF_PROTOCOL_MAX_DEPTH) {
        return SURF_PROTOCOL_ERROR_LIMIT;
    }
    if (parser->position >= parser->length) return SURF_PROTOCOL_ERROR_JSON;
    switch (parser->json[parser->position]) {
    case '{':
        return surf_json_parse_object(parser, parent, depth, out_index);
    case '[':
        return surf_json_parse_array(parser, parent, depth, out_index);
    case '"':
        return surf_json_parse_string(parser, parent, out_index);
    default:
        return surf_json_parse_primitive(parser, parent, out_index);
    }
}

static surf_protocol_result_t surf_json_parse(surf_json_parser_t *parser,
                                              size_t *out_root) {
    surf_protocol_result_t result;
    parser->position = 0;
    parser->workspace->token_count = 0;
    surf_json_skip_space(parser);
    result = surf_json_parse_value(parser, SIZE_MAX, 0, out_root);
    if (result != SURF_PROTOCOL_OK) return result;
    surf_json_skip_space(parser);
    if (parser->position != parser->length) return SURF_PROTOCOL_ERROR_JSON;
    return SURF_PROTOCOL_OK;
}

static int surf_token_raw_equal(const surf_json_parser_t *parser,
                                size_t token_index, const char *value) {
    const surf_json_token_t *token = &parser->workspace->tokens[token_index];
    size_t length = strlen(value);
    return token->type == SURF_JSON_STRING && token->end - token->start == length &&
           memcmp(parser->json + token->start, value, length) == 0;
}

static surf_protocol_result_t surf_object_field(
    const surf_json_parser_t *parser, size_t object, const char *name,
    int required, size_t *out_token) {
    size_t index;
    size_t match = SIZE_MAX;
    int expect_value = 0;
    if (parser->workspace->tokens[object].type != SURF_JSON_OBJECT) {
        return SURF_PROTOCOL_ERROR_TYPE;
    }
    for (index = object + 1; index < parser->workspace->token_count; index++) {
        const surf_json_token_t *token = &parser->workspace->tokens[index];
        if (token->parent != object) continue;
        if (!expect_value) {
            if (token->type != SURF_JSON_STRING) return SURF_PROTOCOL_ERROR_JSON;
            expect_value = surf_token_raw_equal(parser, index, name) ? 2 : 1;
        } else {
            if (expect_value == 2) {
                if (match != SIZE_MAX) return SURF_PROTOCOL_ERROR_FIELD;
                match = index;
            }
            expect_value = 0;
        }
    }
    if (expect_value != 0) return SURF_PROTOCOL_ERROR_JSON;
    if (match == SIZE_MAX && required) return SURF_PROTOCOL_ERROR_FIELD;
    *out_token = match;
    return SURF_PROTOCOL_OK;
}

static int surf_field_allowed(const surf_json_parser_t *parser,
                              size_t key_token, const char *const *allowed,
                              size_t allowed_count) {
    size_t index;
    for (index = 0; index < allowed_count; index++) {
        if (surf_token_raw_equal(parser, key_token, allowed[index])) return 1;
    }
    return 0;
}

static surf_protocol_result_t surf_object_fields(
    const surf_json_parser_t *parser, size_t object,
    const char *const *allowed, size_t allowed_count) {
    size_t index;
    int expect_value = 0;
    for (index = object + 1; index < parser->workspace->token_count; index++) {
        const surf_json_token_t *token = &parser->workspace->tokens[index];
        if (token->parent != object) continue;
        if (!expect_value) {
            if (token->type != SURF_JSON_STRING ||
                !surf_field_allowed(parser, index, allowed, allowed_count)) {
                return SURF_PROTOCOL_ERROR_FIELD;
            }
            expect_value = 1;
        } else {
            expect_value = 0;
        }
    }
    return expect_value ? SURF_PROTOCOL_ERROR_JSON : SURF_PROTOCOL_OK;
}

static surf_protocol_result_t surf_string_append_utf8(
    surf_protocol_workspace_t *workspace, uint32_t value) {
    size_t needed = value <= 0x7f ? 1 : value <= 0x7ff ? 2 : value <= 0xffff ? 3 : 4;
    char *output;
    if (workspace->string_used > workspace->string_capacity - needed) {
        return SURF_PROTOCOL_ERROR_LIMIT;
    }
    output = workspace->strings + workspace->string_used;
    if (needed == 1) {
        output[0] = (char)value;
    } else if (needed == 2) {
        output[0] = (char)(0xc0u | (value >> 6));
        output[1] = (char)(0x80u | (value & 0x3fu));
    } else if (needed == 3) {
        output[0] = (char)(0xe0u | (value >> 12));
        output[1] = (char)(0x80u | ((value >> 6) & 0x3fu));
        output[2] = (char)(0x80u | (value & 0x3fu));
    } else {
        output[0] = (char)(0xf0u | (value >> 18));
        output[1] = (char)(0x80u | ((value >> 12) & 0x3fu));
        output[2] = (char)(0x80u | ((value >> 6) & 0x3fu));
        output[3] = (char)(0x80u | (value & 0x3fu));
    }
    workspace->string_used += needed;
    return SURF_PROTOCOL_OK;
}

static surf_protocol_result_t surf_token_string(
    surf_json_parser_t *parser, size_t token_index, surf_string_view_t *out) {
    const surf_json_token_t *token;
    size_t position;
    size_t start;
    if (token_index == SIZE_MAX) {
        *out = surf_string_view(NULL, 0);
        return SURF_PROTOCOL_OK;
    }
    token = &parser->workspace->tokens[token_index];
    if (token->type != SURF_JSON_STRING) return SURF_PROTOCOL_ERROR_TYPE;
    start = parser->workspace->string_used;
    for (position = token->start; position < token->end; position++) {
        unsigned char value = (unsigned char)parser->json[position];
        if (value != '\\') {
            if (parser->workspace->string_used >=
                parser->workspace->string_capacity) {
                return SURF_PROTOCOL_ERROR_LIMIT;
            }
            parser->workspace->strings[parser->workspace->string_used++] =
                (char)value;
            continue;
        }
        position++;
        if (position >= token->end) return SURF_PROTOCOL_ERROR_JSON;
        switch (parser->json[position]) {
        case '"': value = '"'; break;
        case '\\': value = '\\'; break;
        case '/': value = '/'; break;
        case 'b': value = '\b'; break;
        case 'f': value = '\f'; break;
        case 'n': value = '\n'; break;
        case 'r': value = '\r'; break;
        case 't': value = '\t'; break;
        case 'u': {
            uint32_t code = 0;
            size_t count;
            for (count = 0; count < 4; count++) {
                int digit = surf_json_hex(parser->json[++position]);
                code = (code << 4) | (uint32_t)digit;
            }
            if (code >= 0xd800 && code <= 0xdbff) {
                uint32_t low = 0;
                if (position + 6 >= token->end || parser->json[position + 1] != '\\' ||
                    parser->json[position + 2] != 'u') {
                    return SURF_PROTOCOL_ERROR_JSON;
                }
                position += 2;
                for (count = 0; count < 4; count++) {
                    int digit = surf_json_hex(parser->json[++position]);
                    low = (low << 4) | (uint32_t)digit;
                }
                if (low < 0xdc00 || low > 0xdfff) return SURF_PROTOCOL_ERROR_JSON;
                code = 0x10000u + ((code - 0xd800u) << 10) + (low - 0xdc00u);
            } else if (code >= 0xdc00 && code <= 0xdfff) {
                return SURF_PROTOCOL_ERROR_JSON;
            }
            {
                surf_protocol_result_t result =
                    surf_string_append_utf8(parser->workspace, code);
                if (result != SURF_PROTOCOL_OK) return result;
            }
            continue;
        }
        default:
            return SURF_PROTOCOL_ERROR_JSON;
        }
        if (parser->workspace->string_used >=
            parser->workspace->string_capacity) {
            return SURF_PROTOCOL_ERROR_LIMIT;
        }
        parser->workspace->strings[parser->workspace->string_used++] =
            (char)value;
    }
    out->data = parser->workspace->strings + start;
    out->length = parser->workspace->string_used - start;
    {
        size_t cursor = 0;
        while (cursor < out->length) {
            const unsigned char *bytes =
                (const unsigned char *)out->data + cursor;
            uint32_t code;
            size_t width;
            if (bytes[0] < 0x80) {
                cursor++;
                continue;
            } else if ((bytes[0] & 0xe0u) == 0xc0u) {
                code = bytes[0] & 0x1fu;
                width = 2;
                if (code < 2) return SURF_PROTOCOL_ERROR_JSON;
            } else if ((bytes[0] & 0xf0u) == 0xe0u) {
                code = bytes[0] & 0x0fu;
                width = 3;
            } else if ((bytes[0] & 0xf8u) == 0xf0u) {
                code = bytes[0] & 0x07u;
                width = 4;
            } else {
                return SURF_PROTOCOL_ERROR_JSON;
            }
            if (width > out->length - cursor) return SURF_PROTOCOL_ERROR_JSON;
            {
                size_t offset;
                for (offset = 1; offset < width; offset++) {
                    if ((bytes[offset] & 0xc0u) != 0x80u)
                        return SURF_PROTOCOL_ERROR_JSON;
                    code = (code << 6) | (bytes[offset] & 0x3fu);
                }
            }
            if ((width == 3 && code < 0x800) ||
                (width == 4 && code < 0x10000) || code > 0x10ffff ||
                (code >= 0xd800 && code <= 0xdfff)) {
                return SURF_PROTOCOL_ERROR_JSON;
            }
            cursor += width;
        }
    }
    return SURF_PROTOCOL_OK;
}

static int surf_token_is(const surf_json_parser_t *parser, size_t token_index,
                         const char *value) {
    const surf_json_token_t *token = &parser->workspace->tokens[token_index];
    size_t length = strlen(value);
    return token->type == SURF_JSON_PRIMITIVE &&
           token->end - token->start == length &&
           memcmp(parser->json + token->start, value, length) == 0;
}

static surf_protocol_result_t surf_token_bool(const surf_json_parser_t *parser,
                                              size_t token_index,
                                              int default_value,
                                              int *out) {
    if (token_index == SIZE_MAX) {
        *out = default_value;
        return SURF_PROTOCOL_OK;
    }
    if (surf_token_is(parser, token_index, "true")) *out = 1;
    else if (surf_token_is(parser, token_index, "false")) *out = 0;
    else return SURF_PROTOCOL_ERROR_TYPE;
    return SURF_PROTOCOL_OK;
}

static surf_protocol_result_t surf_token_number_text(
    const surf_json_parser_t *parser, size_t token_index, char *buffer,
    size_t capacity) {
    const surf_json_token_t *token;
    size_t length;
    if (token_index == SIZE_MAX) return SURF_PROTOCOL_ERROR_FIELD;
    token = &parser->workspace->tokens[token_index];
    length = token->end - token->start;
    if (token->type != SURF_JSON_PRIMITIVE || length == 0 || length >= capacity) {
        return SURF_PROTOCOL_ERROR_TYPE;
    }
    memcpy(buffer, parser->json + token->start, length);
    buffer[length] = '\0';
    return SURF_PROTOCOL_OK;
}

static surf_protocol_result_t surf_token_i64(const surf_json_parser_t *parser,
                                             size_t token_index,
                                             int64_t default_value,
                                             int64_t *out) {
    char buffer[64];
    char *end;
    long long value;
    surf_protocol_result_t result;
    if (token_index == SIZE_MAX) {
        *out = default_value;
        return SURF_PROTOCOL_OK;
    }
    result = surf_token_number_text(parser, token_index, buffer, sizeof(buffer));
    if (result != SURF_PROTOCOL_OK) return result;
    errno = 0;
    value = strtoll(buffer, &end, 10);
    if (errno != 0 || *end != '\0') return SURF_PROTOCOL_ERROR_TYPE;
    *out = (int64_t)value;
    return SURF_PROTOCOL_OK;
}

static surf_protocol_result_t surf_token_u64(const surf_json_parser_t *parser,
                                             size_t token_index,
                                             uint64_t default_value,
                                             uint64_t *out) {
    char buffer[64];
    char *end;
    unsigned long long value;
    surf_protocol_result_t result;
    if (token_index == SIZE_MAX) {
        *out = default_value;
        return SURF_PROTOCOL_OK;
    }
    result = surf_token_number_text(parser, token_index, buffer, sizeof(buffer));
    if (result != SURF_PROTOCOL_OK || buffer[0] == '-') {
        return SURF_PROTOCOL_ERROR_TYPE;
    }
    errno = 0;
    value = strtoull(buffer, &end, 10);
    if (errno != 0 || *end != '\0') return SURF_PROTOCOL_ERROR_TYPE;
    *out = (uint64_t)value;
    return SURF_PROTOCOL_OK;
}

static surf_protocol_result_t surf_token_i32(const surf_json_parser_t *parser,
                                             size_t token_index,
                                             int32_t default_value,
                                             int32_t *out) {
    int64_t value;
    surf_protocol_result_t result = surf_token_i64(parser, token_index,
                                                   default_value, &value);
    if (result != SURF_PROTOCOL_OK) return result;
    if (value < INT32_MIN || value > INT32_MAX) return SURF_PROTOCOL_ERROR_LIMIT;
    *out = (int32_t)value;
    return SURF_PROTOCOL_OK;
}

static surf_protocol_result_t surf_token_u32(const surf_json_parser_t *parser,
                                             size_t token_index,
                                             uint32_t default_value,
                                             uint32_t *out) {
    uint64_t value;
    surf_protocol_result_t result = surf_token_u64(parser, token_index,
                                                   default_value, &value);
    if (result != SURF_PROTOCOL_OK) return result;
    if (value > UINT32_MAX) return SURF_PROTOCOL_ERROR_LIMIT;
    *out = (uint32_t)value;
    return SURF_PROTOCOL_OK;
}

static surf_protocol_result_t surf_token_double(const surf_json_parser_t *parser,
                                                size_t token_index,
                                                double default_value,
                                                double *out) {
    char buffer[96];
    char *end;
    double value;
    surf_protocol_result_t result;
    if (token_index == SIZE_MAX) {
        *out = default_value;
        return SURF_PROTOCOL_OK;
    }
    result = surf_token_number_text(parser, token_index, buffer, sizeof(buffer));
    if (result != SURF_PROTOCOL_OK) return result;
    errno = 0;
    value = strtod(buffer, &end);
    if (errno != 0 || *end != '\0' || !isfinite(value)) {
        return SURF_PROTOCOL_ERROR_TYPE;
    }
    *out = value;
    return SURF_PROTOCOL_OK;
}

static surf_protocol_result_t surf_get_field(
    surf_json_parser_t *parser, size_t object, const char *name, int required,
    size_t *out_token) {
    return surf_object_field(parser, object, name, required, out_token);
}

static surf_protocol_result_t surf_get_string(
    surf_json_parser_t *parser, size_t object, const char *name, int required,
    surf_string_view_t *out) {
    size_t token;
    surf_protocol_result_t result =
        surf_get_field(parser, object, name, required, &token);
    if (result != SURF_PROTOCOL_OK) return result;
    return surf_token_string(parser, token, out);
}

static surf_protocol_result_t surf_get_bool(
    surf_json_parser_t *parser, size_t object, const char *name, int required,
    int default_value, int *out) {
    size_t token;
    surf_protocol_result_t result =
        surf_get_field(parser, object, name, required, &token);
    if (result != SURF_PROTOCOL_OK) return result;
    return surf_token_bool(parser, token, default_value, out);
}

static surf_protocol_result_t surf_get_i32(
    surf_json_parser_t *parser, size_t object, const char *name, int required,
    int32_t default_value, int32_t *out) {
    size_t token;
    surf_protocol_result_t result =
        surf_get_field(parser, object, name, required, &token);
    if (result != SURF_PROTOCOL_OK) return result;
    return surf_token_i32(parser, token, default_value, out);
}

static surf_protocol_result_t surf_get_u32(
    surf_json_parser_t *parser, size_t object, const char *name, int required,
    uint32_t default_value, uint32_t *out) {
    size_t token;
    surf_protocol_result_t result =
        surf_get_field(parser, object, name, required, &token);
    if (result != SURF_PROTOCOL_OK) return result;
    return surf_token_u32(parser, token, default_value, out);
}

static surf_protocol_result_t surf_get_i64(
    surf_json_parser_t *parser, size_t object, const char *name, int required,
    int64_t default_value, int64_t *out) {
    size_t token;
    surf_protocol_result_t result =
        surf_get_field(parser, object, name, required, &token);
    if (result != SURF_PROTOCOL_OK) return result;
    return surf_token_i64(parser, token, default_value, out);
}

static surf_protocol_result_t surf_get_u64(
    surf_json_parser_t *parser, size_t object, const char *name, int required,
    uint64_t default_value, uint64_t *out) {
    size_t token;
    surf_protocol_result_t result =
        surf_get_field(parser, object, name, required, &token);
    if (result != SURF_PROTOCOL_OK) return result;
    return surf_token_u64(parser, token, default_value, out);
}

static surf_protocol_result_t surf_get_double(
    surf_json_parser_t *parser, size_t object, const char *name, int required,
    double default_value, double *out) {
    size_t token;
    surf_protocol_result_t result =
        surf_get_field(parser, object, name, required, &token);
    if (result != SURF_PROTOCOL_OK) return result;
    return surf_token_double(parser, token, default_value, out);
}

static int surf_view_equal_cstr(surf_string_view_t view, const char *value) {
    size_t length = strlen(value);
    return view.length == length &&
           (length == 0 || memcmp(view.data, value, length) == 0);
}

static surf_protocol_result_t surf_validate_fields(
    surf_json_parser_t *parser, size_t object, const char *const *fields,
    size_t count) {
    return surf_object_fields(parser, object, fields, count);
}

static surf_protocol_result_t surf_decode_rect(surf_json_parser_t *parser,
                                               size_t token, int *has_rect,
                                               double rect[4]) {
    size_t index;
    size_t count = 0;
    if (token == SIZE_MAX) {
        *has_rect = 0;
        memset(rect, 0, sizeof(double) * 4);
        return SURF_PROTOCOL_OK;
    }
    if (parser->workspace->tokens[token].type != SURF_JSON_ARRAY) {
        return SURF_PROTOCOL_ERROR_TYPE;
    }
    for (index = token + 1; index < parser->workspace->token_count; index++) {
        if (parser->workspace->tokens[index].parent != token) continue;
        if (count >= 4) return SURF_PROTOCOL_ERROR_LIMIT;
        {
            surf_protocol_result_t result =
                surf_token_double(parser, index, 0.0, &rect[count]);
            if (result != SURF_PROTOCOL_OK) return result;
        }
        count++;
    }
    if (count != 0 && count != 4) return SURF_PROTOCOL_ERROR_FIELD;
    *has_rect = count == 4;
    return SURF_PROTOCOL_OK;
}

static surf_protocol_result_t surf_decode_tabs(
    surf_json_parser_t *parser, size_t array, const surf_tab_event_t **out_items,
    size_t *out_count) {
    static const char *const fields[] = {"id", "title", "url", "active", "icon"};
    size_t index;
    size_t start = parser->workspace->tab_used;
    if (array == SIZE_MAX || parser->workspace->tokens[array].type != SURF_JSON_ARRAY) {
        return array == SIZE_MAX ? SURF_PROTOCOL_ERROR_FIELD : SURF_PROTOCOL_ERROR_TYPE;
    }
    for (index = array + 1; index < parser->workspace->token_count; index++) {
        surf_tab_event_t *tab;
        int64_t id;
        surf_protocol_result_t result;
        if (parser->workspace->tokens[index].parent != array) continue;
        if (parser->workspace->tokens[index].type != SURF_JSON_OBJECT) {
            return SURF_PROTOCOL_ERROR_TYPE;
        }
        if (parser->workspace->tab_used >= parser->workspace->item_capacity) {
            return SURF_PROTOCOL_ERROR_LIMIT;
        }
        result = surf_validate_fields(parser, index, fields,
                                      sizeof(fields) / sizeof(fields[0]));
        if (result != SURF_PROTOCOL_OK) return result;
        tab = &parser->workspace->tabs[parser->workspace->tab_used++];
        memset(tab, 0, sizeof(*tab));
        result = surf_get_i64(parser, index, "id", 1, 0, &id);
        if (result == SURF_PROTOCOL_OK) tab->id = id;
        if (result == SURF_PROTOCOL_OK)
            result = surf_get_string(parser, index, "title", 1, &tab->title);
        if (result == SURF_PROTOCOL_OK)
            result = surf_get_string(parser, index, "url", 1, &tab->url);
        if (result == SURF_PROTOCOL_OK)
            result = surf_get_bool(parser, index, "active", 1, 0, &tab->active);
        if (result == SURF_PROTOCOL_OK)
            result = surf_get_string(parser, index, "icon", 0, &tab->icon);
        if (result != SURF_PROTOCOL_OK) return result;
    }
    *out_items = parser->workspace->tabs + start;
    *out_count = parser->workspace->tab_used - start;
    return SURF_PROTOCOL_OK;
}

static surf_protocol_result_t surf_decode_entries(
    surf_json_parser_t *parser, size_t array,
    const surf_protocol_library_entry_t **out_items, size_t *out_count) {
    static const char *const fields[] = {"url", "title", "ts"};
    size_t index;
    size_t start = parser->workspace->entry_used;
    if (array == SIZE_MAX || parser->workspace->tokens[array].type != SURF_JSON_ARRAY) {
        return array == SIZE_MAX ? SURF_PROTOCOL_ERROR_FIELD : SURF_PROTOCOL_ERROR_TYPE;
    }
    for (index = array + 1; index < parser->workspace->token_count; index++) {
        surf_protocol_library_entry_t *entry;
        surf_protocol_result_t result;
        if (parser->workspace->tokens[index].parent != array) continue;
        if (parser->workspace->tokens[index].type != SURF_JSON_OBJECT) {
            return SURF_PROTOCOL_ERROR_TYPE;
        }
        if (parser->workspace->entry_used >= parser->workspace->item_capacity) {
            return SURF_PROTOCOL_ERROR_LIMIT;
        }
        result = surf_validate_fields(parser, index, fields,
                                      sizeof(fields) / sizeof(fields[0]));
        if (result != SURF_PROTOCOL_OK) return result;
        entry = &parser->workspace->entries[parser->workspace->entry_used++];
        memset(entry, 0, sizeof(*entry));
        result = surf_get_string(parser, index, "url", 1, &entry->url);
        if (result == SURF_PROTOCOL_OK)
            result = surf_get_string(parser, index, "title", 1, &entry->title);
        if (result == SURF_PROTOCOL_OK)
            result = surf_get_i64(parser, index, "ts", 1, 0, &entry->timestamp);
        if (result != SURF_PROTOCOL_OK) return result;
    }
    *out_items = parser->workspace->entries + start;
    *out_count = parser->workspace->entry_used - start;
    return SURF_PROTOCOL_OK;
}

static surf_protocol_result_t surf_decode_downloads(
    surf_json_parser_t *parser, size_t array,
    const surf_protocol_download_item_t **out_items, size_t *out_count) {
    static const char *const fields[] = {"name", "size", "ts"};
    size_t index;
    size_t start = parser->workspace->download_used;
    if (array == SIZE_MAX || parser->workspace->tokens[array].type != SURF_JSON_ARRAY) {
        return array == SIZE_MAX ? SURF_PROTOCOL_ERROR_FIELD : SURF_PROTOCOL_ERROR_TYPE;
    }
    for (index = array + 1; index < parser->workspace->token_count; index++) {
        surf_protocol_download_item_t *item;
        surf_protocol_result_t result;
        if (parser->workspace->tokens[index].parent != array) continue;
        if (parser->workspace->tokens[index].type != SURF_JSON_OBJECT) {
            return SURF_PROTOCOL_ERROR_TYPE;
        }
        if (parser->workspace->download_used >= parser->workspace->item_capacity) {
            return SURF_PROTOCOL_ERROR_LIMIT;
        }
        result = surf_validate_fields(parser, index, fields,
                                      sizeof(fields) / sizeof(fields[0]));
        if (result != SURF_PROTOCOL_OK) return result;
        item = &parser->workspace->downloads[parser->workspace->download_used++];
        memset(item, 0, sizeof(*item));
        result = surf_get_string(parser, index, "name", 1, &item->name);
        if (result == SURF_PROTOCOL_OK)
            result = surf_get_i64(parser, index, "size", 1, 0, &item->size);
        if (result == SURF_PROTOCOL_OK)
            result = surf_get_i64(parser, index, "ts", 1, 0, &item->timestamp);
        if (result != SURF_PROTOCOL_OK) return result;
    }
    *out_items = parser->workspace->downloads + start;
    *out_count = parser->workspace->download_used - start;
    return SURF_PROTOCOL_OK;
}

static surf_protocol_result_t surf_decode_options(
    surf_json_parser_t *parser, size_t array,
    const surf_protocol_select_option_t **out_items, size_t *out_count) {
    static const char *const fields[] = {"label", "disabled", "selected"};
    size_t index;
    size_t start = parser->workspace->option_used;
    if (array == SIZE_MAX || parser->workspace->tokens[array].type != SURF_JSON_ARRAY) {
        return array == SIZE_MAX ? SURF_PROTOCOL_ERROR_FIELD : SURF_PROTOCOL_ERROR_TYPE;
    }
    for (index = array + 1; index < parser->workspace->token_count; index++) {
        surf_protocol_select_option_t *item;
        surf_protocol_result_t result;
        if (parser->workspace->tokens[index].parent != array) continue;
        if (parser->workspace->tokens[index].type != SURF_JSON_OBJECT) {
            return SURF_PROTOCOL_ERROR_TYPE;
        }
        if (parser->workspace->option_used >= parser->workspace->item_capacity) {
            return SURF_PROTOCOL_ERROR_LIMIT;
        }
        result = surf_validate_fields(parser, index, fields,
                                      sizeof(fields) / sizeof(fields[0]));
        if (result != SURF_PROTOCOL_OK) return result;
        item = &parser->workspace->options[parser->workspace->option_used++];
        memset(item, 0, sizeof(*item));
        result = surf_get_string(parser, index, "label", 1, &item->label);
        if (result == SURF_PROTOCOL_OK)
            result = surf_get_bool(parser, index, "disabled", 0, 0,
                                   &item->disabled);
        if (result == SURF_PROTOCOL_OK)
            result = surf_get_bool(parser, index, "selected", 0, 0,
                                   &item->selected);
        if (result != SURF_PROTOCOL_OK) return result;
    }
    *out_items = parser->workspace->options + start;
    *out_count = parser->workspace->option_used - start;
    return SURF_PROTOCOL_OK;
}

#define SURF_TRY(expression)                                                   \
    do {                                                                       \
        result = (expression);                                                 \
        if (result != SURF_PROTOCOL_OK) return result;                         \
    } while (0)

#define SURF_EVENT_FIELDS(...)                                                 \
    do {                                                                       \
        static const char *const allowed[] = {__VA_ARGS__};                   \
        SURF_TRY(surf_validate_fields(&parser, root, allowed,                  \
                                      sizeof(allowed) / sizeof(allowed[0])));  \
    } while (0)

surf_protocol_result_t surf_protocol_decode_event(
    surf_protocol_workspace_t *workspace, const char *json, size_t length,
    surf_protocol_event_t *out_event) {
    surf_json_parser_t parser;
    surf_protocol_result_t result;
    surf_string_view_t kind;
    size_t root;
    size_t token;
    if (workspace == NULL || json == NULL || out_event == NULL) {
        return SURF_PROTOCOL_ERROR_ARGUMENT;
    }
    workspace->string_used = 0;
    workspace->tab_used = 0;
    workspace->entry_used = 0;
    workspace->download_used = 0;
    workspace->option_used = 0;
    memset(out_event, 0, sizeof(*out_event));
    memset(&parser, 0, sizeof(parser));
    parser.workspace = workspace;
    parser.json = json;
    parser.length = length;
    SURF_TRY(surf_json_parse(&parser, &root));
    if (workspace->tokens[root].type != SURF_JSON_OBJECT) {
        return SURF_PROTOCOL_ERROR_TYPE;
    }
    SURF_TRY(surf_get_string(&parser, root, "t", 1, &kind));

    if (surf_view_equal_cstr(kind, "hello")) {
        out_event->kind = SURF_PROTOCOL_EVENT_HELLO;
        SURF_EVENT_FIELDS("t", "vw", "vh");
        SURF_TRY(surf_get_i32(&parser, root, "vw", 1, 0,
                              &out_event->data.viewport.width));
        SURF_TRY(surf_get_i32(&parser, root, "vh", 1, 0,
                              &out_event->data.viewport.height));
    } else if (surf_view_equal_cstr(kind, "tabs")) {
        out_event->kind = SURF_PROTOCOL_EVENT_TABS;
        SURF_EVENT_FIELDS("t", "tabs");
        SURF_TRY(surf_get_field(&parser, root, "tabs", 1, &token));
        SURF_TRY(surf_decode_tabs(&parser, token, &out_event->data.tabs.items,
                                  &out_event->data.tabs.count));
    } else if (surf_view_equal_cstr(kind, "video-config")) {
        out_event->kind = SURF_PROTOCOL_EVENT_VIDEO_CONFIG;
        SURF_EVENT_FIELDS("t", "state", "reason", "w", "h", "generation", "profile");
        SURF_TRY(surf_get_string(&parser, root, "state", 1,
                                 &out_event->data.video.state));
        SURF_TRY(surf_get_string(&parser, root, "reason", 0,
                                 &out_event->data.video.reason));
        SURF_TRY(surf_get_i32(&parser, root, "w", 0, 0,
                              &out_event->data.video.width));
        SURF_TRY(surf_get_i32(&parser, root, "h", 0, 0,
                              &out_event->data.video.height));
        SURF_TRY(surf_get_u32(&parser, root, "generation", 0, 0,
                              &out_event->data.video.generation));
        SURF_TRY(surf_get_string(&parser, root, "profile", 0,
                                 &out_event->data.video.profile));
    } else if (surf_view_equal_cstr(kind, "audio-config")) {
        out_event->kind = SURF_PROTOCOL_EVENT_AUDIO_CONFIG;
        SURF_EVENT_FIELDS("t", "ok", "rate", "channels");
        SURF_TRY(surf_get_bool(&parser, root, "ok", 1, 0,
                               &out_event->data.audio.ok));
        SURF_TRY(surf_get_i32(&parser, root, "rate", 0, 0,
                              &out_event->data.audio.rate));
        SURF_TRY(surf_get_i32(&parser, root, "channels", 0, 0,
                              &out_event->data.audio.channels));
    } else if (surf_view_equal_cstr(kind, "loading") ||
               surf_view_equal_cstr(kind, "fullscreen") ||
               surf_view_equal_cstr(kind, "found") ||
               surf_view_equal_cstr(kind, "starred")) {
        if (surf_view_equal_cstr(kind, "loading")) out_event->kind = SURF_PROTOCOL_EVENT_LOADING;
        else if (surf_view_equal_cstr(kind, "fullscreen")) out_event->kind = SURF_PROTOCOL_EVENT_FULLSCREEN;
        else if (surf_view_equal_cstr(kind, "found")) out_event->kind = SURF_PROTOCOL_EVENT_FOUND;
        else out_event->kind = SURF_PROTOCOL_EVENT_STARRED;
        SURF_EVENT_FIELDS("t", "on");
        SURF_TRY(surf_get_bool(&parser, root, "on", 1, 0,
                               &out_event->data.boolean.on));
    } else if (surf_view_equal_cstr(kind, "toast")) {
        out_event->kind = SURF_PROTOCOL_EVENT_TOAST;
        SURF_EVENT_FIELDS("t", "text");
        SURF_TRY(surf_get_string(&parser, root, "text", 1,
                                 &out_event->data.text.text));
    } else if (surf_view_equal_cstr(kind, "clock")) {
        out_event->kind = SURF_PROTOCOL_EVENT_CLOCK;
        SURF_EVENT_FIELDS("t", "c0", "s1", "s2");
        SURF_TRY(surf_get_u64(&parser, root, "c0", 1, 0, &out_event->data.clock.c0));
        SURF_TRY(surf_get_u64(&parser, root, "s1", 1, 0, &out_event->data.clock.s1));
        SURF_TRY(surf_get_u64(&parser, root, "s2", 1, 0, &out_event->data.clock.s2));
    } else if (surf_view_equal_cstr(kind, "url") ||
               surf_view_equal_cstr(kind, "pageerror")) {
        out_event->kind = surf_view_equal_cstr(kind, "url")
                              ? SURF_PROTOCOL_EVENT_URL
                              : SURF_PROTOCOL_EVENT_PAGE_ERROR;
        SURF_EVENT_FIELDS("t", "url", "starred", "security");
        SURF_TRY(surf_get_string(&parser, root, "url", 1, &out_event->data.url.url));
        SURF_TRY(surf_get_bool(&parser, root, "starred", 1, 0,
                               &out_event->data.url.starred));
        SURF_TRY(surf_get_string(&parser, root, "security", 0,
                                 &out_event->data.url.security));
    } else if (surf_view_equal_cstr(kind, "histstate")) {
        out_event->kind = SURF_PROTOCOL_EVENT_HISTORY_STATE;
        SURF_EVENT_FIELDS("t", "back", "fwd");
        SURF_TRY(surf_get_bool(&parser, root, "back", 1, 0,
                               &out_event->data.history_state.back));
        SURF_TRY(surf_get_bool(&parser, root, "fwd", 1, 0,
                               &out_event->data.history_state.forward));
    } else if (surf_view_equal_cstr(kind, "download")) {
        out_event->kind = SURF_PROTOCOL_EVENT_DOWNLOAD;
        SURF_EVENT_FIELDS("t", "name");
        SURF_TRY(surf_get_string(&parser, root, "name", 1,
                                 &out_event->data.name.name));
    } else if (surf_view_equal_cstr(kind, "dlprogress")) {
        out_event->kind = SURF_PROTOCOL_EVENT_DOWNLOAD_PROGRESS;
        SURF_EVENT_FIELDS("t", "name", "pct");
        SURF_TRY(surf_get_string(&parser, root, "name", 1,
                                 &out_event->data.progress.name));
        SURF_TRY(surf_get_i32(&parser, root, "pct", 1, 0,
                              &out_event->data.progress.percent));
    } else if (surf_view_equal_cstr(kind, "suggest")) {
        out_event->kind = SURF_PROTOCOL_EVENT_SUGGEST;
        SURF_EVENT_FIELDS("t", "items");
        SURF_TRY(surf_get_field(&parser, root, "items", 1, &token));
        SURF_TRY(surf_decode_entries(&parser, token, &out_event->data.entries.items,
                                     &out_event->data.entries.count));
    } else if (surf_view_equal_cstr(kind, "hist")) {
        size_t history, bookmarks;
        out_event->kind = SURF_PROTOCOL_EVENT_LIBRARY;
        SURF_EVENT_FIELDS("t", "hist", "bookmarks", "starred");
        SURF_TRY(surf_get_field(&parser, root, "hist", 1, &history));
        SURF_TRY(surf_get_field(&parser, root, "bookmarks", 1, &bookmarks));
        SURF_TRY(surf_decode_entries(&parser, history, &out_event->data.library.history,
                                     &out_event->data.library.history_count));
        SURF_TRY(surf_decode_entries(&parser, bookmarks, &out_event->data.library.bookmarks,
                                     &out_event->data.library.bookmark_count));
        SURF_TRY(surf_get_bool(&parser, root, "starred", 1, 0,
                               &out_event->data.library.starred));
    } else if (surf_view_equal_cstr(kind, "history")) {
        out_event->kind = SURF_PROTOCOL_EVENT_HISTORY;
        SURF_EVENT_FIELDS("t", "q", "items", "offset", "total");
        SURF_TRY(surf_get_string(&parser, root, "q", 0,
                                 &out_event->data.history.query));
        SURF_TRY(surf_get_field(&parser, root, "items", 1, &token));
        SURF_TRY(surf_decode_entries(&parser, token, &out_event->data.history.items,
                                     &out_event->data.history.count));
        SURF_TRY(surf_get_i32(&parser, root, "offset", 1, 0,
                              &out_event->data.history.offset));
        SURF_TRY(surf_get_i32(&parser, root, "total", 1, 0,
                              &out_event->data.history.total));
    } else if (surf_view_equal_cstr(kind, "downloads")) {
        out_event->kind = SURF_PROTOCOL_EVENT_DOWNLOADS;
        SURF_EVENT_FIELDS("t", "items");
        SURF_TRY(surf_get_field(&parser, root, "items", 1, &token));
        SURF_TRY(surf_decode_downloads(&parser, token, &out_event->data.downloads.items,
                                       &out_event->data.downloads.count));
    } else if (surf_view_equal_cstr(kind, "dialog")) {
        out_event->kind = SURF_PROTOCOL_EVENT_DIALOG;
        SURF_EVENT_FIELDS("t", "kind", "text", "def");
        SURF_TRY(surf_get_string(&parser, root, "kind", 1,
                                 &out_event->data.dialog.kind));
        SURF_TRY(surf_get_string(&parser, root, "text", 1,
                                 &out_event->data.dialog.text));
        SURF_TRY(surf_get_string(&parser, root, "def", 1,
                                 &out_event->data.dialog.default_text));
    } else if (surf_view_equal_cstr(kind, "dialogdone") ||
               surf_view_equal_cstr(kind, "log-request") ||
               surf_view_equal_cstr(kind, "log-clear")) {
        if (surf_view_equal_cstr(kind, "dialogdone")) out_event->kind = SURF_PROTOCOL_EVENT_DIALOG_DONE;
        else if (surf_view_equal_cstr(kind, "log-request")) out_event->kind = SURF_PROTOCOL_EVENT_LOG_REQUEST;
        else out_event->kind = SURF_PROTOCOL_EVENT_LOG_CLEAR;
        SURF_EVENT_FIELDS("t");
    } else if (surf_view_equal_cstr(kind, "filechooser")) {
        out_event->kind = SURF_PROTOCOL_EVENT_FILE_CHOOSER;
        SURF_EVENT_FIELDS("t", "multiple");
        SURF_TRY(surf_get_bool(&parser, root, "multiple", 1, 0,
                               &out_event->data.chooser.multiple));
    } else if (surf_view_equal_cstr(kind, "security")) {
        out_event->kind = SURF_PROTOCOL_EVENT_SECURITY;
        SURF_EVENT_FIELDS("t", "state");
        SURF_TRY(surf_get_string(&parser, root, "state", 1,
                                 &out_event->data.security.state));
    } else if (surf_view_equal_cstr(kind, "reader")) {
        out_event->kind = SURF_PROTOCOL_EVENT_READER;
        SURF_EVENT_FIELDS("t", "ok", "title", "html", "url");
        SURF_TRY(surf_get_bool(&parser, root, "ok", 1, 0,
                               &out_event->data.reader.ok));
        SURF_TRY(surf_get_string(&parser, root, "title", 0,
                                 &out_event->data.reader.title));
        SURF_TRY(surf_get_string(&parser, root, "html", 0,
                                 &out_event->data.reader.html));
        SURF_TRY(surf_get_string(&parser, root, "url", 0,
                                 &out_event->data.reader.url));
    } else if (surf_view_equal_cstr(kind, "editable")) {
        out_event->kind = SURF_PROTOCOL_EVENT_EDITABLE;
        SURF_EVENT_FIELDS("t", "on", "show", "kind", "rect");
        SURF_TRY(surf_get_bool(&parser, root, "on", 1, 0,
                               &out_event->data.editable.on));
        SURF_TRY(surf_get_bool(&parser, root, "show", 0, 0,
                               &out_event->data.editable.show_keyboard));
        SURF_TRY(surf_get_string(&parser, root, "kind", 0,
                                 &out_event->data.editable.kind));
        SURF_TRY(surf_get_field(&parser, root, "rect", 0, &token));
        SURF_TRY(surf_decode_rect(&parser, token,
                                  &out_event->data.editable.has_rect,
                                  out_event->data.editable.rect));
    } else if (surf_view_equal_cstr(kind, "select")) {
        size_t options;
        out_event->kind = SURF_PROTOCOL_EVENT_SELECT;
        SURF_EVENT_FIELDS("t", "id", "title", "multiple", "options", "rect");
        SURF_TRY(surf_get_string(&parser, root, "id", 1,
                                 &out_event->data.select.id));
        SURF_TRY(surf_get_string(&parser, root, "title", 0,
                                 &out_event->data.select.title));
        SURF_TRY(surf_get_bool(&parser, root, "multiple", 0, 0,
                               &out_event->data.select.multiple));
        SURF_TRY(surf_get_field(&parser, root, "options", 1, &options));
        SURF_TRY(surf_decode_options(&parser, options,
                                     &out_event->data.select.options,
                                     &out_event->data.select.option_count));
        SURF_TRY(surf_get_field(&parser, root, "rect", 0, &token));
        SURF_TRY(surf_decode_rect(&parser, token,
                                  &out_event->data.select.has_rect,
                                  out_event->data.select.rect));
    } else if (surf_view_equal_cstr(kind, "media-state")) {
        out_event->kind = SURF_PROTOCOL_EVENT_MEDIA_STATE;
        SURF_EVENT_FIELDS("t", "available", "count", "paused", "muted", "volume", "currentTime", "duration", "title");
        SURF_TRY(surf_get_bool(&parser, root, "available", 1, 0,
                               &out_event->data.media.available));
        SURF_TRY(surf_get_i32(&parser, root, "count", 1, 0,
                              &out_event->data.media.count));
        SURF_TRY(surf_get_bool(&parser, root, "paused", 1, 0,
                               &out_event->data.media.paused));
        SURF_TRY(surf_get_bool(&parser, root, "muted", 1, 0,
                               &out_event->data.media.muted));
        SURF_TRY(surf_get_double(&parser, root, "volume", 1, 0.0,
                                 &out_event->data.media.volume));
        SURF_TRY(surf_get_double(&parser, root, "currentTime", 1, 0.0,
                                 &out_event->data.media.current_time));
        SURF_TRY(surf_get_double(&parser, root, "duration", 1, 0.0,
                                 &out_event->data.media.duration));
        SURF_TRY(surf_get_string(&parser, root, "title", 0,
                                 &out_event->data.media.title));
    } else if (surf_view_equal_cstr(kind, "pageframe")) {
        out_event->kind = SURF_PROTOCOL_EVENT_PAGE_FRAME;
        SURF_EVENT_FIELDS("t", "sourceSeq");
        SURF_TRY(surf_get_u32(&parser, root, "sourceSeq", 1, 0,
                              &out_event->data.page_frame.source_sequence));
    } else if (surf_view_equal_cstr(kind, "clipboard")) {
        out_event->kind = SURF_PROTOCOL_EVENT_CLIPBOARD;
        SURF_EVENT_FIELDS("t", "id", "text", "sync");
        SURF_TRY(surf_get_string(&parser, root, "id", 1,
                                 &out_event->data.clipboard.id));
        SURF_TRY(surf_get_string(&parser, root, "text", 1,
                                 &out_event->data.clipboard.text));
        SURF_TRY(surf_get_bool(&parser, root, "sync", 0, 0,
                               &out_event->data.clipboard.sync));
    } else if (surf_view_equal_cstr(kind, "clipboard-sync")) {
        out_event->kind = SURF_PROTOCOL_EVENT_CLIPBOARD_SYNC;
        SURF_EVENT_FIELDS("t", "enabled", "known", "text");
        SURF_TRY(surf_get_bool(&parser, root, "enabled", 1, 0,
                               &out_event->data.clipboard_sync.enabled));
        SURF_TRY(surf_get_bool(&parser, root, "known", 0, 0,
                               &out_event->data.clipboard_sync.known));
        SURF_TRY(surf_get_string(&parser, root, "text", 1,
                                 &out_event->data.clipboard_sync.text));
    } else {
        return SURF_PROTOCOL_ERROR_KIND;
    }
    return SURF_PROTOCOL_OK;
}

#undef SURF_EVENT_FIELDS

static void surf_write_bytes(surf_writer_t *writer, const char *data,
                             size_t length) {
    if (writer->length <= writer->capacity &&
        length <= writer->capacity - writer->length && writer->buffer != NULL) {
        memcpy(writer->buffer + writer->length, data, length);
    } else if (length != 0) {
        writer->failed = 1;
    }
    if (writer->length <= SIZE_MAX - length) writer->length += length;
    else writer->failed = 1;
}

static void surf_write_cstr(surf_writer_t *writer, const char *value) {
    surf_write_bytes(writer, value, strlen(value));
}

static int surf_view_valid(surf_string_view_t value) {
    return value.length == 0 || value.data != NULL;
}

static void surf_write_string(surf_writer_t *writer,
                              surf_string_view_t value) {
    size_t index;
    surf_write_cstr(writer, "\"");
    if (!surf_view_valid(value)) {
        writer->failed = 1;
        return;
    }
    for (index = 0; index < value.length; index++) {
        unsigned char character = (unsigned char)value.data[index];
        switch (character) {
        case '"': surf_write_cstr(writer, "\\\""); break;
        case '\\': surf_write_cstr(writer, "\\\\"); break;
        case '\b': surf_write_cstr(writer, "\\b"); break;
        case '\f': surf_write_cstr(writer, "\\f"); break;
        case '\n': surf_write_cstr(writer, "\\n"); break;
        case '\r': surf_write_cstr(writer, "\\r"); break;
        case '\t': surf_write_cstr(writer, "\\t"); break;
        default:
            if (character < 0x20) {
                char escaped[7];
                (void)snprintf(escaped, sizeof(escaped), "\\u%04x",
                               (unsigned int)character);
                surf_write_bytes(writer, escaped, 6);
            } else {
                surf_write_bytes(writer, value.data + index, 1);
            }
            break;
        }
    }
    surf_write_cstr(writer, "\"");
}

static void surf_write_i64(surf_writer_t *writer, int64_t value) {
    char buffer[32];
    int length = snprintf(buffer, sizeof(buffer), "%" PRId64, value);
    if (length < 0 || (size_t)length >= sizeof(buffer)) writer->failed = 1;
    else surf_write_bytes(writer, buffer, (size_t)length);
}

static void surf_write_u64(surf_writer_t *writer, uint64_t value) {
    char buffer[32];
    int length = snprintf(buffer, sizeof(buffer), "%" PRIu64, value);
    if (length < 0 || (size_t)length >= sizeof(buffer)) writer->failed = 1;
    else surf_write_bytes(writer, buffer, (size_t)length);
}

static void surf_write_double(surf_writer_t *writer, double value) {
    char buffer[64];
    int length;
    int index;
    if (!isfinite(value)) {
        writer->failed = 1;
        return;
    }
    length = snprintf(buffer, sizeof(buffer), "%.17g", value);
    if (length < 0 || (size_t)length >= sizeof(buffer)) {
        writer->failed = 1;
        return;
    }
    for (index = 0; index < length; index++) {
        if (buffer[index] == ',') buffer[index] = '.';
    }
    surf_write_bytes(writer, buffer, (size_t)length);
}

static void surf_write_bool(surf_writer_t *writer, int value) {
    surf_write_cstr(writer, value ? "true" : "false");
}

static void surf_write_key(surf_writer_t *writer, const char *name) {
    surf_write_cstr(writer, ",\"");
    surf_write_cstr(writer, name);
    surf_write_cstr(writer, "\":");
}

static void surf_write_field_string(surf_writer_t *writer, const char *name,
                                    surf_string_view_t value) {
    surf_write_key(writer, name);
    surf_write_string(writer, value);
}

static void surf_write_field_i64(surf_writer_t *writer, const char *name,
                                 int64_t value) {
    surf_write_key(writer, name);
    surf_write_i64(writer, value);
}

static void surf_write_field_u64(surf_writer_t *writer, const char *name,
                                 uint64_t value) {
    surf_write_key(writer, name);
    surf_write_u64(writer, value);
}

static void surf_write_field_double(surf_writer_t *writer, const char *name,
                                    double value) {
    surf_write_key(writer, name);
    surf_write_double(writer, value);
}

static void surf_write_field_bool(surf_writer_t *writer, const char *name,
                                  int value) {
    surf_write_key(writer, name);
    surf_write_bool(writer, value);
}

static const char *surf_command_name(surf_protocol_command_kind_t kind) {
    switch (kind) {
    case SURF_PROTOCOL_COMMAND_SIZE: return "size";
    case SURF_PROTOCOL_COMMAND_CLOCK: return "clock";
    case SURF_PROTOCOL_COMMAND_TAB: return "tab";
    case SURF_PROTOCOL_COMMAND_NAVIGATE: return "nav";
    case SURF_PROTOCOL_COMMAND_OPEN_NEW: return "opennew";
    case SURF_PROTOCOL_COMMAND_HISTORY_DELETE: return "histdel";
    case SURF_PROTOCOL_COMMAND_BOOKMARK_DELETE: return "bmdel";
    case SURF_PROTOCOL_COMMAND_AUDIO: return "audio";
    case SURF_PROTOCOL_COMMAND_MOBILE: return "mobile";
    case SURF_PROTOCOL_COMMAND_DARK: return "dark";
    case SURF_PROTOCOL_COMMAND_FULLSCREEN: return "fullscreen";
    case SURF_PROTOCOL_COMMAND_TOUCH: return "touch";
    case SURF_PROTOCOL_COMMAND_POINTER: return "pointer";
    case SURF_PROTOCOL_COMMAND_WHEEL: return "wheel";
    case SURF_PROTOCOL_COMMAND_KEY: return "key";
    case SURF_PROTOCOL_COMMAND_PASTE: return "paste";
    case SURF_PROTOCOL_COMMAND_COMPOSE: return "compose";
    case SURF_PROTOCOL_COMMAND_SUGGEST: return "suggest";
    case SURF_PROTOCOL_COMMAND_HISTORY: return "history";
    case SURF_PROTOCOL_COMMAND_FIND: return "find";
    case SURF_PROTOCOL_COMMAND_DOWNLOAD_DELETE: return "dldel";
    case SURF_PROTOCOL_COMMAND_CLEAR: return "clear";
    case SURF_PROTOCOL_COMMAND_DIALOG_REPLY: return "dialogreply";
    case SURF_PROTOCOL_COMMAND_SELECT_REPLY: return "selectreply";
    case SURF_PROTOCOL_COMMAND_MEDIA_STATS: return "media-stats";
    case SURF_PROTOCOL_COMMAND_MEDIA_VOLUME: return "media-volume";
    case SURF_PROTOCOL_COMMAND_CLIPBOARD_RESULT: return "clipboard-result";
    case SURF_PROTOCOL_COMMAND_CLIPBOARD_CHANGE: return "clipboard-change";
    case SURF_PROTOCOL_COMMAND_LOG_RECORD: return "log-record";
    case SURF_PROTOCOL_COMMAND_LOG_CLEARED: return "log-cleared";
    case SURF_PROTOCOL_COMMAND_BACK: return "back";
    case SURF_PROTOCOL_COMMAND_FORWARD: return "fwd";
    case SURF_PROTOCOL_COMMAND_RELOAD: return "reload";
    case SURF_PROTOCOL_COMMAND_STOP: return "stop";
    case SURF_PROTOCOL_COMMAND_VIDEO_RETRY: return "video-retry";
    case SURF_PROTOCOL_COMMAND_REQUEST_KEYFRAME: return "reqkeyframe";
    case SURF_PROTOCOL_COMMAND_LIBRARY: return "hist";
    case SURF_PROTOCOL_COMMAND_BOOKMARK: return "bookmark";
    case SURF_PROTOCOL_COMMAND_DOWNLOADS: return "downloads";
    case SURF_PROTOCOL_COMMAND_READER: return "reader";
    case SURF_PROTOCOL_COMMAND_MEDIA_PLAY_PAUSE: return "media-playpause";
    case SURF_PROTOCOL_COMMAND_MEDIA_MUTE: return "media-mute";
    case SURF_PROTOCOL_COMMAND_MEDIA_QUERY: return "media-query";
    default: return NULL;
    }
}

static int surf_command_is_toggle(surf_protocol_command_kind_t kind) {
    return kind == SURF_PROTOCOL_COMMAND_AUDIO ||
           kind == SURF_PROTOCOL_COMMAND_MOBILE ||
           kind == SURF_PROTOCOL_COMMAND_DARK ||
           kind == SURF_PROTOCOL_COMMAND_FULLSCREEN;
}

static int surf_command_is_url(surf_protocol_command_kind_t kind) {
    return kind == SURF_PROTOCOL_COMMAND_NAVIGATE ||
           kind == SURF_PROTOCOL_COMMAND_OPEN_NEW ||
           kind == SURF_PROTOCOL_COMMAND_BOOKMARK_DELETE;
}

static void surf_write_causal(surf_writer_t *writer,
                              surf_protocol_causal_t causal) {
    if (causal.interaction_id != 0)
        surf_write_field_u64(writer, "iid", causal.interaction_id);
    if (causal.client_ns != 0)
        surf_write_field_u64(writer, "clientNs", causal.client_ns);
}

surf_protocol_result_t surf_protocol_encode_command(
    const surf_protocol_command_t *command, char *buffer, size_t capacity,
    size_t *out_length) {
    surf_writer_t writer;
    const char *name;
    size_t index;
    if (command == NULL || out_length == NULL ||
        (buffer == NULL && capacity != 0)) {
        return SURF_PROTOCOL_ERROR_ARGUMENT;
    }
    name = surf_command_name(command->kind);
    if (name == NULL) return SURF_PROTOCOL_ERROR_KIND;
    memset(&writer, 0, sizeof(writer));
    writer.buffer = buffer;
    writer.capacity = capacity;
    surf_write_cstr(&writer, "{\"t\":\"");
    surf_write_cstr(&writer, name);
    surf_write_cstr(&writer, "\"");

    if (command->kind == SURF_PROTOCOL_COMMAND_SIZE) {
        surf_write_field_i64(&writer, "w", command->data.size.width);
        surf_write_field_i64(&writer, "h", command->data.size.height);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_CLOCK) {
        surf_write_field_u64(&writer, "c0", command->data.clock.client_send_ns);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_TAB) {
        surf_write_field_string(&writer, "action", command->data.tab.action);
        surf_write_field_i64(&writer, "id", command->data.tab.id);
    } else if (surf_command_is_url(command->kind)) {
        surf_write_field_string(&writer, "url", command->data.url.url);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_HISTORY_DELETE) {
        surf_write_field_string(&writer, "url",
                                command->data.history_delete.url);
        surf_write_field_i64(&writer, "ts",
                             command->data.history_delete.timestamp);
    } else if (surf_command_is_toggle(command->kind)) {
        surf_write_field_bool(&writer, "on", command->data.toggle.on);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_TOUCH) {
        if (command->data.touch.point_count > 16 ||
            (command->data.touch.point_count != 0 &&
             command->data.touch.points == NULL)) {
            return SURF_PROTOCOL_ERROR_LIMIT;
        }
        surf_write_field_string(&writer, "phase", command->data.touch.phase);
        surf_write_field_u64(&writer, "seq", command->data.touch.sequence);
        surf_write_field_u64(&writer, "surface", command->data.touch.surface);
        surf_write_field_u64(&writer, "ts", command->data.touch.timestamp_ns);
        surf_write_key(&writer, "points");
        surf_write_cstr(&writer, "[");
        for (index = 0; index < command->data.touch.point_count; index++) {
            const surf_protocol_touch_point_t *point =
                &command->data.touch.points[index];
            if (index != 0) surf_write_cstr(&writer, ",");
            surf_write_cstr(&writer, "{\"id\":");
            surf_write_i64(&writer, point->id);
            surf_write_field_double(&writer, "x", point->x);
            surf_write_field_double(&writer, "y", point->y);
            if (point->radius_x != 0.0)
                surf_write_field_double(&writer, "rx", point->radius_x);
            if (point->radius_y != 0.0)
                surf_write_field_double(&writer, "ry", point->radius_y);
            if (point->force != 0.0)
                surf_write_field_double(&writer, "force", point->force);
            surf_write_cstr(&writer, "}");
        }
        surf_write_cstr(&writer, "]");
    } else if (command->kind == SURF_PROTOCOL_COMMAND_POINTER) {
        surf_write_field_string(&writer, "phase",
                                command->data.pointer.phase);
        surf_write_field_u64(&writer, "seq", command->data.pointer.sequence);
        surf_write_field_u64(&writer, "surface",
                             command->data.pointer.surface);
        surf_write_field_u64(&writer, "ts",
                             command->data.pointer.timestamp_ns);
        surf_write_field_double(&writer, "x", command->data.pointer.x);
        surf_write_field_double(&writer, "y", command->data.pointer.y);
        surf_write_field_string(&writer, "button",
                                command->data.pointer.button);
        surf_write_field_i64(&writer, "buttons",
                             command->data.pointer.buttons);
        surf_write_field_i64(&writer, "mods",
                             command->data.pointer.modifiers);
        surf_write_field_i64(&writer, "clicks",
                             command->data.pointer.clicks);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_WHEEL) {
        surf_write_field_u64(&writer, "seq", command->data.wheel.sequence);
        surf_write_field_u64(&writer, "surface", command->data.wheel.surface);
        surf_write_field_u64(&writer, "ts", command->data.wheel.timestamp_ns);
        surf_write_field_double(&writer, "x", command->data.wheel.x);
        surf_write_field_double(&writer, "y", command->data.wheel.y);
        surf_write_field_double(&writer, "dx", command->data.wheel.delta_x);
        surf_write_field_double(&writer, "dy", command->data.wheel.delta_y);
        surf_write_field_i64(&writer, "buttons", command->data.wheel.buttons);
        surf_write_field_i64(&writer, "mods", command->data.wheel.modifiers);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_KEY) {
        surf_write_field_bool(&writer, "down", command->data.key.down);
        surf_write_field_string(&writer, "key", command->data.key.key);
        surf_write_field_string(&writer, "code", command->data.key.code);
        surf_write_field_i64(&writer, "keyCode", command->data.key.key_code);
        surf_write_field_string(&writer, "text", command->data.key.text);
        if (command->data.key.modifiers != 0)
            surf_write_field_i64(&writer, "mods",
                                 command->data.key.modifiers);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_PASTE ||
               command->kind == SURF_PROTOCOL_COMMAND_CLIPBOARD_CHANGE) {
        surf_write_field_string(&writer, "text", command->data.text.text);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_COMPOSE) {
        surf_write_field_string(&writer, "phase", command->data.compose.phase);
        surf_write_field_string(&writer, "text", command->data.compose.text);
        if (command->data.compose.start != 0)
            surf_write_field_i64(&writer, "start", command->data.compose.start);
        if (command->data.compose.end != 0)
            surf_write_field_i64(&writer, "end", command->data.compose.end);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_SUGGEST ||
               command->kind == SURF_PROTOCOL_COMMAND_HISTORY) {
        surf_write_field_string(&writer, "q", command->data.query.query);
        surf_write_field_i64(&writer, "offset", command->data.query.offset);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_FIND) {
        surf_write_field_string(&writer, "q", command->data.find.query);
        surf_write_field_i64(&writer, "dir", command->data.find.direction);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_DOWNLOAD_DELETE) {
        surf_write_field_string(&writer, "name", command->data.name.name);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_CLEAR) {
        surf_write_field_string(&writer, "what", command->data.clear.what);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_DIALOG_REPLY) {
        surf_write_field_bool(&writer, "accept",
                              command->data.dialog_reply.accept);
        surf_write_field_string(&writer, "text",
                                command->data.dialog_reply.text);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_SELECT_REPLY) {
        if (command->data.select_reply.index_count != 0 &&
            command->data.select_reply.indices == NULL) {
            return SURF_PROTOCOL_ERROR_ARGUMENT;
        }
        surf_write_field_string(&writer, "id", command->data.select_reply.id);
        if (command->data.select_reply.cancel)
            surf_write_field_bool(&writer, "cancel", 1);
        if (command->data.select_reply.index_count != 0) {
            surf_write_key(&writer, "indices");
            surf_write_cstr(&writer, "[");
            for (index = 0; index < command->data.select_reply.index_count;
                 index++) {
                if (index != 0) surf_write_cstr(&writer, ",");
                surf_write_i64(&writer,
                               command->data.select_reply.indices[index]);
            }
            surf_write_cstr(&writer, "]");
        }
    } else if (command->kind == SURF_PROTOCOL_COMMAND_MEDIA_STATS) {
        surf_write_field_double(&writer, "fps",
                                command->data.media_stats.fps);
        surf_write_field_double(&writer, "presentedFps",
                                command->data.media_stats.presented_fps);
        surf_write_field_double(&writer, "decodeFps",
                                command->data.media_stats.decode_fps);
        surf_write_field_double(&writer, "auRate",
                                command->data.media_stats.au_rate);
        surf_write_field_string(&writer, "renderer",
                                command->data.media_stats.renderer);
        surf_write_field_double(&writer, "rendererFps",
                                command->data.media_stats.renderer_fps);
        surf_write_field_double(&writer, "rendererMs",
                                command->data.media_stats.renderer_ms);
        surf_write_field_i64(
            &writer, "rendererBackpressure",
            command->data.media_stats.renderer_backpressure);
        surf_write_field_i64(&writer, "rendererRecoveries",
                             command->data.media_stats.renderer_recoveries);
        surf_write_field_i64(&writer, "rendererFailures",
                             command->data.media_stats.renderer_failures);
        surf_write_field_double(&writer, "callbackMs",
                                command->data.media_stats.callback_ms);
        surf_write_field_double(&writer, "gapMs",
                                command->data.media_stats.gap_ms);
        surf_write_field_double(&writer, "frameAgeMs",
                                command->data.media_stats.frame_age_ms);
        surf_write_field_double(&writer, "windowMs",
                                command->data.media_stats.window_ms);
        surf_write_field_double(&writer, "dropPct",
                                command->data.media_stats.drop_percent);
        surf_write_field_i64(&writer, "queue",
                             command->data.media_stats.queue_depth);
        surf_write_field_i64(&writer, "decodeErrors",
                             command->data.media_stats.decode_errors);
        surf_write_field_bool(&writer, "memoryWarn",
                              command->data.media_stats.memory_warn);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_MEDIA_VOLUME) {
        surf_write_field_double(&writer, "value", command->data.volume.value);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_CLIPBOARD_RESULT) {
        surf_write_field_string(&writer, "id",
                                command->data.clipboard_result.id);
        surf_write_field_bool(&writer, "ok",
                              command->data.clipboard_result.ok);
    } else if (command->kind == SURF_PROTOCOL_COMMAND_LOG_RECORD) {
        if (!surf_view_valid(command->data.log_record.json)) {
            return SURF_PROTOCOL_ERROR_ARGUMENT;
        }
        surf_write_key(&writer, "record");
        surf_write_bytes(&writer, command->data.log_record.json.data,
                         command->data.log_record.json.length);
    }
    surf_write_causal(&writer, command->causal);
    surf_write_cstr(&writer, "}");
    *out_length = writer.length;
    if (writer.length < capacity && buffer != NULL) buffer[writer.length] = '\0';
    return writer.failed ? SURF_PROTOCOL_ERROR_BUFFER : SURF_PROTOCOL_OK;
}

const char *surf_protocol_result_string(surf_protocol_result_t result) {
    switch (result) {
    case SURF_PROTOCOL_OK: return "ok";
    case SURF_PROTOCOL_ERROR_ARGUMENT: return "invalid argument";
    case SURF_PROTOCOL_ERROR_JSON: return "invalid JSON";
    case SURF_PROTOCOL_ERROR_KIND: return "unknown protocol kind";
    case SURF_PROTOCOL_ERROR_FIELD: return "invalid protocol field";
    case SURF_PROTOCOL_ERROR_TYPE: return "invalid protocol type";
    case SURF_PROTOCOL_ERROR_LIMIT: return "protocol limit exceeded";
    case SURF_PROTOCOL_ERROR_BUFFER: return "output buffer too small";
    default: return "unknown protocol error";
    }
}

#undef SURF_TRY
