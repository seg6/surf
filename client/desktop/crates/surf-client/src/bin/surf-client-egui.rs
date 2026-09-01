use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::{Duration, Instant};

use eframe::egui::{
    self, Align, Color32, CornerRadius, FontData, FontDefinitions, FontFamily, FontId, Frame,
    Layout, Margin, RichText, Sense, Stroke, TextEdit, Vec2,
};
use surf_core::{
    ClockSync, Core, DiagnosticsReport, DiagnosticsSample, Effect, Event as CoreEvent,
    PipelineDiagnostics, SemanticCompletion, Snapshot, Tab, monotonic_ns,
};
use surf_media::{MediaEvent, MediaPipeline};
use surf_protocol::{Causal, Command, Event as WireEvent};
use surf_session::{
    DiscoveredServer, FailureKind, PairingStatus, SavedServer, ServerInfo, SessionAction,
    SessionClient, SessionEvent, Storage,
};

const LUCIDE: &[u8] = include_bytes!("../../../../../client/ios/Resources/Lucide.ttf");
const APP_ICON: &[u8] = include_bytes!("../../../../../backend/cmd/surf/surf-icon.png");
const ICON_FONT: &str = "surf-lucide";

#[path = "../browser_ui.rs"]
mod browser_ui;
#[path = "../theme.rs"]
mod theme;
#[path = "../video_surface.rs"]
mod video_surface;

use video_surface::VideoSurface;

#[path = "../page_input.rs"]
mod page_input;

use browser_ui::{BrowserUi, DialogPrompt, ReaderDocument, reader_text};
use page_input::PageInput;

mod icon {
    pub const BACK: char = '\u{e06e}';
    pub const FORWARD: char = '\u{e06f}';
    pub const RELOAD: char = '\u{e145}';
    pub const STOP: char = '\u{e167}';
    pub const CLOSE: char = '\u{e1b2}';
    pub const PLUS: char = '\u{e13d}';
    pub const MORE: char = '\u{e0b6}';
    pub const LOCK: char = '\u{e531}';
    pub const WARNING: char = '\u{e193}';
    pub const STAR: char = '\u{e176}';
    pub const BOOK: char = '\u{e05f}';
    pub const SEARCH: char = '\u{e151}';
    pub const READER: char = '\u{e348}';
    pub const MEDIA: char = '\u{e080}';
    pub const SETTINGS: char = '\u{e29a}';
    pub const EXPAND: char = '\u{e112}';
    pub const GAUGE: char = '\u{e1bf}';
}

fn main() -> eframe::Result {
    let options = eframe::NativeOptions {
        renderer: eframe::Renderer::Glow,
        viewport: egui::ViewportBuilder::default()
            .with_title("Surf")
            .with_inner_size([1180.0, 760.0])
            .with_min_inner_size([720.0, 480.0])
            .with_app_id("space.seg6.surf.client")
            .with_icon(load_app_icon()),
        ..Default::default()
    };
    eframe::run_native(
        "Surf",
        options,
        Box::new(|creation| Ok(Box::new(SurfDesktop::new(creation)))),
    )
}

fn load_app_icon() -> Arc<egui::IconData> {
    let icon = image::load_from_memory(APP_ICON)
        .expect("bundled Surf icon decodes")
        .into_rgba8();
    let (width, height) = icon.dimensions();
    Arc::new(egui::IconData {
        rgba: icon.into_raw(),
        width,
        height,
    })
}

struct SurfDesktop {
    core: Core,
    snapshot: Snapshot,
    address: String,
    address_focused: bool,
    address_editing: bool,
    session: Option<SessionClient>,
    media: Option<MediaPipeline>,
    video_surface: Arc<VideoSurface>,
    endpoint: String,
    pairing_code: String,
    status: String,
    inspected: Option<ServerInfo>,
    paired: bool,
    pairing: Option<PairingStatus>,
    saved_servers: Vec<SavedServer>,
    discovered_servers: Vec<DiscoveredServer>,
    discovery_note: Option<String>,
    connected: bool,
    frames_received: u64,
    core_presented_count: u64,
    last_frame: String,
    remote_viewport: Option<(i32, i32)>,
    connect_after_inspect: bool,
    startup_url: Option<String>,
    startup_navigation_sent: bool,
    smoke_frame_target: Option<u64>,
    smoke_started: Option<Instant>,
    smoke_last_heartbeat: Instant,
    smoke_reported: bool,
    smoke_interaction: bool,
    smoke_interaction_step: u8,
    smoke_stall_ms: Option<u64>,
    smoke_stalled: bool,
    audio_available: bool,
    clock_available: bool,
    media_stats_available: bool,
    clock_sync: ClockSync,
    pipeline_diagnostics: PipelineDiagnostics,
    latest_diagnostics: Option<DiagnosticsReport>,
    page_input: PageInput,
    dark_mode: bool,
    mobile_mode: bool,
    browser: BrowserUi,
    forget_server: Option<SavedServer>,
}

impl SurfDesktop {
    fn new(creation: &eframe::CreationContext<'_>) -> Self {
        install_fonts(&creation.egui_ctx);
        theme::apply(&creation.egui_ctx, true);
        let mut core = Core::new().expect("portable Surf core initializes");
        core.dispatch(&CoreEvent::Tabs(vec![Tab {
            id: 1,
            title: "New Tab".to_owned(),
            url: "about:blank#surf-new".to_owned(),
            icon: String::new(),
            active: true,
        }]))
        .expect("initial desktop tab is valid");
        core.dispatch(&CoreEvent::Url {
            url: "about:blank#surf-new".to_owned(),
            security: String::new(),
            starred: false,
        })
        .expect("initial desktop URL is valid");
        let snapshot = core.snapshot().expect("initial snapshot is valid");
        let video_surface = VideoSurface::new();
        let (session, media, saved_servers, status) = match Storage::system() {
            Ok(storage) => {
                let servers = storage.servers().unwrap_or_default();
                match MediaPipeline::spawn() {
                    Ok(media) => {
                        match SessionClient::spawn_with_frame_sink(storage, media.frame_sink()) {
                            Ok(session) => (
                                Some(session),
                                Some(media),
                                servers,
                                "Choose or pair a Surf server".to_owned(),
                            ),
                            Err(error) => (None, Some(media), servers, error.to_string()),
                        }
                    }
                    Err(error) => (None, None, servers, error.to_string()),
                }
            }
            Err(error) => (None, None, Vec::new(), error.to_string()),
        };
        let startup_endpoint =
            (saved_servers.len() == 1).then(|| saved_servers[0].endpoint.clone());
        let startup_url = std::env::args()
            .skip(1)
            .find(|argument| !argument.starts_with('-'));
        let smoke_frame_target = std::env::var("SURF_SMOKE_EXIT_AFTER_FRAMES")
            .ok()
            .and_then(|value| value.parse::<u64>().ok())
            .filter(|value| *value > 0)
            .or_else(|| {
                std::env::var_os("SURF_SMOKE_EXIT_AFTER_FRAME")
                    .is_some()
                    .then_some(1)
            });
        let smoke_stall_ms = std::env::var("SURF_SMOKE_STALL_MS")
            .ok()
            .and_then(|value| value.parse::<u64>().ok())
            .filter(|value| *value > 0);
        let mut client = Self {
            core,
            snapshot,
            address: String::new(),
            address_focused: false,
            address_editing: false,
            session,
            media,
            video_surface,
            endpoint: "127.0.0.1:18080".to_owned(),
            pairing_code: String::new(),
            status,
            inspected: None,
            paired: false,
            pairing: None,
            saved_servers,
            discovered_servers: Vec::new(),
            discovery_note: None,
            connected: false,
            frames_received: 0,
            core_presented_count: 0,
            last_frame: String::new(),
            remote_viewport: None,
            connect_after_inspect: false,
            startup_url,
            startup_navigation_sent: false,
            smoke_frame_target,
            smoke_started: None,
            smoke_last_heartbeat: Instant::now(),
            smoke_reported: false,
            smoke_interaction: std::env::var_os("SURF_SMOKE_INTERACTION").is_some(),
            smoke_interaction_step: 0,
            smoke_stall_ms,
            smoke_stalled: false,
            audio_available: false,
            clock_available: false,
            media_stats_available: false,
            clock_sync: ClockSync::new(),
            pipeline_diagnostics: PipelineDiagnostics::new(),
            latest_diagnostics: None,
            page_input: PageInput::new(),
            dark_mode: true,
            mobile_mode: false,
            browser: preview_browser_ui(),
            forget_server: None,
        };
        if let Some(endpoint) = startup_endpoint {
            client.inspect(endpoint, true);
        }
        client
    }

    fn refresh(&mut self) {
        self.snapshot = self.core.snapshot().expect("core snapshot stays valid");
    }

    fn acknowledge_presented_frame(&mut self) {
        let presented = self.video_surface.presented_frame();
        if presented.count == self.core_presented_count {
            return;
        }
        self.core_presented_count = presented.count;
        if let Err(error) = self
            .core
            .present_frame(presented.generation, presented.source_sequence)
        {
            self.status = format!("portable core rejected presented frame: {error}");
        }
        self.refresh();
    }

    fn clear_page_presentation(&mut self) {
        if self.browser.dialog.is_some() {
            self.send_command(Command::DialogReply {
                accept: false,
                text: String::new(),
                causal: Causal::default(),
            });
        }
        if let Some(request_id) = self.browser.select.as_ref().map(|select| select.id.clone()) {
            self.send_command(Command::SelectReply {
                id: request_id,
                cancel: true,
                indices: Vec::new(),
                causal: Causal::default(),
            });
        }
        if self.browser.upload_multiple.is_some() {
            self.send(SessionAction::CancelUpload);
        }
        self.browser.reset_page();
    }

    fn send(&mut self, action: SessionAction) {
        if let Some(session) = &self.session
            && let Err(error) = session.send(action)
        {
            self.status = error.to_string();
        }
    }

    fn send_command(&mut self, command: Command) {
        if self.connected {
            self.send(SessionAction::Send(command));
        } else {
            self.status = "Connect to a Surf server first".to_owned();
        }
    }

    fn inspect(&mut self, endpoint: String, connect_when_paired: bool) {
        self.endpoint = endpoint;
        self.inspected = None;
        self.pairing = None;
        self.paired = false;
        self.connect_after_inspect = connect_when_paired;
        self.send(SessionAction::Inspect(self.endpoint.clone()));
    }

