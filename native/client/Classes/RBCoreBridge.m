#import "RBCoreBridge.h"

#include "surf/core.h"
#include "surf/protocol.h"

static NSString *const RBCoreBridgeErrorDomain = @"space.seg6.surf.core";

@interface RBCoreBridge () {
    surf_core_t *_core;
    void *_protocolMemory;
    surf_protocol_workspace_t *_protocolWorkspace;
    uint64_t _connectionGeneration;
}
@property(nonatomic, copy, readwrite) NSArray *tabs;
@property(nonatomic, copy, readwrite) NSString *activeTitle;
@property(nonatomic, copy, readwrite) NSString *currentURL;
@property(nonatomic, copy, readwrite) NSString *security;
@property(nonatomic, copy, readwrite) NSString *editableKind;
@property(nonatomic, copy, readwrite) NSArray *editableRect;
@property(nonatomic, assign, readwrite) unsigned long long revision;
@property(nonatomic, assign, readwrite) long long activeTabID;
@property(nonatomic, assign, readwrite) unsigned int awaitedSourceSequence;
@property(nonatomic, assign, readwrite) BOOL hasActiveTab;
@property(nonatomic, assign, readwrite) BOOL showStartPage;
@property(nonatomic, assign, readwrite) BOOL loading;
@property(nonatomic, assign, readwrite) BOOL canGoBack;
@property(nonatomic, assign, readwrite) BOOL canGoForward;
@property(nonatomic, assign, readwrite) BOOL starred;
@property(nonatomic, assign, readwrite) BOOL fullscreen;
@property(nonatomic, assign, readwrite) BOOL editable;
@property(nonatomic, assign, readwrite) BOOL editableHasRect;
@property(nonatomic, assign, readwrite) BOOL keyboardVisible;
@property(nonatomic, assign, readwrite) BOOL awaitingPageFrame;
@property(nonatomic, strong) NSMutableSet *pendingEffects;
@end

@implementation RBCoreBridge

static NSString *RBStringFromCore(surf_string_view_t value) {
    if (!value.data || !value.length) return @"";
    NSString *string = [[NSString alloc] initWithBytes:value.data
        length:value.length encoding:NSUTF8StringEncoding];
    return string ?: @"";
}

- (id)init {
    self = [super init];
    if (self) {
        surf_core_config_t config;
        surf_core_config_init(&config);
        if (surf_core_create(&config, &_core) != SURF_CORE_OK) return nil;
        _connectionGeneration = surf_core_connection_generation(_core);
        size_t protocolSize = surf_protocol_workspace_size(NULL);
        _protocolMemory = malloc(protocolSize);
        if (!_protocolMemory ||
            surf_protocol_workspace_init(&_protocolWorkspace, _protocolMemory,
                                         protocolSize, NULL) != SURF_PROTOCOL_OK) {
            surf_core_destroy(_core);
            _core = NULL;
            free(_protocolMemory);
            return nil;
        }
        self.pendingEffects = [NSMutableSet set];
        [self refreshSnapshot];
    }
    return self;
}

- (void)dealloc {
    surf_core_destroy(_core);
    _core = NULL;
    free(_protocolMemory);
    _protocolMemory = NULL;
    _protocolWorkspace = NULL;
}

- (BOOL)dispatchEvent:(surf_event_t *)event error:(NSError **)error {
    surf_core_result_t result = surf_core_dispatch_scoped(
        _core, _connectionGeneration, event);
    if (result != SURF_CORE_OK) {
        if (error) {
            NSString *message = [NSString stringWithUTF8String:
                surf_core_result_string(result)] ?: @"Portable client error";
            *error = [NSError errorWithDomain:RBCoreBridgeErrorDomain code:result
                userInfo:@{NSLocalizedDescriptionKey: message}];
        }
        return NO;
    }
    surf_effect_t effect;
    while (surf_core_next_effect(_core, &effect)) {
        [self.pendingEffects addObject:@((NSInteger)effect.kind)];
    }
    [self refreshSnapshot];
    return YES;
}

