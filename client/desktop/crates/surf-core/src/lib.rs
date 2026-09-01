use std::{
    error::Error as StdError,
    ffi::{CStr, c_int},
    fmt,
    marker::PhantomData,
    ptr::{self, NonNull},
    rc::Rc,
    slice, str,
};

use surf_core_sys as sys;

/// Process-local monotonic time used for protocol timestamps and media
/// diagnostics. Its epoch is intentionally opaque and only differences or a
/// synchronized backend-clock translation are meaningful.
pub fn monotonic_ns() -> u64 {
    static EPOCH: std::sync::OnceLock<std::time::Instant> = std::sync::OnceLock::new();
    u64::try_from(
        EPOCH
            .get_or_init(std::time::Instant::now)
            .elapsed()
            .as_nanos(),
    )
    .unwrap_or(u64::MAX)
    .saturating_add(1)
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Error {
    code: c_int,
    message: String,
}

impl Error {
    fn from_code(code: c_int) -> Self {
        let message = unsafe {
            let pointer = sys::surf_core_result_string(code);
            if pointer.is_null() {
                "unknown core error".to_owned()
            } else {
                CStr::from_ptr(pointer).to_string_lossy().into_owned()
            }
        };
        Self { code, message }
    }

    fn invalid_utf8() -> Self {
        Self {
            code: -1,
            message: "core snapshot contains invalid UTF-8".to_owned(),
        }
    }

    fn from_frame_code(code: c_int) -> Self {
        let message = unsafe {
            let pointer = sys::surf_frame_result_string(code);
            if pointer.is_null() {
                "unknown frame error".to_owned()
            } else {
                CStr::from_ptr(pointer).to_string_lossy().into_owned()
            }
        };
        Self { code, message }
    }

    fn invariant(message: &str) -> Self {
        Self {
            code: -1,
            message: message.to_owned(),
        }
    }

    pub fn code(&self) -> i32 {
        self.code
    }
}

impl fmt::Display for Error {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&self.message)
    }
}

impl StdError for Error {}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct RetryDecision {
    pub attempt: u8,
    pub maximum: u8,
    pub delay: std::time::Duration,
}

pub struct ReconnectPolicy {
    raw: sys::surf_reconnect_policy_t,
}

impl ReconnectPolicy {
    pub fn new() -> Self {
        let mut raw = std::mem::MaybeUninit::<sys::surf_reconnect_policy_t>::uninit();
        // SAFETY: the C initializer writes every field of the non-null output.
        unsafe { sys::surf_reconnect_policy_init(raw.as_mut_ptr()) };
        // SAFETY: `surf_reconnect_policy_init` initializes the complete value.
        let raw = unsafe { raw.assume_init() };
        Self { raw }
    }

    pub fn failure(
        &mut self,
        retryable: bool,
        connected_for: std::time::Duration,
    ) -> Result<Option<RetryDecision>, Error> {
        let mut attempt = 0_u8;
        let mut delay_ms = 0_u32;
        // SAFETY: all pointers refer to initialized, uniquely borrowed values.
        let result = unsafe {
            sys::surf_reconnect_policy_failure(
                &mut self.raw,
                i32::from(retryable),
                u64::try_from(connected_for.as_millis()).unwrap_or(u64::MAX),
                &mut attempt,
                &mut delay_ms,
            )
        };
        match result {
            sys::SURF_RECONNECT_RETRY => Ok(Some(RetryDecision {
                attempt,
                maximum: self.raw.maximum_attempts,
                delay: std::time::Duration::from_millis(u64::from(delay_ms)),
            })),
            sys::SURF_RECONNECT_STOP => Ok(None),
            code => Err(Error::invariant(&format!(
                "reconnect policy failed with code {code}"
            ))),
        }
    }

    pub fn reset(&mut self) {
        // SAFETY: `self.raw` is a valid, uniquely borrowed policy.
        unsafe { sys::surf_reconnect_policy_reset(&mut self.raw) };
    }
}

