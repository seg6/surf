use std::sync::Arc;
use std::time::Duration;

use eframe::egui::{
    self, Align, Color32, CornerRadius, FontData, FontDefinitions, FontFamily, FontId, Frame,
    Layout, Margin, RichText, Sense, Stroke, TextEdit, Vec2,
};
use surf_core::{Core, Event as CoreEvent, Frame as BinaryFrame, Snapshot, Tab};
use surf_protocol::{Causal, Command, Event as WireEvent};
use surf_session::{
    DiscoveredServer, FailureKind, PairingStatus, SavedServer, ServerInfo, SessionAction,
    SessionClient, SessionEvent, Storage,
};

const LUCIDE: &[u8] = include_bytes!("../../../../../native/client/Resources/Lucide.ttf");
const ICON_FONT: &str = "surf-lucide";

mod icon {
    pub const BACK: char = '\u{e06e}';
    pub const FORWARD: char = '\u{e06f}';
    pub const RELOAD: char = '\u{e145}';
    pub const PLUS: char = '\u{e13d}';
    pub const MORE: char = '\u{e0b6}';
    pub const LOCK: char = '\u{e531}';
}

fn main() -> eframe::Result {
    let options = eframe::NativeOptions {
        renderer: eframe::Renderer::Glow,
        viewport: egui::ViewportBuilder::default()
            .with_title("Surf")
            .with_inner_size([1180.0, 760.0])
            .with_min_inner_size([720.0, 480.0])
            .with_app_id("space.seg6.surf.client"),
        ..Default::default()
    };
    eframe::run_native(
        "Surf",
        options,
        Box::new(|creation| Ok(Box::new(SurfDesktop::new(creation)))),
    )
}

struct SurfDesktop {
    core: Core,
    snapshot: Snapshot,
    address: String,
    address_focused: bool,
    session: Option<SessionClient>,
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
    last_frame: String,
}

impl SurfDesktop {
    fn new(creation: &eframe::CreationContext<'_>) -> Self {
        install_fonts(&creation.egui_ctx);
        install_style(&creation.egui_ctx);
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
        let (session, saved_servers, status) = match Storage::system() {
            Ok(storage) => {
                let servers = storage.servers().unwrap_or_default();
                match SessionClient::spawn(storage) {
                    Ok(session) => (
                        Some(session),
                        servers,
                        "Choose or pair a Surf server".to_owned(),
                    ),
                    Err(error) => (None, servers, error.to_string()),
                }
            }
            Err(error) => (None, Vec::new(), error.to_string()),
        };
        Self {
            core,
            snapshot,
            address: String::new(),
            address_focused: false,
            session,
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
            last_frame: String::new(),
        }
    }