- (void)refreshSnapshot {
    surf_snapshot_t snapshot;
    if (surf_core_snapshot(_core, &snapshot) != SURF_CORE_OK) return;
    NSMutableArray *tabs = [NSMutableArray arrayWithCapacity:snapshot.tab_count];
    for (size_t index = 0; index < snapshot.tab_count; index++) {
        const surf_tab_snapshot_t *tab = &snapshot.tabs[index];
        NSMutableDictionary *item = [NSMutableDictionary dictionaryWithObjectsAndKeys:
            @(tab->id), @"id", RBStringFromCore(tab->title), @"title",
            RBStringFromCore(tab->url), @"url", @(tab->active != 0), @"active", nil];
        NSString *icon = RBStringFromCore(tab->icon);
        if ([icon length]) [item setObject:icon forKey:@"icon"];
        [tabs addObject:item];
    }
    self.tabs = tabs;
    self.activeTitle = RBStringFromCore(snapshot.active_title);
    self.currentURL = RBStringFromCore(snapshot.current_url);
    self.security = RBStringFromCore(snapshot.security);
    self.editableKind = RBStringFromCore(snapshot.editable_kind);
    self.editableRect = snapshot.editable_has_rect
        ? @[@(snapshot.editable_rect[0]), @(snapshot.editable_rect[1]),
            @(snapshot.editable_rect[2]), @(snapshot.editable_rect[3])] : @[];
    self.revision = snapshot.revision;
    self.activeTabID = snapshot.active_tab_id;
    self.awaitedSourceSequence = snapshot.awaited_source_sequence;
    self.hasActiveTab = snapshot.has_active_tab != 0;
    self.showStartPage = snapshot.show_start_page != 0;
    self.loading = snapshot.loading != 0;
    self.canGoBack = snapshot.can_go_back != 0;
    self.canGoForward = snapshot.can_go_forward != 0;
    self.starred = snapshot.starred != 0;
    self.fullscreen = snapshot.fullscreen != 0;
    self.editable = snapshot.editable != 0;
    self.editableHasRect = snapshot.editable_has_rect != 0;
    self.keyboardVisible = snapshot.keyboard_visible != 0;
    self.awaitingPageFrame = snapshot.awaiting_page_frame != 0;
}

- (BOOL)consumeControlMessage:(NSDictionary *)message error:(NSError **)error {
    NSData *data = [NSJSONSerialization dataWithJSONObject:message options:0 error:nil];
    if (!data) return NO;
    return [self consumeControlData:data message:message error:error];
}

