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

const BAR_HEIGHT: f32 = 32.0;
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
    pub const BACK: &str = "<";
    pub const FORWARD: &str = ">";
    pub const RELOAD: &str = "R";
    pub const STOP: &str = "X";
    pub const CLOSE: &str = "x";
    pub const PLUS: &str = "+";
    pub const MORE: &str = "...";
    pub const STAR: &str = "*";
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
    controller: ClientController,
    video: VideoSurface,
    page_input: PageInput,
    page_rect: PageRect,
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
    observed_url: String,
    viewport_candidate: Option<(i32, i32)>,
    viewport_candidate_since: Instant,
    viewport_committed: Option<(i32, i32)>,
    render_diagnostics: RenderDiagnostics,
    smoke: SmokeState,
}

impl DesktopApp {
    pub fn new(gl: &glow::Context) -> Result<Self, String> {
        Ok(Self {
            controller: ClientController::new()?,
            video: VideoSurface::new(gl)?,
            page_input: PageInput::new(),
            page_rect: PageRect::default(),
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
            library_section: LibrarySection::History,
            dialog_input: String::new(),
            dialog_signature: String::new(),
            select_signature: String::new(),
            select_values: Vec::new(),
            file_picker: None,
            fullscreen: false,
            fullscreen_request: None,
            fullscreen_pending: None,
            theme_request: None,
            device_preset: 5,
            device_landscape: false,
            window_size_request: None,
            pending_window_size: None,
            ui_wants_pointer: false,
            ui_wants_keyboard: false,
            observed_url: String::new(),
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
        if current_url != self.observed_url {
            self.observed_url.clone_from(&current_url);
            self.address.clone_from(&current_url);
            self.address_editing = false;
            self.focus_address = false;
        }
        self.page_rect = PageRect {
            x: 0.0,
            y: if connected {
                f64::from(BAR_HEIGHT)
            } else {
                0.0
            },
            width: f64::from(display[0].max(1.0)),
            height: f64::from((display[1] - if connected { BAR_HEIGHT } else { 0.0 }).max(1.0)),
        };
        if connected {
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
        self.draw_suggestions(ui);
        self.draw_page_dialog(ui);
        self.draw_page_select(ui);
        self.draw_page_error(ui);
        self.draw_toast(ui);

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
        self.ui_wants_keyboard = ui.io().want_text_input;
    }

    fn draw_chrome(&mut self, ui: &Ui) {
        let display = ui.io().display_size;
        let narrow = display[0] < 520.0;
        let flags = WindowFlags::NO_DECORATION
            | WindowFlags::NO_MOVE
            | WindowFlags::NO_SAVED_SETTINGS
            | WindowFlags::NO_BRING_TO_FRONT_ON_FOCUS;
        ui.window("##surf-chrome")
            .position([0.0, 0.0], Condition::Always)
            .size([display[0], BAR_HEIGHT], Condition::Always)
            .bg_alpha(0.96)
            .flags(flags)
            .build(|| {
                if compact_button(ui, icon::BACK, "Back", [24.0, 20.0]) {
                    self.controller.command(Command::Back {
                        causal: Causal::default(),
                    });
                }
                if !narrow {
                    ui.same_line();
                    if compact_button(ui, icon::FORWARD, "Forward", [24.0, 20.0]) {
                        self.controller.command(Command::Forward {
                            causal: Causal::default(),
                        });
                    }
                }
                ui.same_line();
                let reload = if self.controller.snapshot.loading {
                    icon::STOP
                } else {
                    icon::RELOAD
                };
                if compact_button(ui, reload, "Reload / stop", [24.0, 20.0]) {
                    self.reload_or_stop();
                }
                ui.same_line();

                let tabs = &self.controller.snapshot.tabs;
                let active_title = tabs
                    .iter()
                    .find(|tab| tab.active)
                    .map(|tab| {
                        if tab.title.trim().is_empty() {
                            compact_address(&tab.url)
                        } else {
                            tab.title.clone()
                        }
                    })
                    .unwrap_or_else(|| "No tab".to_owned());
                let tab_width = if narrow {
                    (display[0] * 0.18).clamp(54.0, 76.0)
                } else {
                    (display[0] * 0.17).clamp(112.0, 180.0)
                };
                let tab_label = if narrow {
                    format!("[{}]##tabs", tabs.len())
                } else {
                    format!("{}  [{}]##tabs", truncate(&active_title, 22), tabs.len())
                };
                if ui.button_with_size(tab_label, [tab_width, 20.0]) {
                    self.panel = toggle(self.panel, Panel::Tabs);
                    self.page_focused = false;
                }
                if ui.is_item_hovered() {
                    ui.tooltip_text("Tabs");
                }
                ui.same_line();
                if compact_button(ui, icon::PLUS, "New tab", [24.0, 20.0]) {
                    self.new_tab();
                }
                ui.same_line();

                let utility_width = if narrow { 28.0 } else { 58.0 };
                let address_width = (ui.content_region_avail()[0] - utility_width).max(if narrow {
                    60.0
                } else {
                    120.0
                });
                ui.set_next_item_width(address_width);
                if self.address_editing {
                    if self.focus_address {
                        ui.set_keyboard_focus_here();
                        self.focus_address = false;
                    }
                    let previous = self.address.clone();
                    let submitted = ui
                        .input_text("##omnibox", &mut self.address)
                        .auto_select_all(true)
                        .enter_returns_true(true)
                        .build();
                    self.omnibox_rect = item_rect(ui);
                    if previous != self.address {
                        self.controller.command(Command::Suggest {
                            q: self.address.clone(),
                            offset: 0,
                            causal: Causal::default(),
                        });
                    }
                    if submitted {
                        let address = self.address.clone();
                        self.controller.navigate(&address);
                        self.address_editing = false;
                        self.page_focused = true;
                    } else if ui.is_key_pressed(ImKey::Escape) {
                        self.address_editing = false;
                        self.page_focused = true;
                    }
                } else {
                    let compact = compact_address(&self.controller.snapshot.current_url);
                    if ui.button_with_size(
                        format!("{compact}##omnibox-display"),
                        [address_width, 20.0],
                    ) {
                        self.edit_address();
                    }
                    self.omnibox_rect = item_rect(ui);
                    if ui.is_item_hovered() {
                        ui.tooltip_text(&self.controller.snapshot.current_url);
                    }
                }
                if !narrow {
                    ui.same_line();
                    let star = if self.controller.snapshot.starred {
                        "[*]".to_owned()
                    } else {
                        icon::STAR.to_string()
                    };
                    if compact_button(ui, &star, "Bookmark", [24.0, 20.0]) {
                        self.controller.command(Command::Bookmark {
                            causal: Causal::default(),
                        });
                    }
                }
                ui.same_line();
                if compact_button(ui, icon::MORE, "Browser tools", [24.0, 20.0]) {
                    self.panel = toggle(self.panel, Panel::More);
                    self.page_focused = false;
                }

                if self.controller.snapshot.loading {
                    let draw = ui.get_window_draw_list();
                    let t = (ui.time() as f32 * 0.7).fract();
                    let width = display[0] * 0.28;
                    let start = (display[0] + width) * t - width;
                    draw.add_line(
                        [start, BAR_HEIGHT - 1.0],
                        [(start + width).min(display[0]), BAR_HEIGHT - 1.0],
                        [0.325, 0.718, 0.824, 1.0],
                    )
                    .thickness(1.0)
                    .build();
                }
            });
        if self.controller.frames_received == 0 {
            small_overlay(
                ui,
                [display[0] * 0.5, PANEL_TOP + 8.0],
                "Waiting for video…",
            );
        }
    }

    fn draw_start(&mut self, ui: &Ui) {
        let display = ui.io().display_size;
        let width = (display[0] - 16.0).clamp(280.0, 450.0);
        let flags = WindowFlags::NO_COLLAPSE
            | WindowFlags::NO_RESIZE
            | WindowFlags::NO_SAVED_SETTINGS
            | WindowFlags::ALWAYS_AUTO_RESIZE;
        let mut actions = Vec::new();
        ui.window("Connect to Surf")
            .position([display[0] * 0.5, display[1] * 0.5], Condition::Always)
            .position_pivot([0.5, 0.5])
            .size_constraints([width, 0.0], [width, (display[1] - 16.0).max(220.0)])
            .flags(flags)
            .build(|| {
                ui.text("AVAILABLE COMPUTERS");
                ui.separator();
                let mut servers: Vec<(String, String)> = self
                    .controller
                    .saved_servers
                    .iter()
                    .map(|server| (server.name.clone(), server.endpoint.clone()))
                    .collect();
                for server in &self.controller.discovered_servers {
                    if !servers
                        .iter()
                        .any(|(_, endpoint)| endpoint == &server.endpoint)
                    {
                        servers.push((server.name.clone(), server.endpoint.clone()));
                    }
                }
                if servers.is_empty() {
                    ui.text_disabled("Searching the LAN…");
                }
                for (name, endpoint) in servers {
                    let label = format!(
                        "{}  {}##server-{endpoint}",
                        truncate(&name, 28),
                        endpoint.trim_start_matches("https://")
                    );
                    if ui.selectable_config(label).size([0.0, 26.0]).build() {
                        actions.push(Action::Inspect(endpoint, true));
                    }
                }
                ui.spacing();
                ui.set_next_item_width(ui.content_region_avail()[0] - 82.0);
                ui.input_text("##server-address", &mut self.endpoint)
                    .hint("Address or host name")
                    .build();
                ui.same_line();
                if ui.button_with_size("Add", [76.0, 0.0]) && !self.endpoint.trim().is_empty() {
                    actions.push(Action::Inspect(self.endpoint.trim().to_owned(), false));
                }

                if let Some(info) = &self.controller.inspected {
                    ui.separator();
                    ui.text(&info.name);
                    ui.same_line();
                    ui.text_disabled(self.controller.endpoint.trim_start_matches("https://"));
                    if let Some(pairing) = &self.controller.pairing {
                        ui.spacing();
                        ui.text("Compare these words with the server:");
                        ui.text_wrapped(&pairing.phrase);
                        if ui.button("Words match") {
                            actions.push(Action::ConfirmPairing);
                        }
                    } else if !self.controller.paired {
                        ui.set_next_item_width(180.0);
                        ui.input_text("##pair-code", &mut self.pairing_code)
                            .hint("Six-digit code")
                            .chars_decimal(true)
                            .build();
                        ui.same_line();
                        if ui.button("Pair") {
                            actions.push(Action::Pair(self.pairing_code.clone()));
                        }
                    } else if ui.button("Connect") {
                        actions.push(Action::Connect);
                    }
                }
                ui.separator();
                ui.text_wrapped(&self.controller.status);
                if let Some(note) = &self.controller.discovery_note {
                    ui.text_disabled(note);
                }
            });
        self.apply_actions(actions);
    }

    fn draw_panel(&mut self, ui: &Ui) {
        match self.panel {
            Some(Panel::Tabs) => self.draw_tabs(ui),
            Some(Panel::More) => self.draw_more(ui),
            Some(Panel::Library) => self.draw_library(ui),
            Some(Panel::Find) => self.draw_find(ui),
            Some(Panel::Reader) => self.draw_reader(ui),
            Some(Panel::Media) => self.draw_media(ui),
            Some(Panel::Settings) => self.draw_settings(ui),
            Some(Panel::Performance) => self.draw_performance(ui),
            Some(Panel::Files) => self.draw_files(ui),
            None => {}
        }
    }

    fn draw_tabs(&mut self, ui: &Ui) {
        let display = ui.io().display_size;
        let panel_width = (display[0] - 8.0).clamp(280.0, 340.0);
        let panel_x = if display[0] < 520.0 { 4.0 } else { 82.0 };
        let tabs = self.controller.snapshot.tabs.clone();
        let mut action = None;
        ui.window("Tabs##panel")
            .position([panel_x, PANEL_TOP], Condition::Always)
            .size_constraints(
                [panel_width, 0.0],
                [panel_width, (display[1] * 0.7).max(160.0)],
            )
            .flags(overlay_flags() | WindowFlags::ALWAYS_AUTO_RESIZE | WindowFlags::NO_TITLE_BAR)
            .build(|| {
                for (index, tab) in tabs.iter().enumerate() {
                    let title = if tab.title.trim().is_empty() {
                        compact_address(&tab.url)
                    } else {
                        tab.title.clone()
                    };
                    let selected = tab.active;
                    let row_width = (ui.content_region_avail()[0] - 30.0).max(80.0);
                    if ui
                        .selectable_config(format!("{}##tab-{index}", truncate(&title, 42)))
                        .selected(selected)
                        .size([row_width, 18.0])
                        .build()
                    {
                        action = Some(("select", tab.id));
                    }
                    if ui.is_item_hovered() {
                        ui.tooltip_text(&tab.url);
                    }
                    ui.same_line();
                    if ui.small_button(format!("{}##tab-close-{index}", icon::CLOSE)) {
                        action = Some(("close", tab.id));
                    }
                }
            });
        if let Some((action, id)) = action {
            let id = i32::try_from(id).unwrap_or_default();
            if action == "close" {
                self.close_tab(id);
            } else {
                self.controller.command(Command::Tab {
                    action: action.to_owned(),
                    id,
                    causal: Causal::default(),
                });
            }
            self.panel = None;
            self.page_focused = true;
        }
    }

    fn draw_more(&mut self, ui: &Ui) {
        let display = ui.io().display_size;
        let flags = overlay_flags() | WindowFlags::ALWAYS_AUTO_RESIZE | WindowFlags::NO_TITLE_BAR;
        let mut next = self.panel;
        ui.window("Tools##panel")
            .position([display[0], PANEL_TOP], Condition::Always)
            .position_pivot([1.0, 0.0])
            .size_constraints([220.0, 0.0], [220.0, display[1] - PANEL_TOP - 8.0])
            .flags(flags)
            .build(|| {
                if full_button(ui, "Library", 206.0) {
                    self.controller.command(Command::Library {
                        causal: Causal::default(),
                    });
                    self.controller.command(Command::Downloads {
                        causal: Causal::default(),
                    });
                    next = Some(Panel::Library);
                }
                if full_button(ui, "Find on page", 206.0) {
                    next = Some(Panel::Find);
                }
                if full_button(ui, "Reader", 206.0) {
                    self.controller.command(Command::Reader {
                        causal: Causal::default(),
                    });
                    next = None;
                }
                if full_button(ui, "Media", 206.0) {
                    self.controller.command(Command::MediaQuery {
                        causal: Causal::default(),
                    });
                    next = Some(Panel::Media);
                }
                ui.separator();
                if full_button(ui, "Performance", 206.0) {
                    next = Some(Panel::Performance);
                }
                if full_button(ui, "Settings", 206.0) {
                    next = Some(Panel::Settings);
                }
                if full_button(ui, "Fullscreen", 206.0) {
                    let on = !self.fullscreen;
                    self.set_fullscreen_command(on);
                    next = None;
                }
                ui.separator();
                if full_button(ui, "Disconnect", 206.0) {
                    self.controller.disconnect();
                    next = None;
                }
            });
        self.panel = next;
    }

    fn draw_library(&mut self, ui: &Ui) {
        let display = ui.io().display_size;
        let available_width = (display[0] - 16.0).max(280.0);
        let available_height = (display[1] - 16.0).max(220.0);
        let width = (display[0] * 0.72).clamp(280.0, 820.0).min(available_width);
        let height = (display[1] * 0.72)
            .clamp(220.0, 620.0)
            .min(available_height);
        let mut open = true;
        let mut actions = Vec::new();
        ui.window("Library")
            .position([display[0] * 0.5, display[1] * 0.5], Condition::Appearing)
            .position_pivot([0.5, 0.5])
            .size([width, height], Condition::Appearing)
            .size_constraints([280.0, 220.0], [available_width, available_height])
            .opened(&mut open)
            .flags(overlay_flags())
            .build(|| {
                for (section, label) in [
                    (LibrarySection::History, "History"),
                    (LibrarySection::Bookmarks, "Bookmarks"),
                    (LibrarySection::Downloads, "Downloads"),
                ] {
                    if section != LibrarySection::History {
                        ui.same_line();
                    }
                    let selected = self.library_section == section;
                    if ui.selectable_config(label).selected(selected).build() {
                        self.library_section = section;
                    }
                }
                ui.separator();
                ui.child_window("##library-list")
                    .size([0.0, 0.0])
                    .build(|| match self.library_section {
                        LibrarySection::History => {
                            let items = self.controller.browser.history.clone();
                            if items.is_empty() {
                                ui.text_disabled("No browsing history yet.");
                            }
                            for (index, item) in items.into_iter().enumerate() {
                                let label = row_label(&item.title, &item.url, index);
                                let row_width = (ui.content_region_avail()[0] - 30.0).max(120.0);
                                if ui.selectable_config(label).size([row_width, 24.0]).build() {
                                    actions.push(Action::Navigate(item.url.clone()));
                                }
                                ui.same_line();
                                if compact_button(
                                    ui,
                                    &format!("{}##hist-{index}", icon::CLOSE),
                                    "Remove",
                                    [25.0, 22.0],
                                ) {
                                    actions.push(Action::Command(Command::HistoryDelete {
                                        url: item.url,
                                        ts: item.ts,
                                        causal: Causal::default(),
                                    }));
                                    actions.push(Action::Command(Command::Library {
                                        causal: Causal::default(),
                                    }));
                                }
                            }
                        }
                        LibrarySection::Bookmarks => {
                            let items = self.controller.browser.bookmarks.clone();
                            if items.is_empty() {
                                ui.text_disabled("Bookmarks appear here.");
                            }
                            for (index, item) in items.into_iter().enumerate() {
                                let label = row_label(&item.title, &item.url, index);
                                let row_width = (ui.content_region_avail()[0] - 30.0).max(120.0);
                                if ui.selectable_config(label).size([row_width, 24.0]).build() {
                                    actions.push(Action::Navigate(item.url.clone()));
                                }
                                ui.same_line();
                                if compact_button(
                                    ui,
                                    &format!("{}##bookmark-{index}", icon::CLOSE),
                                    "Remove",
                                    [25.0, 22.0],
                                ) {
                                    actions.push(Action::Command(Command::BookmarkDelete {
                                        url: item.url,
                                        causal: Causal::default(),
                                    }));
                                    actions.push(Action::Command(Command::Library {
                                        causal: Causal::default(),
                                    }));
                                }
                            }
                        }
                        LibrarySection::Downloads => {
                            let items = self.controller.browser.downloads.clone();
                            if items.is_empty() {
                                ui.text_disabled("Downloads from this server appear here.");
                            }
                            for (index, item) in items.into_iter().enumerate() {
                                ui.text(truncate(&item.name, 58));
                                ui.same_line();
                                let progress = self
                                    .controller
                                    .browser
                                    .download_progress
                                    .get(&item.name)
                                    .map(|value| format!("{value}%"))
                                    .unwrap_or_else(|| format_bytes(item.size));
                                ui.text_disabled(progress);
                                ui.same_line();
                                if ui.small_button(format!("Save##download-{index}")) {
                                    actions.push(Action::Download(item.name.clone()));
                                }
                                ui.same_line();
                                if ui.small_button(format!("Remove##download-remove-{index}")) {
                                    actions.push(Action::Command(Command::DownloadDelete {
                                        name: item.name,
                                        causal: Causal::default(),
                                    }));
                                    actions.push(Action::Command(Command::Downloads {
                                        causal: Causal::default(),
                                    }));
                                }
                            }
                        }
                    });
            });
        if !open {
            self.panel = None;
        }
        self.apply_actions(actions);
    }

    fn draw_find(&mut self, ui: &Ui) {
        let display = ui.io().display_size;
        let width = (display[0] - 8.0).clamp(280.0, 330.0);
        let mut open = true;
        let mut command = None;
        ui.window("Find##panel")
            .position([display[0], PANEL_TOP], Condition::Always)
            .position_pivot([1.0, 0.0])
            .size([width, 0.0], Condition::Always)
            .opened(&mut open)
            .flags(overlay_flags() | WindowFlags::ALWAYS_AUTO_RESIZE)
            .build(|| {
                ui.set_next_item_width((ui.content_region_avail()[0] - 70.0).max(100.0));
                let previous = self.controller.browser.find_query.clone();
                let changed = ui
                    .input_text("##find", &mut self.controller.browser.find_query)
                    .hint("Find on page")
                    .build();
                if changed || previous != self.controller.browser.find_query {
                    command = Some(1);
                }
                ui.same_line();
                if ui.small_button("Up") {
                    command = Some(-1);
                }
                ui.same_line();
                if ui.small_button("Down") {
                    command = Some(1);
                }
                if let Some(found) = self.controller.browser.find_found {
                    ui.text_disabled(if found { "Match" } else { "No match" });
                }
            });
        if let Some(dir) = command {
            self.controller.command(Command::Find {
                q: self.controller.browser.find_query.clone(),
                dir,
                causal: Causal::default(),
            });
        }
        if !open {
            self.panel = None;
        }
    }

    fn draw_reader(&mut self, ui: &Ui) {
        let Some(reader) = self.controller.browser.reader.clone() else {
            return;
        };
        let display = ui.io().display_size;
        let width = (display[0] * 0.72)
            .clamp(280.0, 900.0)
            .min((display[0] - 16.0).max(280.0));
        let height = (display[1] * 0.78)
            .clamp(220.0, 720.0)
            .min((display[1] - 16.0).max(220.0));
        let mut open = true;
        let mut navigate = false;
        ui.window(if reader.title.is_empty() {
            "Reader"
        } else {
            &reader.title
        })
        .position([display[0] * 0.5, display[1] * 0.5], Condition::Appearing)
        .position_pivot([0.5, 0.5])
        .size([width, height], Condition::Appearing)
        .opened(&mut open)
        .flags(overlay_flags())
        .build(|| {
            ui.text_disabled(compact_address(&reader.url));
            ui.same_line();
            if ui.small_button("Open page") {
                navigate = true;
            }
            ui.separator();
            ui.child_window("##reader-text").size([0.0, 0.0]).build(|| {
                ui.text_wrapped(&reader.text);
            });
        });
        if navigate {
            self.controller.navigate(&reader.url);
            open = false;
        }
        if !open {
            self.controller.browser.reader = None;
            self.panel = None;
        }
    }

    fn draw_media(&mut self, ui: &Ui) {
        let display = ui.io().display_size;
        let width = (display[0] - 8.0).clamp(280.0, PANEL_WIDTH);
        let media = self.controller.browser.media.clone();
        let mut open = true;
        let mut actions = Vec::new();
        ui.window("Media")
            .position([display[0], PANEL_TOP], Condition::Always)
            .position_pivot([1.0, 0.0])
            .size_constraints([width, 0.0], [width, 260.0])
            .opened(&mut open)
            .flags(overlay_flags() | WindowFlags::ALWAYS_AUTO_RESIZE)
            .build(|| {
                if !media.available {
                    ui.text_disabled("No controllable media on this page.");
                    return;
                }
                ui.text(if media.title.is_empty() {
                    "Page media"
                } else {
                    &media.title
                });
                ui.text_disabled(format!(
                    "{} / {}",
                    format_time(media.current_time),
                    format_time(media.duration)
                ));
                if ui.button(if media.paused { "Play" } else { "Pause" }) {
                    actions.push(Action::Command(Command::MediaPlayPause {
                        causal: Causal::default(),
                    }));
                }
                ui.same_line();
                if ui.button(if media.muted { "Unmute" } else { "Mute" }) {
                    actions.push(Action::Command(Command::MediaMute {
                        causal: Causal::default(),
                    }));
                }
                let mut volume = media.volume.clamp(0.0, 1.0) as f32;
                ui.set_next_item_width(-1.0);
                if ui.slider("Volume", 0.0, 1.0, &mut volume) {
                    actions.push(Action::Command(Command::MediaVolume {
                        value: f64::from(volume),
                        causal: Causal::default(),
                    }));
                }
            });
        if !open {
            self.panel = None;
        }
        self.apply_actions(actions);
    }

    fn draw_settings(&mut self, ui: &Ui) {
        let display = ui.io().display_size;
        let width = (display[0] - 16.0).clamp(280.0, 470.0);
        let max_height = (display[1] - 16.0).max(220.0);
        let mut open = true;
        let mut actions = Vec::new();
        ui.window("Settings")
            .position([display[0] * 0.5, display[1] * 0.5], Condition::Appearing)
            .position_pivot([0.5, 0.5])
            .size([width, 0.0], Condition::Appearing)
            .size_constraints([width, 0.0], [width, max_height])
            .opened(&mut open)
            .flags(overlay_flags() | WindowFlags::ALWAYS_AUTO_RESIZE)
            .build(|| {
                ui.text_disabled("LINUX DEVICE WINDOW");
                ui.text_wrapped(
                    "Match an iPhone or iPad layout using UIKit points. Retina scale does not change the layout size.",
                );
                let preset = DEVICE_PRESETS[self.device_preset.min(DEVICE_PRESETS.len() - 1)];
                let size = preset.size(self.device_landscape);
                let preview = format!("{} — {} x {} pt", preset.label, size[0], size[1]);
                ui.set_next_item_width(-1.0);
                if let Some(_combo) = ui.begin_combo("##device-preset", preview) {
                    for (index, candidate) in DEVICE_PRESETS.iter().enumerate() {
                        let candidate_size = candidate.size(self.device_landscape);
                        let selected = index == self.device_preset;
                        if ui
                            .selectable_config(format!(
                                "{} — {} x {} pt",
                                candidate.label, candidate_size[0], candidate_size[1]
                            ))
                            .selected(selected)
                            .build()
                        {
                            self.device_preset = index;
                        }
                        if selected {
                            ui.set_item_default_focus();
                        }
                    }
                }
                if ui.radio_button_bool("Portrait", !self.device_landscape) {
                    self.device_landscape = false;
                }
                ui.same_line();
                if ui.radio_button_bool("Landscape", self.device_landscape) {
                    self.device_landscape = true;
                }
                let preset = DEVICE_PRESETS[self.device_preset.min(DEVICE_PRESETS.len() - 1)];
                let size = preset.size(self.device_landscape);
                if ui.button_with_size("Apply exact client size", [ui.content_region_avail()[0], 0.0]) {
                    self.window_size_request = Some(WindowSizeRequest {
                        label: preset.label,
                        size,
                    });
                }
                ui.text_disabled(format!(
                    "Current client area: {:.0} x {:.0} pt",
                    display[0], display[1]
                ));
                if let Some(pending) = &self.pending_window_size {
                    if let Some((width, height)) = pending.viewport {
                        ui.text_disabled(format!("Applying browser: {width} x {height} px…"));
                    } else {
                        ui.text_disabled("Applying window size…");
                    }
                } else if let Some((width, height)) = self.controller.video_dimensions {
                    ui.text_disabled(format!("Browser video: {width} x {height} px"));
                }
                ui.separator();
                ui.text_disabled("APPEARANCE");
                let mut dark = self.controller.dark_mode;
                if ui.checkbox("Dark interface and websites", &mut dark) {
                    actions.push(Action::Dark(dark));
                }
                let mut mobile = self.controller.mobile_mode;
                if ui.checkbox("Request mobile websites", &mut mobile) {
                    actions.push(Action::Mobile(mobile));
                }
                ui.separator();
                ui.text_disabled("BROWSING DATA");
                if ui.button("Clear history") {
                    actions.push(Action::Command(Command::Clear {
                        what: "history".to_owned(),
                        causal: Causal::default(),
                    }));
                }
                ui.separator();
                ui.text_disabled("SERVERS");
                let servers = self.controller.saved_servers.clone();
                for (index, server) in servers.into_iter().enumerate() {
                    ui.text(&server.name);
                    ui.same_line();
                    ui.text_disabled(server.endpoint.trim_start_matches("https://"));
                    ui.same_line();
                    if ui.small_button(format!("Forget##server-forget-{index}")) {
                        actions.push(Action::Forget(server.server_id));
                    }
                }
                ui.separator();
                ui.text_wrapped(&self.controller.status);
                if self.controller.connected && ui.button("Disconnect") {
                    actions.push(Action::Disconnect);
                }
                let server_version = self
                    .controller
                    .inspected
                    .as_ref()
                    .map_or("—", |info| info.version.as_str());
                ui.text_disabled(format!(
                    "Surf client {}  |  server {server_version}  |  core C99",
                    SURF_VERSION.trim()
                ));
            });
        if !open {
            self.panel = None;
        }
        self.apply_actions(actions);
    }

    fn draw_performance(&mut self, ui: &Ui) {
        let display = ui.io().display_size;
        let width = (display[0] - 8.0).clamp(280.0, 360.0);
        let report = self.controller.latest_diagnostics.unwrap_or_default();
        let media = self.controller.media_diagnostics();
        let mut open = true;
        ui.window("Performance")
            .position([display[0], PANEL_TOP], Condition::Always)
            .position_pivot([1.0, 0.0])
            .size([width, 0.0], Condition::Always)
            .opened(&mut open)
            .flags(overlay_flags() | WindowFlags::ALWAYS_AUTO_RESIZE)
            .build(|| {
                metric(ui, "PRESENT", format!("{:.1} fps", report.presentation_fps));
                ui.same_line();
                metric(ui, "DECODE", format!("{:.1} fps", report.decode_fps));
                ui.same_line();
                metric(ui, "DROP", format!("{:.1}%", report.drop_percent));
                ui.separator();
                ui.text(format!("decode       {:>7} us", report.decode_us));
                ui.text(format!("GPU upload   {:>7} us", report.upload_us));
                ui.text(format!("frame age    {:>7} us", report.frame_age_us));
                ui.text(format!("network      {:>7} us", report.network_us));
                ui.text(format!("round trip   {:>7} us", report.rtt_us));
                ui.text(format!(
                    "queues       {} / {} / {}",
                    report.encoded_video_depth, report.decoded_video_depth, report.audio_depth
                ));
                ui.separator();
                ui.text_disabled(format!(
                    "{:?}  |  {} gaps  |  {} decode errors  |  {} received",
                    report.health, report.sequence_gaps, report.decode_errors, media.ingress_frames
                ));
            });
        if !open {
            self.panel = None;
        }
    }

    fn draw_files(&mut self, ui: &Ui) {
        let Some(picker) = &mut self.file_picker else {
            self.panel = None;
            return;
        };
        let display = ui.io().display_size;
        let width = (display[0] * 0.72)
            .clamp(280.0, 800.0)
            .min((display[0] - 16.0).max(280.0));
        let height = (display[1] * 0.72)
            .clamp(220.0, 600.0)
            .min((display[1] - 16.0).max(220.0));
        let mut open = true;
        let mut complete: Option<Vec<PathBuf>> = None;
        ui.window("Choose file")
            .position([display[0] * 0.5, display[1] * 0.5], Condition::Appearing)
            .position_pivot([0.5, 0.5])
            .size([width, height], Condition::Appearing)
            .opened(&mut open)
            .flags(overlay_flags())
            .build(|| {
                if ui.small_button("Up")
                    && let Some(parent) = picker.directory.parent()
                {
                    picker.directory = parent.to_owned();
                    picker.selected.clear();
                }
                ui.same_line();
                ui.text_disabled(picker.directory.display().to_string());
                ui.separator();
                ui.child_window("##files").size([0.0, -31.0]).build(|| {
                    let mut entries = match fs::read_dir(&picker.directory) {
                        Ok(entries) => entries.flatten().collect::<Vec<_>>(),
                        Err(error) => {
                            picker.error = Some(error.to_string());
                            Vec::new()
                        }
                    };
                    entries.sort_by_key(|entry| {
                        let is_file = entry.file_type().map_or(true, |kind| kind.is_file());
                        (is_file, entry.file_name().to_string_lossy().to_lowercase())
                    });
                    for entry in entries {
                        let path = entry.path();
                        let is_dir = entry.file_type().is_ok_and(|kind| kind.is_dir());
                        let selected = picker.selected.contains(&path);
                        let prefix = if is_dir { "[dir] " } else { "" };
                        let clicked = ui
                            .selectable_config(format!(
                                "{prefix}{}##{}",
                                entry.file_name().to_string_lossy(),
                                path.display()
                            ))
                            .selected(selected)
                            .allow_double_click(true)
                            .build();
                        if !clicked {
                            continue;
                        }
                        if is_dir && ui.is_mouse_double_clicked(ImMouseButton::Left) {
                            picker.directory = path;
                            picker.selected.clear();
                            break;
                        }
                        if !is_dir {
                            if !picker.multiple {
                                picker.selected.clear();
                            }
                            if !picker.selected.insert(path.clone()) {
                                picker.selected.remove(&path);
                            }
                        }
                    }
                    if let Some(error) = &picker.error {
                        ui.text_disabled(error);
                    }
                });
                let choose_label = if picker.multiple {
                    "Upload selected"
                } else {
                    "Upload"
                };
                if ui.button(choose_label) && !picker.selected.is_empty() {
                    complete = Some(picker.selected.iter().cloned().collect());
                }
                ui.same_line();
                if ui.button("Cancel") {
                    complete = Some(Vec::new());
                }
            });
        if !open && complete.is_none() {
            complete = Some(Vec::new());
        }
        if let Some(paths) = complete {
            self.file_picker = None;
            self.panel = None;
            self.controller.choose_files(paths);
        }
    }

    fn draw_suggestions(&mut self, ui: &Ui) {
        if !self.address_editing || self.controller.browser.suggestions.is_empty() {
            return;
        }
        let display = ui.io().display_size;
        let width = (self.omnibox_rect[2] - self.omnibox_rect[0])
            .max(80.0)
            .min((display[0] - 8.0).max(80.0));
        let suggestions = self.controller.browser.suggestions.clone();
        let mut selected = None;
        ui.window("##suggestions")
            .position(
                [self.omnibox_rect[0], self.omnibox_rect[3] + 2.0],
                Condition::Always,
            )
            .size([width, 0.0], Condition::Always)
            .size_constraints([width, 0.0], [width, 260.0])
            // Suggestions are a visual/click target owned by the omnibox. They must not
            // become the active keyboard window when the first result arrives.
            .flags(
                overlay_flags()
                    | WindowFlags::ALWAYS_AUTO_RESIZE
                    | WindowFlags::NO_FOCUS_ON_APPEARING
                    | WindowFlags::NO_BRING_TO_FRONT_ON_FOCUS,
            )
            .build(|| {
                for (index, item) in suggestions.into_iter().take(8).enumerate() {
                    let title = if item.title.trim().is_empty() {
                        compact_address(&item.url)
                    } else {
                        item.title.clone()
                    };
                    if ui.selectable(format!(
                        "{}  {}##suggestion-{index}",
                        truncate(&title, 34),
                        compact_address(&item.url)
                    )) {
                        selected = Some(item.url);
                    }
                }
            });
        if let Some(url) = selected {
            self.controller.navigate(&url);
            self.address_editing = false;
            self.page_focused = true;
        }
    }

    fn draw_page_dialog(&mut self, ui: &Ui) {
        let Some(prompt) = self.controller.browser.dialog.clone() else {
            self.dialog_signature.clear();
            return;
        };
        let signature = format!("{}|{}|{}", prompt.kind, prompt.text, prompt.input);
        if self.dialog_signature != signature {
            self.dialog_signature = signature;
            self.dialog_input.clone_from(&prompt.input);
        }
        let display = ui.io().display_size;
        let width = (display[0] - 32.0).clamp(260.0, 520.0);
        let mut action = None;
        ui.window("This page says")
            .position([display[0] * 0.5, display[1] * 0.5], Condition::Always)
            .position_pivot([0.5, 0.5])
            .size_constraints(
                [width.min(360.0), 0.0],
                [width, (display[1] - 32.0).max(180.0)],
            )
            .flags(overlay_flags() | WindowFlags::ALWAYS_AUTO_RESIZE)
            .build(|| {
                ui.text_wrapped(&prompt.text);
                if prompt.kind == "prompt" {
                    ui.set_next_item_width(-1.0);
                    ui.input_text("##dialog-input", &mut self.dialog_input)
                        .enter_returns_true(true)
                        .build();
                }
                if prompt.kind != "alert" && ui.button("Cancel") {
                    action = Some(false);
                }
                if prompt.kind != "alert" {
                    ui.same_line();
                }
                if ui.button("OK") {
                    action = Some(true);
                }
            });
        if let Some(accept) = action {
            self.controller
                .reply_dialog(accept, self.dialog_input.clone());
        }
    }

    fn draw_page_select(&mut self, ui: &Ui) {
        let Some(prompt) = self.controller.browser.select.clone() else {
            self.select_signature.clear();
            self.select_values.clear();
            return;
        };
        if self.select_signature != prompt.id {
            self.select_signature.clone_from(&prompt.id);
            self.select_values.clone_from(&prompt.selected);
        }
        let display = ui.io().display_size;
        let position = prompt
            .rect
            .map_or([display[0] * 0.5, display[1] * 0.5], |rect| {
                [
                    (self.page_rect.x + rect[0] * self.page_rect.width) as f32,
                    (self.page_rect.y + (rect[1] + rect[3]) * self.page_rect.height) as f32,
                ]
            });
        let mut reply = None;
        ui.window("Choose##page-select")
            .position(position, Condition::Always)
            .position_pivot(if prompt.rect.is_some() {
                [0.0, 0.0]
            } else {
                [0.5, 0.5]
            })
            .size_constraints([220.0, 0.0], [420.0, display[1] * 0.6])
            .flags(overlay_flags() | WindowFlags::ALWAYS_AUTO_RESIZE)
            .build(|| {
                if !prompt.title.is_empty() {
                    ui.text_wrapped(&prompt.title);
                    ui.separator();
                }
                for (index, option) in prompt.options.iter().enumerate() {
                    let selected = self.select_values.get(index).copied().unwrap_or(false);
                    if ui
                        .selectable_config(format!("{}##select-{index}", option.label))
                        .selected(selected)
                        .disabled(option.disabled)
                        .build()
                    {
                        if prompt.multiple {
                            if let Some(value) = self.select_values.get_mut(index) {
                                *value = !*value;
                            }
                        } else {
                            reply = Some(vec![i32::try_from(index).unwrap_or(i32::MAX)]);
                        }
                    }
                }
                if prompt.multiple && ui.button("Choose") {
                    reply = Some(
                        self.select_values
                            .iter()
                            .enumerate()
                            .filter_map(|(index, selected)| {
                                selected.then_some(i32::try_from(index).unwrap_or(i32::MAX))
                            })
                            .collect(),
                    );
                }
                ui.same_line();
                if ui.button("Cancel") {
                    self.controller.reply_select(true, Vec::new());
                }
            });
        if let Some(indices) = reply {
            self.controller.reply_select(false, indices);
        }
    }

    fn draw_page_error(&mut self, ui: &Ui) {
        let Some(url) = self.controller.browser.page_error.clone() else {
            return;
        };
        let display = ui.io().display_size;
        let mut close = false;
        ui.window("Page unavailable")
            .position([display[0] * 0.5, display[1] * 0.5], Condition::Always)
            .position_pivot([0.5, 0.5])
            .flags(overlay_flags() | WindowFlags::ALWAYS_AUTO_RESIZE)
            .build(|| {
                ui.text_disabled(compact_address(&url));
                if ui.button("Close") {
                    close = true;
                }
            });
        if close {
            self.controller.browser.page_error = None;
        }
    }

    fn draw_toast(&self, ui: &Ui) {
        let Some(toast) = &self.controller.browser.toast else {
            return;
        };
        let display = ui.io().display_size;
        small_overlay(ui, [display[0] * 0.5, display[1] - 34.0], &toast.text);
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
        if let Some(frame) = self.video.render(gl, [x, y, width, height]) {
            self.controller
                .report_presented_frame(frame.generation, frame.source_sequence);
        }
        let diagnostics = self.video.diagnostics();
        self.render_diagnostics = diagnostics;
        self.controller.set_render_diagnostics(diagnostics);
        if let Some(error) = self.video.take_error() {
            self.controller.report_renderer_error(error);
        }
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
                if self.page_accepts_pointer(position.x, position.y) {
                    match self.page_input.motion(
                        position.x,
                        position.y,
                        self.page_rect,
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
                    let page_hit = self.page_accepts_pointer(x, y);
                    self.page_focused = page_hit;
                    if page_hit {
                        self.address_editing = false;
                        self.focus_address = false;
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
                        self.page_rect,
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
                        self.page_rect,
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
            WindowEvent::Focused(false) => self.page_focused = false,
            _ => {}
        }
    }

    fn handle_key(&mut self, event: &KeyEvent) {
        let pressed = event.state == ElementState::Pressed;
        if self.handle_shortcut(event, pressed) {
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
                        self.panel = Some(Panel::Find);
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
                self.address_editing = false;
                self.page_focused = true;
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
            && self.page_rect.contains(x, y)
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
        self.address = self.controller.snapshot.current_url.clone();
        self.observed_url
            .clone_from(&self.controller.snapshot.current_url);
        // Do not flash a result set left over from the previous edit session. A fresh
        // response will repopulate this after the user changes the query.
        self.controller.browser.suggestions.clear();
        self.address_editing = true;
        self.focus_address = true;
        self.page_focused = false;
    }

    fn update_viewport(&mut self, scale: f64) {
        let candidate = (
            (self.page_rect.width * scale).round().max(64.0) as i32,
            (self.page_rect.height * scale).round().max(64.0) as i32,
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
        assert_eq!(viewport_for_window([768, 1024], 1.0), (768, 992));
        assert_eq!(viewport_for_window([375, 667], 2.0), (750, 1270));
    }
}