impl Default for ReconnectPolicy {
    fn default() -> Self {
        Self::new()
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct CausalStamp {
    pub interaction_id: u64,
    pub client_ns: u64,
}

#[derive(Clone, Copy, Debug, PartialEq)]
pub struct InputSample {
    pub sequence: u64,
    pub interaction_id: u64,
    pub client_ns: u64,
    pub event_ns: u64,
    pub surface_generation: u32,
    pub x: f64,
    pub y: f64,
    pub delta_x: f64,
    pub delta_y: f64,
}

pub struct InputState {
    raw: sys::surf_input_state_t,
}

impl InputState {
    pub fn new() -> Self {
        let mut raw = std::mem::MaybeUninit::<sys::surf_input_state_t>::uninit();
        // SAFETY: the C initializer writes the complete non-null value.
        unsafe { sys::surf_input_state_init(raw.as_mut_ptr()) };
        // SAFETY: the complete value was initialized above.
        Self {
            raw: unsafe { raw.assume_init() },
        }
    }

    pub fn set_surface(&mut self, generation: u32) {
        // SAFETY: `self.raw` is valid and uniquely borrowed.
        unsafe { sys::surf_input_set_surface(&mut self.raw, generation) };
    }

    pub fn causal(&mut self, timestamp_ns: u64) -> Result<CausalStamp, Error> {
        let mut raw = std::mem::MaybeUninit::<sys::surf_input_causal_t>::uninit();
        // SAFETY: both pointers are valid and uniquely borrowed. A successful
        // result initializes the complete output value.
        let result =
            unsafe { sys::surf_input_next_causal(&mut self.raw, timestamp_ns, raw.as_mut_ptr()) };
        if result != sys::SURF_INPUT_OK {
            return Err(Error::invariant(&format!(
                "input causal sequencing failed with code {result}"
            )));
        }
        // SAFETY: SURF_INPUT_OK guarantees initialized output.
        let raw = unsafe { raw.assume_init() };
        Ok(CausalStamp {
            interaction_id: raw.interaction_id,
            client_ns: raw.client_ns,
        })
    }

    pub fn pointer(
        &mut self,
        local: (f64, f64),
        surface: (f64, f64),
        timestamp_ns: u64,
    ) -> Result<InputSample, Error> {
        self.sample(local, (0.0, 0.0), surface, timestamp_ns, false)
    }

    pub fn wheel(
        &mut self,
        local: (f64, f64),
        delta: (f64, f64),
        surface: (f64, f64),
        timestamp_ns: u64,
    ) -> Result<InputSample, Error> {
        self.sample(local, delta, surface, timestamp_ns, true)
    }

    fn sample(
        &mut self,
        local: (f64, f64),
        delta: (f64, f64),
        surface: (f64, f64),
        timestamp_ns: u64,
        wheel: bool,
    ) -> Result<InputSample, Error> {
        let mut raw = std::mem::MaybeUninit::<sys::surf_input_sample_t>::uninit();
        // SAFETY: both pointers are valid and uniquely borrowed. The selected
        // C function initializes the output only on success.
        let result = unsafe {
            if wheel {
                sys::surf_input_wheel_sample(
                    &mut self.raw,
                    local.0,
                    local.1,
                    delta.0,
                    delta.1,
                    surface.0,
                    surface.1,
                    timestamp_ns,
                    raw.as_mut_ptr(),
                )
            } else {
                sys::surf_input_pointer_sample(
                    &mut self.raw,
                    local.0,
                    local.1,
                    surface.0,
                    surface.1,
                    timestamp_ns,
                    raw.as_mut_ptr(),
                )
            }
        };
        if result != sys::SURF_INPUT_OK {
            return Err(Error::invariant(&format!(
                "input normalization failed with code {result}"
            )));
        }
        // SAFETY: SURF_INPUT_OK guarantees initialized output.
        let raw = unsafe { raw.assume_init() };
        Ok(InputSample {
            sequence: raw.sequence,
            interaction_id: raw.interaction_id,
            client_ns: raw.client_ns,
            event_ns: raw.event_ns,
            surface_generation: raw.surface_generation,
            x: raw.x,
            y: raw.y,
            delta_x: raw.delta_x,
            delta_y: raw.delta_y,
        })
    }
}

impl Default for InputState {
    fn default() -> Self {
        Self::new()
    }
}

pub struct ClockSync {
    raw: sys::surf_clock_sync_t,
}

impl ClockSync {
    pub fn new() -> Self {
        let mut raw = std::mem::MaybeUninit::<sys::surf_clock_sync_t>::uninit();
        // SAFETY: the C initializer writes the complete non-null value.
        unsafe { sys::surf_clock_sync_init(raw.as_mut_ptr()) };
        // SAFETY: the complete value was initialized above.
        Self {
            raw: unsafe { raw.assume_init() },
        }
    }

    pub fn reset(&mut self) {
        // SAFETY: `self.raw` is a valid, uniquely borrowed clock state.
        unsafe { sys::surf_clock_sync_reset(&mut self.raw) };
    }

    pub fn probe(&mut self, now_ns: u64) -> Option<u64> {
        let mut client_send_ns = 0_u64;
        // SAFETY: both pointers are valid and uniquely borrowed.
        let result =
            unsafe { sys::surf_clock_sync_probe(&mut self.raw, now_ns, &mut client_send_ns) };
        (result == 1).then_some(client_send_ns)
    }

    pub fn consume(
        &mut self,
        client_send_ns: u64,
        backend_receive_ns: u64,
        backend_send_ns: u64,
        client_receive_ns: u64,
    ) -> bool {
        // SAFETY: `self.raw` is valid and uniquely borrowed; times are values.
        unsafe {
            sys::surf_clock_sync_consume(
                &mut self.raw,
                client_send_ns,
                backend_receive_ns,
                backend_send_ns,
                client_receive_ns,
            ) == 1
        }
    }

    pub fn synchronized(&self) -> bool {
        self.raw.synchronized != 0
    }

    pub fn rtt_ns(&self) -> Option<u64> {
        self.synchronized().then_some(self.raw.best_rtt_ns)
    }

    pub fn server_minus_client_ns(&self) -> Option<i64> {
        self.synchronized()
            .then_some(self.raw.server_minus_client_ns)
    }

    pub fn server_to_client_ns(&self, server_ns: u64) -> Option<u64> {
        let mut client_ns = 0_u64;
        // SAFETY: both pointers are valid and the output is uniquely borrowed.
        let converted =
            unsafe { sys::surf_clock_sync_server_to_client(&self.raw, server_ns, &mut client_ns) };
        (converted == 1).then_some(client_ns)
    }
}

impl Default for ClockSync {
    fn default() -> Self {
        Self::new()
    }
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub enum DiagnosticsHealth {
    Offline,
    #[default]
    Smooth,
    Delayed,
    Unstable,
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub enum DiagnosticsReason {
    #[default]
    None,
    Offline,
    DecodeError,
    VideoBacklog,
    Network,
    FrameAge,
    DecodeTime,
    AudioUnderrun,
    FrameDrops,
    PresentationGap,
    Unknown,
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct DiagnosticsSample {
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
    pub timing_synchronized: bool,
    pub connected: bool,
}

#[derive(Clone, Copy, Debug, Default, PartialEq)]
pub struct DiagnosticsReport {
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
    pub timing_synchronized: bool,
    pub health: DiagnosticsHealth,
    pub reason: DiagnosticsReason,
}

pub struct PipelineDiagnostics {
    raw: sys::surf_diagnostics_t,
}

impl PipelineDiagnostics {
    pub fn new() -> Self {
        let mut raw = std::mem::MaybeUninit::<sys::surf_diagnostics_t>::uninit();
        // SAFETY: the C initializer writes the complete non-null value.
        unsafe { sys::surf_diagnostics_init(raw.as_mut_ptr()) };
        // SAFETY: the complete value was initialized above.
        Self {
            raw: unsafe { raw.assume_init() },
        }
    }

    pub fn reset(&mut self) {
        // SAFETY: `self.raw` is a valid, uniquely borrowed diagnostics state.
        unsafe { sys::surf_diagnostics_reset(&mut self.raw) };
    }

    pub fn update(&mut self, sample: DiagnosticsSample) -> Option<DiagnosticsReport> {
        let raw_sample = sys::surf_diagnostics_sample_t {
            now_ns: sample.now_ns,
            video_packets: sample.video_packets,
            decoded_frames: sample.decoded_frames,
            presented_frames: sample.presented_frames,
            ingress_replaced: sample.ingress_replaced,
            output_replaced: sample.output_replaced,
            presentation_replaced: sample.presentation_replaced,
            sequence_gaps: sample.sequence_gaps,
            decode_errors: sample.decode_errors,
            audio_underruns: sample.audio_underruns,
            backend_capture_to_encode_us: sample.backend_capture_to_encode_us,
            backend_encode_to_write_us: sample.backend_encode_to_write_us,
            network_us: sample.network_us,
            decode_us: sample.decode_us,
            upload_us: sample.upload_us,
            frame_age_us: sample.frame_age_us,
            rtt_us: sample.rtt_us,
            clock_uncertainty_us: sample.clock_uncertainty_us,
            maximum_presentation_gap_us: sample.maximum_presentation_gap_us,
            encoded_video_depth: sample.encoded_video_depth,
            decoded_video_depth: sample.decoded_video_depth,
            audio_depth: sample.audio_depth,
            timing_synchronized: i32::from(sample.timing_synchronized),
            connected: i32::from(sample.connected),
        };
        let mut raw_report = sys::surf_diagnostics_report_t::default();
        // SAFETY: all pointers refer to initialized values with the required
        // exclusive access to the diagnostics state and output.
        let result =
            unsafe { sys::surf_diagnostics_update(&mut self.raw, &raw_sample, &mut raw_report) };
        if result != 1 {
            return None;
        }
        Some(DiagnosticsReport {
            window_ms: raw_report.window_ms,
            video_fps: raw_report.video_fps,
            decode_fps: raw_report.decode_fps,
            presentation_fps: raw_report.presentation_fps,
            drop_percent: raw_report.drop_percent,
            dropped_frames: raw_report.dropped_frames,
            sequence_gaps: raw_report.sequence_gaps,
            decode_errors: raw_report.decode_errors,
            audio_underruns: raw_report.audio_underruns,
            backend_capture_to_encode_us: raw_report.backend_capture_to_encode_us,
            backend_encode_to_write_us: raw_report.backend_encode_to_write_us,
            network_us: raw_report.network_us,
            decode_us: raw_report.decode_us,
            upload_us: raw_report.upload_us,
            frame_age_us: raw_report.frame_age_us,
            rtt_us: raw_report.rtt_us,
            clock_uncertainty_us: raw_report.clock_uncertainty_us,
            maximum_presentation_gap_us: raw_report.maximum_presentation_gap_us,
            encoded_video_depth: raw_report.encoded_video_depth,
            decoded_video_depth: raw_report.decoded_video_depth,
            audio_depth: raw_report.audio_depth,
            timing_synchronized: raw_report.timing_synchronized != 0,
            health: match raw_report.health {
                sys::SURF_DIAGNOSTICS_OFFLINE => DiagnosticsHealth::Offline,
                sys::SURF_DIAGNOSTICS_SMOOTH => DiagnosticsHealth::Smooth,
                sys::SURF_DIAGNOSTICS_DELAYED => DiagnosticsHealth::Delayed,
                _ => DiagnosticsHealth::Unstable,
            },
            reason: match raw_report.reason {
                0 => DiagnosticsReason::None,
                1 => DiagnosticsReason::Offline,
                2 => DiagnosticsReason::DecodeError,
                3 => DiagnosticsReason::VideoBacklog,
                4 => DiagnosticsReason::Network,
                5 => DiagnosticsReason::FrameAge,
                6 => DiagnosticsReason::DecodeTime,
                7 => DiagnosticsReason::AudioUnderrun,
                8 => DiagnosticsReason::FrameDrops,
                9 => DiagnosticsReason::PresentationGap,
                _ => DiagnosticsReason::Unknown,
            },
        })
    }
}

impl Default for PipelineDiagnostics {
    fn default() -> Self {
        Self::new()
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum MediaAction {
    DropRequestKeyframe,
    Decode,
    ResetAndDecode,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct MediaAdmission {
    pub action: MediaAction,
    pub generation_changed: bool,
    pub sequence_gap: bool,
}

pub struct MediaAdmissionPolicy {
    raw: sys::surf_media_policy_t,
}

impl MediaAdmissionPolicy {
    pub fn new() -> Self {
        let mut raw = std::mem::MaybeUninit::<sys::surf_media_policy_t>::uninit();
        // SAFETY: the C initializer writes every field of the non-null output.
        unsafe { sys::surf_media_policy_init(raw.as_mut_ptr()) };
        // SAFETY: `surf_media_policy_init` initializes the complete value.
        let raw = unsafe { raw.assume_init() };
        Self { raw }
    }

    pub fn reset(&mut self) {
        // SAFETY: `self.raw` is a valid, uniquely borrowed policy.
        unsafe { sys::surf_media_policy_reset(&mut self.raw) };
    }

    pub fn admit(
        &mut self,
        generation: u32,
        sequence: u32,
        is_idr: bool,
    ) -> Result<MediaAdmission, Error> {
        let mut raw = std::mem::MaybeUninit::<sys::surf_media_admission_t>::uninit();
        // SAFETY: both pointers are valid and uniquely borrowed; C initializes
        // the admission on SURF_MEDIA_OK.
        let result = unsafe {
            sys::surf_media_policy_admit(
                &mut self.raw,
                generation,
                sequence,
                i32::from(is_idr),
                raw.as_mut_ptr(),
            )
        };
        if result != sys::SURF_MEDIA_OK {
            return Err(Error::invariant(
                "media admission policy rejected its arguments",
            ));
        }
        // SAFETY: SURF_MEDIA_OK guarantees initialized output.
        let raw = unsafe { raw.assume_init() };
        let action = match raw.action {
            sys::SURF_MEDIA_ACTION_DROP_REQUEST_KEYFRAME => MediaAction::DropRequestKeyframe,
            sys::SURF_MEDIA_ACTION_DECODE => MediaAction::Decode,
            sys::SURF_MEDIA_ACTION_RESET_AND_DECODE => MediaAction::ResetAndDecode,
            value => {
                return Err(Error::invariant(&format!(
                    "unknown media admission action {value}"
                )));
            }
        };
        Ok(MediaAdmission {
            action,
            generation_changed: raw.generation_changed != 0,
            sequence_gap: raw.sequence_gap != 0,
        })
    }
}

impl Default for MediaAdmissionPolicy {
    fn default() -> Self {
        Self::new()
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum FrameKind {
    Video,
    Audio,
}

#[derive(Clone, Copy, Debug)]
pub struct Frame<'a> {
    pub kind: FrameKind,
    pub idr: bool,
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
    pub payload: &'a [u8],
}

impl<'a> Frame<'a> {
    pub fn parse(data: &'a [u8]) -> Result<Self, Error> {
        let mut raw = std::mem::MaybeUninit::<sys::surf_frame_view_t>::uninit();
        // SAFETY: the parser receives exactly the valid extent of `data` and
        // writes `raw` only when it returns SURF_FRAME_OK.
        let result = unsafe { sys::surf_frame_parse(data.as_ptr(), data.len(), raw.as_mut_ptr()) };
        if result != sys::SURF_FRAME_OK {
            return Err(Error::from_frame_code(result));
        }
        // SAFETY: SURF_FRAME_OK guarantees initialized output.
        let raw = unsafe { raw.assume_init() };
        let kind = match raw.type_ {
            3 => FrameKind::Video,
            4 => FrameKind::Audio,
            _ => return Err(Error::invariant("unsupported frame type")),
        };
        let input_start = data.as_ptr() as usize;
        let payload_start = raw.payload as usize;
        let payload_offset = payload_start
            .checked_sub(input_start)
            .ok_or_else(|| Error::invariant("frame payload precedes input"))?;
        let payload_end = payload_offset
            .checked_add(raw.payload_length)
            .ok_or_else(|| Error::invariant("frame payload overflow"))?;
        if payload_end > data.len() {
            return Err(Error::invariant("frame payload escaped its input"));
        }
        Ok(Self {
            kind,
            idr: raw.flags & 1 != 0,
            sequence: raw.sequence,
            source_sequence: raw.source_sequence,
            width: raw.width,
            height: raw.height,
            interaction_id: raw.interaction_id,
            source_receive_ns: raw.source_receive_ns,
            encode_complete_ns: raw.encode_complete_ns,
            socket_write_ns: raw.socket_write_ns,
            encoder_generation: raw.encoder_generation,
            input_receive_ns: raw.input_receive_ns,
            cdp_accepted_ns: raw.cdp_accepted_ns,
            profile: raw.profile,
            payload: &data[payload_offset..payload_end],
        })
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Tab {
    pub id: i64,
    pub title: String,
    pub url: String,
    pub icon: String,
    pub active: bool,
}

#[derive(Debug, Clone, PartialEq)]
pub enum Event {
    Reset,
    Tabs(Vec<Tab>),
    Url {
        url: String,
        security: String,
        starred: bool,
    },
    History {
        can_go_back: bool,
        can_go_forward: bool,
    },
    Loading(bool),
    Editable {
        on: bool,
        show_keyboard: bool,
        kind: String,
        rect: Option<[f64; 4]>,
    },
    Fullscreen(bool),
    Security(String),
    Starred(bool),
    PageFrame(u32),
    FramePresented(u32),
    KeyboardVisible(bool),
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Effect {
    RequestLibrary,
    ShowKeyboard,
    HideKeyboard,
    Unknown(i32),
}

#[derive(Debug, Clone, PartialEq)]
pub struct Snapshot {
    pub revision: u64,
    pub tabs: Vec<Tab>,
    pub active_tab_id: Option<i64>,
    pub active_title: String,
    pub current_url: String,
    pub security: String,
    pub editable_kind: String,
    pub editable_rect: Option<[f64; 4]>,
    pub awaited_source_sequence: Option<u32>,
    pub show_start_page: bool,
    pub loading: bool,
    pub can_go_back: bool,
    pub can_go_forward: bool,
    pub starred: bool,
    pub fullscreen: bool,
    pub editable: bool,
    pub keyboard_visible: bool,
    pub awaiting_page_frame: bool,
}

pub struct Core {
    raw: NonNull<sys::surf_core_t>,
    connection_generation: u64,
    _single_owner: PhantomData<Rc<()>>,
}

impl Core {
    pub fn new() -> Result<Self, Error> {
        let mut config = std::mem::MaybeUninit::<sys::surf_core_config_t>::uninit();
        let mut raw = ptr::null_mut();
        let result = unsafe {
            sys::surf_core_config_init(config.as_mut_ptr());
            sys::surf_core_create(config.as_ptr(), &mut raw)
        };
        check(result)?;
        let raw = NonNull::new(raw).ok_or_else(|| Error {
            code: -1,
            message: "core returned a null handle".to_owned(),
        })?;
        Ok(Self {
            raw,
            connection_generation: 1,
            _single_owner: PhantomData,
        })
    }

    pub fn begin_connection(&mut self) -> Result<u64, Error> {
        let generation = self
            .connection_generation
            .checked_add(1)
            .ok_or_else(|| Error::invariant("connection generation exhausted"))?;
        check(unsafe { sys::surf_core_begin_connection(self.raw.as_ptr(), generation) })?;
        self.connection_generation = generation;
        Ok(generation)
    }

    pub fn connection_generation(&self) -> u64 {
        self.connection_generation
    }

    pub fn stale_event_count(&self) -> u64 {
        unsafe { sys::surf_core_stale_event_count(self.raw.as_ptr()) }
    }

    pub fn dispatch(&mut self, event: &Event) -> Result<Vec<Effect>, Error> {
        match event {
            Event::Tabs(tabs) => self.dispatch_tabs(tabs)?,
            Event::Url {
                url,
                security,
                starred,
            } => {
                let raw = sys::surf_event_t {
                    kind: sys::SURF_EVENT_URL,
                    data: sys::surf_event_data_t {
                        url: sys::surf_url_event_t {
                            url: string_view(url),
                            security: string_view(security),
                            starred: int(*starred),
                        },
                    },
                };
                self.dispatch_raw(&raw)?;
            }
            Event::History {
                can_go_back,
                can_go_forward,
            } => {
                let raw = sys::surf_event_t {
                    kind: sys::SURF_EVENT_HISTORY_STATE,
                    data: sys::surf_event_data_t {
                        history: sys::surf_history_event_t {
                            can_go_back: int(*can_go_back),
                            can_go_forward: int(*can_go_forward),
                        },
                    },
                };
                self.dispatch_raw(&raw)?;
            }
            Event::Editable {
                on,
                show_keyboard,
                kind,
                rect,
            } => {
                let raw = sys::surf_event_t {
                    kind: sys::SURF_EVENT_EDITABLE,
                    data: sys::surf_event_data_t {
                        editable: sys::surf_editable_event_t {
                            on: int(*on),
                            show_keyboard: int(*show_keyboard),
                            kind: string_view(kind),
                            has_rect: int(rect.is_some()),
                            rect: rect.unwrap_or_default(),
                        },
                    },
                };
                self.dispatch_raw(&raw)?;
            }
            Event::Security(state) => {
                let raw = sys::surf_event_t {
                    kind: sys::SURF_EVENT_SECURITY,
                    data: sys::surf_event_data_t {
                        security: sys::surf_security_event_t {
                            state: string_view(state),
                        },
                    },
                };
                self.dispatch_raw(&raw)?;
            }
            Event::PageFrame(sequence) | Event::FramePresented(sequence) => {
                let kind = if matches!(event, Event::PageFrame(_)) {
                    sys::SURF_EVENT_PAGE_FRAME
                } else {
                    sys::SURF_EVENT_FRAME_PRESENTED
                };
                let raw = sys::surf_event_t {
                    kind,
                    data: sys::surf_event_data_t {
                        page_frame: sys::surf_page_frame_event_t {
                            source_sequence: *sequence,
                        },
                    },
                };
                self.dispatch_raw(&raw)?;
            }
            Event::Reset
            | Event::Loading(_)
            | Event::Fullscreen(_)
            | Event::Starred(_)
            | Event::KeyboardVisible(_) => {
                let (kind, value) = match event {
                    Event::Reset => (sys::SURF_EVENT_RESET, false),
                    Event::Loading(value) => (sys::SURF_EVENT_LOADING, *value),
                    Event::Fullscreen(value) => (sys::SURF_EVENT_FULLSCREEN, *value),
                    Event::Starred(value) => (sys::SURF_EVENT_STARRED, *value),
                    Event::KeyboardVisible(value) => (sys::SURF_EVENT_KEYBOARD_VISIBILITY, *value),
                    _ => unreachable!(),
                };
                let raw = sys::surf_event_t {
                    kind,
                    data: sys::surf_event_data_t {
                        boolean: sys::surf_boolean_event_t { on: int(value) },
                    },
                };
                self.dispatch_raw(&raw)?;
            }
        }
        Ok(self.drain_effects())
    }

    fn dispatch_tabs(&mut self, tabs: &[Tab]) -> Result<(), Error> {
        let raw_tabs: Vec<_> = tabs
            .iter()
            .map(|tab| sys::surf_tab_event_t {
                id: tab.id,
                title: string_view(&tab.title),
                url: string_view(&tab.url),
                icon: string_view(&tab.icon),
                active: int(tab.active),
            })
            .collect();
        let raw = sys::surf_event_t {
            kind: sys::SURF_EVENT_TABS,
            data: sys::surf_event_data_t {
                tabs: sys::surf_tabs_event_t {
                    items: if raw_tabs.is_empty() {
                        ptr::null()
                    } else {
                        raw_tabs.as_ptr()
                    },
                    count: raw_tabs.len(),
                },
            },
        };
        self.dispatch_raw(&raw)
    }

    fn dispatch_raw(&mut self, event: &sys::surf_event_t) -> Result<(), Error> {
        let result = unsafe {
            sys::surf_core_dispatch_scoped(self.raw.as_ptr(), self.connection_generation, event)
        };
        check(result)
    }

    pub fn snapshot(&self) -> Result<Snapshot, Error> {
        let mut raw = std::mem::MaybeUninit::<sys::surf_snapshot_t>::uninit();
        check(unsafe { sys::surf_core_snapshot(self.raw.as_ptr(), raw.as_mut_ptr()) })?;
        let raw = unsafe { raw.assume_init() };
        let raw_tabs = if raw.tab_count == 0 {
            &[][..]
        } else {
            if raw.tabs.is_null() {
                return Err(Error {
                    code: -1,
                    message: "core returned null tabs with a nonzero count".to_owned(),
                });
            }
            unsafe { slice::from_raw_parts(raw.tabs, raw.tab_count) }
        };
        let tabs = raw_tabs
            .iter()
            .map(|tab| {
                Ok(Tab {
                    id: tab.id,
                    title: owned_string(tab.title)?,
                    url: owned_string(tab.url)?,
                    icon: owned_string(tab.icon)?,
                    active: tab.active != 0,
                })
            })
            .collect::<Result<Vec<_>, Error>>()?;
        Ok(Snapshot {
            revision: raw.revision,
            tabs,
            active_tab_id: (raw.has_active_tab != 0).then_some(raw.active_tab_id),
            active_title: owned_string(raw.active_title)?,
            current_url: owned_string(raw.current_url)?,
            security: owned_string(raw.security)?,
            editable_kind: owned_string(raw.editable_kind)?,
            editable_rect: (raw.editable_has_rect != 0).then_some(raw.editable_rect),
            awaited_source_sequence: (raw.awaiting_page_frame != 0)
                .then_some(raw.awaited_source_sequence),
            show_start_page: raw.show_start_page != 0,
            loading: raw.loading != 0,
            can_go_back: raw.can_go_back != 0,
            can_go_forward: raw.can_go_forward != 0,
            starred: raw.starred != 0,
            fullscreen: raw.fullscreen != 0,
            editable: raw.editable != 0,
            keyboard_visible: raw.keyboard_visible != 0,
            awaiting_page_frame: raw.awaiting_page_frame != 0,
        })
    }

    fn drain_effects(&mut self) -> Vec<Effect> {
        let mut effects = Vec::new();
        loop {
            let mut raw = sys::surf_effect_t { kind: 0 };
            if unsafe { sys::surf_core_next_effect(self.raw.as_ptr(), &mut raw) } == 0 {
                break;
            }
            effects.push(match raw.kind {
                1 => Effect::RequestLibrary,
                2 => Effect::ShowKeyboard,
                3 => Effect::HideKeyboard,
                other => Effect::Unknown(other),
            });
        }
        effects
    }
}

impl Drop for Core {
    fn drop(&mut self) {
        unsafe { sys::surf_core_destroy(self.raw.as_ptr()) };
    }
}

fn check(code: c_int) -> Result<(), Error> {
    if code == sys::SURF_CORE_OK {
        Ok(())
    } else {
        Err(Error::from_code(code))
    }
}

fn int(value: bool) -> c_int {
    i32::from(value)
}

fn string_view(value: &str) -> sys::surf_string_view_t {
    sys::surf_string_view_t {
        data: value.as_ptr().cast(),
        length: value.len(),
    }
}

fn owned_string(value: sys::surf_string_view_t) -> Result<String, Error> {
    if value.length == 0 {
        return Ok(String::new());
    }
    if value.data.is_null() {
        return Err(Error {
            code: -1,
            message: "core returned a null string with nonzero length".to_owned(),
        });
    }
    let bytes = unsafe { slice::from_raw_parts(value.data.cast::<u8>(), value.length) };
    Ok(str::from_utf8(bytes)
        .map_err(|_| Error::invalid_utf8())?
        .to_owned())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rust_drives_the_c_browser_model() {
        let mut core = Core::new().unwrap();
        assert!(core.snapshot().unwrap().show_start_page);
        core.dispatch(&Event::Tabs(vec![Tab {
            id: 7,
            title: "https://surf.test/page".to_owned(),
            url: "https://surf.test/page".to_owned(),
            icon: String::new(),
            active: true,
        }]))
        .unwrap();
        let snapshot = core.snapshot().unwrap();
        assert_eq!(snapshot.active_tab_id, Some(7));
        assert_eq!(snapshot.active_title, "surf.test");

        core.dispatch(&Event::Loading(true)).unwrap();
        core.dispatch(&Event::Editable {
            on: true,
            show_keyboard: true,
            kind: "text".to_owned(),
            rect: Some([0.1, 0.2, 0.3, 0.4]),
        })
        .unwrap();
        let snapshot = core.snapshot().unwrap();
        assert!(snapshot.loading && snapshot.editable);
        assert_eq!(snapshot.editable_rect, Some([0.1, 0.2, 0.3, 0.4]));
    }

    #[test]
    fn event_strings_are_copied_by_the_core() {
        let mut core = Core::new().unwrap();
        let mut title = "Original".to_owned();
        core.dispatch(&Event::Tabs(vec![Tab {
            id: 1,
            title: title.clone(),
            url: "https://example.test".to_owned(),
            icon: String::new(),
            active: true,
        }]))
        .unwrap();
        title.replace_range(.., "Changed!");
        assert_eq!(core.snapshot().unwrap().active_title, "Original");
    }

    #[test]
    fn new_connection_discards_previous_browser_state() {
        let mut core = Core::new().unwrap();
        core.dispatch(&Event::Loading(true)).unwrap();
        assert_eq!(core.connection_generation(), 1);
        assert_eq!(core.begin_connection().unwrap(), 2);
        let snapshot = core.snapshot().unwrap();
        assert!(!snapshot.loading);
        assert!(snapshot.tabs.is_empty());
        assert_eq!(core.connection_generation(), 2);
        assert_eq!(core.stale_event_count(), 0);
    }

    #[test]
    fn binary_frame_view_borrows_validated_payload() {
        let mut data = vec![0_u8; 87];
        data[0..4].copy_from_slice(b"RBR1");
        data[4] = 3;
        data[5] = 1;
        data[7] = 84;
        data[11] = 7;
        data[19] = 2;
        data[23] = 3;
        data[84..].copy_from_slice(&[1, 2, 3]);
        let frame = Frame::parse(&data).unwrap();
        assert_eq!(frame.kind, FrameKind::Video);
        assert!(frame.idr);
        assert_eq!(frame.sequence, 7);
        assert_eq!(frame.height, 2);
        assert_eq!(frame.payload, &[1, 2, 3]);
    }

    #[test]
    fn reconnect_policy_is_shared_with_platform_hosts() {
        let mut policy = ReconnectPolicy::new();
        let delays: Vec<_> = (0..5)
            .map(|_| {
                policy
                    .failure(true, std::time::Duration::ZERO)
                    .unwrap()
                    .unwrap()
                    .delay
                    .as_millis()
            })
            .collect();
        assert_eq!(delays, vec![250, 500, 1000, 2000, 4000]);
        assert_eq!(
            policy.failure(true, std::time::Duration::ZERO).unwrap(),
            None
        );
        policy.reset();
        assert_eq!(
            policy
                .failure(true, std::time::Duration::ZERO)
                .unwrap()
                .unwrap()
                .delay
                .as_millis(),
            250
        );
        assert_eq!(
            policy.failure(false, std::time::Duration::ZERO).unwrap(),
            None
        );
        let stable = policy
            .failure(true, std::time::Duration::from_secs(30))
            .unwrap()
            .unwrap();
        assert_eq!(stable.attempt, 1);
    }

    #[test]
    fn clock_and_pipeline_diagnostics_are_shared_with_platform_hosts() {
        let mut clock = ClockSync::new();
        let first = clock.probe(1_000_000_000).unwrap();
        assert!(clock.consume(first, 6_005_000_000, 6_006_000_000, 1_011_000_000));
        let second = clock.probe(2_011_000_000).unwrap();
        assert!(clock.consume(second, 7_015_000_000, 7_016_000_000, 2_023_000_000));
        assert!(clock.synchronized());
        assert_eq!(clock.rtt_ns(), Some(10_000_000));
        assert_eq!(
            clock.server_to_client_ns(8_000_000_000),
            Some(3_000_000_000)
        );

        let mut diagnostics = PipelineDiagnostics::new();
        assert!(
            diagnostics
                .update(DiagnosticsSample {
                    now_ns: 1_000_000_000,
                    connected: true,
                    ..DiagnosticsSample::default()
                })
                .is_none()
        );
        let report = diagnostics
            .update(DiagnosticsSample {
                now_ns: 3_100_000_000,
                video_packets: 126,
                decoded_frames: 126,
                presented_frames: 125,
                decode_us: 3_000,
                frame_age_us: 18_000,
                connected: true,
                ..DiagnosticsSample::default()
            })
            .unwrap();
        assert_eq!(report.video_fps, 60.0);
        assert_eq!(report.health, DiagnosticsHealth::Smooth);
    }

    #[test]
    fn media_recovery_policy_is_shared_with_platform_hosts() {
        let mut policy = MediaAdmissionPolicy::new();
        assert_eq!(
            policy.admit(1, 1, false).unwrap().action,
            MediaAction::DropRequestKeyframe
        );
        assert_eq!(
            policy.admit(1, 2, true).unwrap().action,
            MediaAction::ResetAndDecode
        );
        assert_eq!(
            policy.admit(1, 3, false).unwrap().action,
            MediaAction::Decode
        );
        let gap = policy.admit(1, 5, false).unwrap();
        assert!(gap.sequence_gap);
        assert_eq!(gap.action, MediaAction::DropRequestKeyframe);
        let generation = policy.admit(2, 1, true).unwrap();
        assert!(generation.generation_changed);
        assert_eq!(generation.action, MediaAction::ResetAndDecode);
    }

    #[test]
    fn input_ordering_and_normalization_are_shared_with_platform_hosts() {
        let mut input = InputState::new();
        input.set_surface(7);
        let first = input.pointer((50.0, 25.0), (100.0, 100.0), 100).unwrap();
        assert_eq!(first.sequence, 1);
        assert_eq!(first.interaction_id, 1);
        assert_eq!((first.x, first.y), (0.5, 0.25));
        let wheel = input
            .wheel((20.0, 60.0), (-10.0, 30.0), (200.0, 100.0), 99)
            .unwrap();
        assert_eq!(wheel.sequence, 2);
        assert_eq!(wheel.client_ns, 101);
        assert_eq!((wheel.delta_x, wheel.delta_y), (-0.05, 0.3));
        input.set_surface(8);
        assert_eq!(
            input.pointer((0.0, 0.0), (1.0, 1.0), 102).unwrap().sequence,
            1
        );
        assert_eq!(input.causal(102).unwrap().client_ns, 103);
    }
}