    fn drain_session(&mut self) {
        while let Some(event) = self.session.as_ref().and_then(SessionClient::try_recv) {
            match event {
                SessionEvent::Status { phase, message } => {
                    self.status = format!("{phase}: {message}");
                }
                SessionEvent::Discovered(server) => {
                    let existing = self.discovered_servers.iter().position(|candidate| {
                        (!server.server_id.is_empty() && candidate.server_id == server.server_id)
                            || candidate.endpoint == server.endpoint
                    });
                    match existing {
                        Some(index) => self.discovered_servers[index] = server,
                        None => self.discovered_servers.push(server),
                    }
                    self.discovered_servers
                        .sort_by(|left, right| left.name.cmp(&right.name));
                }
                SessionEvent::DiscoveryUnavailable(message) => {
                    self.discovery_note = Some(format!(
                        "Nearby search is unavailable ({message}); manual addresses still work."
                    ));
                }
                SessionEvent::Inspected {
                    info,
                    endpoint,
                    paired,
                } => {
                    self.endpoint = endpoint;
                    self.status = if paired {
                        format!("{} is verified and already paired", info.name)
                    } else if info.pairing {
                        format!("{} is ready for its six-digit code", info.name)
                    } else {
                        format!("Open pairing on {} to continue", info.name)
                    };
                    self.inspected = Some(info);
                    self.paired = paired;
                    self.pairing = None;
                    if paired && std::mem::take(&mut self.connect_after_inspect) {
                        self.send(SessionAction::Connect);
                    }
                }
                SessionEvent::PairingPhrase(pairing) => {
                    self.status = "Compare these six words, then confirm".to_owned();
                    self.pairing = Some(pairing);
                }
                SessionEvent::Paired(server) => {
                    self.paired = true;
                    self.status = format!("Paired with {}", server.name);
                    self.saved_servers
                        .retain(|saved| saved.server_id != server.server_id);
                    self.saved_servers.push(server);
                    self.saved_servers
                        .sort_by(|left, right| left.name.cmp(&right.name));
                }
                SessionEvent::Connected { info, config } => {
                    if let Err(error) = self.core.begin_connection() {
                        self.status = error.to_string();
                        continue;
                    }
                    self.browser.reset_connection();
                    self.refresh();
                    self.connected = true;
                    self.clock_available =
                        config.caps.iter().any(|capability| capability == "clock");
                    self.media_stats_available = config
                        .caps
                        .iter()
                        .any(|capability| capability == "media-stats");
                    self.page_input.set_native_pointer(
                        config
                            .caps
                            .iter()
                            .any(|capability| capability == "pointer-input"),
                    );
                    self.status = format!("Connected securely to {}", info.name);
                    self.pairing = None;
                    self.remote_viewport = None;
                    self.send(SessionAction::Send(Command::Dark {
                        on: self.dark_mode,
                        causal: Causal::default(),
                    }));
                    self.send(SessionAction::Send(Command::Mobile {
                        on: self.mobile_mode,
                        causal: Causal::default(),
                    }));
                    if self.audio_available {
                        self.send(SessionAction::Send(Command::Audio {
                            on: true,
                            causal: Causal::default(),
                        }));
                    }
                    if !self.startup_navigation_sent
                        && let Some(url) = self.startup_url.clone()
                    {
                        self.startup_navigation_sent = true;
                        self.address.clone_from(&url);
                        self.send(SessionAction::Send(Command::Navigate {
                            url,
                            causal: Causal::default(),
                        }));
                    }
                }
                SessionEvent::Reconnecting {
                    attempt,
                    maximum,
                    delay,
                    reason,
                } => {
                    self.connected = false;
                    self.clock_available = false;
                    self.media_stats_available = false;
                    self.latest_diagnostics = None;
                    self.clock_sync.reset();
                    self.pipeline_diagnostics.reset();
                    self.page_input.reset();
                    if let Some(media) = &self.media {
                        media.set_clock_offset(None);
                    }
                    self.video_surface.clear();
                    self.status = format!(
                        "Connection interrupted. Retrying {attempt}/{maximum} in {:.1}s — {reason}",
                        delay.as_secs_f32()
                    );
                }
                SessionEvent::Control(event) => self.apply_control(event),
                SessionEvent::Transfer {
                    kind,
                    name,
                    ok,
                    message,
                } => {
                    let operation = match kind {
                        surf_session::TransferKind::Upload => "Upload",
                        surf_session::TransferKind::Download => "Download",
                    };
                    self.status = if ok {
                        format!("{operation} finished: {name}")
                    } else {
                        format!("{operation} failed for {name}: {message}")
                    };
                    if kind == surf_session::TransferKind::Download {
                        self.browser.pending_downloads.retain(|item| item != &name);
                    }
                    self.browser.toast(self.status.clone());
                }
                SessionEvent::Disconnected(message) => {
                    self.connected = false;
                    self.clock_available = false;
                    self.media_stats_available = false;
                    self.latest_diagnostics = None;
                    self.clock_sync.reset();
                    self.pipeline_diagnostics.reset();
                    self.page_input.reset();
                    if let Some(media) = &self.media {
                        media.set_clock_offset(None);
                    }
                    self.video_surface.clear();
                    self.remote_viewport = None;
                    self.status = message;
                    let _ = self.core.dispatch(&CoreEvent::Loading(false));
                    self.refresh();
                }
                SessionEvent::Failure(failure) => {
                    self.connected = false;
                    self.clock_available = false;
                    self.media_stats_available = false;
                    self.latest_diagnostics = None;
                    self.clock_sync.reset();
                    self.pipeline_diagnostics.reset();
                    self.page_input.reset();
                    if let Some(media) = &self.media {
                        media.set_clock_offset(None);
                    }
                    self.video_surface.clear();
                    self.remote_viewport = None;
                    let heading = match failure.kind {
                        FailureKind::Endpoint => "Server address",
                        FailureKind::Trust => "Server identity",
                        FailureKind::Identity => "Device key",
                        FailureKind::Pairing => "Pairing",
                        FailureKind::Authentication => "Authentication",
                        FailureKind::Compatibility => "Compatibility",
                        FailureKind::Storage => "Saved data",
                        FailureKind::Protocol => "Protocol",
                        FailureKind::Transport => "Connection",
                        FailureKind::Internal => "Surf",
                    };
                    self.status = format!("{heading}: {}", failure.message);
                }
            }
        }
        while let Some(event) = self.media.as_ref().and_then(MediaPipeline::try_recv_event) {
            match event {
                MediaEvent::RequestKeyframe => self.send_command(Command::RequestKeyframe {
                    causal: Causal::default(),
                }),
                MediaEvent::DecoderError(message) => {
                    self.status = format!("Video decoder: {message}");
                }
                MediaEvent::AudioReady { .. } => {
                    self.audio_available = true;
                    if self.connected {
                        self.send(SessionAction::Send(Command::Audio {
                            on: true,
                            causal: Causal::default(),
                        }));
                    }
                }
                MediaEvent::AudioUnavailable(_) => {
                    self.audio_available = false;
                }
                MediaEvent::AudioError(message) => {
                    self.status = format!("Audio output: {message}");
                }
            }
        }
        if let Some(frame) = self
            .media
            .as_ref()
            .and_then(MediaPipeline::take_latest_frame)
        {
            self.frames_received = self.frames_received.saturating_add(1);
            self.last_frame = format!(
                "{}×{} · generation {} · frame {} · decode {:.2} ms",
                frame.width,
                frame.height,
                frame.generation,
                frame.sequence,
                frame.decode_time.as_secs_f64() * 1_000.0,
            );
            self.video_surface.submit(frame);
        }
        if let Some(error) = self.video_surface.take_error() {
            self.status = format!("Video renderer: {error}");
        }
    }

    fn apply_control(&mut self, event: WireEvent) {
        let effects = match self.core.dispatch_wire_event(&event) {
            Ok(effects) => effects,
            Err(error) => {
                self.status = format!("Rejected control event: {error}");
                return;
            }
        };
        for effect in effects {
            match effect {
                Effect::RequestLibrary => self.send_command(Command::Library {
                    causal: Causal::default(),
                }),
                Effect::ClearPagePresentation => self.clear_page_presentation(),
                Effect::ShowKeyboard | Effect::HideKeyboard | Effect::Unknown(_) => {}
            }
        }
        match event {
            WireEvent::Clock { c0, s1, s2 } => {
                if self.clock_sync.consume(c0, s1, s2, monotonic_ns())
                    && let Some(media) = &self.media
                {
                    media.set_clock_offset(self.clock_sync.server_minus_client_ns());
                }
            }
            WireEvent::Url { url, .. } => {
                if !self.address_focused {
                    self.address.clone_from(&url);
                }
            }
            WireEvent::Hello { vw, vh } => {
                self.remote_viewport = Some((vw, vh));
            }
            WireEvent::VideoConfig { state, reason, .. } => {
                if state != "ready" && !reason.is_empty() {
                    self.status = format!("Video: {reason}");
                }
            }
            WireEvent::AudioConfig { ok, .. } => {
                self.audio_available = ok;
            }
            WireEvent::Found { on } => {
                self.browser.find_found = Some(on);
            }
            WireEvent::Toast { text } => {
                self.browser.toast(text);
            }
            WireEvent::Download { name } => {
                let destination = default_download_path(&name);
                self.send(SessionAction::Download {
                    name: name.clone(),
                    destination,
                });
                self.browser.pending_downloads.push(name);
            }
            WireEvent::DownloadProgress { name, pct } => {
                self.browser.download_progress.insert(name, pct);
            }
            WireEvent::Suggest { items } => {
                self.browser.suggestions = items;
            }
            WireEvent::Library {
                hist, bookmarks, ..
            } => {
                self.browser.history = hist;
                self.browser.bookmarks = bookmarks;
            }
            WireEvent::History { items, .. } => {
                self.browser.history = items;
            }
            WireEvent::Downloads { items } => {
                self.browser.downloads = items;
            }
            WireEvent::Dialog {
                kind,
                text,
                default,
            } => {
                self.browser.dialog = Some(DialogPrompt {
                    kind,
                    text,
                    input: default,
                });
            }
            WireEvent::DialogDone => {
                self.browser.dialog = None;
            }
            WireEvent::FileChooser { multiple } => {
                self.browser.upload_multiple = Some(multiple);
                self.browser.upload_paths.clear();
            }
            WireEvent::PageError { url, .. } => {
                self.browser.page_error = Some(url.clone());
                if !self.address_focused {
                    self.address.clone_from(&url);
                }
            }
            WireEvent::Reader {
                ok,
                title,
                html,
                url,
            } => {
                if ok {
                    self.browser.reader = Some(ReaderDocument {
                        title,
                        url,
                        text: reader_text(&html),
                    });
                    self.browser.reader_open = true;
                } else {
                    self.browser
                        .toast("Reader mode is not available for this page");
                }
            }
            WireEvent::Select {
                id,
                title,
                multiple,
                options,
                ..
            } => {
                self.browser.open_select(id, title, multiple, options);
            }
            WireEvent::MediaState {
                available,
                count,
                paused,
                muted,
                volume,
                current_time,
                duration,
                title,
            } => {
                self.browser.media = browser_ui::MediaState {
                    available,
                    count,
                    paused,
                    muted,
                    volume,
                    current_time,
                    duration,
                    title,
                };
            }
            WireEvent::Clipboard { id, text, .. } => {
                self.browser.pending_clipboard = Some((id, text));
            }
            WireEvent::ClipboardSync {
                enabled,
                known,
                text,
            } => {
                self.browser.clipboard_sync = enabled;
                self.browser.clipboard_known = known;
                self.browser.clipboard_text.clone_from(&text);
                if enabled && known {
                    self.browser.pending_clipboard = Some((String::new(), text));
                }
            }
            WireEvent::LogRequest => {
                self.send_command(Command::LogRecord {
                    record: serde_json::json!({
                        "source": "desktop",
                        "level": "info",
                        "message": "Surf Desktop is connected",
                        "version": include_str!("../../../../../VERSION").trim(),
                    }),
                    causal: Causal::default(),
                });
            }
            WireEvent::LogClear => {
                self.send_command(Command::LogCleared {
                    causal: Causal::default(),
                });
            }
            WireEvent::Tabs { .. }
            | WireEvent::HistoryState { .. }
            | WireEvent::Loading { .. }
            | WireEvent::Editable { .. }
            | WireEvent::Fullscreen { .. }
            | WireEvent::Security { .. }
            | WireEvent::Starred { .. }
            | WireEvent::PageFrame { .. } => {}
        }
        self.refresh();
    }