    fn refresh(&mut self) {
        self.snapshot = self.core.snapshot().expect("core snapshot stays valid");
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

    fn inspect(&mut self, endpoint: String) {
        self.endpoint = endpoint;
        self.inspected = None;
        self.pairing = None;
        self.paired = false;
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
                SessionEvent::Connected { info, .. } => {
                    self.connected = true;
                    self.status = format!("Connected securely to {}", info.name);
                    self.pairing = None;
                }
                SessionEvent::Reconnecting {
                    attempt,
                    maximum,
                    delay,
                    reason,
                } => {
                    self.connected = false;
                    self.status = format!(
                        "Connection interrupted. Retrying {attempt}/{maximum} in {:.1}s — {reason}",
                        delay.as_secs_f32()
                    );
                }
                SessionEvent::Control(event) => self.apply_control(event),
                SessionEvent::Disconnected(message) => {
                    self.connected = false;
                    self.status = message;
                    let _ = self.core.dispatch(&CoreEvent::Loading(false));
                    self.refresh();
                }
                SessionEvent::Failure(failure) => {
                    self.connected = false;
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
        if let Some(data) = self
            .session
            .as_ref()
            .and_then(SessionClient::take_latest_frame)
            && let Ok(frame) = BinaryFrame::parse(&data)
        {
            self.frames_received = self.frames_received.saturating_add(1);
            self.last_frame = format!(
                "{}×{} · generation {} · frame {}",
                frame.width, frame.height, frame.encoder_generation, frame.sequence
            );
        }
    }

    fn apply_control(&mut self, event: WireEvent) {
        let mapped = match event {
            WireEvent::Tabs { tabs } => Some(CoreEvent::Tabs(
                tabs.into_iter()
                    .map(|tab| Tab {
                        id: i64::from(tab.id),
                        title: tab.title,
                        url: tab.url,
                        icon: tab.icon,
                        active: tab.active,
                    })
                    .collect(),
            )),
            WireEvent::Url {
                url,
                starred,
                security,
            } => {
                if !self.address_focused {
                    self.address.clone_from(&url);
                }
                Some(CoreEvent::Url {
                    url,
                    security,
                    starred,
                })
            }
            WireEvent::HistoryState { back, fwd } => Some(CoreEvent::History {
                can_go_back: back,
                can_go_forward: fwd,
            }),
            WireEvent::Loading { on } => Some(CoreEvent::Loading(on)),
            WireEvent::Editable {
                on,
                show_keyboard,
                kind,
                rect,
            } => Some(CoreEvent::Editable {
                on,
                show_keyboard,
                kind,
                rect: rect.as_slice().try_into().ok(),
            }),
            WireEvent::Fullscreen { on } => Some(CoreEvent::Fullscreen(on)),
            WireEvent::Security { state } => Some(CoreEvent::Security(state)),
            WireEvent::Starred { on } => Some(CoreEvent::Starred(on)),
            WireEvent::PageFrame { source_seq } => Some(CoreEvent::PageFrame(source_seq)),
            _ => None,
        };
        if let Some(event) = mapped {
            if let Err(error) = self.core.dispatch(&event) {
                self.status = format!("portable core rejected server state: {error}");
            }
            self.refresh();
        }
    }

    fn chrome(&mut self, root: &mut egui::Ui) {
        egui::Panel::top("browser_chrome")
            .frame(
                Frame::new()
                    .fill(theme::CHROME)
                    .inner_margin(Margin::symmetric(14, 10))
                    .stroke(Stroke::new(1.0, theme::SEPARATOR)),
            )
            .show(root, |ui| {
                ui.horizontal(|ui| {
                    ui.spacing_mut().item_spacing.x = 6.0;
                    if chrome_icon(ui, icon::BACK, self.snapshot.can_go_back, "Back") {
                        self.send_command(Command::Back {
                            causal: Causal::default(),
                        });
                    }
                    if chrome_icon(ui, icon::FORWARD, self.snapshot.can_go_forward, "Forward") {
                        self.send_command(Command::Forward {
                            causal: Causal::default(),
                        });
                    }
                    if chrome_icon(
                        ui,
                        icon::RELOAD,
                        self.connected,
                        if self.snapshot.loading {
                            "Stop"
                        } else {
                            "Reload"
                        },
                    ) {
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
                    ui.add_space(6.0);

                    let address_width = (ui.available_width() - 122.0).max(220.0);
                    Frame::new()
                        .fill(theme::FIELD)
                        .corner_radius(CornerRadius::same(10))
                        .stroke(Stroke::new(1.0, theme::FIELD_BORDER))
                        .inner_margin(Margin::symmetric(12, 7))
                        .show(ui, |ui| {
                            ui.set_width(address_width);
                            ui.horizontal(|ui| {
                                if !self.address_focused {
                                    ui.label(
                                        RichText::new(icon::LOCK.to_string())
                                            .family(icon_family())
                                            .color(theme::MUTED),
                                    );
                                }
                                let hint = if self.address_focused {
                                    "Search or enter address"
                                } else {
                                    "Search the modern web"
                                };
                                let response = ui.add_sized(
                                    [ui.available_width(), 24.0],
                                    TextEdit::singleline(&mut self.address)
                                        .id_source("address")
                                        .hint_text(hint)
                                        .font(FontId::proportional(15.0))
                                        .frame(Frame::NONE),
                                );
                                self.address_focused = response.has_focus();
                                if response.lost_focus()
                                    && ui.input(|input| input.key_pressed(egui::Key::Enter))
                                {
                                    self.navigate();
                                }
                            });
                        });
                    ui.add_space(6.0);
                    if chrome_icon(ui, icon::PLUS, self.connected, "New tab") {
                        self.send_command(Command::Tab {
                            action: "new".to_owned(),
                            id: 0,
                            causal: Causal::default(),
                        });
                    }
                    chrome_icon(ui, icon::MORE, false, "Browser tools are coming next");
                });

                ui.add_space(8.0);
                ui.horizontal(|ui| {
                    let tabs = self.snapshot.tabs.clone();
                    for tab in tabs {
                        self.tab(ui, &tab);
                    }
                });
            });
    }

    fn tab(&mut self, ui: &mut egui::Ui, tab: &Tab) {
        let fill = if tab.active {
            theme::TAB_ACTIVE
        } else {
            theme::CHROME
        };
        let response = Frame::new()
            .fill(fill)
            .corner_radius(CornerRadius::same(8))
            .stroke(Stroke::new(
                1.0,
                if tab.active {
                    theme::TAB_BORDER
                } else {
                    Color32::TRANSPARENT
                },
            ))
            .inner_margin(Margin::symmetric(12, 7))
            .show(ui, |ui| {
                ui.set_min_width(160.0);
                ui.set_max_width(240.0);
                ui.label(RichText::new(&tab.title).size(13.5).color(if tab.active {
                    theme::TEXT
                } else {
                    theme::MUTED
                }));
            })
            .response
            .interact(Sense::click());
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

    fn content(&mut self, root: &mut egui::Ui) {
        egui::CentralPanel::default()
            .frame(
                Frame::new()
                    .fill(theme::BACKGROUND)
                    .inner_margin(Margin::same(0)),
            )
            .show(root, |ui| {
                let available = ui.available_rect_before_wrap();
                let painter = ui.painter();
                painter.rect_filled(available, CornerRadius::ZERO, theme::BACKGROUND);

                if self.connected {
                    let surface = available.shrink(18.0);
                    painter.rect_filled(surface, CornerRadius::same(12), Color32::BLACK);
                    painter.rect_stroke(
                        surface,
                        CornerRadius::same(12),
                        Stroke::new(1.0, theme::SEPARATOR),
                        egui::StrokeKind::Inside,
                    );
                    let label = if self.frames_received == 0 {
                        "Secure stream connected · waiting for the first frame".to_owned()
                    } else {
                        format!(
                            "{} frames received · {}",
                            self.frames_received, self.last_frame
                        )
                    };
                    painter.text(
                        surface.left_top() + Vec2::new(16.0, 16.0),
                        egui::Align2::LEFT_TOP,
                        label,
                        FontId::proportional(13.0),
                        theme::MUTED,
                    );
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
                painter.rect_filled(card, CornerRadius::same(18), theme::SURFACE);
                painter.rect_stroke(
                    card,
                    CornerRadius::same(18),
                    Stroke::new(1.0, theme::SEPARATOR),
                    egui::StrokeKind::Inside,
                );
                ui.scope_builder(egui::UiBuilder::new().max_rect(card.shrink(30.0)), |ui| {
                    ui.with_layout(Layout::top_down(Align::Min), |ui| {
                        ui.vertical_centered(|ui| {
                            ui.label(RichText::new("Surf").size(31.0).strong().color(theme::TEXT));
                            ui.label(
                                RichText::new("Pair once. Browse through a faster machine.")
                                    .size(14.0)
                                    .color(theme::MUTED),
                            );
                        });
                        ui.add_space(18.0);
                        let mut chosen_endpoint = None;
                        if !self.saved_servers.is_empty() {
                            ui.label(
                                RichText::new("PAIRED SERVERS")
                                    .size(10.0)
                                    .color(theme::MUTED),
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
                            ui.label(RichText::new("NEARBY").size(10.0).color(theme::MUTED));
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
                            self.inspect(endpoint);
                        }
                        ui.label(
                            RichText::new("Server address")
                                .size(12.0)
                                .color(theme::MUTED),
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
                            self.inspect(self.endpoint.clone());
                        }

                        if let Some(pairing) = self.pairing.clone() {
                            ui.add_space(12.0);
                            Frame::new()
                                .fill(theme::ACCENT_SOFT)
                                .corner_radius(CornerRadius::same(10))
                                .inner_margin(Margin::symmetric(14, 12))
                                .show(ui, |ui| {
                                    ui.set_width(ui.available_width());
                                    ui.label(
                                        RichText::new(pairing.phrase)
                                            .size(17.0)
                                            .strong()
                                            .color(theme::ACCENT_TEXT),
                                    );
                                    ui.label(
                                        RichText::new(
                                            "Confirm only if the server shows the same words.",
                                        )
                                        .size(12.0)
                                        .color(theme::MUTED),
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
                                    .color(theme::TEXT),
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
                            ui.label(RichText::new(note).size(11.0).color(theme::MUTED));
                            ui.add_space(4.0);
                        }
                        Frame::new()
                            .fill(theme::FIELD)
                            .corner_radius(CornerRadius::same(9))
                            .inner_margin(Margin::symmetric(12, 9))
                            .show(ui, |ui| {
                                ui.set_width(ui.available_width());
                                ui.label(
                                    RichText::new(&self.status).size(12.0).color(theme::MUTED),
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
        if context.input(|input| input.modifiers.command && input.key_pressed(egui::Key::L)) {
            context.memory_mut(|memory| memory.request_focus(egui::Id::new("address")));
        }
        self.chrome(ui);
        self.content(ui);
        context.request_repaint_after(Duration::from_millis(16));
    }
}

fn chrome_icon(ui: &mut egui::Ui, glyph: char, enabled: bool, label: &str) -> bool {
    let text = RichText::new(glyph.to_string())
        .family(icon_family())
        .size(18.0)
        .color(if enabled {
            theme::TEXT
        } else {
            theme::DISABLED
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

fn install_style(context: &egui::Context) {
    context.set_theme(egui::ThemePreference::Dark);
    let mut style = (*context.style_of(egui::Theme::Dark)).clone();
    style.spacing.item_spacing = Vec2::new(8.0, 8.0);
    style.visuals.dark_mode = true;
    style.visuals.window_fill = theme::SURFACE;
    style.visuals.panel_fill = theme::BACKGROUND;
    style.visuals.override_text_color = Some(theme::TEXT);
    style.visuals.selection.bg_fill = theme::ACCENT;
    style.visuals.selection.stroke = Stroke::new(1.0, theme::ACCENT_TEXT);
    context.set_style_of(egui::Theme::Dark, style);
}

mod theme {
    use eframe::egui::Color32;

    pub const BACKGROUND: Color32 = Color32::from_rgb(17, 19, 23);
    pub const CHROME: Color32 = Color32::from_rgb(25, 28, 34);
    pub const FIELD: Color32 = Color32::from_rgb(38, 42, 49);
    pub const FIELD_BORDER: Color32 = Color32::from_rgb(58, 64, 74);
    pub const SURFACE: Color32 = Color32::from_rgb(27, 30, 36);
    pub const TAB_ACTIVE: Color32 = Color32::from_rgb(42, 47, 55);
    pub const TAB_BORDER: Color32 = Color32::from_rgb(75, 83, 95);
    pub const SEPARATOR: Color32 = Color32::from_rgb(49, 54, 63);
    pub const TEXT: Color32 = Color32::from_rgb(238, 241, 246);
    pub const MUTED: Color32 = Color32::from_rgb(158, 166, 179);
    pub const DISABLED: Color32 = Color32::from_rgb(89, 95, 105);
    pub const ACCENT: Color32 = Color32::from_rgb(74, 157, 209);
    pub const ACCENT_SOFT: Color32 = Color32::from_rgb(31, 61, 76);
    pub const ACCENT_TEXT: Color32 = Color32::from_rgb(142, 211, 232);
}
