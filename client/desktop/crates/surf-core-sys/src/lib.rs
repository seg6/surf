#![allow(non_camel_case_types)]

use std::ffi::{c_char, c_int, c_void};

pub const SURF_CORE_ABI_VERSION: u32 = 1;
pub const SURF_CORE_OK: c_int = 0;
pub const SURF_FRAME_OK: c_int = 0;
pub const SURF_RECONNECT_STOP: c_int = 0;
pub const SURF_RECONNECT_RETRY: c_int = 1;
pub const SURF_MEDIA_OK: c_int = 0;
pub const SURF_MEDIA_ACTION_DROP_REQUEST_KEYFRAME: c_int = 0;
pub const SURF_MEDIA_ACTION_DECODE: c_int = 1;
pub const SURF_MEDIA_ACTION_RESET_AND_DECODE: c_int = 2;
pub const SURF_EVENT_RESET: c_int = 1;
pub const SURF_EVENT_TABS: c_int = 2;
pub const SURF_EVENT_URL: c_int = 3;
pub const SURF_EVENT_HISTORY_STATE: c_int = 4;
pub const SURF_EVENT_LOADING: c_int = 5;
pub const SURF_EVENT_EDITABLE: c_int = 6;
pub const SURF_EVENT_FULLSCREEN: c_int = 7;
pub const SURF_EVENT_SECURITY: c_int = 8;
pub const SURF_EVENT_STARRED: c_int = 9;
pub const SURF_EVENT_PAGE_FRAME: c_int = 10;
pub const SURF_EVENT_FRAME_PRESENTED: c_int = 11;
pub const SURF_EVENT_KEYBOARD_VISIBILITY: c_int = 12;