    fn update_diagnostics(&mut self) {
        let now_ns = monotonic_ns();
        if self.connected
            && self.clock_available
            && let Some(client_send_ns) = self.clock_sync.probe(now_ns)
        {
            self.send(SessionAction::Send(Command::Clock {
                c0: client_send_ns,
                causal: Causal::default(),
            }));
        }
        let media = self
            .media
            .as_ref()
            .map(MediaPipeline::diagnostics)
            .unwrap_or_default();
        let surface = self.video_surface.diagnostics();
        let sample = DiagnosticsSample {
            now_ns,
            video_packets: media.video_packets,
            decoded_frames: media.decoded_frames,
            presented_frames: surface.presented,
            ingress_replaced: media.ingress_replaced,
            output_replaced: media.output_replaced,
            presentation_replaced: surface.replaced,
            sequence_gaps: media.gaps,
            decode_errors: media.decode_errors,
            audio_underruns: media.audio_underruns,
            backend_capture_to_encode_us: media.latest_backend_capture_to_encode_us,
            backend_encode_to_write_us: media.latest_backend_encode_to_write_us,
            network_us: media.latest_network_us,
            decode_us: media.latest_decode_us,
            upload_us: surface.latest_upload_us,
            frame_age_us: surface.latest_frame_age_us,
            rtt_us: self.clock_sync.rtt_ns().unwrap_or(0) / 1_000,
            clock_uncertainty_us: self.clock_sync.rtt_ns().unwrap_or(0) / 2_000,
            maximum_presentation_gap_us: surface.latest_presentation_gap_us,
            encoded_video_depth: media.encoded_video_depth,
            decoded_video_depth: media.decoded_video_depth,
            audio_depth: media.audio_depth,
            timing_synchronized: self.clock_sync.synchronized(),
            connected: self.connected,
        };
        let Some(report) = self.pipeline_diagnostics.update(sample) else {
            return;
        };
        self.latest_diagnostics = Some(report);
        if self.connected && self.media_stats_available {
            self.send(SessionAction::Send(Command::MediaStats {
                fps: report.presentation_fps,
                presented_fps: report.presentation_fps,
                decode_fps: report.decode_fps,
                au_rate: report.video_fps,
                renderer: "glow-yuv".to_owned(),
                renderer_fps: report.presentation_fps,
                renderer_ms: report.upload_us as f64 / 1_000.0,
                renderer_backpressure: bounded_i32(report.dropped_frames),
                renderer_recoveries: 0,
                renderer_failures: 0,
                callback_ms: report.decode_us as f64 / 1_000.0,
                gap_ms: report.maximum_presentation_gap_us as f64 / 1_000.0,
                frame_age_ms: report.frame_age_us as f64 / 1_000.0,
                window_ms: report.window_ms,
                drop_pct: report.drop_percent,
                queue: bounded_i32(
                    u64::from(report.encoded_video_depth) + u64::from(report.decoded_video_depth),
                ),
                decode_errors: bounded_i32(report.decode_errors),
                memory_warn: false,
                causal: Causal::default(),
            }));
        }
    }

    fn chrome(&mut self, root: &mut egui::Ui) {
        let colors = theme::palette(self.dark_mode);
        egui::Panel::top("browser_chrome")
            .frame(
                Frame::new()
                    .fill(colors.chrome)
                    .inner_margin(Margin::same(0))
                    .stroke(Stroke::new(1.0, colors.separator)),
            )
            .show(root, |ui| {
                self.tab_runway(ui);
                ui.separator();
                Frame::new()
                    .fill(colors.command_rail)
                    .inner_margin(Margin::symmetric(12, 6))
                    .show(ui, |ui| self.command_rail(ui));
                if self.snapshot.loading {
                    let rail = ui.max_rect();
                    let width = (rail.width() * 0.24).clamp(90.0, 260.0);
                    let travel = (rail.width() - width).max(1.0);
                    let x = rail.left()
                        + (ui.input(|input| input.time) * 210.0 % f64::from(travel)) as f32;
                    ui.painter().rect_filled(
                        egui::Rect::from_min_size(
                            egui::pos2(x, rail.bottom() - 2.0),
                            egui::vec2(width, 2.0),
                        ),
                        CornerRadius::ZERO,
                        colors.accent,
                    );
                }
            });
    }

    fn tab_runway(&mut self, ui: &mut egui::Ui) {
        ui.set_height(36.0);
        ui.horizontal(|ui| {
            ui.spacing_mut().item_spacing.x = 2.0;
            let available = (ui.available_width() - 42.0).max(120.0);
            let count = self.snapshot.tabs.len().max(1) as f32;
            let tab_width = (available / count).clamp(132.0, 228.0);
            egui::ScrollArea::horizontal()
                .id_salt("tab_runway")
                .scroll_bar_visibility(egui::scroll_area::ScrollBarVisibility::AlwaysHidden)
                .max_width(available)
                .show(ui, |ui| {
                    ui.horizontal(|ui| {
                        let tabs = self.snapshot.tabs.clone();
                        for tab in tabs {
                            self.tab(ui, &tab, tab_width);
                        }
                    });
                });
            if chrome_icon(ui, icon::PLUS, self.connected, "New tab (Ctrl+T)") {
                self.new_tab();
            }
        });
    }

    fn command_rail(&mut self, ui: &mut egui::Ui) {
        let colors = theme::palette(self.dark_mode);
        ui.set_height(34.0);
        ui.horizontal(|ui| {
            ui.spacing_mut().item_spacing.x = 4.0;
            if chrome_icon(ui, icon::BACK, self.snapshot.can_go_back, "Back (Alt+Left)") {
                self.send_command(Command::Back {
                    causal: Causal::default(),
                });
            }
            if chrome_icon(
                ui,
                icon::FORWARD,
                self.snapshot.can_go_forward,
                "Forward (Alt+Right)",
            ) {
                self.send_command(Command::Forward {
                    causal: Causal::default(),
                });
            }
            let reload_icon = if self.snapshot.loading {
                icon::STOP
            } else {
                icon::RELOAD
            };
            if chrome_icon(
                ui,
                reload_icon,
                self.connected,
                if self.snapshot.loading {
                    "Stop (Esc)"
                } else {
                    "Reload (Ctrl+R)"
                },
            ) {
                self.reload_or_stop();
            }
            ui.add_space(4.0);

            let address_width = (ui.available_width() - 86.0).max(220.0);
            let focused = self.address_editing || self.address_focused;
            Frame::new()
                .fill(colors.field)
                .corner_radius(CornerRadius::same(9))
                .stroke(Stroke::new(
                    1.0,
                    if focused {
                        colors.accent
                    } else {
                        colors.field_border
                    },
                ))
                .inner_margin(Margin::symmetric(11, 4))
                .show(ui, |ui| {
                    ui.set_width(address_width);
                    ui.horizontal(|ui| {
                        let security_icon = if self.snapshot.security == "dangerous" {
                            icon::WARNING
                        } else {
                            icon::LOCK
                        };
                        let security_color = if self.snapshot.security == "dangerous" {
                            colors.danger
                        } else if self.snapshot.current_url.starts_with("https://") {
                            colors.accent_text
                        } else {
                            colors.muted
                        };
                        ui.label(
                            RichText::new(security_icon.to_string())
                                .family(icon_family())
                                .size(14.0)
                                .color(security_color),
                        );
                        if self.address_editing {
                            let response = ui.add_sized(
                                [ui.available_width(), 24.0],
                                TextEdit::singleline(&mut self.address)
                                    .id_source("address")
                                    .hint_text("Search or enter address")
                                    .font(FontId::proportional(14.5))
                                    .frame(Frame::NONE),
                            );
                            self.address_focused = response.has_focus();
                            if response.changed() {
                                self.send_command(Command::Suggest {
                                    q: self.address.clone(),
                                    offset: 0,
                                    causal: Causal::default(),
                                });
                            }
                            let enter = ui.input(|input| input.key_pressed(egui::Key::Enter));
                            let escape = ui.input(|input| input.key_pressed(egui::Key::Escape));
                            if enter {
                                self.navigate();
                                self.address_editing = false;
                                response.surrender_focus();
                            } else if escape {
                                self.address.clone_from(&self.snapshot.current_url);
                                self.address_editing = false;
                                response.surrender_focus();
                            } else if response.lost_focus() {
                                self.address_editing = false;
                            }
                        } else {
                            let display = compact_address(&self.snapshot.current_url);
                            let response = ui.add_sized(
                                [ui.available_width(), 24.0],
                                egui::Button::new(
                                    RichText::new(display).size(14.0).color(colors.text),
                                )
                                .frame(false),
                            );
                            if response.clicked() {
                                self.address.clone_from(&self.snapshot.current_url);
                                self.address_editing = true;
                                ui.ctx().memory_mut(|memory| {
                                    memory.request_focus(egui::Id::new("address"));
                                });
                            }
                        }
                    });
                });
            ui.add_space(4.0);
            if chrome_icon(
                ui,
                icon::STAR,
                self.connected,
                if self.snapshot.starred {
                    "Remove bookmark (Ctrl+D)"
                } else {
                    "Bookmark (Ctrl+D)"
                },
            ) {
                self.send_command(Command::Bookmark {
                    causal: Causal::default(),
                });
            }
            if chrome_icon(ui, icon::MORE, true, "Browser tools") {
                self.browser.menu_open = !self.browser.menu_open;
            }
        });
    }

