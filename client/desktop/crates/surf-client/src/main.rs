use std::sync::Arc;

use eframe::egui::{
    self, Align, Color32, CornerRadius, FontData, FontDefinitions, FontFamily, FontId, Frame,
    Layout, Margin, RichText, Sense, Stroke, TextEdit, Vec2,
};
use surf_core::{Core, Event, Snapshot, Tab};

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
}

impl SurfDesktop {
    fn new(creation: &eframe::CreationContext<'_>) -> Self {
        install_fonts(&creation.egui_ctx);
        install_style(&creation.egui_ctx);
        let mut core = Core::new().expect("portable Surf core initializes");
        core.dispatch(&Event::Tabs(vec![Tab {
            id: 1,
            title: "New Tab".to_owned(),
            url: "about:blank#surf-new".to_owned(),
            icon: String::new(),
            active: true,
        }]))
        .expect("initial desktop tab is valid");
        core.dispatch(&Event::Url {
            url: "about:blank#surf-new".to_owned(),
            security: String::new(),
            starred: false,
        })
        .expect("initial desktop URL is valid");
        let snapshot = core.snapshot().expect("initial snapshot is valid");
        Self {
            core,
            snapshot,
            address: String::new(),
            address_focused: false,
        }
    }

    fn refresh(&mut self) {
        self.snapshot = self.core.snapshot().expect("core snapshot stays valid");
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
                    chrome_icon(ui, icon::BACK, self.snapshot.can_go_back, "Back");
                    chrome_icon(ui, icon::FORWARD, self.snapshot.can_go_forward, "Forward");
                    chrome_icon(ui, icon::RELOAD, true, "Reload");
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
                    chrome_icon(ui, icon::PLUS, true, "New tab");
                    chrome_icon(ui, icon::MORE, true, "Browser tools");
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
            let _ = self.core.dispatch(&Event::Tabs(next));
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
        let _ = self.core.dispatch(&Event::Url {
            url: url.clone(),
            security: "secure".to_owned(),
            starred: false,
        });
        let _ = self.core.dispatch(&Event::Loading(true));
        self.address = url;
        self.refresh();
    }

    fn content(&self, root: &mut egui::Ui) {
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

                let center = available.center();
                let card = egui::Rect::from_center_size(center, Vec2::new(430.0, 220.0))
                    .intersect(available.shrink(24.0));
                painter.rect_filled(card, CornerRadius::same(18), theme::SURFACE);
                painter.rect_stroke(
                    card,
                    CornerRadius::same(18),
                    Stroke::new(1.0, theme::SEPARATOR),
                    egui::StrokeKind::Inside,
                );
                ui.scope_builder(egui::UiBuilder::new().max_rect(card.shrink(32.0)), |ui| {
                    ui.with_layout(Layout::top_down(Align::Center), |ui| {
                        ui.add_space(18.0);
                        ui.label(RichText::new("Surf").size(34.0).strong().color(theme::TEXT));
                        ui.add_space(8.0);
                        ui.label(
                            RichText::new("Your modern browser, rendered somewhere faster.")
                                .size(15.0)
                                .color(theme::MUTED),
                        );
                        ui.add_space(24.0);
                        Frame::new()
                            .fill(theme::ACCENT_SOFT)
                            .corner_radius(CornerRadius::same(10))
                            .inner_margin(Margin::symmetric(16, 10))
                            .show(ui, |ui| {
                                ui.label(
                                    RichText::new("Choose or pair a Surf server")
                                        .size(14.0)
                                        .color(theme::ACCENT_TEXT),
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
        if context.input(|input| input.modifiers.command && input.key_pressed(egui::Key::L)) {
            context.memory_mut(|memory| memory.request_focus(egui::Id::new("address")));
        }
        self.chrome(ui);
        self.content(ui);
    }
}

fn chrome_icon(ui: &mut egui::Ui, glyph: char, enabled: bool, label: &str) {
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
    .on_hover_text(label);
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
