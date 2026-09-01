mod browser;

use std::path::{Path, PathBuf};

pub use browser::{
    BrowserState, DialogPrompt, MediaState, ReaderDocument, SelectPrompt, Toast, reader_text,
};
use surf_core::{
    ClockSync, Core, DiagnosticsReport, DiagnosticsSample, Effect, Event as CoreEvent,
    PipelineDiagnostics, SemanticCompletion, Snapshot, Tab, monotonic_ns,
};
use surf_media::{DecodedFrame, Diagnostics as MediaDiagnostics, MediaEvent, MediaPipeline};
use surf_protocol::{Causal, Command, Event as WireEvent};
use surf_session::{
    DiscoveredServer, FailureKind, PairingStatus, SavedServer, ServerInfo, SessionAction,
    SessionClient, SessionEvent, Storage, TransferKind,
};

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct RenderDiagnostics {
    pub presented: u64,
    pub replaced: u64,
    pub latest_upload_us: u64,
    pub latest_frame_age_us: u64,
    pub latest_presentation_gap_us: u64,
}

#[derive(Clone, Debug, PartialEq)]
pub enum HostEffect {
    ClearVideo,
    ClearPagePresentation,
    SetClipboard { request_id: String, text: String },
}

#[derive(Debug, Default)]
pub struct Tick {
    pub frame: Option<DecodedFrame>,
    pub effects: Vec<HostEffect>,
}

pub struct ClientController {
    core: Core,
    storage: Storage,
    pub snapshot: Snapshot,
    session: Option<SessionClient>,
    media: Option<MediaPipeline>,
    pub endpoint: String,
    pub status: String,
    pub inspected: Option<ServerInfo>,
    pub paired: bool,
    pub pairing: Option<PairingStatus>,
    pub saved_servers: Vec<SavedServer>,
    pub discovered_servers: Vec<DiscoveredServer>,
    pub discovery_note: Option<String>,
    pub connected: bool,
    pub frames_received: u64,
    pub last_frame_dimensions: Option<(u32, u32)>,
    pub remote_viewport: Option<(i32, i32)>,
    pub video_dimensions: Option<(i32, i32)>,
    pub dark_mode: bool,
    pub mobile_mode: bool,
    pub browser: BrowserState,
    pub latest_diagnostics: Option<DiagnosticsReport>,
    pub native_pointer: bool,
    connect_after_inspect: bool,
    startup_url: Option<String>,
    startup_navigation_sent: bool,
    audio_available: bool,
    clock_available: bool,
    media_stats_available: bool,
    clock_sync: ClockSync,
    pipeline_diagnostics: PipelineDiagnostics,
    render_diagnostics: RenderDiagnostics,
    requested_viewport: Option<(i32, i32)>,
}

impl ClientController {
    pub fn new() -> Result<Self, String> {
        let mut core = Core::new().map_err(|error| error.to_string())?;
        core.dispatch(&CoreEvent::Tabs(vec![Tab {
            id: 1,
            title: "New Tab".to_owned(),
            url: "about:blank#surf-new".to_owned(),
            icon: String::new(),
            active: true,
        }]))
        .map_err(|error| error.to_string())?;
        core.dispatch(&CoreEvent::Url {
            url: "about:blank#surf-new".to_owned(),
            security: String::new(),
            starred: false,
        })
        .map_err(|error| error.to_string())?;
        let snapshot = core.snapshot().map_err(|error| error.to_string())?;
        let storage = std::env::var_os("SURF_CLIENT_HOME")
            .map(Storage::at)
            .map_or_else(Storage::system, Ok)
            .map_err(|error| error.to_string())?;
        let saved_servers = storage.servers().unwrap_or_default();
        let media = MediaPipeline::spawn().map_err(|error| error.to_string())?;
        let session = SessionClient::spawn_with_frame_sink(storage.clone(), media.frame_sink())
            .map_err(|error| error.to_string())?;
        let startup_endpoint =
            (saved_servers.len() == 1).then(|| saved_servers[0].endpoint.clone());
        let startup_url = std::env::args()
            .skip(1)
            .find(|argument| !argument.starts_with('-'));
        let mut controller = Self {
            core,
            storage,
            snapshot,
            session: Some(session),
            media: Some(media),
            endpoint: String::new(),
            status: if saved_servers.is_empty() {
                "Add a Surf server to begin".to_owned()
            } else {
                "Choose a Surf server".to_owned()
            },
            inspected: None,
            paired: false,
            pairing: None,
            saved_servers,
            discovered_servers: Vec::new(),
            discovery_note: None,
            connected: false,
            frames_received: 0,
            last_frame_dimensions: None,
            remote_viewport: None,
            video_dimensions: None,
            dark_mode: true,
            mobile_mode: false,
            browser: BrowserState::default(),
            latest_diagnostics: None,
            native_pointer: false,
            connect_after_inspect: false,
            startup_url,
            startup_navigation_sent: false,
            audio_available: false,
            clock_available: false,
            media_stats_available: false,
            clock_sync: ClockSync::new(),
            pipeline_diagnostics: PipelineDiagnostics::new(),
            render_diagnostics: RenderDiagnostics::default(),
            requested_viewport: None,
        };
        if let Some(endpoint) = startup_endpoint {
            controller.inspect(endpoint, true);
        }
        Ok(controller)
    }