    fn tab(&mut self, ui: &mut egui::Ui, tab: &Tab, width: f32) {
        let colors = theme::palette(self.dark_mode);
        let fill = if tab.active {
            colors.tab_active
        } else {
            colors.chrome
        };
        let inner = Frame::new()
            .fill(fill)
            .corner_radius(CornerRadius::same(6))
            .stroke(Stroke::new(
                1.0,
                if tab.active {
                    colors.tab_border
                } else {
                    Color32::TRANSPARENT
                },
            ))
            .inner_margin(Margin::symmetric(9, 3))
            .show(ui, |ui| {
                ui.set_width(width);
                ui.horizontal(|ui| {
                    ui.spacing_mut().item_spacing.x = 5.0;
                    let title = if tab.title.trim().is_empty() {
                        compact_address(&tab.url)
                    } else {
                        tab.title.clone()
                    };
                    ui.add_sized(
                        [(ui.available_width() - 24.0).max(50.0), 24.0],
                        egui::Label::new(RichText::new(title).size(12.5).color(if tab.active {
                            colors.text
                        } else {
                            colors.muted
                        }))
                        .truncate(),
                    );
                    let close = ui
                        .add(
                            egui::Button::new(
                                RichText::new(icon::CLOSE.to_string())
                                    .family(icon_family())
                                    .size(11.0)
                                    .color(colors.muted),
                            )
                            .frame(false)
                            .min_size(Vec2::splat(22.0)),
                        )
                        .on_hover_text("Close tab (Ctrl+W)");
                    close.clicked()
                })
            });
        let close_clicked = inner.inner.inner;
        let response = inner.response.interact(Sense::click());
        if close_clicked {
            self.close_tab(tab.id);
            return;
        }
        if response.clicked() && !tab.active {
            self.send_command(Command::Tab {
                action: "select".to_owned(),
                id: i32::try_from(tab.id).unwrap_or_default(),
                causal: Causal::default(),
            });
            let next = self
                .snapshot
                .tabs
                .iter()
                .cloned()
                .map(|mut item| {
                    item.active = item.id == tab.id;
                    item
                })
                .collect();
            let _ = self.core.dispatch(&CoreEvent::Tabs(next));
            self.refresh();
        }
    }

    fn new_tab(&mut self) {
        self.send_command(Command::Tab {
            action: "new".to_owned(),
            id: 0,
            causal: Causal::default(),
        });
    }

    fn close_tab(&mut self, id: i64) {
        self.send_command(Command::Tab {
            action: "close".to_owned(),
            id: i32::try_from(id).unwrap_or_default(),
            causal: Causal::default(),
        });
    }

    fn reload_or_stop(&mut self) {
        self.send_command(if self.snapshot.loading {
            Command::Stop {
                causal: Causal::default(),
            }
        } else {
            Command::Reload {
                causal: Causal::default(),
            }
        });
    }

    fn navigate(&mut self) {
        let value = self.address.trim();
        if value.is_empty() {
            return;
        }
        let url = if value.contains("://") {
            value.to_owned()
        } else if value.contains('.') && !value.contains(' ') {
            format!("https://{value}")
        } else {
            format!(
                "https://www.google.com/search?q={}",
                value.replace(' ', "+")
            )
        };
        self.send_command(Command::Navigate {
            url: url.clone(),
            causal: Causal::default(),
        });
        self.address = url;
    }

    fn handle_shortcuts(&mut self, context: &egui::Context) {
        let input = context.input(|input| {
            (
                input.modifiers.command,
                input.modifiers.shift,
                input.modifiers.alt,
                input.key_pressed(egui::Key::L),
                input.key_pressed(egui::Key::T),
                input.key_pressed(egui::Key::W),
                input.key_pressed(egui::Key::R),
                input.key_pressed(egui::Key::F5),
                input.key_pressed(egui::Key::D),
                input.key_pressed(egui::Key::F),
                input.key_pressed(egui::Key::ArrowLeft),
                input.key_pressed(egui::Key::ArrowRight),
                input.key_pressed(egui::Key::Escape),
            )
        });
        let (
            command,
            _shift,
            alt,
            key_l,
            key_t,
            key_w,
            key_r,
            key_f5,
            key_d,
            key_f,
            left,
            right,
            escape,
        ) = input;
        if command && key_l {
            self.address.clone_from(&self.snapshot.current_url);
            self.address_editing = true;
            context.memory_mut(|memory| memory.request_focus(egui::Id::new("address")));
        }
        if command && key_t {
            self.new_tab();
        }
        if command
            && key_w
            && let Some(id) = self.snapshot.active_tab_id
        {
            self.close_tab(id);
        }
        if (command && key_r) || key_f5 {
            self.reload_or_stop();
        }
        if command && key_d {
            self.send_command(Command::Bookmark {
                causal: Causal::default(),
            });
        }
        if command && key_f {
            self.browser.find_open = true;
            context.memory_mut(|memory| memory.request_focus(egui::Id::new("find_query")));
        }
        if alt && left && self.snapshot.can_go_back {
            self.send_command(Command::Back {
                causal: Causal::default(),
            });
        }
        if alt && right && self.snapshot.can_go_forward {
            self.send_command(Command::Forward {
                causal: Causal::default(),
            });
        }
        if escape && self.browser.close_transient_overlays() {
            return;
        }
        if escape && self.snapshot.loading && !self.address_editing {
            self.send_command(Command::Stop {
                causal: Causal::default(),
            });
        }
    }

    fn browser_overlays(&mut self, context: &egui::Context) {
        self.tools_menu(context);
        self.find_overlay(context);
        self.suggestions_overlay(context);
        self.library_window(context);
        self.reader_window(context);
        self.media_window(context);
        self.settings_window(context);
        self.forget_server_window(context);
        self.diagnostics_window(context);
        self.dialog_window(context);
        self.select_window(context);
        self.upload_window(context);
        self.page_error_overlay(context);
        self.toast_overlay(context);
    }

    fn tools_menu(&mut self, context: &egui::Context) {
        if !self.browser.menu_open {
            return;
        }
        let mut open = true;
        let mut action = None;
        egui::Window::new("Browser tools")
            .id(egui::Id::new("browser_tools"))
            .anchor(egui::Align2::RIGHT_TOP, [-12.0, 82.0])
            .open(&mut open)
            .title_bar(false)
            .collapsible(false)
            .resizable(false)
            .default_width(248.0)
            .show(context, |ui| {
                ui.spacing_mut().item_spacing.y = 3.0;
                for (glyph, title, value) in [
                    (icon::BOOK, "Library", "library"),
                    (icon::READER, "Reader", "reader"),
                    (icon::SEARCH, "Find on page", "find"),
                    (icon::MEDIA, "Media controls", "media"),
                    (icon::EXPAND, "Fullscreen", "fullscreen"),
                    (icon::GAUGE, "Performance", "diagnostics"),
                    (icon::SETTINGS, "Settings", "settings"),
                ] {
                    if tool_row(ui, glyph, title) {
                        action = Some(value);
                    }
                }
            });
        if action.is_some() {
            open = false;
        }
        self.browser.menu_open = open;
        match action {
            Some("library") => {
                self.browser.library_open = true;
                self.send_command(Command::Library {
                    causal: Causal::default(),
                });
                self.send_command(Command::Downloads {
                    causal: Causal::default(),
                });
            }
            Some("reader") => {
                self.browser.toast("Preparing reader…");
                self.send_command(Command::Reader {
                    causal: Causal::default(),
                });
            }
            Some("find") => {
                self.browser.find_open = true;
                context.memory_mut(|memory| memory.request_focus(egui::Id::new("find_query")));
            }
            Some("media") => {
                self.browser.media_open = true;
                self.send_command(Command::MediaQuery {
                    causal: Causal::default(),
                });
            }
            Some("fullscreen") => self.send_command(Command::Fullscreen {
                on: !self.snapshot.fullscreen,
                causal: Causal::default(),
            }),
            Some("diagnostics") => self.browser.diagnostics_open = true,
            Some("settings") => self.browser.settings_open = true,
            _ => {}
        }
    }

    fn find_overlay(&mut self, context: &egui::Context) {
        if !self.browser.find_open {
            return;
        }
        let colors = theme::palette(self.dark_mode);
        let mut close = false;
        let mut direction = None;
        egui::Area::new(egui::Id::new("find_overlay"))
            .anchor(egui::Align2::RIGHT_TOP, [-14.0, 88.0])
            .order(egui::Order::Foreground)
            .show(context, |ui| {
                Frame::new()
                    .fill(colors.surface)
                    .stroke(Stroke::new(1.0, colors.separator))
                    .corner_radius(CornerRadius::same(9))
                    .inner_margin(Margin::symmetric(10, 7))
                    .show(ui, |ui| {
                        ui.horizontal(|ui| {
                            let response = ui.add_sized(
                                [240.0, 28.0],
                                TextEdit::singleline(&mut self.browser.find_query)
                                    .id_source("find_query")
                                    .hint_text("Find on page"),
                            );
                            if response.changed() {
                                direction = Some(1);
                            }
                            let label = match self.browser.find_found {
                                Some(true) => "Found",
                                Some(false) => "No match",
                                None => "",
                            };
                            ui.label(RichText::new(label).size(11.0).color(colors.muted));
                            if ui.small_button("↑").clicked() {
                                direction = Some(-1);
                            }
                            if ui.small_button("↓").clicked()
                                || (response.has_focus()
                                    && ui.input(|input| input.key_pressed(egui::Key::Enter)))
                            {
                                direction = Some(1);
                            }
                            if ui.small_button("×").clicked() {
                                close = true;
                            }
                        });
                    });
            });
        if let Some(dir) = direction {
            self.send_command(Command::Find {
                q: self.browser.find_query.clone(),
                dir,
                causal: Causal::default(),
            });
        }
        if close {
            self.browser.find_open = false;
        }
    }

