mod browser;
mod connection;
mod prompts;
mod tools;
mod widgets;
use crate::layout::{BrowserLayout, CONTROL, Density};
use crate::preferences::Preferences;
use crate::theme::Palette;
use widgets::{icon_button, row, section};

use std::collections::BTreeSet;
use std::fs;
use std::path::{Path, PathBuf};
use std::time::{Duration, Instant};

use imgui::{Condition, Key as ImKey, MouseButton as ImMouseButton, Ui, WindowFlags};
use imgui_glow_renderer::glow;
use imgui_winit_support::winit;
use surf_client_app::{ClientController, HostEffect, RenderDiagnostics, compact_address};
use surf_protocol::{Causal, Command};
use winit::dpi::{LogicalPosition, LogicalSize};
use winit::event::{
    ElementState, Ime, KeyEvent, Modifiers as WinitModifiers, MouseButton, MouseScrollDelta,
    WindowEvent,
};
use winit::keyboard::{Key, NamedKey, PhysicalKey};
use winit::window::{Fullscreen, Window};

use crate::page_input::{Modifiers, PageInput, PageRect};
use crate::video_surface::VideoSurface;

const BAR_HEIGHT: f32 = crate::layout::RAIL_HEIGHT;
const PANEL_TOP: f32 = BAR_HEIGHT + 4.0;
const PANEL_WIDTH: f32 = 330.0;
const VIEWPORT_SETTLE: Duration = Duration::from_millis(120);
const WINDOW_RESIZE_TIMEOUT: Duration = Duration::from_secs(4);
const SURF_VERSION: &str = include_str!("../../../../../VERSION");

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
struct DevicePreset {
    label: &'static str,
    portrait: [u32; 2],
}

impl DevicePreset {
    const fn size(self, landscape: bool) -> [u32; 2] {
        if landscape {
            [self.portrait[1], self.portrait[0]]
        } else {
            self.portrait
        }
    }
}

const DEVICE_PRESETS: &[DevicePreset] = &[
    DevicePreset {
        label: "iPhone 3.5-inch",
        portrait: [320, 480],
    },
    DevicePreset {
        label: "iPhone 4-inch",
        portrait: [320, 568],
    },
    DevicePreset {
        label: "iPhone 4.7-inch",
        portrait: [375, 667],
    },
    DevicePreset {
        label: "iPhone 5.5-inch",
        portrait: [414, 736],
    },
    DevicePreset {
        label: "iPhone full-screen",
        portrait: [375, 812],
    },
    DevicePreset {
        label: "iPad / iPad mini / 9.7-inch",
        portrait: [768, 1024],
    },
    DevicePreset {
        label: "iPad 10.5-inch",
        portrait: [834, 1112],
    },
    DevicePreset {
        label: "iPad 11-inch",
        portrait: [834, 1194],
    },
    DevicePreset {
        label: "iPad 12.9-inch",
        portrait: [1024, 1366],
    },
];