    pub fn inspect(&mut self, endpoint: impl Into<String>, connect_when_paired: bool) {
        self.endpoint = endpoint.into();
        self.inspected = None;
        self.pairing = None;
        self.paired = false;
        self.connect_after_inspect = connect_when_paired;
        self.send(SessionAction::Inspect(self.endpoint.clone()));
    }

    pub fn pair(&mut self, code: String, device_name: String) {
        self.send(SessionAction::Pair { code, device_name });
    }

    pub fn confirm_pairing(&mut self) {
        self.send(SessionAction::ConfirmPairing);
    }

    pub fn connect(&mut self) {
        self.send(SessionAction::Connect);
    }

    pub fn disconnect(&mut self) {
        self.send(SessionAction::Disconnect);
    }

    pub fn command(&mut self, command: Command) {
        if self.connected {
            self.send(SessionAction::Send(command));
        } else {
            self.status = "Connect to a Surf server first".to_owned();
        }
    }

    pub fn navigate(&mut self, input: &str) {
        let input = input.trim();
        if input.is_empty() {
            return;
        }
        let url = normalize_navigation(input);
        self.command(Command::Navigate {
            url,
            causal: Causal::default(),
        });
    }

    pub fn set_viewport(&mut self, width: i32, height: i32) -> bool {
        // Native hosts may briefly report a zero-sized or tiny surface while a
        // window is mapped or its layout is being replaced. That is not a real
        // browser viewport change.
        if !self.connected || width < 64 || height < 64 {
            return false;
        }
        let viewport = (width.max(2) & !1, height.max(2) & !1);
        if self.requested_viewport == Some(viewport) {
            return false;
        }
        self.requested_viewport = Some(viewport);
        self.command(Command::Size {
            w: viewport.0,
            h: viewport.1,
            causal: Causal::default(),
        });
        true
    }

    pub fn set_dark_mode(&mut self, enabled: bool) {
        if self.dark_mode == enabled {
            return;
        }
        self.dark_mode = enabled;
        self.command(Command::Dark {
            on: enabled,
            causal: Causal::default(),
        });
    }

    pub fn set_mobile_mode(&mut self, enabled: bool) {
        if self.mobile_mode == enabled {
            return;
        }
        self.mobile_mode = enabled;
        self.command(Command::Mobile {
            on: enabled,
            causal: Causal::default(),
        });
    }

    pub fn report_presented_frame(&mut self, generation: u32, source_sequence: u32) {
        if let Err(error) = self.core.present_frame(generation, source_sequence) {
            self.status = format!("portable core rejected presented frame: {error}");
        }
        self.refresh();
    }

    pub fn set_render_diagnostics(&mut self, diagnostics: RenderDiagnostics) {
        self.render_diagnostics = diagnostics;
    }

    pub fn report_renderer_error(&mut self, error: impl Into<String>) {
        self.status = format!("Video renderer: {}", error.into());
    }

    pub fn complete_clipboard(&mut self, request_id: String, ok: bool) {
        if !request_id.is_empty() {
            self.command(Command::ClipboardResult {
                id: request_id,
                ok,
                causal: Causal::default(),
            });
        }
        let _ = self.core.complete_semantic(SemanticCompletion::Clipboard);
    }

    pub fn reply_dialog(&mut self, accept: bool, text: String) {
        self.command(Command::DialogReply {
            accept,
            text: if accept { text } else { String::new() },
            causal: Causal::default(),
        });
        let _ = self.core.complete_semantic(SemanticCompletion::Dialog);
        self.browser.dialog = None;
    }

    pub fn reply_select(&mut self, cancel: bool, indices: Vec<i32>) {
        let Some(prompt) = self.browser.select.take() else {
            return;
        };
        self.command(Command::SelectReply {
            id: prompt.id,
            cancel,
            indices,
            causal: Causal::default(),
        });
        let _ = self.core.complete_semantic(SemanticCompletion::Select);
    }