    fn suggestions_overlay(&mut self, context: &egui::Context) {
        if !self.address_editing || self.browser.suggestions.is_empty() {
            return;
        }
        let colors = theme::palette(self.dark_mode);
        let mut selected = None;
        egui::Area::new(egui::Id::new("address_suggestions"))
            .anchor(egui::Align2::CENTER_TOP, [0.0, 92.0])
            .order(egui::Order::Foreground)
            .show(context, |ui| {
                Frame::new()
                    .fill(colors.surface)
                    .stroke(Stroke::new(1.0, colors.separator))
                    .corner_radius(CornerRadius::same(9))
                    .inner_margin(Margin::symmetric(8, 7))
                    .show(ui, |ui| {
                        ui.set_width(620.0_f32.min(context.content_rect().width() - 40.0));
                        for item in self.browser.suggestions.iter().take(8) {
                            if ui
                                .add_sized(
                                    [ui.available_width(), 34.0],
                                    egui::Button::new(
                                        RichText::new(if item.title.trim().is_empty() {
                                            compact_address(&item.url)
                                        } else {
                                            item.title.clone()
                                        })
                                        .size(13.0),
                                    )
                                    .frame(false),
                                )
                                .on_hover_text(&item.url)
                                .clicked()
                            {
                                selected = Some(item.url.clone());
                            }
                        }
                    });
            });
        if let Some(url) = selected {
            self.address = url;
            self.navigate();
            self.address_editing = false;
            self.browser.suggestions.clear();
        }
    }

