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
pub const SURF_INPUT_OK: c_int = 0;
pub const SURF_DIAGNOSTICS_OFFLINE: c_int = 0;
pub const SURF_DIAGNOSTICS_SMOOTH: c_int = 1;
pub const SURF_DIAGNOSTICS_DELAYED: c_int = 2;
pub const SURF_DIAGNOSTICS_UNSTABLE: c_int = 3;
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

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_input_state_t {
    pub sequence: u64,
    pub interaction_id: u64,
    pub last_timestamp_ns: u64,
    pub surface_generation: u32,
    pub reserved: u32,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_input_causal_t {
    pub interaction_id: u64,
    pub client_ns: u64,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_input_sample_t {
    pub sequence: u64,
    pub interaction_id: u64,
    pub client_ns: u64,
    pub event_ns: u64,
    pub surface_generation: u32,
    pub reserved: u32,
    pub x: f64,
    pub y: f64,
    pub delta_x: f64,
    pub delta_y: f64,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_clock_sync_t {
    pub pending_client_send_ns: u64,
    pub last_probe_ns: u64,
    pub best_rtt_ns: u64,
    pub server_minus_client_ns: i64,
    pub sample_rtt_ns: [u64; 8],
    pub sample_offset_ns: [i64; 8],
    pub sample_count: u8,
    pub next_sample: u8,
    pub awaiting_reply: u8,
    pub synchronized: u8,
}

#[repr(C)]
#[derive(Clone, Copy, Default)]
pub struct surf_diagnostics_sample_t {
    pub now_ns: u64,
    pub video_packets: u64,
    pub decoded_frames: u64,
    pub presented_frames: u64,
    pub ingress_replaced: u64,
    pub output_replaced: u64,
    pub presentation_replaced: u64,
    pub sequence_gaps: u64,
    pub decode_errors: u64,
    pub audio_underruns: u64,
    pub backend_capture_to_encode_us: u64,
    pub backend_encode_to_write_us: u64,
    pub network_us: u64,
    pub decode_us: u64,
    pub upload_us: u64,
    pub frame_age_us: u64,
    pub rtt_us: u64,
    pub clock_uncertainty_us: u64,
    pub maximum_presentation_gap_us: u64,
    pub encoded_video_depth: u32,
    pub decoded_video_depth: u32,
    pub audio_depth: u32,
    pub timing_synchronized: c_int,
    pub connected: c_int,
}

#[repr(C)]
#[derive(Clone, Copy, Default)]
pub struct surf_diagnostics_report_t {
    pub window_ms: f64,
    pub video_fps: f64,
    pub decode_fps: f64,
    pub presentation_fps: f64,
    pub drop_percent: f64,
    pub dropped_frames: u64,
    pub sequence_gaps: u64,
    pub decode_errors: u64,
    pub audio_underruns: u64,
    pub backend_capture_to_encode_us: u64,
    pub backend_encode_to_write_us: u64,
    pub network_us: u64,
    pub decode_us: u64,
    pub upload_us: u64,
    pub frame_age_us: u64,
    pub rtt_us: u64,
    pub clock_uncertainty_us: u64,
    pub maximum_presentation_gap_us: u64,
    pub encoded_video_depth: u32,
    pub decoded_video_depth: u32,
    pub audio_depth: u32,
    pub timing_synchronized: c_int,
    pub health: c_int,
    pub reason: c_int,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct surf_diagnostics_t {
    pub baseline: surf_diagnostics_sample_t,
    pub maximum_presentation_gap_us: u64,
    pub initialized: u8,
    pub reserved: [u8; 7],
}

unsafe extern "C" {
    pub fn surf_core_config_init(config: *mut surf_core_config_t);
    pub fn surf_core_create(
        config: *const surf_core_config_t,
        out_core: *mut *mut surf_core_t,
    ) -> c_int;
    pub fn surf_core_destroy(core: *mut surf_core_t);
    pub fn surf_core_begin_connection(core: *mut surf_core_t, generation: u64) -> c_int;
    pub fn surf_core_dispatch_scoped(
        core: *mut surf_core_t,
        generation: u64,
        event: *const surf_event_t,
    ) -> c_int;
    pub fn surf_core_dispatch(core: *mut surf_core_t, event: *const surf_event_t) -> c_int;
    pub fn surf_core_snapshot(
        core: *const surf_core_t,
        out_snapshot: *mut surf_snapshot_t,
    ) -> c_int;
    pub fn surf_core_next_effect(core: *mut surf_core_t, out_effect: *mut surf_effect_t) -> c_int;
    pub fn surf_core_connection_generation(core: *const surf_core_t) -> u64;
    pub fn surf_core_stale_event_count(core: *const surf_core_t) -> u64;
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
    pub fn surf_input_state_init(state: *mut surf_input_state_t);
    pub fn surf_input_set_surface(state: *mut surf_input_state_t, generation: u32);
    pub fn surf_input_next_causal(
        state: *mut surf_input_state_t,
        timestamp_ns: u64,
        out_causal: *mut surf_input_causal_t,
    ) -> c_int;
    pub fn surf_input_pointer_sample(
        state: *mut surf_input_state_t,
        local_x: f64,
        local_y: f64,
        surface_width: f64,
        surface_height: f64,
        timestamp_ns: u64,
        out_sample: *mut surf_input_sample_t,
    ) -> c_int;
    pub fn surf_input_wheel_sample(
        state: *mut surf_input_state_t,
        local_x: f64,
        local_y: f64,
        delta_x: f64,
        delta_y: f64,
        surface_width: f64,
        surface_height: f64,
        timestamp_ns: u64,
        out_sample: *mut surf_input_sample_t,
    ) -> c_int;
    pub fn surf_input_state_sizeof() -> usize;
    pub fn surf_input_causal_sizeof() -> usize;
    pub fn surf_input_sample_sizeof() -> usize;
    pub fn surf_clock_sync_init(sync: *mut surf_clock_sync_t);
    pub fn surf_clock_sync_reset(sync: *mut surf_clock_sync_t);
    pub fn surf_clock_sync_probe(
        sync: *mut surf_clock_sync_t,
        now_ns: u64,
        out_client_send_ns: *mut u64,
    ) -> c_int;
    pub fn surf_clock_sync_consume(
        sync: *mut surf_clock_sync_t,
        client_send_ns: u64,
        backend_receive_ns: u64,
        backend_send_ns: u64,
        client_receive_ns: u64,
    ) -> c_int;
    pub fn surf_clock_sync_server_to_client(
        sync: *const surf_clock_sync_t,
        server_ns: u64,
        out_client_ns: *mut u64,
    ) -> c_int;
    pub fn surf_clock_sync_sizeof() -> usize;
    pub fn surf_diagnostics_init(diagnostics: *mut surf_diagnostics_t);
    pub fn surf_diagnostics_reset(diagnostics: *mut surf_diagnostics_t);
    pub fn surf_diagnostics_update(
        diagnostics: *mut surf_diagnostics_t,
        sample: *const surf_diagnostics_sample_t,
        out_report: *mut surf_diagnostics_report_t,
    ) -> c_int;
    pub fn surf_diagnostics_health_string(health: c_int) -> *const c_char;
    pub fn surf_diagnostics_reason_string(reason: c_int) -> *const c_char;
    pub fn surf_diagnostics_sizeof() -> usize;
    pub fn surf_diagnostics_sample_sizeof() -> usize;
    pub fn surf_diagnostics_report_sizeof() -> usize;
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
            assert_eq!(surf_input_state_sizeof(), size_of::<surf_input_state_t>());
            assert_eq!(surf_input_causal_sizeof(), size_of::<surf_input_causal_t>());
            assert_eq!(surf_input_sample_sizeof(), size_of::<surf_input_sample_t>());
            assert_eq!(surf_clock_sync_sizeof(), size_of::<surf_clock_sync_t>());
            assert_eq!(surf_diagnostics_sizeof(), size_of::<surf_diagnostics_t>());
            assert_eq!(
                surf_diagnostics_sample_sizeof(),
                size_of::<surf_diagnostics_sample_t>()
            );
            assert_eq!(
                surf_diagnostics_report_sizeof(),
                size_of::<surf_diagnostics_report_t>()
            );
        }
    }
}