    pub fn choose_files(&mut self, paths: Vec<PathBuf>) {
        if paths.is_empty() {
            self.send(SessionAction::CancelUpload);
        } else {
            self.send(SessionAction::Upload(paths));
        }
        let _ = self.core.complete_semantic(SemanticCompletion::FileChooser);
        self.browser.upload_multiple = None;
    }

    pub fn forget_server(&mut self, server_id: &str) -> Result<(), String> {
        self.storage
            .forget_server(server_id)
            .map_err(|error| error.to_string())?;
        let connected_to_server = self
            .inspected
            .as_ref()
            .is_some_and(|info| info.server_id == server_id);
        if connected_to_server {
            self.disconnect();
        }
        self.saved_servers
            .retain(|server| server.server_id != server_id);
        Ok(())
    }

    pub fn request_download(&mut self, name: String, destination: PathBuf) {
        self.browser.pending_downloads.push(name.clone());
        self.send(SessionAction::Download { name, destination });
    }

    pub fn media_diagnostics(&self) -> MediaDiagnostics {
        self.media
            .as_ref()
            .map(MediaPipeline::diagnostics)
            .unwrap_or_default()
    }

    pub fn tick(&mut self) -> Tick {
        let mut tick = Tick::default();
        while let Some(event) = self.session.as_ref().and_then(SessionClient::try_recv) {
            self.apply_session(event, &mut tick.effects);
        }
        while let Some(event) = self.media.as_ref().and_then(MediaPipeline::try_recv_event) {
            self.apply_media(event);
        }
        if let Some(frame) = self
            .media
            .as_ref()
            .and_then(MediaPipeline::take_latest_frame)
        {
            self.frames_received = self.frames_received.saturating_add(1);
            self.last_frame_dimensions = Some((frame.width, frame.height));
            tick.frame = Some(frame);
        }
        if self.browser.prune() {
            let _ = self.core.complete_semantic(SemanticCompletion::Toast);
        }
        self.update_diagnostics();
        tick
    }

    fn send(&mut self, action: SessionAction) {
        if let Some(session) = &self.session
            && let Err(error) = session.send(action)
        {
            self.status = error.to_string();
        }
    }

    fn refresh(&mut self) {
        match self.core.snapshot() {
            Ok(snapshot) => self.snapshot = snapshot,
            Err(error) => self.status = error.to_string(),
        }
    }

    fn reset_transport(&mut self, effects: &mut Vec<HostEffect>) {
        self.connected = false;
        self.clock_available = false;
        self.media_stats_available = false;
        self.latest_diagnostics = None;
        self.clock_sync.reset();
        self.pipeline_diagnostics.reset();
        if let Some(media) = &self.media {
            media.set_clock_offset(None);
        }
        self.remote_viewport = None;
        self.requested_viewport = None;
        self.video_dimensions = None;
        effects.push(HostEffect::ClearVideo);
    }