    fn library_window(&mut self, context: &egui::Context) {
        if !self.browser.library_open {
            return;
        }
        let mut open = true;
        let mut navigate = None;
        let mut remove_history = None;
        let mut remove_bookmark = None;
        let mut save_download = None;
        egui::Window::new("Library")
            .id(egui::Id::new("library"))
            .open(&mut open)
            .default_size([520.0, 500.0])
            .min_size([360.0, 300.0])
            .show(context, |ui| {
                ui.horizontal(|ui| {
                    ui.selectable_value(&mut self.browser.library_section, 0, "History");
                    ui.selectable_value(&mut self.browser.library_section, 1, "Bookmarks");
                    ui.selectable_value(&mut self.browser.library_section, 2, "Downloads");
                });
                ui.separator();
                egui::ScrollArea::vertical().show(ui, |ui| match self.browser.library_section {
                    0 => {
                        for item in self.browser.history.clone() {
                            library_row(ui, &item.title, &item.url, |choice| match choice {
                                0 => navigate = Some(item.url.clone()),
                                _ => remove_history = Some((item.url.clone(), item.ts)),
                            });
                        }
                    }
                    1 => {
                        for item in self.browser.bookmarks.clone() {
                            library_row(ui, &item.title, &item.url, |choice| match choice {
                                0 => navigate = Some(item.url.clone()),
                                _ => remove_bookmark = Some(item.url.clone()),
                            });
                        }
                    }
                    _ => {
                        for item in self.browser.downloads.clone() {
                            ui.horizontal(|ui| {
                                ui.vertical(|ui| {
                                    ui.label(RichText::new(&item.name).strong());
                                    let progress = self.browser.download_progress.get(&item.name);
                                    let detail = progress.map_or_else(
                                        || format_bytes(item.size),
                                        |pct| format!("{} · {pct}%", format_bytes(item.size)),
                                    );
                                    ui.small(detail);
                                });
                                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                                    if ui.button("Save").clicked() {
                                        save_download = Some(item.name.clone());
                                    }
                                });
                            });
                            ui.separator();
                        }
                    }
                });
            });
        self.browser.library_open = open;
        if let Some(url) = navigate {
            self.send_command(Command::Navigate {
                url,
                causal: Causal::default(),
            });
        }
        if let Some((url, ts)) = remove_history {
            self.send_command(Command::HistoryDelete {
                url,
                ts,
                causal: Causal::default(),
            });
            self.send_command(Command::Library {
                causal: Causal::default(),
            });
        }
        if let Some(url) = remove_bookmark {
            self.send_command(Command::BookmarkDelete {
                url,
                causal: Causal::default(),
            });
            self.send_command(Command::Library {
                causal: Causal::default(),
            });
        }
        if let Some(name) = save_download {
            self.send(SessionAction::Download {
                destination: default_download_path(&name),
                name,
            });
        }
    }

    fn reader_window(&mut self, context: &egui::Context) {
        if !self.browser.reader_open {
            return;
        }
        let Some(document) = self.browser.reader.clone() else {
            self.browser.reader_open = false;
            return;
        };
        let mut open = true;
        let mut navigate = false;
        egui::Window::new(if document.title.is_empty() {
            "Reader"
        } else {
            &document.title
        })
        .id(egui::Id::new("reader"))
        .open(&mut open)
        .default_size([680.0, 620.0])
        .show(context, |ui| {
            ui.horizontal(|ui| {
                ui.label(RichText::new(compact_address(&document.url)).weak());
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    navigate = ui.button("Open page").clicked();
                });
            });
            ui.separator();
            egui::ScrollArea::vertical().show(ui, |ui| {
                ui.add(
                    egui::Label::new(
                        RichText::new(&document.text)
                            .size(17.0)
                            .line_height(Some(25.0)),
                    )
                    .wrap(),
                );
            });
        });
        self.browser.reader_open = open;
        if navigate {
            self.send_command(Command::Navigate {
                url: document.url,
                causal: Causal::default(),
            });
            self.browser.reader_open = false;
        }
    }

    fn media_window(&mut self, context: &egui::Context) {
        if !self.browser.media_open {
            return;
        }
        let mut open = true;
        let mut play_pause = false;
        let mut mute = false;
        let mut volume = self.browser.media.volume;
        egui::Window::new("Media")
            .id(egui::Id::new("media"))
            .open(&mut open)
            .resizable(false)
            .default_width(340.0)
            .show(context, |ui| {
                if !self.browser.media.available {
                    ui.label("No controllable media on this page.");
                    return;
                }
                let title = if self.browser.media.title.is_empty() {
                    format!("{} media element(s)", self.browser.media.count)
                } else {
                    self.browser.media.title.clone()
                };
                ui.label(RichText::new(title).strong());
                ui.label(format!(
                    "{} / {}",
                    format_time(self.browser.media.current_time),
                    format_time(self.browser.media.duration)
                ));
                ui.horizontal(|ui| {
                    play_pause = ui
                        .button(if self.browser.media.paused {
                            "Play"
                        } else {
                            "Pause"
                        })
                        .clicked();
                    mute = ui
                        .button(if self.browser.media.muted {
                            "Unmute"
                        } else {
                            "Mute"
                        })
                        .clicked();
                });
                ui.add(egui::Slider::new(&mut volume, 0.0..=1.0).text("Volume"));
            });
        self.browser.media_open = open;
        if play_pause {
            self.send_command(Command::MediaPlayPause {
                causal: Causal::default(),
            });
        }
        if mute {
            self.send_command(Command::MediaMute {
                causal: Causal::default(),
            });
        }
        if (volume - self.browser.media.volume).abs() > f64::EPSILON {
            self.browser.media.volume = volume;
            self.send_command(Command::MediaVolume {
                value: volume,
                causal: Causal::default(),
            });
        }
    }

    fn settings_window(&mut self, context: &egui::Context) {
        if !self.browser.settings_open {
            return;
        }
        let mut open = true;
        let mut dark = self.dark_mode;
        let mut mobile = self.mobile_mode;
        let mut disconnect = false;
        let mut clear_history = false;
        let mut forget = None;
        egui::Window::new("Surf settings")
            .id(egui::Id::new("settings"))
            .open(&mut open)
            .collapsible(false)
            .resizable(false)
            .default_width(460.0)
            .show(context, |ui| {
                section_label(ui, "Appearance");
                setting_toggle(
                    ui,
                    "Dark appearance",
                    "Apply the same color preference to Surf and remote websites.",
                    &mut dark,
                );
                setting_toggle(
                    ui,
                    "Mobile websites",
                    "Ask Chromium for compact mobile versions where available.",
                    &mut mobile,
                );
                ui.add_space(16.0);
                section_label(ui, "Privacy");
                clear_history = ui
                    .add_sized(
                        [ui.available_width(), 36.0],
                        egui::Button::new("Clear browsing history"),
                    )
                    .clicked();
                ui.add_space(16.0);
                section_label(ui, "Connection");
                Frame::group(ui.style())
                    .inner_margin(Margin::symmetric(12, 10))
                    .show(ui, |ui| {
                        ui.set_width(ui.available_width());
                        ui.label(RichText::new(&self.status).size(13.0));
                    });
                for server in self.saved_servers.clone() {
                    ui.horizontal(|ui| {
                        ui.vertical(|ui| {
                            ui.label(RichText::new(&server.name).strong());
                            ui.label(
                                RichText::new(server.endpoint.trim_start_matches("https://"))
                                    .small()
                                    .weak(),
                            );
                        });
                        ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                            if ui.small_button("Forget").clicked() {
                                forget = Some(server.clone());
                            }
                        });
                    });
                }
                disconnect = ui
                    .add_sized(
                        [ui.available_width(), 36.0],
                        egui::Button::new("Disconnect from server"),
                    )
                    .clicked();
                ui.add_space(16.0);
                ui.separator();
                ui.add_space(6.0);
                ui.label(
                    RichText::new(format!(
                        "Surf {} · C99 portable core",
                        include_str!("../../../../../VERSION").trim()
                    ))
                    .monospace()
                    .weak(),
                );
            });
        self.browser.settings_open = open;
        if forget.is_some() {
            self.forget_server = forget;
        }
        if dark != self.dark_mode {
            self.dark_mode = dark;
            theme::apply(context, dark);
            self.send_command(Command::Dark {
                on: dark,
                causal: Causal::default(),
            });
        }
        if mobile != self.mobile_mode {
            self.mobile_mode = mobile;
            self.send_command(Command::Mobile {
                on: mobile,
                causal: Causal::default(),
            });
        }
        if clear_history {
            self.send_command(Command::Clear {
                what: "history".to_owned(),
                causal: Causal::default(),
            });
            self.browser.toast("Browsing history cleared");
        }
        if disconnect {
            self.send(SessionAction::Disconnect);
        }
    }

    fn forget_server_window(&mut self, context: &egui::Context) {
        let Some(server) = self.forget_server.clone() else {
            return;
        };
        let mut cancel = false;
        let mut confirm = false;
        egui::Window::new("Forget server?")
            .id(egui::Id::new("forget_server"))
            .collapsible(false)
            .resizable(false)
            .default_width(380.0)
            .show(context, |ui| {
                ui.label(format!(
                    "Surf will remove {} and this computer's private pairing key for it.",
                    server.name
                ));
                ui.add_space(8.0);
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    confirm = ui.button("Forget server").clicked();
                    cancel = ui.button("Cancel").clicked();
                });
            });
        if cancel {
            self.forget_server = None;
        } else if confirm {
            let connected_to_server = self
                .inspected
                .as_ref()
                .is_some_and(|info| info.server_id == server.server_id);
            match Storage::system().and_then(|storage| storage.forget_server(&server.server_id)) {
                Ok(()) => {
                    if connected_to_server {
                        self.send(SessionAction::Disconnect);
                    }
                    self.saved_servers
                        .retain(|saved| saved.server_id != server.server_id);
                    self.browser.toast(format!("Forgot {}", server.name));
                }
                Err(error) => self.browser.toast(error.to_string()),
            }
            self.forget_server = None;
        }
    }

    fn diagnostics_window(&mut self, context: &egui::Context) {
        if !self.browser.diagnostics_open {
            return;
        }
        let mut open = true;
        let report = self.latest_diagnostics.unwrap_or_default();
        let media = self
            .media
            .as_ref()
            .map(MediaPipeline::diagnostics)
            .unwrap_or_default();
        egui::Window::new("Performance")
            .id(egui::Id::new("diagnostics"))
            .open(&mut open)
            .resizable(false)
            .default_width(380.0)
            .show(context, |ui| {
                ui.horizontal(|ui| {
                    metric(
                        ui,
                        "Presented",
                        format!("{:.1} fps", report.presentation_fps),
                    );
                    metric(ui, "Decoded", format!("{:.1} fps", report.decode_fps));
                    metric(ui, "Dropped", format!("{:.1}%", report.drop_percent));
                });
                ui.separator();
                egui::Grid::new("diagnostics_grid")
                    .num_columns(2)
                    .spacing([18.0, 5.0])
                    .show(ui, |ui| {
                        diagnostic_row(ui, "Decode", report.decode_us, "µs");
                        diagnostic_row(ui, "GPU upload", report.upload_us, "µs");
                        diagnostic_row(ui, "Frame age", report.frame_age_us, "µs");
                        diagnostic_row(ui, "Network", report.network_us, "µs");
                        diagnostic_row(ui, "Round trip", report.rtt_us, "µs");
                        ui.label("Queues");
                        ui.monospace(format!(
                            "{} / {} / {}",
                            report.encoded_video_depth,
                            report.decoded_video_depth,
                            report.audio_depth
                        ));
                        ui.end_row();
                    });
                ui.separator();
                ui.monospace(format!(
                    "health {:?} · gaps {} · decode errors {} · ingress {}",
                    report.health, report.sequence_gaps, report.decode_errors, media.ingress_frames
                ));
            });
        self.browser.diagnostics_open = open;
    }

    fn dialog_window(&mut self, context: &egui::Context) {
        let Some(mut prompt) = self.browser.dialog.clone() else {
            return;
        };
        let mut reply = None;
        egui::Window::new("This page says")
            .id(egui::Id::new("page_dialog"))
            .collapsible(false)
            .resizable(false)
            .default_width(420.0)
            .show(context, |ui| {
                ui.label(&prompt.text);
                if prompt.kind == "prompt" {
                    ui.add_sized(
                        [ui.available_width(), 30.0],
                        TextEdit::singleline(&mut prompt.input),
                    );
                }
                ui.add_space(8.0);
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    if ui.button("OK").clicked() {
                        reply = Some(true);
                    }
                    if prompt.kind != "alert" && ui.button("Cancel").clicked() {
                        reply = Some(false);
                    }
                });
            });
        if let Some(accept) = reply {
            self.send_command(Command::DialogReply {
                accept,
                text: if accept { prompt.input } else { String::new() },
                causal: Causal::default(),
            });
            let _ = self.core.complete_semantic(SemanticCompletion::Dialog);
            self.browser.dialog = None;
        } else {
            self.browser.dialog = Some(prompt);
        }
    }

    fn select_window(&mut self, context: &egui::Context) {
        let Some(mut prompt) = self.browser.select.clone() else {
            return;
        };
        let mut submit = false;
        let mut cancel = false;
        egui::Window::new(if prompt.title.is_empty() {
            "Choose an option"
        } else {
            &prompt.title
        })
        .id(egui::Id::new("page_select"))
        .collapsible(false)
        .default_width(420.0)
        .show(context, |ui| {
            egui::ScrollArea::vertical()
                .max_height(420.0)
                .show(ui, |ui| {
                    for (index, option) in prompt.options.iter().enumerate() {
                        let selected = prompt.selected[index];
                        let response = ui.add_enabled(
                            !option.disabled,
                            egui::Button::selectable(selected, &option.label),
                        );
                        if response.clicked() {
                            if prompt.multiple {
                                prompt.selected[index] = !selected;
                            } else {
                                prompt.selected.fill(false);
                                prompt.selected[index] = true;
                                submit = true;
                            }
                        }
                    }
                });
            ui.separator();
            ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                if prompt.multiple && ui.button("Choose").clicked() {
                    submit = true;
                }
                if ui.button("Cancel").clicked() {
                    cancel = true;
                }
            });
        });
        if submit || cancel {
            let indices = prompt
                .selected
                .iter()
                .enumerate()
                .filter_map(|(index, selected)| selected.then_some(index as i32))
                .collect();
            self.send_command(Command::SelectReply {
                id: prompt.id,
                cancel,
                indices,
                causal: Causal::default(),
            });
            let _ = self.core.complete_semantic(SemanticCompletion::Select);
            self.browser.select = None;
        } else {
            self.browser.select = Some(prompt);
        }
    }

    fn upload_window(&mut self, context: &egui::Context) {
        let Some(multiple) = self.browser.upload_multiple else {
            return;
        };
        let mut upload = false;
        let mut cancel = false;
        egui::Window::new("Choose file")
            .id(egui::Id::new("file_chooser"))
            .collapsible(false)
            .resizable(false)
            .default_width(480.0)
            .show(context, |ui| {
                ui.label(if multiple {
                    "Enter one local file path per line."
                } else {
                    "Enter a local file path."
                });
                ui.add_sized(
                    [ui.available_width(), if multiple { 100.0 } else { 30.0 }],
                    TextEdit::multiline(&mut self.browser.upload_paths)
                        .hint_text("/home/me/Documents/file.pdf"),
                );
                ui.horizontal(|ui| {
                    upload = ui.button("Upload").clicked();
                    cancel = ui.button("Cancel").clicked();
                });
            });
        if upload {
            let mut paths: Vec<PathBuf> = self
                .browser
                .upload_paths
                .lines()
                .map(str::trim)
                .filter(|line| !line.is_empty())
                .map(PathBuf::from)
                .collect();
            if !multiple {
                paths.truncate(1);
            }
            if paths.is_empty() || paths.iter().any(|path| !path.is_file()) {
                self.browser.toast("Choose an existing local file");
            } else {
                self.send(SessionAction::Upload(paths));
                let _ = self.core.complete_semantic(SemanticCompletion::FileChooser);
                self.browser.upload_multiple = None;
            }
        } else if cancel {
            self.send(SessionAction::CancelUpload);
            let _ = self.core.complete_semantic(SemanticCompletion::FileChooser);
            self.browser.upload_multiple = None;
        }
    }

    fn page_error_overlay(&mut self, context: &egui::Context) {
        let Some(url) = self.browser.page_error.clone() else {
            return;
        };
        let mut retry = false;
        let mut dismiss = false;
        egui::Area::new(egui::Id::new("page_error"))
            .anchor(egui::Align2::CENTER_CENTER, [0.0, 20.0])
            .order(egui::Order::Foreground)
            .show(context, |ui| {
                Frame::popup(ui.style()).show(ui, |ui| {
                    ui.set_width(420.0);
                    ui.heading("This page is unavailable");
                    ui.label(compact_address(&url));
                    ui.horizontal(|ui| {
                        retry = ui.button("Try again").clicked();
                        dismiss = ui.button("Dismiss").clicked();
                    });
                });
            });
        if retry {
            self.send_command(Command::Reload {
                causal: Causal::default(),
            });
            self.browser.page_error = None;
        } else if dismiss {
            self.browser.page_error = None;
        }
    }

    fn toast_overlay(&self, context: &egui::Context) {
        let Some(toast) = &self.browser.toast else {
            return;
        };
        egui::Area::new(egui::Id::new("toast"))
            .anchor(egui::Align2::CENTER_BOTTOM, [0.0, -22.0])
            .order(egui::Order::Tooltip)
            .interactable(false)
            .show(context, |ui| {
                Frame::popup(ui.style())
                    .inner_margin(Margin::symmetric(14, 9))
                    .show(ui, |ui| {
                        ui.label(&toast.text);
                    });
            });
    }

    fn content(&mut self, root: &mut egui::Ui) {
        let colors = theme::palette(self.dark_mode);
        egui::CentralPanel::default()
            .frame(
                Frame::new()
                    .fill(colors.background)
                    .inner_margin(Margin::same(0)),
            )
            .show(root, |ui| {
                let available = ui.available_rect_before_wrap();
                let painter = ui.painter();
                painter.rect_filled(available, CornerRadius::ZERO, colors.background);

                if self.connected {
                    let surface = available;
                    let viewport = (
                        ((surface.width().floor() as i32).max(2)) & !1,
                        ((surface.height().floor() as i32).max(2)) & !1,
                    );
                    if self.remote_viewport != Some(viewport) {
                        self.remote_viewport = Some(viewport);
                        self.send_command(Command::Size {
                            w: viewport.0,
                            h: viewport.1,
                            causal: Causal::default(),
                        });
                    }
                    painter.rect_filled(surface, CornerRadius::ZERO, Color32::BLACK);
                    painter.add(self.video_surface.callback(surface));
                    let label = if self.frames_received == 0 {
                        "Secure stream connected · waiting for the first frame".to_owned()
                    } else {
                        let media = self
                            .media
                            .as_ref()
                            .map(MediaPipeline::diagnostics)
                            .unwrap_or_default();
                        format!(
                            "{} decoded · {} presented · {} · upload {:.2} ms · gaps {}",
                            self.frames_received,
                            self.video_surface.presented(),
                            self.last_frame,
                            self.video_surface.latest_upload_us() as f64 / 1_000.0,
                            media.gaps,
                        )
                    };
                    if self.frames_received == 0 {
                        painter.text(
                            surface.left_top() + Vec2::new(16.0, 16.0),
                            egui::Align2::LEFT_TOP,
                            label,
                            FontId::proportional(13.0),
                            colors.muted,
                        );
                    }
                    let response = ui.interact(
                        surface,
                        egui::Id::new("remote_page_surface"),
                        Sense::click_and_drag(),
                    );
                    let events = ui.input(|input| input.events.clone());
                    let pressed_inside = events.iter().any(|event| {
                        matches!(
                            event,
                            egui::Event::PointerButton {
                                pos,
                                pressed: true,
                                ..
                            } if surface.contains(*pos)
                        )
                    });
                    if pressed_inside {
                        response.request_focus();
                    }
                    let keyboard_focused = response.has_focus() && !self.address_focused;
                    match self.page_input.translate(
                        &events,
                        surface,
                        self.video_surface.surface_generation(),
                        keyboard_focused,
                    ) {
                        Ok(commands) => {
                            for command in commands {
                                self.send_command(command);
                            }
                        }
                        Err(error) => {
                            self.status = format!("Input: {error}");
                        }
                    }
                    return;
                }

                let center = available.center();
                let has_choices =
                    !self.saved_servers.is_empty() || !self.discovered_servers.is_empty();
                let desired_height = if self.pairing.is_some() {
                    500.0
                } else if has_choices {
                    520.0
                } else {
                    410.0
                };
                let card = egui::Rect::from_center_size(center, Vec2::new(500.0, desired_height))
                    .intersect(available.shrink(20.0));
                painter.rect_filled(card, CornerRadius::same(18), colors.surface);
                painter.rect_stroke(
                    card,
                    CornerRadius::same(18),
                    Stroke::new(1.0, colors.separator),
                    egui::StrokeKind::Inside,
                );
                ui.scope_builder(egui::UiBuilder::new().max_rect(card.shrink(30.0)), |ui| {
                    ui.with_layout(Layout::top_down(Align::Min), |ui| {
                        ui.vertical_centered(|ui| {
                            ui.label(RichText::new("Surf").size(31.0).strong().color(colors.text));
                            ui.label(
                                RichText::new("Pair once. Browse through a faster machine.")
                                    .size(14.0)
                                    .color(colors.muted),
                            );
                        });
                        ui.add_space(18.0);
                        let mut chosen_endpoint = None;
                        if !self.saved_servers.is_empty() {
                            ui.label(
                                RichText::new("PAIRED SERVERS")
                                    .size(10.0)
                                    .color(colors.muted),
                            );
                            ui.horizontal_wrapped(|ui| {
                                for server in &self.saved_servers {
                                    if ui
                                        .button(format!(
                                            "{}  ·  {}",
                                            server.name,
                                            server.endpoint.trim_start_matches("https://")
                                        ))
                                        .clicked()
                                    {
                                        chosen_endpoint = Some(server.endpoint.clone());
                                    }
                                }
                            });
                            ui.add_space(8.0);
                        }
                        if !self.discovered_servers.is_empty() {
                            ui.label(RichText::new("NEARBY").size(10.0).color(colors.muted));
                            ui.horizontal_wrapped(|ui| {
                                for server in &self.discovered_servers {
                                    if ui
                                        .button(format!("{}  ·  {}", server.name, server.endpoint))
                                        .clicked()
                                    {
                                        chosen_endpoint = Some(server.endpoint.clone());
                                    }
                                }
                            });
                            ui.add_space(8.0);
                        }
                        if let Some(endpoint) = chosen_endpoint {
                            self.inspect(endpoint, true);
                        }
                        ui.label(
                            RichText::new("Server address")
                                .size(12.0)
                                .color(colors.muted),
                        );
                        ui.add_sized(
                            [ui.available_width(), 34.0],
                            TextEdit::singleline(&mut self.endpoint)
                                .hint_text("192.168.1.10:18080")
                                .font(FontId::proportional(14.0)),
                        );
                        ui.add_space(8.0);
                        if ui
                            .add_sized(
                                [ui.available_width(), 34.0],
                                egui::Button::new("Verify server"),
                            )
                            .clicked()
                        {
                            self.inspect(self.endpoint.clone(), false);
                        }

                        if let Some(pairing) = self.pairing.clone() {
                            ui.add_space(12.0);
                            Frame::new()
                                .fill(colors.accent_soft)
                                .corner_radius(CornerRadius::same(10))
                                .inner_margin(Margin::symmetric(14, 12))
                                .show(ui, |ui| {
                                    ui.set_width(ui.available_width());
                                    ui.label(
                                        RichText::new(pairing.phrase)
                                            .size(17.0)
                                            .strong()
                                            .color(colors.accent_text),
                                    );
                                    ui.label(
                                        RichText::new(
                                            "Confirm only if the server shows the same words.",
                                        )
                                        .size(12.0)
                                        .color(colors.muted),
                                    );
                                });
                            ui.add_space(8.0);
                            if ui
                                .add_sized(
                                    [ui.available_width(), 34.0],
                                    egui::Button::new("Words match — pair and connect"),
                                )
                                .clicked()
                            {
                                self.send(SessionAction::ConfirmPairing);
                            }
                        } else if let Some(info) = self.inspected.clone() {
                            ui.add_space(12.0);
                            ui.label(
                                RichText::new(format!("{} · Surf {}", info.name, info.version))
                                    .size(13.0)
                                    .color(colors.text),
                            );
                            if self.paired {
                                if ui
                                    .add_sized(
                                        [ui.available_width(), 34.0],
                                        egui::Button::new("Connect securely"),
                                    )
                                    .clicked()
                                {
                                    self.send(SessionAction::Connect);
                                }
                            } else {
                                ui.horizontal(|ui| {
                                    ui.add_sized(
                                        [(ui.available_width() - 112.0).max(120.0), 34.0],
                                        TextEdit::singleline(&mut self.pairing_code)
                                            .hint_text("Six-digit code")
                                            .char_limit(6)
                                            .font(FontId::proportional(14.0)),
                                    );
                                    if ui
                                        .add_sized([104.0, 34.0], egui::Button::new("Pair"))
                                        .clicked()
                                    {
                                        let device_name = std::env::var("HOSTNAME")
                                            .unwrap_or_else(|_| "Linux device".to_owned());
                                        self.send(SessionAction::Pair {
                                            code: self.pairing_code.clone(),
                                            device_name,
                                        });
                                    }
                                });
                            }
                        }

                        ui.add_space(12.0);
                        if let Some(note) = &self.discovery_note {
                            ui.label(RichText::new(note).size(11.0).color(colors.muted));
                            ui.add_space(4.0);
                        }
                        Frame::new()
                            .fill(colors.field)
                            .corner_radius(CornerRadius::same(9))
                            .inner_margin(Margin::symmetric(12, 9))
                            .show(ui, |ui| {
                                ui.set_width(ui.available_width());
                                ui.label(
                                    RichText::new(&self.status).size(12.0).color(colors.muted),
                                );
                            });
                    });
                });
            });
    }
}