mod icon {
    pub const BACK: &str = "\u{e06e}";
    pub const FORWARD: &str = "\u{e06f}";
    pub const RELOAD: &str = "\u{e145}";
    pub const STOP: &str = "\u{e167}";
    pub const CLOSE: &str = "\u{e1b2}";
    pub const PLUS: &str = "\u{e13d}";
    pub const MORE: &str = "\u{e0b6}";
    pub const STAR: &str = "\u{e176}";
    pub const BOOK: &str = "\u{e05f}";
    pub const TABS: &str = "\u{e12c}";
    pub const SEARCH: &str = "\u{e151}";
    pub const GEAR: &str = "\u{e154}";
    pub const GAUGE: &str = "\u{e1bf}";
    pub const SHARE: &str = "\u{e155}";
    pub const EXPAND: &str = "\u{e112}";
    pub const READER: &str = "\u{e348}";
    pub const MEDIA: &str = "\u{e080}";
    pub const SERVER: &str = "\u{e11d}";
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum Panel {
    Tabs,
    More,
    Library,
    Find,
    Reader,
    Media,
    Settings,
    Performance,
    Files,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum LibrarySection {
    History,
    Bookmarks,
    Downloads,
}

enum Action {
    Command(Command),
    Navigate(String),
    Inspect(String, bool),
    Pair(String),
    ConfirmPairing,
    Connect,
    Disconnect,
    Dark(bool),
    Mobile(bool),
    Forget(String),
    Download(String),
}

struct FilePicker {
    directory: PathBuf,
    selected: BTreeSet<PathBuf>,
    multiple: bool,
    error: Option<String>,
}

impl FilePicker {
    fn new(multiple: bool) -> Self {
        let directory = directories::UserDirs::new()
            .map(|dirs| dirs.home_dir().to_owned())
            .or_else(|| std::env::current_dir().ok())
            .unwrap_or_else(|| PathBuf::from("."));
        Self {
            directory,
            selected: BTreeSet::new(),
            multiple,
            error: None,
        }
    }
}

struct SmokeState {
    target: Option<u64>,
    started: Option<Instant>,
    last_heartbeat: Instant,
    interaction: bool,
    step: u8,
    stall_ms: Option<u64>,
    stalled: bool,
    done: bool,
}

struct PendingFullscreen {
    on: bool,
    started: Instant,
    previous_viewport: Option<(i32, i32)>,
    command_sent: bool,
}

#[derive(Clone, Copy)]
struct WindowSizeRequest {
    label: &'static str,
    size: [u32; 2],
}

struct PendingWindowSize {
    request: WindowSizeRequest,
    started: Instant,
    viewport: Option<(i32, i32)>,
}

impl SmokeState {
    fn from_environment() -> Self {
        let target = std::env::var("SURF_SMOKE_EXIT_AFTER_FRAMES")
            .ok()
            .and_then(|value| value.parse::<u64>().ok())
            .filter(|value| *value > 0)
            .or_else(|| {
                std::env::var_os("SURF_SMOKE_EXIT_AFTER_FRAME")
                    .is_some()
                    .then_some(1)
            });
        Self {
            target,
            started: None,
            last_heartbeat: Instant::now(),
            interaction: std::env::var_os("SURF_SMOKE_INTERACTION").is_some(),
            step: 0,
            stall_ms: std::env::var("SURF_SMOKE_STALL_MS")
                .ok()
                .and_then(|value| value.parse().ok()),
            stalled: false,
            done: false,
        }
    }
}

pub struct DesktopApp {
    assets: crate::assets::Assets,
    controller: ClientController,
    video: VideoSurface,
    page_input: PageInput,
    page_rect: PageRect,
    input_rect: PageRect,
    cursor: Option<(f64, f64)>,
    modifiers: Modifiers,
    page_focused: bool,
    address: String,
    address_editing: bool,
    focus_address: bool,
    omnibox_rect: [f32; 4],
    endpoint: String,
    pairing_code: String,
    panel: Option<Panel>,
    previous_panel: Option<Panel>,
    library_section: LibrarySection,
    dialog_input: String,
    dialog_signature: String,
    select_signature: String,
    select_values: Vec<bool>,
    file_picker: Option<FilePicker>,
    fullscreen: bool,
    fullscreen_request: Option<bool>,
    fullscreen_pending: Option<PendingFullscreen>,
    theme_request: Option<bool>,
    device_preset: usize,
    device_landscape: bool,
    window_size_request: Option<WindowSizeRequest>,
    pending_window_size: Option<PendingWindowSize>,
    ui_wants_pointer: bool,
    ui_wants_keyboard: bool,
    remote_keys: std::collections::HashMap<PhysicalKey, (String, String, i32)>,
    local_preedit: String,
    observed_url: String,
    observed_tab: Option<i64>,
    focus_new_tab: bool,
    suggestion_index: Option<usize>,
    preferences: Preferences,
    saved_preferences: Preferences,
    layout: BrowserLayout,
    find_open: bool,
    performance_open: bool,
    library_filter: String,
    viewport_candidate: Option<(i32, i32)>,
    viewport_candidate_since: Instant,
    viewport_committed: Option<(i32, i32)>,
    render_diagnostics: RenderDiagnostics,
    smoke: SmokeState,
}

impl DesktopApp {
    pub fn new(gl: &std::rc::Rc<glow::Context>) -> Result<Self, String> {
        let preferences = Preferences::load();
        Ok(Self {
            assets: crate::assets::Assets::new(gl)?,
            controller: ClientController::new_with_options(surf_client_app::ClientOptions {
                dark_mode: preferences.dark,
                mobile_mode: preferences.mobile,
            })?,
            video: VideoSurface::new(gl)?,
            page_input: PageInput::new(),
            page_rect: PageRect::default(),
            input_rect: PageRect::default(),
            cursor: None,
            modifiers: Modifiers::default(),
            page_focused: false,
            address: String::new(),
            address_editing: false,
            focus_address: false,
            omnibox_rect: [0.0; 4],
            endpoint: String::new(),
            pairing_code: String::new(),
            panel: None,
            previous_panel: None,
            library_section: LibrarySection::History,
            dialog_input: String::new(),
            dialog_signature: String::new(),
            select_signature: String::new(),
            select_values: Vec::new(),
            file_picker: None,
            fullscreen: false,
            fullscreen_request: None,
            fullscreen_pending: None,
            theme_request: Some(preferences.dark),
            device_preset: preferences.device_preset.min(DEVICE_PRESETS.len() - 1),
            device_landscape: preferences.landscape,
            window_size_request: None,
            pending_window_size: None,
            ui_wants_pointer: false,
            ui_wants_keyboard: false,
            remote_keys: std::collections::HashMap::new(),
            local_preedit: String::new(),
            observed_url: String::new(),
            observed_tab: None,
            focus_new_tab: false,
            suggestion_index: None,
            saved_preferences: preferences.clone(),
            preferences,
            layout: BrowserLayout::new([1180.0, 760.0], true, false),
            find_open: false,
            performance_open: false,
            library_filter: String::new(),
            viewport_candidate: None,
            viewport_candidate_since: Instant::now(),
            viewport_committed: None,
            render_diagnostics: RenderDiagnostics::default(),
            smoke: SmokeState::from_environment(),
        })
    }

    pub fn report_host_error(&mut self, error: String) {
        self.controller.report_renderer_error(error);
    }

    fn release_page_input(&mut self) {
        for (_, (key, code, key_code)) in std::mem::take(&mut self.remote_keys) {
            if let Ok(command) = self.page_input.key(
                false,
                key,
                code,
                key_code,
                String::new(),
                Modifiers::default(),
            ) {
                self.controller.command(command);
            }
        }
        for command in self
            .page_input
            .release_buttons(self.page_rect, self.video.surface_generation())
        {
            self.controller.command(command);
        }
        if self.page_input.composition_active()
            && let Ok(command) = self.page_input.compose("cancel", String::new(), 0, 0)
        {
            self.controller.command(command);
        }
        self.page_focused = false;
        self.local_preedit.clear();
    }

    // WinitPlatform currently does not forward IME events. This narrow host adapter
    // commits local text once; preedit is presentation-only, never an InputText draft.
    pub fn handle_local_ime(&mut self, io: &mut imgui::Io, event: &winit::event::Event<()>) {
        if self.page_accepts_keyboard() {
            return;
        }
        if let winit::event::Event::WindowEvent {
            event: WindowEvent::Ime(ime),
            ..
        } = event
        {
            match ime {
                Ime::Preedit(text, _) => self.local_preedit.clone_from(text),
                Ime::Commit(text) => {
                    self.local_preedit.clear();
                    if io.want_text_input {
                        for c in text.chars() {
                            io.add_input_character(c);
                        }
                    }
                }
                Ime::Disabled => self.local_preedit.clear(),
                Ime::Enabled => {}
            }
        }
    }

    pub fn tick(&mut self) {
        let tick = self.controller.tick();
        if let Some(frame) = tick.frame {
            self.video.submit(frame);
        }
        for effect in tick.effects {
            match effect {
                HostEffect::ClearVideo => {
                    self.video.clear();
                    self.page_input.reset();
                }
                HostEffect::ClearPagePresentation => {
                    if matches!(
                        self.panel,
                        Some(Panel::Find | Panel::Reader | Panel::Media | Panel::Files)
                    ) {
                        self.panel = None;
                    }
                }
                HostEffect::SetClipboard { request_id, text } => {
                    let ok = arboard::Clipboard::new()
                        .and_then(|mut clipboard| clipboard.set_text(text))
                        .is_ok();
                    self.controller.complete_clipboard(request_id, ok);
                }
            }
        }
        if self.controller.browser.upload_multiple.is_some() && self.file_picker.is_none() {
            let multiple = self.controller.browser.upload_multiple.unwrap_or(false);
            self.file_picker = Some(FilePicker::new(multiple));
            self.panel = Some(Panel::Files);
        }
        if self.controller.browser.reader.is_some() && self.panel != Some(Panel::Reader) {
            self.panel = Some(Panel::Reader);
        }
        self.page_input.configure(
            self.controller.native_pointer,
            self.video.surface_generation(),
        );
    }

    pub fn draw(&mut self, ui: &Ui, window: &Window) {
        let display = ui.io().display_size;
        let connected = self.controller.connected;
        let current_url = self.controller.snapshot.current_url.clone();
        let current_tab = self
            .controller
            .snapshot
            .tabs
            .iter()
            .find(|tab| tab.active)
            .map(|tab| tab.id);
        let tab_changed = current_tab != self.observed_tab;
        self.observed_tab = current_tab;
        if current_url != self.observed_url || tab_changed {
            self.observed_url.clone_from(&current_url);
            if !self.address_editing || tab_changed {
                self.address.clone_from(&current_url);
            }
            if tab_changed {
                self.address_editing = false;
                self.controller.clear_suggestions();
                self.suggestion_index = None;
            }
        }
        if self.focus_new_tab && tab_changed && current_url.starts_with("about:blank") {
            self.focus_new_tab = false;
            self.edit_address();
        }
        self.layout = BrowserLayout::new(display, self.preferences.bottom, self.find_open);
        self.page_rect = self.layout.page;
        if connected {
            if current_url.starts_with("about:blank") {
                self.draw_new_tab(ui);
            }
            self.draw_chrome(ui);
            if self.pending_window_size.is_none() {
                self.update_viewport(window.scale_factor());
            }
        } else {
            self.viewport_candidate = None;
            self.viewport_committed = None;
            self.fullscreen_pending = None;
            self.draw_start(ui);
        }
        self.draw_panel(ui);
        if self.find_open {
            self.draw_find(ui);
        }
        if self.performance_open {
            self.draw_performance(ui);
        }
        self.draw_suggestions(ui);
        self.draw_page_dialog(ui);
        self.draw_page_select(ui);
        self.draw_page_error(ui);
        self.draw_toast(ui);
        if !self.local_preedit.is_empty() {
            small_overlay(
                ui,
                [
                    self.omnibox_rect[0] + 80.0,
                    (self.omnibox_rect[1] - 34.0).max(8.0),
                ],
                &self.local_preedit,
            );
        }

        let title = self.controller.snapshot.active_title.trim();
        let window_title = if title.is_empty() {
            "Surf".to_owned()
        } else {
            format!("{title} — Surf")
        };
        window.set_title(&window_title);
        if self.fullscreen_pending.as_ref().is_some_and(|pending| {
            pending.command_sent && self.controller.snapshot.fullscreen == pending.on
        }) || self.fullscreen_pending.as_ref().is_some_and(|pending| {
            pending.command_sent && pending.started.elapsed() > Duration::from_secs(3)
        }) {
            self.fullscreen_pending = None;
        }
        if self.fullscreen_pending.is_none()
            && self.fullscreen != self.controller.snapshot.fullscreen
        {
            self.set_fullscreen(window, self.controller.snapshot.fullscreen);
        }
        if let Some(on) = self.fullscreen_request.take() {
            self.set_fullscreen(window, on);
        }
        self.update_window_size(window);
        self.ui_wants_pointer = ui.io().want_capture_mouse;
        self.ui_wants_keyboard = ui.io().want_capture_keyboard;
        self.preferences.dark = self.controller.dark_mode;
        self.preferences.mobile = self.controller.mobile_mode;
        self.preferences.device_preset = self.device_preset;
        self.preferences.landscape = self.device_landscape;
        if self.preferences != self.saved_preferences {
            match self.preferences.save() {
                Ok(()) => self.saved_preferences = self.preferences.clone(),
                Err(error) => self.report_host_error(format!("Could not save settings: {error}")),
            }
        }
    }

    pub fn take_theme_request(&mut self) -> Option<bool> {
        self.theme_request.take()
    }

    pub fn render_video(&mut self, gl: &glow::Context, scale: f64, framebuffer_height: u32) {
        if !self.controller.connected {
            return;
        }
        let x = (self.page_rect.x * scale).round() as i32;
        let width = (self.page_rect.width * scale).round().max(1.0) as i32;
        let height = (self.page_rect.height * scale).round().max(1.0) as i32;
        let top = (self.page_rect.y * scale).round() as i32;
        let y = i32::try_from(framebuffer_height)
            .unwrap_or(i32::MAX)
            .saturating_sub(top)
            .saturating_sub(height);
        self.video.render(gl, [x, y, width, height]);
        let [x, y, w, h] = self.video.drawn_viewport();
        self.input_rect = PageRect {
            x: f64::from(x) / scale,
            y: (f64::from(framebuffer_height) - f64::from(y + h)) / scale,
            width: f64::from(w) / scale,
            height: f64::from(h) / scale,
        };
        if let Some(error) = self.video.take_error() {
            self.controller.report_renderer_error(error);
        }
    }

    pub fn after_swap(&mut self, success: bool) {
        if let Some(frame) = self.video.after_swap(success) {
            self.controller
                .report_presented_frame(frame.generation, frame.source_sequence);
        }
        let diagnostics = self.video.diagnostics();
        self.render_diagnostics = diagnostics;
        self.controller.set_render_diagnostics(diagnostics);
    }

    pub fn after_present(&mut self, window: &Window) -> bool {
        let Some(target) = self.smoke.target else {
            return false;
        };
        let diagnostics = self.controller.latest_diagnostics.unwrap_or_default();
        let presented = self.render_diagnostics.presented;
        if presented == 0 {
            return false;
        }
        let started = *self.smoke.started.get_or_insert_with(|| {
            eprintln!("SURF_SMOKE_STEP first-frame presented={presented}");
            Instant::now()
        });
        if self.smoke.interaction {
            if self.smoke.step == 0 {
                let _ = window.request_inner_size(LogicalSize::new(940.0, 620.0));
                eprintln!("SURF_SMOKE_STEP resize-small presented={presented}");
                self.smoke.step = 1;
            } else if self.smoke.step == 1 && presented >= (target / 4).max(2) {
                self.edit_address();
                self.address.push_str("#smoke");
                eprintln!("SURF_SMOKE_STEP edit-omnibox presented={presented}");
                self.address_editing = false;
                self.smoke.step = 2;
            } else if self.smoke.step == 2 && presented >= (target / 2).max(3) {
                let _ = window.request_inner_size(LogicalSize::new(1260.0, 760.0));
                eprintln!("SURF_SMOKE_STEP resize-large presented={presented}");
                self.smoke.step = 3;
            } else if self.smoke.step == 3 && presented >= (target * 3 / 4).max(4) {
                if let Some(milliseconds) = self.smoke.stall_ms
                    && !self.smoke.stalled
                {
                    self.smoke.stalled = true;
                    eprintln!("SURF_SMOKE_STEP ui-stall presented={presented} ms={milliseconds}");
                    std::thread::sleep(Duration::from_millis(milliseconds));
                }
                self.smoke.step = 4;
            }
        }
        if self.smoke.last_heartbeat.elapsed() >= Duration::from_secs(2) {
            let media = self.controller.media_diagnostics();
            eprintln!(
                "SURF_SMOKE_HEARTBEAT presented={presented} decoded={} ingress={} encoded_depth={} decoded_depth={} connected={}",
                media.decoded_frames,
                media.ingress_frames,
                media.encoded_video_depth,
                media.decoded_video_depth,
                self.controller.connected,
            );
            self.smoke.last_heartbeat = Instant::now();
        }
        if presented < target || self.smoke.done {
            return false;
        }
        self.smoke.done = true;
        let elapsed = started.elapsed();
        let fps = presented as f64 / elapsed.as_secs_f64().max(0.001);
        let media = self.controller.media_diagnostics();
        println!(
            "SURF_SMOKE_RESULT presented={presented} elapsed_ms={} fps={fps:.2} decoded={} ingress_replaced={} output_replaced={} presentation_replaced={} gaps={} decode_errors={} encoded_depth={} decoded_depth={} upload_us={} rtt_us={} network_us={} clock_uncertainty_us={} frame_age_us={} timing_synchronized={} health={:?}",
            elapsed.as_millis(),
            media.decoded_frames,
            media.ingress_replaced,
            media.output_replaced,
            self.render_diagnostics.replaced,
            media.gaps,
            media.decode_errors,
            media.encoded_video_depth,
            media.decoded_video_depth,
            diagnostics.upload_us,
            diagnostics.rtt_us,
            diagnostics.network_us,
            diagnostics.clock_uncertainty_us,
            diagnostics.frame_age_us,
            diagnostics.timing_synchronized,
            diagnostics.health,
        );
        true
    }

    pub fn handle_window_event(&mut self, window: &Window, event: &WindowEvent) {
        match event {
            WindowEvent::ModifiersChanged(modifiers) => {
                self.modifiers = modifiers_from_winit(*modifiers);
            }
            WindowEvent::CursorMoved { position, .. } => {
                let position: LogicalPosition<f64> = position.to_logical(window.scale_factor());
                self.cursor = Some((position.x, position.y));
                if self.page_accepts_pointer(position.x, position.y) || self.page_input.dragging() {
                    match self.page_input.motion(
                        position.x,
                        position.y,
                        self.input_rect,
                        self.video.surface_generation(),
                        self.modifiers,
                    ) {
                        Ok(Some(command)) => self.controller.command(command),
                        Ok(None) => {}
                        Err(error) => self.report_host_error(format!("pointer input: {error}")),
                    }
                }
            }
            WindowEvent::MouseInput { state, button, .. } => {
                let Some((x, y)) = self.cursor else {
                    return;
                };
                let pressed = *state == ElementState::Pressed;
                if pressed {
                    // Dismissal is a local click, not an accidental click through to a page.
                    if self.panel.is_some() && !self.ui_wants_pointer {
                        self.panel = None;
                        self.page_focused = false;
                        return;
                    }
                    let page_hit = self.page_accepts_pointer(x, y);
                    self.page_focused = page_hit;
                    if page_hit {
                        self.finish_address_edit();
                    }
                }
                if (self.page_accepts_pointer(x, y) || !pressed)
                    && let Some(number) = mouse_button_number(*button)
                {
                    match self.page_input.button(
                        pressed,
                        number,
                        x,
                        y,
                        self.input_rect,
                        self.video.surface_generation(),
                        self.modifiers,
                    ) {
                        Ok(Some(command)) => self.controller.command(command),
                        Ok(None) => {}
                        Err(error) => self.report_host_error(format!("pointer input: {error}")),
                    }
                }
            }
            WindowEvent::MouseWheel { delta, .. } => {
                let (dx, dy) = match delta {
                    MouseScrollDelta::LineDelta(x, y) => {
                        (f64::from(*x) * 50.0, f64::from(*y) * 50.0)
                    }
                    MouseScrollDelta::PixelDelta(position) => {
                        let position: LogicalPosition<f64> =
                            position.to_logical(window.scale_factor());
                        (position.x, position.y)
                    }
                };
                if self
                    .cursor
                    .is_some_and(|(x, y)| self.page_accepts_pointer(x, y))
                {
                    match self.page_input.wheel(
                        dx,
                        -dy,
                        self.input_rect,
                        self.video.surface_generation(),
                        self.modifiers,
                    ) {
                        Ok(commands) => {
                            for command in commands {
                                self.controller.command(command);
                            }
                        }
                        Err(error) => self.report_host_error(format!("wheel input: {error}")),
                    }
                }
            }
            WindowEvent::KeyboardInput { event, .. } => self.handle_key(event),
            WindowEvent::Ime(ime) if self.page_accepts_keyboard() => {
                let command = match ime {
                    Ime::Preedit(text, cursor) if text.is_empty() => self
                        .page_input
                        .composition_active()
                        .then(|| self.page_input.compose("cancel", String::new(), 0, 0)),
                    Ime::Preedit(text, cursor) => {
                        let (start, end) = cursor.unwrap_or((text.len(), text.len()));
                        Some(self.page_input.compose(
                            "update",
                            text.clone(),
                            utf16_offset(text, start),
                            utf16_offset(text, end),
                        ))
                    }
                    Ime::Commit(text) => Some(self.page_input.compose(
                        if text.is_empty() { "cancel" } else { "commit" },
                        text.clone(),
                        0,
                        0,
                    )),
                    Ime::Enabled | Ime::Disabled => None,
                };
                if let Some(command) = command {
                    match command {
                        Ok(command) => self.controller.command(command),
                        Err(error) => self.report_host_error(format!("composition input: {error}")),
                    }
                }
            }
            WindowEvent::Focused(false) => self.release_page_input(),
            _ => {}
        }
    }

    fn handle_key(&mut self, event: &KeyEvent) {
        let pressed = event.state == ElementState::Pressed;
        if !pressed
            && let Some((key, code, key_code)) = self.remote_keys.remove(&event.physical_key)
        {
            if let Ok(command) =
                self.page_input
                    .key(false, key, code, key_code, String::new(), self.modifiers)
            {
                self.controller.command(command);
            }
            return;
        }
        if self.controller.browser.dialog.is_none()
            && self.controller.browser.select.is_none()
            && self.panel != Some(Panel::Files)
            && self.handle_shortcut(event, pressed)
        {
            return;
        }
        if !self.page_accepts_keyboard() {
            return;
        }
        if pressed
            && (self.modifiers.control || self.modifiers.command)
            && logical_character(&event.logical_key)
                .is_some_and(|value| value.eq_ignore_ascii_case("v"))
        {
            if let Ok(text) = arboard::Clipboard::new().and_then(|mut value| value.get_text())
                && let Ok(command) = self.page_input.paste(text)
            {
                self.controller.command(command);
            }
            return;
        }
        let plain_text = plain_key_text(
            &event.logical_key,
            event.text.as_deref(),
            self.modifiers,
            pressed,
            self.page_input.composition_active(),
        );
        if let Some(text) = plain_text {
            match self
                .page_input
                .key(true, String::new(), String::new(), 0, text, self.modifiers)
            {
                Ok(command) => self.controller.command(command),
                Err(error) => self.report_host_error(format!("text input: {error}")),
            }
            return;
        }
        let (key, code, key_code, _) = dom_key(event);
        if pressed {
            self.remote_keys
                .insert(event.physical_key, (key.clone(), code.clone(), key_code));
        }
        let textual_key = logical_character(&event.logical_key).is_some()
            || matches!(event.logical_key, Key::Named(NamedKey::Space));
        if !pressed && textual_key && !self.modifiers.control && !self.modifiers.alt {
            return;
        }
        match self
            .page_input
            .key(pressed, key, code, key_code, String::new(), self.modifiers)
        {
            Ok(command) => self.controller.command(command),
            Err(error) => self.report_host_error(format!("keyboard input: {error}")),
        }
    }

    fn handle_shortcut(&mut self, event: &KeyEvent, pressed: bool) -> bool {
        let character = logical_character(&event.logical_key).map(str::to_ascii_lowercase);
        let command = self.modifiers.control || self.modifiers.command;
        if command {
            match character {
                Some(ref value) if value == "l" => {
                    if pressed {
                        self.edit_address();
                    }
                    return true;
                }
                Some(ref value) if value == "t" => {
                    if pressed {
                        self.new_tab();
                    }
                    return true;
                }
                Some(ref value) if value == "w" => {
                    if pressed {
                        self.close_active_tab();
                    }
                    return true;
                }
                Some(ref value) if value == "r" => {
                    if pressed {
                        self.reload_or_stop();
                    }
                    return true;
                }
                Some(ref value) if value == "d" => {
                    if pressed {
                        self.controller.command(Command::Bookmark {
                            causal: Causal::default(),
                        });
                    }
                    return true;
                }
                Some(ref value) if value == "f" => {
                    if pressed {
                        self.find_open = true;
                        self.page_focused = false;
                    }
                    return true;
                }
                _ => {}
            }
        }
        if self.modifiers.alt {
            match event.logical_key {
                Key::Named(NamedKey::ArrowLeft) => {
                    if pressed {
                        self.controller.command(Command::Back {
                            causal: Causal::default(),
                        });
                    }
                    return true;
                }
                Key::Named(NamedKey::ArrowRight) => {
                    if pressed {
                        self.controller.command(Command::Forward {
                            causal: Causal::default(),
                        });
                    }
                    return true;
                }
                _ => {}
            }
        }
        match event.logical_key {
            Key::Named(NamedKey::F5) => {
                if pressed {
                    self.reload_or_stop();
                }
                true
            }
            Key::Named(NamedKey::F11) => {
                if pressed {
                    let on = !self.fullscreen;
                    self.set_fullscreen_command(on);
                }
                true
            }
            Key::Named(NamedKey::Escape) if pressed && self.panel.is_some() => {
                if self.panel == Some(Panel::Files) {
                    self.controller.choose_files(Vec::new());
                    self.file_picker = None;
                }
                self.panel = None;
                self.page_focused = true;
                true
            }
            Key::Named(NamedKey::Escape) if pressed && self.address_editing => {
                self.finish_address_edit();
                true
            }
            _ => false,
        }
    }

    fn page_accepts_pointer(&self, x: f64, y: f64) -> bool {
        self.controller.connected
            && !self.ui_wants_pointer
            && self.controller.browser.dialog.is_none()
            && self.controller.browser.select.is_none()
            && self.controller.browser.page_error.is_none()
            && self.input_rect.contains(x, y)
    }

    fn page_accepts_keyboard(&self) -> bool {
        self.controller.connected
            && self.page_focused
            && !self.address_editing
            && !self.ui_wants_keyboard
            && self.controller.browser.dialog.is_none()
            && self.controller.browser.select.is_none()
            && self.controller.browser.page_error.is_none()
    }

    fn edit_address(&mut self) {
        self.address = if self
            .controller
            .snapshot
            .current_url
            .starts_with("about:blank")
        {
            String::new()
        } else {
            self.controller.snapshot.current_url.clone()
        };
        self.observed_url
            .clone_from(&self.controller.snapshot.current_url);
        // Do not flash a result set left over from the previous edit session. A fresh
        // response will repopulate this after the user changes the query.
        self.controller.clear_suggestions();
        self.address_editing = true;
        self.focus_address = true;
        self.page_focused = false;
    }

    fn update_viewport(&mut self, scale: f64) {
        let candidate = (
            ((self.page_rect.width * scale).round().max(64.0) as i32) & !1,
            ((self.page_rect.height * scale).round().max(64.0) as i32) & !1,
        );
        if self.viewport_candidate != Some(candidate) {
            self.viewport_candidate = Some(candidate);
            self.viewport_candidate_since = Instant::now();
        }
        let initial = self.viewport_committed.is_none();
        if self.viewport_committed != Some(candidate)
            && (initial || self.viewport_candidate_since.elapsed() >= VIEWPORT_SETTLE)
        {
            let _ = self.controller.set_viewport(candidate.0, candidate.1);
            self.viewport_committed = Some(candidate);
        }
        let fullscreen_to_send = self.fullscreen_pending.as_ref().and_then(|pending| {
            let geometry_changed = self.viewport_committed != pending.previous_viewport;
            let stable = self.viewport_candidate_since.elapsed() >= VIEWPORT_SETTLE;
            (!pending.command_sent
                && stable
                && (geometry_changed || pending.started.elapsed() >= Duration::from_millis(300)))
            .then_some(pending.on)
        });
        if let Some(on) = fullscreen_to_send {
            self.controller.command(Command::Fullscreen {
                on,
                causal: Causal::default(),
            });
            if let Some(pending) = &mut self.fullscreen_pending {
                pending.command_sent = true;
            }
        }
    }

    fn new_tab(&mut self) {
        self.focus_new_tab = true;
        self.controller.command(Command::Tab {
            action: "new".to_owned(),
            id: 0,
            causal: Causal::default(),
        });
    }

    fn close_active_tab(&mut self) {
        let id = self
            .controller
            .snapshot
            .tabs
            .iter()
            .find(|tab| tab.active)
            .map_or(0, |tab| i32::try_from(tab.id).unwrap_or_default());
        self.close_tab(id);
    }

    fn close_tab(&mut self, id: i32) {
        if self.controller.snapshot.tabs.len() <= 1 {
            self.controller.navigate("about:blank#surf-new");
            return;
        }
        self.controller.command(Command::Tab {
            action: "close".to_owned(),
            id,
            causal: Causal::default(),
        });
    }

    fn reload_or_stop(&mut self) {
        self.controller
            .command(if self.controller.snapshot.loading {
                Command::Stop {
                    causal: Causal::default(),
                }
            } else {
                Command::Reload {
                    causal: Causal::default(),
                }
            });
    }

    fn set_fullscreen_command(&mut self, on: bool) {
        self.fullscreen_request = Some(on);
        self.fullscreen_pending = Some(PendingFullscreen {
            on,
            started: Instant::now(),
            previous_viewport: self.viewport_committed,
            command_sent: false,
        });
    }

    fn set_fullscreen(&mut self, window: &Window, on: bool) {
        window.set_fullscreen(on.then(|| Fullscreen::Borderless(None)));
        self.fullscreen = on;
    }

    fn update_window_size(&mut self, window: &Window) {
        if let Some(request) = self.window_size_request.take() {
            if self.fullscreen {
                self.controller
                    .browser
                    .toast("Exit fullscreen before applying a device size");
            } else {
                let _ = window.request_inner_size(LogicalSize::new(
                    f64::from(request.size[0]),
                    f64::from(request.size[1]),
                ));
                self.pending_window_size = Some(PendingWindowSize {
                    request,
                    started: Instant::now(),
                    viewport: None,
                });
            }
        }

        let current = logical_window_size(window);
        let viewport_to_send = self.pending_window_size.as_ref().and_then(|pending| {
            (pending.viewport.is_none()
                && window_size_matches(current, pending.request.size)
                && self.controller.connected)
                .then(|| viewport_for_window(current, window.scale_factor()))
        });
        if let Some(candidate) = viewport_to_send
            && let Some(viewport) = self.controller.force_viewport(candidate.0, candidate.1)
        {
            self.viewport_candidate = Some(viewport);
            self.viewport_candidate_since = Instant::now();
            self.viewport_committed = Some(viewport);
            self.video.clear();
            self.page_input.reset();
            if let Some(pending) = &mut self.pending_window_size {
                pending.viewport = Some(viewport);
                pending.started = Instant::now();
            }
        }

        let result = self.pending_window_size.as_ref().and_then(|pending| {
            if let Some(viewport) = pending.viewport {
                let video_ready = self.controller.video_dimensions == Some(viewport)
                    && self.controller.last_frame_dimensions
                        == Some((viewport.0 as u32, viewport.1 as u32));
                if video_ready {
                    return Some(format!(
                        "{}: window {} x {} pt, browser {} x {} px",
                        pending.request.label,
                        pending.request.size[0],
                        pending.request.size[1],
                        viewport.0,
                        viewport.1
                    ));
                }
                if pending.started.elapsed() >= WINDOW_RESIZE_TIMEOUT {
                    let video = self.controller.video_dimensions.map_or_else(
                        || "none".to_owned(),
                        |(width, height)| format!("{width} x {height}"),
                    );
                    return Some(format!(
                        "Browser resize stalled: requested {} x {}, video {video}",
                        viewport.0, viewport.1
                    ));
                }
            } else if pending.started.elapsed() >= WINDOW_RESIZE_TIMEOUT {
                return Some(format!(
                    "Window manager kept {} x {} pt; float the window and apply again",
                    current[0], current[1]
                ));
            }
            None
        });
        if let Some(message) = result {
            self.pending_window_size = None;
            self.controller.browser.toast(message);
        }
    }

    fn apply_actions(&mut self, actions: Vec<Action>) {
        for action in actions {
            match action {
                Action::Command(command) => self.controller.command(command),
                Action::Navigate(url) => {
                    self.controller.navigate(&url);
                    self.panel = None;
                    self.page_focused = true;
                }
                Action::Inspect(endpoint, connect) => {
                    self.endpoint.clone_from(&endpoint);
                    self.controller.inspect(endpoint, connect);
                }
                Action::Pair(code) => {
                    let name = std::env::var("HOSTNAME").unwrap_or_else(|_| "Surf desktop".into());
                    self.controller.pair(code, name);
                }
                Action::ConfirmPairing => self.controller.confirm_pairing(),
                Action::Connect => self.controller.connect(),
                Action::Disconnect => {
                    self.controller.disconnect();
                    self.panel = None;
                }
                Action::Dark(on) => {
                    self.controller.set_dark_mode(on);
                    self.theme_request = Some(on);
                }
                Action::Mobile(on) => self.controller.set_mobile_mode(on),
                Action::Forget(server_id) => match self.controller.forget_server(&server_id) {
                    Ok(()) => self.controller.browser.toast("Server forgotten"),
                    Err(error) => self.controller.browser.toast(error),
                },
                Action::Download(name) => {
                    let destination = download_destination(&name);
                    self.controller.request_download(name, destination);
                }
            }
        }
    }
}

fn overlay_flags() -> WindowFlags {
    WindowFlags::NO_SAVED_SETTINGS | WindowFlags::NO_COLLAPSE
}

fn logical_window_size(window: &Window) -> [u32; 2] {
    let size: LogicalSize<f64> = window.inner_size().to_logical(window.scale_factor());
    [
        size.width.round().max(1.0) as u32,
        size.height.round().max(1.0) as u32,
    ]
}

fn viewport_for_window(window: [u32; 2], scale: f64) -> (i32, i32) {
    (
        (f64::from(window[0]) * scale).round().max(64.0) as i32,
        ((f64::from(window[1]) - f64::from(BAR_HEIGHT)) * scale)
            .round()
            .max(64.0) as i32,
    )
}

fn window_size_matches(current: [u32; 2], requested: [u32; 2]) -> bool {
    current[0].abs_diff(requested[0]) <= 1 && current[1].abs_diff(requested[1]) <= 1
}

fn toggle(current: Option<Panel>, panel: Panel) -> Option<Panel> {
    (current != Some(panel)).then_some(panel)
}

fn compact_button(ui: &Ui, label: &str, tooltip: &str, size: [f32; 2]) -> bool {
    let clicked = ui.button_with_size(label, size);
    if ui.is_item_hovered() {
        ui.tooltip_text(tooltip);
    }
    clicked
}

fn full_button(ui: &Ui, label: &str, width: f32) -> bool {
    ui.button_with_size(label, [width, 20.0])
}

fn item_rect(ui: &Ui) -> [f32; 4] {
    let min = ui.item_rect_min();
    let max = ui.item_rect_max();
    [min[0], min[1], max[0], max[1]]
}

fn small_overlay(ui: &Ui, position: [f32; 2], text: &str) {
    ui.window(format!("##status-{text}"))
        .position(position, Condition::Always)
        .position_pivot([0.5, 0.0])
        .flags(overlay_flags() | WindowFlags::ALWAYS_AUTO_RESIZE | WindowFlags::NO_INPUTS)
        .build(|| ui.text(text));
}

fn metric(ui: &Ui, label: &str, value: String) {
    ui.group(|| {
        ui.text_disabled(label);
        ui.text(value);
    });
}

fn row_label(title: &str, url: &str, index: usize) -> String {
    let title = if title.trim().is_empty() {
        compact_address(url)
    } else {
        truncate(title, 42)
    };
    format!("{title}  {}##library-{index}", compact_address(url))
}

fn truncate(text: &str, maximum: usize) -> String {
    if text.chars().count() <= maximum {
        return text.to_owned();
    }
    let mut value = text
        .chars()
        .take(maximum.saturating_sub(1))
        .collect::<String>();
    value.push('…');
    value
}

fn format_bytes(bytes: i64) -> String {
    let bytes = bytes.max(0) as f64;
    if bytes >= 1024.0 * 1024.0 * 1024.0 {
        format!("{:.1} GB", bytes / (1024.0 * 1024.0 * 1024.0))
    } else if bytes >= 1024.0 * 1024.0 {
        format!("{:.1} MB", bytes / (1024.0 * 1024.0))
    } else if bytes >= 1024.0 {
        format!("{:.1} KB", bytes / 1024.0)
    } else {
        format!("{} B", bytes as u64)
    }
}

fn format_time(seconds: f64) -> String {
    if !seconds.is_finite() || seconds < 0.0 {
        return "0:00".to_owned();
    }
    let seconds = seconds.round() as u64;
    format!("{}:{:02}", seconds / 60, seconds % 60)
}

fn download_destination(name: &str) -> PathBuf {
    let directory = directories::UserDirs::new()
        .and_then(|dirs| dirs.download_dir().map(Path::to_owned))
        .or_else(|| std::env::current_dir().ok())
        .unwrap_or_else(|| PathBuf::from("."));
    let safe = Path::new(name)
        .file_name()
        .map_or_else(|| "download".into(), |value| value.to_owned());
    let candidate = directory.join(&safe);
    if !candidate.exists() {
        return candidate;
    }
    let source = Path::new(&safe);
    let stem = source.file_stem().unwrap_or_default().to_string_lossy();
    let extension = source.extension().map(|value| value.to_string_lossy());
    for index in 2..10_000 {
        let file = extension.as_ref().map_or_else(
            || format!("{stem} ({index})"),
            |extension| format!("{stem} ({index}).{extension}"),
        );
        let candidate = directory.join(file);
        if !candidate.exists() {
            return candidate;
        }
    }
    directory.join(format!("{stem}-surf-download"))
}

fn mouse_button_number(button: MouseButton) -> Option<u32> {
    match button {
        MouseButton::Left => Some(1),
        MouseButton::Middle => Some(2),
        MouseButton::Right => Some(3),
        MouseButton::Back => Some(8),
        MouseButton::Forward => Some(9),
        MouseButton::Other(number) => Some(u32::from(number)),
    }
}

fn modifiers_from_winit(modifiers: WinitModifiers) -> Modifiers {
    let state = modifiers.state();
    Modifiers {
        alt: state.alt_key(),
        control: state.control_key(),
        command: state.super_key(),
        shift: state.shift_key(),
    }
}

fn logical_character(key: &Key) -> Option<&str> {
    match key {
        Key::Character(value) => Some(value.as_str()),
        _ => None,
    }
}

fn plain_key_text(
    key: &Key,
    event_text: Option<&str>,
    modifiers: Modifiers,
    pressed: bool,
    composing: bool,
) -> Option<String> {
    if !pressed || modifiers.control || modifiers.alt || modifiers.command || composing {
        return None;
    }
    event_text
        .map(str::to_owned)
        .or_else(|| matches!(key, Key::Named(NamedKey::Space)).then(|| " ".to_owned()))
        .filter(|value| !value.is_empty() && !value.chars().all(char::is_control))
}

fn dom_key(event: &KeyEvent) -> (String, String, i32, bool) {
    let (key, key_code, named) = match &event.logical_key {
        Key::Character(value) => {
            let key_code = value
                .chars()
                .next()
                .filter(char::is_ascii)
                .map_or(0, |value| value.to_ascii_uppercase() as i32);
            (value.to_string(), key_code, false)
        }
        Key::Named(named) => {
            let (name, legacy) = named_key(*named);
            (name.to_owned(), legacy, true)
        }
        Key::Dead(value) => (
            value.map_or("Dead".to_owned(), |value| value.to_string()),
            0,
            true,
        ),
        Key::Unidentified(_) => ("Unidentified".to_owned(), 0, true),
    };
    let code = match event.physical_key {
        PhysicalKey::Code(code) => format!("{code:?}"),
        PhysicalKey::Unidentified(_) => "Unidentified".to_owned(),
    };
    (key, code, key_code, named)
}

fn named_key(key: NamedKey) -> (&'static str, i32) {
    match key {
        NamedKey::ArrowDown => ("ArrowDown", 40),
        NamedKey::ArrowLeft => ("ArrowLeft", 37),
        NamedKey::ArrowRight => ("ArrowRight", 39),
        NamedKey::ArrowUp => ("ArrowUp", 38),
        NamedKey::Escape => ("Escape", 27),
        NamedKey::Tab => ("Tab", 9),
        NamedKey::Backspace => ("Backspace", 8),
        NamedKey::Enter => ("Enter", 13),
        NamedKey::Space => (" ", 32),
        NamedKey::Insert => ("Insert", 45),
        NamedKey::Delete => ("Delete", 46),
        NamedKey::Home => ("Home", 36),
        NamedKey::End => ("End", 35),
        NamedKey::PageUp => ("PageUp", 33),
        NamedKey::PageDown => ("PageDown", 34),
        NamedKey::F1 => ("F1", 112),
        NamedKey::F2 => ("F2", 113),
        NamedKey::F3 => ("F3", 114),
        NamedKey::F4 => ("F4", 115),
        NamedKey::F5 => ("F5", 116),
        NamedKey::F6 => ("F6", 117),
        NamedKey::F7 => ("F7", 118),
        NamedKey::F8 => ("F8", 119),
        NamedKey::F9 => ("F9", 120),
        NamedKey::F10 => ("F10", 121),
        NamedKey::F11 => ("F11", 122),
        NamedKey::F12 => ("F12", 123),
        _ => ("Unidentified", 0),
    }
}

fn utf16_offset(text: &str, bytes: usize) -> i32 {
    let bytes = bytes.min(text.len());
    let boundary = (0..=bytes)
        .rev()
        .find(|index| text.is_char_boundary(*index))
        .unwrap_or(0);
    i32::try_from(text[..boundary].encode_utf16().count()).unwrap_or(i32::MAX)
}

#[cfg(test)]
mod tests {
    use super::{
        DEVICE_PRESETS, Key, Modifiers, NamedKey, format_bytes, plain_key_text, truncate,
        utf16_offset, viewport_for_window, window_size_matches,
    };

    #[test]
    fn compact_helpers_are_unicode_and_size_safe() {
        assert_eq!(truncate("abcdefgh", 5), "abcd…");
        assert_eq!(truncate("aé日", 3), "aé日");
        assert_eq!(utf16_offset("a😀b", 5), 3);
        assert_eq!(format_bytes(1_048_576), "1.0 MB");
    }

    #[test]
    fn named_space_is_inserted_as_text() {
        assert_eq!(
            plain_key_text(
                &Key::Named(NamedKey::Space),
                None,
                Modifiers::default(),
                true,
                false,
            ),
            Some(" ".to_owned())
        );
    }

    #[test]
    fn device_presets_use_uikit_points_and_swap_orientation() {
        let classic_ipad = DEVICE_PRESETS
            .iter()
            .find(|preset| preset.label.starts_with("iPad /"))
            .expect("classic iPad preset");
        assert_eq!(classic_ipad.size(false), [768, 1024]);
        assert_eq!(classic_ipad.size(true), [1024, 768]);
        assert_eq!(DEVICE_PRESETS[0].size(false), [320, 480]);
    }

    #[test]
    fn compositor_rounding_allows_one_logical_point() {
        assert!(window_size_matches([767, 1025], [768, 1024]));
        assert!(!window_size_matches([766, 1024], [768, 1024]));
    }

    #[test]
    fn preset_window_maps_to_the_page_below_chrome() {
        assert_eq!(viewport_for_window([768, 1024], 1.0), (768, 982));
        assert_eq!(viewport_for_window([375, 667], 2.0), (750, 1250));
    }
}