- (BOOL)consumeControlData:(NSData *)data message:(NSDictionary *)message
                     error:(NSError **)error {
    surf_protocol_event_t protocolEvent;
    surf_protocol_result_t decode = surf_protocol_decode_event(
        _protocolWorkspace, [data bytes], [data length], &protocolEvent);
    if (decode != SURF_PROTOCOL_OK) {
        if (error) {
            NSString *reason = [NSString stringWithUTF8String:
                surf_protocol_result_string(decode)] ?: @"Invalid control event";
            *error = [NSError errorWithDomain:RBCoreBridgeErrorDomain code:decode
                userInfo:@{NSLocalizedDescriptionKey: reason}];
        }
        // The type may have looked familiar to Foundation, but malformed wire
        // data is considered handled so UIKit never falls back to it.
        return YES;
    }
    NSString *type = [message objectForKey:@"t"];
    if (![type isKindOfClass:[NSString class]]) return NO;
    surf_event_t event;
    memset(&event, 0, sizeof(event));

    if (protocolEvent.kind == SURF_PROTOCOL_EVENT_TABS) {
        event.kind = SURF_EVENT_TABS;
        event.data.tabs.items = protocolEvent.data.tabs.items;
        event.data.tabs.count = protocolEvent.data.tabs.count;
        [self dispatchEvent:&event error:error];
        return YES;
    }
    if (protocolEvent.kind == SURF_PROTOCOL_EVENT_URL) {
        event.kind = SURF_EVENT_URL;
        event.data.url.url = protocolEvent.data.url.url;
        event.data.url.security = protocolEvent.data.url.security;
        event.data.url.starred = protocolEvent.data.url.starred;
    } else if (protocolEvent.kind == SURF_PROTOCOL_EVENT_HISTORY_STATE) {
        event.kind = SURF_EVENT_HISTORY_STATE;
        event.data.history.can_go_back = protocolEvent.data.history_state.back;
        event.data.history.can_go_forward = protocolEvent.data.history_state.forward;
    } else if (protocolEvent.kind == SURF_PROTOCOL_EVENT_LOADING) {
        event.kind = SURF_EVENT_LOADING;
        event.data.boolean.on = protocolEvent.data.boolean.on;
    } else if (protocolEvent.kind == SURF_PROTOCOL_EVENT_EDITABLE) {
        event.kind = SURF_EVENT_EDITABLE;
        event.data.editable.on = protocolEvent.data.editable.on;
        event.data.editable.show_keyboard =
            protocolEvent.data.editable.show_keyboard;
        event.data.editable.kind = protocolEvent.data.editable.kind;
        event.data.editable.has_rect = protocolEvent.data.editable.has_rect;
        memcpy(event.data.editable.rect, protocolEvent.data.editable.rect,
               sizeof(event.data.editable.rect));
    } else if (protocolEvent.kind == SURF_PROTOCOL_EVENT_FULLSCREEN) {
        event.kind = SURF_EVENT_FULLSCREEN;
        event.data.boolean.on = protocolEvent.data.boolean.on;
    } else if (protocolEvent.kind == SURF_PROTOCOL_EVENT_SECURITY) {
        event.kind = SURF_EVENT_SECURITY;
        event.data.security.state = protocolEvent.data.security.state;
    } else if (protocolEvent.kind == SURF_PROTOCOL_EVENT_STARRED) {
        event.kind = SURF_EVENT_STARRED;
        event.data.boolean.on = protocolEvent.data.boolean.on;
    } else if (protocolEvent.kind == SURF_PROTOCOL_EVENT_PAGE_FRAME) {
        event.kind = SURF_EVENT_PAGE_FRAME;
        event.data.page_frame.source_sequence =
            protocolEvent.data.page_frame.source_sequence;
    } else {
        return NO;
    }
    [self dispatchEvent:&event error:error];
    return YES;
}

- (void)notePresentedSourceSequence:(unsigned int)sourceSequence {
    surf_event_t event;
    memset(&event, 0, sizeof(event));
    event.kind = SURF_EVENT_FRAME_PRESENTED;
    event.data.frame_presented.source_sequence = sourceSequence;
    [self dispatchEvent:&event error:nil];
}

- (void)noteKeyboardVisible:(BOOL)visible {
    surf_event_t event;
    memset(&event, 0, sizeof(event));
    event.kind = SURF_EVENT_KEYBOARD_VISIBILITY;
    event.data.boolean.on = visible;
    [self dispatchEvent:&event error:nil];
}

- (void)reset {
    if (_connectionGeneration != UINT64_MAX &&
        surf_core_begin_connection(_core, _connectionGeneration + 1) ==
            SURF_CORE_OK) {
        _connectionGeneration++;
        [self refreshSnapshot];
    }
    [self.pendingEffects removeAllObjects];
}

- (BOOL)consumeEffect:(RBCoreEffect)effect {
    NSNumber *value = @((NSInteger)effect);
    if (![self.pendingEffects containsObject:value]) return NO;
    [self.pendingEffects removeObject:value];
    return YES;
}

@end