impl eframe::App for SurfDesktop {
    fn ui(&mut self, ui: &mut egui::Ui, _frame: &mut eframe::Frame) {
        let context = ui.ctx().clone();
        self.drain_session();
        self.acknowledge_presented_frame();
        if let Some((request_id, text)) = self.browser.pending_clipboard.take() {
            context.copy_text(text);
            if !request_id.is_empty() {
                self.send_command(Command::ClipboardResult {
                    id: request_id,
                    ok: true,
                    causal: Causal::default(),
                });
            }
            let _ = self.core.complete_semantic(SemanticCompletion::Clipboard);
        }
        if self.browser.prune() {
            let _ = self.core.complete_semantic(SemanticCompletion::Toast);
        }
        self.update_diagnostics();
        self.handle_shortcuts(&context);
        let title = self.snapshot.active_title.trim();
        context.send_viewport_cmd(egui::ViewportCommand::Title(if title.is_empty() {
            "Surf".to_owned()
        } else {
            format!("{title} — Surf")
        }));
        self.chrome(ui);
        self.content(ui);
        self.browser_overlays(&context);
        let presented = self.video_surface.presented();
        if self.smoke_frame_target.is_some()
            && self.smoke_last_heartbeat.elapsed() >= Duration::from_secs(2)
        {
            self.smoke_last_heartbeat = Instant::now();
            let media = self
                .media
                .as_ref()
                .map(MediaPipeline::diagnostics)
                .unwrap_or_default();
            eprintln!(
                "SURF_SMOKE_HEARTBEAT presented={presented} decoded={} ingress={} encoded_depth={} decoded_depth={} connected={}",
                media.decoded_frames,
                media.ingress_frames,
                media.encoded_video_depth,
                media.decoded_video_depth,
                self.connected,
            );
        }
        if presented > 0 && self.smoke_started.is_none() {
            self.smoke_started = Some(Instant::now());
            eprintln!("SURF_SMOKE_STEP first-frame presented={presented}");
        }
        if self.smoke_interaction {
            if self.smoke_interaction_step == 0 && presented >= 45 {
                self.smoke_interaction_step = 1;
                eprintln!("SURF_SMOKE_STEP resize-small presented={presented}");
                context
                    .send_viewport_cmd(egui::ViewportCommand::InnerSize(Vec2::new(940.0, 680.0)));
            } else if self.smoke_interaction_step == 1 && presented >= 90 {
                self.smoke_interaction_step = 2;
                eprintln!("SURF_SMOKE_STEP edit-omnibox presented={presented}");
                self.address = "Editing the omnibox while video remains live".to_owned();
                context.memory_mut(|memory| memory.request_focus(egui::Id::new("address")));
            } else if self.smoke_interaction_step == 2 && presented >= 135 {
                self.smoke_interaction_step = 3;
                eprintln!("SURF_SMOKE_STEP resize-large presented={presented}");
                context
                    .send_viewport_cmd(egui::ViewportCommand::InnerSize(Vec2::new(1_260.0, 800.0)));
            }
        }
        if !self.smoke_stalled
            && presented >= 165
            && let Some(stall_ms) = self.smoke_stall_ms
        {
            self.smoke_stalled = true;
            eprintln!("SURF_SMOKE_STEP ui-stall presented={presented} ms={stall_ms}");
            std::thread::sleep(Duration::from_millis(stall_ms));
        }
        if !self.smoke_reported
            && self
                .smoke_frame_target
                .is_some_and(|target| presented >= target)
        {
            self.smoke_reported = true;
            let elapsed = self
                .smoke_started
                .map(|started| started.elapsed())
                .unwrap_or(Duration::ZERO);
            let intervals = presented.saturating_sub(1);
            let fps = if elapsed.is_zero() {
                0.0
            } else {
                intervals as f64 / elapsed.as_secs_f64()
            };
            let media = self
                .media
                .as_ref()
                .map(MediaPipeline::diagnostics)
                .unwrap_or_default();
            let diagnostics = self.latest_diagnostics.unwrap_or_default();
            let surface = self.video_surface.diagnostics();
            println!(
                "SURF_SMOKE_RESULT presented={presented} elapsed_ms={} fps={fps:.2} decoded={} ingress_replaced={} output_replaced={} presentation_replaced={} gaps={} decode_errors={} encoded_depth={} decoded_depth={} upload_us={} rtt_us={} network_us={} clock_uncertainty_us={} frame_age_us={} timing_synchronized={} health={:?}",
                elapsed.as_millis(),
                media.decoded_frames,
                media.ingress_replaced,
                media.output_replaced,
                surface.replaced,
                media.gaps,
                media.decode_errors,
                media.encoded_video_depth,
                media.decoded_video_depth,
                self.video_surface.latest_upload_us(),
                diagnostics.rtt_us,
                diagnostics.network_us,
                diagnostics.clock_uncertainty_us,
                diagnostics.frame_age_us,
                diagnostics.timing_synchronized,
                diagnostics.health,
            );
            context.send_viewport_cmd(egui::ViewportCommand::Close);
        }
        context.request_repaint_after(Duration::from_millis(16));
    }
}

