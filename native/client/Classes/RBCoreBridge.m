#import "RBCoreBridge.h"

#include "surf/core.h"

static NSString *const RBCoreBridgeErrorDomain = @"space.seg6.surf.core";

@interface RBCoreBridge () {
    surf_core_t *_core;
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

static surf_string_view_t RBCoreStringView(NSString *string) {
    if (![string isKindOfClass:[NSString class]]) return surf_string_view(NULL, 0);
    return surf_string_view([string UTF8String],
        [string lengthOfBytesUsingEncoding:NSUTF8StringEncoding]);
}

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
        self.pendingEffects = [NSMutableSet set];
        [self refreshSnapshot];
    }
    return self;
}

- (void)dealloc {
    surf_core_destroy(_core);
    _core = NULL;
}

- (BOOL)dispatchEvent:(surf_event_t *)event error:(NSError **)error {
    surf_core_result_t result = surf_core_dispatch(_core, event);
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
    NSString *type = [message objectForKey:@"t"];
    if (![type isKindOfClass:[NSString class]]) return NO;
    surf_event_t event;
    memset(&event, 0, sizeof(event));

    if ([type isEqualToString:@"tabs"]) {
        NSArray *items = [message objectForKey:@"tabs"];
        if (![items isKindOfClass:[NSArray class]] || [items count] > SURF_CORE_HARD_MAX_TABS) {
            if (error) *error = [NSError errorWithDomain:RBCoreBridgeErrorDomain
                code:SURF_CORE_ERROR_LIMIT userInfo:@{NSLocalizedDescriptionKey: @"Invalid tab list"}];
            return YES;
        }
        size_t count = [items count];
        surf_tab_event_t *tabs = count ? calloc(count, sizeof(*tabs)) : NULL;
        if (count && !tabs) {
            if (error) *error = [NSError errorWithDomain:RBCoreBridgeErrorDomain
                code:SURF_CORE_ERROR_ALLOCATE userInfo:@{NSLocalizedDescriptionKey: @"Could not allocate tab list"}];
            return YES;
        }
        BOOL valid = YES;
        for (size_t index = 0; index < count; index++) {
            NSDictionary *item = [items objectAtIndex:index];
            if (![item isKindOfClass:[NSDictionary class]]) { valid = NO; break; }
            tabs[index].id = [[item objectForKey:@"id"] longLongValue];
            tabs[index].title = RBCoreStringView([item objectForKey:@"title"]);
            tabs[index].url = RBCoreStringView([item objectForKey:@"url"]);
            tabs[index].icon = RBCoreStringView([item objectForKey:@"icon"]);
            tabs[index].active = [[item objectForKey:@"active"] boolValue];
        }
        if (!valid) {
            free(tabs);
            if (error) *error = [NSError errorWithDomain:RBCoreBridgeErrorDomain
                code:SURF_CORE_ERROR_ARGUMENT userInfo:@{NSLocalizedDescriptionKey: @"Invalid tab entry"}];
            return YES;
        }
        event.kind = SURF_EVENT_TABS;
        event.data.tabs.items = tabs;
        event.data.tabs.count = count;
        [self dispatchEvent:&event error:error];
        free(tabs);
        return YES;
    }
    if ([type isEqualToString:@"url"]) {
        event.kind = SURF_EVENT_URL;
        event.data.url.url = RBCoreStringView([message objectForKey:@"url"]);
        event.data.url.security = RBCoreStringView([message objectForKey:@"security"]);
        event.data.url.starred = [[message objectForKey:@"starred"] boolValue];
    } else if ([type isEqualToString:@"histstate"]) {
        event.kind = SURF_EVENT_HISTORY_STATE;
        event.data.history.can_go_back = [[message objectForKey:@"back"] boolValue];
        event.data.history.can_go_forward = [[message objectForKey:@"fwd"] boolValue];
    } else if ([type isEqualToString:@"loading"]) {
        event.kind = SURF_EVENT_LOADING;
        event.data.boolean.on = [[message objectForKey:@"on"] boolValue];
    } else if ([type isEqualToString:@"editable"]) {
        event.kind = SURF_EVENT_EDITABLE;
        event.data.editable.on = [[message objectForKey:@"on"] boolValue];
        event.data.editable.show_keyboard = [[message objectForKey:@"show"] boolValue];
        event.data.editable.kind = RBCoreStringView([message objectForKey:@"kind"]);
        NSArray *rect = [message objectForKey:@"rect"];
        if ([rect isKindOfClass:[NSArray class]] && [rect count] == 4) {
            event.data.editable.has_rect = 1;
            for (NSUInteger index = 0; index < 4; index++) {
                event.data.editable.rect[index] = [[rect objectAtIndex:index] doubleValue];
            }
        }
    } else if ([type isEqualToString:@"fullscreen"]) {
        event.kind = SURF_EVENT_FULLSCREEN;
        event.data.boolean.on = [[message objectForKey:@"on"] boolValue];
    } else if ([type isEqualToString:@"security"]) {
        event.kind = SURF_EVENT_SECURITY;
        event.data.security.state = RBCoreStringView([message objectForKey:@"state"]);
    } else if ([type isEqualToString:@"starred"]) {
        event.kind = SURF_EVENT_STARRED;
        event.data.boolean.on = [[message objectForKey:@"on"] boolValue];
    } else if ([type isEqualToString:@"pageframe"]) {
        event.kind = SURF_EVENT_PAGE_FRAME;
        event.data.page_frame.source_sequence = [[message objectForKey:@"sourceSeq"] unsignedIntValue];
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
    surf_event_t event;
    memset(&event, 0, sizeof(event));
    event.kind = SURF_EVENT_RESET;
    [self dispatchEvent:&event error:nil];
    [self.pendingEffects removeAllObjects];
}

- (BOOL)consumeEffect:(RBCoreEffect)effect {
    NSNumber *value = @((NSInteger)effect);
    if (![self.pendingEffects containsObject:value]) return NO;
    [self.pendingEffects removeObject:value];
    return YES;
}

@end