#[repr(C)]
pub struct surf_core_t {
    _private: [u8; 0],
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_string_view_t {
    pub data: *const c_char,
    pub length: usize,
}

pub type surf_allocate_fn =
    Option<unsafe extern "C" fn(context: *mut c_void, size: usize) -> *mut c_void>;
pub type surf_deallocate_fn =
    Option<unsafe extern "C" fn(context: *mut c_void, pointer: *mut c_void)>;

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_allocator_t {
    pub context: *mut c_void,
    pub allocate: surf_allocate_fn,
    pub deallocate: surf_deallocate_fn,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_core_config_t {
    pub abi_version: u32,
    pub max_tabs: usize,
    pub allocator: surf_allocator_t,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_tab_event_t {
    pub id: i64,
    pub title: surf_string_view_t,
    pub url: surf_string_view_t,
    pub icon: surf_string_view_t,
    pub active: c_int,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_tabs_event_t {
    pub items: *const surf_tab_event_t,
    pub count: usize,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_url_event_t {
    pub url: surf_string_view_t,
    pub security: surf_string_view_t,
    pub starred: c_int,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_history_event_t {
    pub can_go_back: c_int,
    pub can_go_forward: c_int,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_boolean_event_t {
    pub on: c_int,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_editable_event_t {
    pub on: c_int,
    pub show_keyboard: c_int,
    pub kind: surf_string_view_t,
    pub has_rect: c_int,
    pub rect: [f64; 4],
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_security_event_t {
    pub state: surf_string_view_t,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_page_frame_event_t {
    pub source_sequence: u32,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub union surf_event_data_t {
    pub tabs: surf_tabs_event_t,
    pub url: surf_url_event_t,
    pub history: surf_history_event_t,
    pub boolean: surf_boolean_event_t,
    pub editable: surf_editable_event_t,
    pub security: surf_security_event_t,
    pub page_frame: surf_page_frame_event_t,
    pub frame_presented: surf_page_frame_event_t,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_event_t {
    pub kind: c_int,
    pub data: surf_event_data_t,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_effect_t {
    pub kind: c_int,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_tab_snapshot_t {
    pub id: i64,
    pub title: surf_string_view_t,
    pub url: surf_string_view_t,
    pub icon: surf_string_view_t,
    pub active: c_int,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_snapshot_t {
    pub revision: u64,
    pub tabs: *const surf_tab_snapshot_t,
    pub tab_count: usize,
    pub active_tab_id: i64,
    pub active_title: surf_string_view_t,
    pub current_url: surf_string_view_t,
    pub security: surf_string_view_t,
    pub editable_kind: surf_string_view_t,
    pub editable_rect: [f64; 4],
    pub awaited_source_sequence: u32,
    pub has_active_tab: c_int,
    pub show_start_page: c_int,
    pub loading: c_int,
    pub can_go_back: c_int,
    pub can_go_forward: c_int,
    pub starred: c_int,
    pub fullscreen: c_int,
    pub editable: c_int,
    pub editable_has_rect: c_int,
    pub keyboard_visible: c_int,
    pub awaiting_page_frame: c_int,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_frame_view_t {
    pub type_: u8,
    pub flags: u8,
    pub sequence: u32,
    pub source_sequence: u32,
    pub width: u16,
    pub height: u16,
    pub interaction_id: u64,
    pub source_receive_ns: u64,
    pub encode_complete_ns: u64,
    pub socket_write_ns: u64,
    pub encoder_generation: u32,
    pub input_receive_ns: u64,
    pub cdp_accepted_ns: u64,
    pub profile: u8,
    pub payload: *const u8,
    pub payload_length: usize,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_reconnect_policy_t {
    pub attempts: u8,
    pub maximum_attempts: u8,
    pub reserved: u16,
    pub base_delay_ms: u32,
    pub maximum_delay_ms: u32,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_media_policy_t {
    pub generation: u32,
    pub last_sequence: u32,
    pub has_generation: u8,
    pub has_sequence: u8,
    pub waiting_for_idr: u8,
    pub reset_pending: u8,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_media_admission_t {
    pub action: c_int,
    pub generation_changed: c_int,
    pub sequence_gap: c_int,
}

unsafe extern "C" {
    pub fn surf_core_config_init(config: *mut surf_core_config_t);
    pub fn surf_core_create(
        config: *const surf_core_config_t,
        out_core: *mut *mut surf_core_t,
    ) -> c_int;
    pub fn surf_core_destroy(core: *mut surf_core_t);
    pub fn surf_core_dispatch(core: *mut surf_core_t, event: *const surf_event_t) -> c_int;
    pub fn surf_core_snapshot(
        core: *const surf_core_t,
        out_snapshot: *mut surf_snapshot_t,
    ) -> c_int;
    pub fn surf_core_next_effect(core: *mut surf_core_t, out_effect: *mut surf_effect_t) -> c_int;
    pub fn surf_core_result_string(result: c_int) -> *const c_char;
    pub fn surf_core_abi_version() -> u32;
    pub fn surf_core_sizeof_string_view() -> usize;
    pub fn surf_core_sizeof_config() -> usize;
    pub fn surf_core_sizeof_event() -> usize;
    pub fn surf_core_sizeof_snapshot() -> usize;
    pub fn surf_frame_parse(
        data: *const u8,
        length: usize,
        out_frame: *mut surf_frame_view_t,
    ) -> c_int;
    pub fn surf_frame_result_string(result: c_int) -> *const c_char;
    pub fn surf_frame_sizeof_view() -> usize;
    pub fn surf_reconnect_policy_init(policy: *mut surf_reconnect_policy_t);
    pub fn surf_reconnect_policy_reset(policy: *mut surf_reconnect_policy_t);
    pub fn surf_reconnect_policy_failure(
        policy: *mut surf_reconnect_policy_t,
        retryable: c_int,
        connected_ms: u64,
        out_attempt: *mut u8,
        out_delay_ms: *mut u32,
    ) -> c_int;
    pub fn surf_reconnect_policy_sizeof() -> usize;
    pub fn surf_media_policy_init(policy: *mut surf_media_policy_t);
    pub fn surf_media_policy_reset(policy: *mut surf_media_policy_t);
    pub fn surf_media_policy_admit(
        policy: *mut surf_media_policy_t,
        generation: u32,
        sequence: u32,
        is_idr: c_int,
        out_admission: *mut surf_media_admission_t,
    ) -> c_int;
    pub fn surf_media_policy_sizeof() -> usize;
    pub fn surf_media_admission_sizeof() -> usize;
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::mem::size_of;

    #[test]
    fn raw_layout_matches_c_compiler() {
        unsafe {
            assert_eq!(surf_core_abi_version(), SURF_CORE_ABI_VERSION);
            assert_eq!(
                surf_core_sizeof_string_view(),
                size_of::<surf_string_view_t>()
            );
            assert_eq!(surf_core_sizeof_config(), size_of::<surf_core_config_t>());
            assert_eq!(surf_core_sizeof_event(), size_of::<surf_event_t>());
            assert_eq!(surf_core_sizeof_snapshot(), size_of::<surf_snapshot_t>());
            assert_eq!(surf_frame_sizeof_view(), size_of::<surf_frame_view_t>());
            assert_eq!(
                surf_reconnect_policy_sizeof(),
                size_of::<surf_reconnect_policy_t>()
            );
            assert_eq!(surf_media_policy_sizeof(), size_of::<surf_media_policy_t>());
            assert_eq!(
                surf_media_admission_sizeof(),
                size_of::<surf_media_admission_t>()
            );
        }
    }
}