fn bounded_i32(value: u64) -> i32 {
    i32::try_from(value).unwrap_or(i32::MAX)
}

fn preview_browser_ui() -> BrowserUi {
    let mut browser = BrowserUi::default();
    match std::env::var("SURF_DESKTOP_PREVIEW").as_deref() {
        Ok("tools") => browser.menu_open = true,
        Ok("settings") => browser.settings_open = true,
        Ok("performance") => browser.diagnostics_open = true,
        _ => {}
    }
    browser
}

fn tool_row(ui: &mut egui::Ui, glyph: char, title: &str) -> bool {
    let colors = theme::palette(ui.visuals().dark_mode);
    ui.horizontal(|ui| {
        ui.label(
            RichText::new(glyph.to_string())
                .family(icon_family())
                .size(16.0)
                .color(colors.muted),
        );
        ui.add_sized(
            [(ui.available_width() - 4.0).max(120.0), 32.0],
            egui::Button::new(RichText::new(title).size(14.0)).frame(false),
        )
        .clicked()
    })
    .inner
}

fn library_row(ui: &mut egui::Ui, title: &str, subtitle: &str, mut action: impl FnMut(u8)) {
    ui.horizontal(|ui| {
        ui.vertical(|ui| {
            let title = if title.trim().is_empty() {
                compact_address(subtitle)
            } else {
                title.to_owned()
            };
            if ui.link(RichText::new(title).strong()).clicked() {
                action(0);
            }
            ui.add(egui::Label::new(RichText::new(subtitle).weak().small()).truncate());
        });
        ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
            if ui.small_button("Remove").clicked() {
                action(1);
            }
        });
    });
    ui.separator();
}

fn format_bytes(bytes: i64) -> String {
    let value = bytes.max(0) as f64;
    if value >= 1024.0 * 1024.0 * 1024.0 {
        format!("{:.1} GB", value / (1024.0 * 1024.0 * 1024.0))
    } else if value >= 1024.0 * 1024.0 {
        format!("{:.1} MB", value / (1024.0 * 1024.0))
    } else if value >= 1024.0 {
        format!("{:.1} KB", value / 1024.0)
    } else {
        format!("{} B", value as u64)
    }
}

fn format_time(seconds: f64) -> String {
    let seconds = seconds.max(0.0).round() as u64;
    format!("{}:{:02}", seconds / 60, seconds % 60)
}

fn metric(ui: &mut egui::Ui, name: &str, value: String) {
    Frame::group(ui.style()).show(ui, |ui| {
        ui.set_min_width(96.0);
        ui.label(RichText::new(value).monospace().strong());
        ui.small(name);
    });
}

fn section_label(ui: &mut egui::Ui, text: &str) {
    let colors = theme::palette(ui.visuals().dark_mode);
    ui.label(
        RichText::new(text.to_uppercase())
            .size(10.0)
            .strong()
            .color(colors.muted),
    );
    ui.add_space(2.0);
}

fn setting_toggle(ui: &mut egui::Ui, title: &str, detail: &str, value: &mut bool) {
    let colors = theme::palette(ui.visuals().dark_mode);
    Frame::new()
        .fill(colors.field)
        .stroke(Stroke::new(1.0, colors.field_border))
        .corner_radius(CornerRadius::same(8))
        .inner_margin(Margin::symmetric(12, 10))
        .show(ui, |ui| {
            ui.set_width(ui.available_width());
            ui.horizontal(|ui| {
                ui.vertical(|ui| {
                    ui.label(RichText::new(title).size(14.0).strong());
                    ui.label(RichText::new(detail).size(11.5).color(colors.muted));
                });
                ui.with_layout(Layout::right_to_left(Align::Center), |ui| {
                    if ui
                        .add(egui::Button::selectable(
                            *value,
                            if *value { "On" } else { "Off" },
                        ))
                        .clicked()
                    {
                        *value = !*value;
                    }
                });
            });
        });
}

fn diagnostic_row(ui: &mut egui::Ui, name: &str, value: u64, unit: &str) {
    ui.label(name);
    ui.monospace(format!("{value} {unit}"));
    ui.end_row();
}

fn default_download_path(name: &str) -> PathBuf {
    let directory = directories::UserDirs::new()
        .and_then(|dirs| dirs.download_dir().map(Path::to_path_buf))
        .or_else(|| std::env::current_dir().ok())
        .unwrap_or_else(|| PathBuf::from("."));
    let safe_name = Path::new(name)
        .file_name()
        .filter(|part| !part.is_empty())
        .unwrap_or_else(|| std::ffi::OsStr::new("download"));
    let initial = directory.join(safe_name);
    if !initial.exists() {
        return initial;
    }
    let source = Path::new(safe_name);
    let stem = source
        .file_stem()
        .unwrap_or_else(|| std::ffi::OsStr::new("download"))
        .to_string_lossy();
    let extension = source.extension().map(|value| value.to_string_lossy());
    for index in 2..10_000 {
        let candidate_name = match &extension {
            Some(extension) => format!("{stem} ({index}).{extension}"),
            None => format!("{stem} ({index})"),
        };
        let candidate = directory.join(candidate_name);
        if !candidate.exists() {
            return candidate;
        }
    }
    directory.join(format!("{stem}-surf-download"))
}

fn compact_address(url: &str) -> String {
    let value = url.trim();
    if value.is_empty() || value.starts_with("about:blank") {
        return "New tab".to_owned();
    }
    if value.starts_with("data:") {
        return "Local page".to_owned();
    }
    let without_scheme = value
        .strip_prefix("https://")
        .or_else(|| value.strip_prefix("http://"))
        .unwrap_or(value);
    let authority = without_scheme
        .split(['/', '?', '#'])
        .next()
        .unwrap_or(value);
    let host = authority.rsplit('@').next().unwrap_or(authority);
    let host = host.strip_prefix("www.").unwrap_or(host);
    if host.is_empty() {
        value.to_owned()
    } else {
        host.to_owned()
    }
}

fn chrome_icon(ui: &mut egui::Ui, glyph: char, enabled: bool, label: &str) -> bool {
    let colors = theme::palette(ui.visuals().dark_mode);
    let text = RichText::new(glyph.to_string())
        .family(icon_family())
        .size(18.0)
        .color(if enabled {
            colors.text
        } else {
            colors.disabled
        });
    ui.add_enabled(
        enabled,
        egui::Button::new(text)
            .frame(false)
            .min_size(Vec2::splat(34.0)),
    )
    .on_hover_text(label)
    .clicked()
}

fn icon_family() -> FontFamily {
    FontFamily::Name(ICON_FONT.into())
}

fn install_fonts(context: &egui::Context) {
    let mut fonts = FontDefinitions::default();
    fonts.font_data.insert(
        ICON_FONT.to_owned(),
        Arc::new(FontData::from_static(LUCIDE)),
    );
    fonts
        .families
        .insert(icon_family(), vec![ICON_FONT.to_owned()]);
    context.set_fonts(fonts);
}

#[cfg(test)]
mod tests {
    use super::compact_address;

    #[test]
    fn compact_address_keeps_only_the_meaningful_location() {
        assert_eq!(
            compact_address("https://www.example.com/path?q=1"),
            "example.com"
        );
        assert_eq!(
            compact_address("http://user@host.test:8080/a"),
            "host.test:8080"
        );
        assert_eq!(compact_address("about:blank#surf-new"), "New tab");
        assert_eq!(compact_address("data:text/html,hello"), "Local page");
    }
}