    fn apply_session(&mut self, event: SessionEvent, effects: &mut Vec<HostEffect>) {
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
                    "Nearby search is unavailable ({message}); enter an address instead."
                ));
            }
            SessionEvent::Inspected {
                info,
                endpoint,
                saved_pairing,
            } => {
                self.endpoint = endpoint;
                // An open server-side pairing session is an explicit request
                // to pair the selected client. Prefer it over a stale local
                // record; otherwise clicking the server can detour through an
                // avoidable 401 before revealing the code field.
                let can_connect = saved_pairing && !info.pairing;
                self.status = if info.pairing {
                    format!("Enter the six-digit code shown by {}", info.name)
                } else if can_connect {
                    format!("{} is verified and already paired", info.name)
                } else {
                    format!("Open pairing on {} to continue", info.name)
                };
                self.inspected = Some(info);
                self.paired = can_connect;
                self.pairing = None;
                if can_connect && std::mem::take(&mut self.connect_after_inspect) {
                    self.send(SessionAction::Connect);
                } else {
                    self.connect_after_inspect = false;
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
                    return;
                }
                self.browser.reset_connection();
                self.refresh();
                self.connected = true;
                self.clock_available = config.caps.iter().any(|capability| capability == "clock");
                self.media_stats_available = config
                    .caps
                    .iter()
                    .any(|capability| capability == "media-stats");
                self.native_pointer = config
                    .caps
                    .iter()
                    .any(|capability| capability == "pointer-input");
                self.status = format!("Connected securely to {}", info.name);
                self.pairing = None;
                self.remote_viewport = None;
                self.requested_viewport = None;
                self.command(Command::Dark {
                    on: self.dark_mode,
                    causal: Causal::default(),
                });
                self.command(Command::Mobile {
                    on: self.mobile_mode,
                    causal: Causal::default(),
                });
                if self.audio_available {
                    self.command(Command::Audio {
                        on: true,
                        causal: Causal::default(),
                    });
                }
                if !self.startup_navigation_sent
                    && let Some(url) = self.startup_url.clone()
                {
                    self.startup_navigation_sent = true;
                    self.command(Command::Navigate {
                        url,
                        causal: Causal::default(),
                    });
                }
            }
            SessionEvent::Reconnecting {
                attempt,
                maximum,
                delay,
                reason,
            } => {
                self.reset_transport(effects);
                self.status = format!(
                    "Connection interrupted. Retrying {attempt}/{maximum} in {:.1}s — {reason}",
                    delay.as_secs_f32()
                );
            }
            SessionEvent::Control(event) => self.apply_control(event, effects),
            SessionEvent::Transfer {
                kind,
                name,
                ok,
                message,
            } => {
                let operation = match kind {
                    TransferKind::Upload => "Upload",
                    TransferKind::Download => "Download",
                };
                self.status = if ok {
                    format!("{operation} finished: {name}")
                } else {
                    format!("{operation} failed for {name}: {message}")
                };
                if kind == TransferKind::Download {
                    self.browser.pending_downloads.retain(|item| item != &name);
                }
                self.browser.toast(self.status.clone());
            }
            SessionEvent::Disconnected(message) => {
                self.reset_transport(effects);
                self.status = message;
                let _ = self.core.dispatch(&CoreEvent::Loading(false));
                self.refresh();
            }
            SessionEvent::Failure(failure) => {
                self.reset_transport(effects);
                if failure.kind == FailureKind::Authentication {
                    // A saved key only proves that this client paired at some
                    // point. A 401/403 is the server's authoritative answer:
                    // it no longer accepts that key, so expose pairing again
                    // instead of leaving the user at a dead Connect button.
                    self.paired = false;
                    self.pairing = None;
                    self.connect_after_inspect = false;
                    if let Some(info) = &self.inspected {
                        self.status = if info.pairing {
                            format!(
                                "{} no longer recognizes this computer. Enter the six-digit pairing code",
                                info.name
                            )
                        } else {
                            format!(
                                "{} no longer recognizes this computer. Open pairing on the server to continue",
                                info.name
                            )
                        };
                        return;
                    }
                }
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

    fn apply_media(&mut self, event: MediaEvent) {
        match event {
            MediaEvent::RequestKeyframe => self.command(Command::RequestKeyframe {
                causal: Causal::default(),
            }),
            MediaEvent::DecoderError(message) => {
                self.status = format!("Video decoder: {message}");
            }
            MediaEvent::AudioReady { .. } => {
                self.audio_available = true;
                if self.connected {
                    self.command(Command::Audio {
                        on: true,
                        causal: Causal::default(),
                    });
                }
            }
            MediaEvent::AudioUnavailable(_) => self.audio_available = false,
            MediaEvent::AudioError(message) => {
                self.status = format!("Audio output: {message}");
            }
        }
    }

    fn clear_page_presentation(&mut self, effects: &mut Vec<HostEffect>) {
        if self.browser.dialog.is_some() {
            self.command(Command::DialogReply {
                accept: false,
                text: String::new(),
                causal: Causal::default(),
            });
        }
        if let Some(request_id) = self.browser.select.as_ref().map(|select| select.id.clone()) {
            self.command(Command::SelectReply {
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
        effects.push(HostEffect::ClearPagePresentation);
    }

    fn apply_control(&mut self, event: WireEvent, effects: &mut Vec<HostEffect>) {
        let core_effects = match self.core.dispatch_wire_event(&event) {
            Ok(effects) => effects,
            Err(error) => {
                self.status = format!("Rejected control event: {error}");
                return;
            }
        };
        for effect in core_effects {
            match effect {
                Effect::RequestLibrary => self.command(Command::Library {
                    causal: Causal::default(),
                }),
                Effect::ClearPagePresentation => self.clear_page_presentation(effects),
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
            WireEvent::Hello { vw, vh } => self.remote_viewport = Some((vw, vh)),
            WireEvent::VideoConfig {
                state,
                reason,
                w,
                h,
                ..
            } => {
                if state == "ready" {
                    self.video_dimensions = Some((w, h));
                } else if state == "starting" {
                    self.video_dimensions = None;
                    effects.push(HostEffect::ClearVideo);
                } else if !reason.is_empty() {
                    self.status = format!("Video: {reason}");
                }
            }
            WireEvent::AudioConfig { ok, .. } => self.audio_available = ok,
            WireEvent::Found { on } => self.browser.find_found = Some(on),
            WireEvent::Toast { text } => self.browser.toast(text),
            WireEvent::Download { name } => {
                let destination = default_download_path(&name);
                self.request_download(name, destination);
            }
            WireEvent::DownloadProgress { name, pct } => {
                self.browser.download_progress.insert(name, pct);
            }
            WireEvent::Suggest { items } => self.browser.suggestions = items,
            WireEvent::Library {
                hist, bookmarks, ..
            } => {
                self.browser.history = hist;
                self.browser.bookmarks = bookmarks;
            }
            WireEvent::History { items, .. } => self.browser.history = items,
            WireEvent::Downloads { items } => self.browser.downloads = items,
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
            WireEvent::DialogDone => self.browser.dialog = None,
            WireEvent::FileChooser { multiple } => {
                self.browser.upload_multiple = Some(multiple);
            }
            WireEvent::PageError { url, .. } => self.browser.page_error = Some(url),
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
                rect,
            } => {
                let rect = (rect.len() == 4).then(|| [rect[0], rect[1], rect[2], rect[3]]);
                self.browser.open_select(id, title, multiple, options, rect);
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
                self.browser.media = MediaState {
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
            WireEvent::Clipboard { id, text, .. } => effects.push(HostEffect::SetClipboard {
                request_id: id,
                text,
            }),
            WireEvent::ClipboardSync {
                enabled,
                known,
                text,
            } => {
                self.browser.clipboard_sync = enabled;
                self.browser.clipboard_known = known;
                self.browser.clipboard_text.clone_from(&text);
                if enabled && known {
                    effects.push(HostEffect::SetClipboard {
                        request_id: String::new(),
                        text,
                    });
                }
            }
            WireEvent::LogRequest => self.command(Command::LogRecord {
                record: serde_json::json!({
                    "source": "desktop",
                    "level": "info",
                    "message": "Surf Desktop is connected",
                    "version": include_str!("../../../../../VERSION").trim(),
                }),
                causal: Causal::default(),
            }),
            WireEvent::LogClear => self.command(Command::LogCleared {
                causal: Causal::default(),
            }),
            WireEvent::Tabs { .. }
            | WireEvent::Url { .. }
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
        let media = self.media_diagnostics();
        let surface = self.render_diagnostics;
        let sample = diagnostics_sample(now_ns, media, surface, &self.clock_sync, self.connected);
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
                renderer: "imgui-opengl-yuv".to_owned(),
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
}

fn diagnostics_sample(
    now_ns: u64,
    media: MediaDiagnostics,
    surface: RenderDiagnostics,
    clock: &ClockSync,
    connected: bool,
) -> DiagnosticsSample {
    DiagnosticsSample {
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
        rtt_us: clock.rtt_ns().unwrap_or(0) / 1_000,
        clock_uncertainty_us: clock.rtt_ns().unwrap_or(0) / 2_000,
        maximum_presentation_gap_us: surface.latest_presentation_gap_us,
        encoded_video_depth: media.encoded_video_depth,
        decoded_video_depth: media.decoded_video_depth,
        audio_depth: media.audio_depth,
        timing_synchronized: clock.synchronized(),
        connected,
    }
}

pub fn compact_address(url: &str) -> String {
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
    host.strip_prefix("www.").unwrap_or(host).to_owned()
}

pub fn normalize_navigation(value: &str) -> String {
    if value.contains("://") {
        value.to_owned()
    } else if value.contains('.') && !value.contains(' ') {
        format!("https://{value}")
    } else {
        format!(
            "https://www.google.com/search?q={}",
            value.replace(' ', "+")
        )
    }
}

pub fn default_download_path(name: &str) -> PathBuf {
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

fn bounded_i32(value: u64) -> i32 {
    i32::try_from(value).unwrap_or(i32::MAX)
}

#[cfg(test)]
mod tests {
    use super::{compact_address, normalize_navigation};

    #[test]
    fn compact_address_keeps_only_the_meaningful_location() {
        assert_eq!(
            compact_address("https://www.example.com/path?q=1"),
            "example.com"
        );
        assert_eq!(compact_address("about:blank#surf-new"), "New tab");
        assert_eq!(compact_address("data:text/plain,hello"), "Local page");
    }

    #[test]
    fn navigation_distinguishes_addresses_and_searches() {
        assert_eq!(normalize_navigation("example.com"), "https://example.com");
        assert_eq!(
            normalize_navigation("surf browser"),
            "https://www.google.com/search?q=surf+browser"
        );
    }
}
