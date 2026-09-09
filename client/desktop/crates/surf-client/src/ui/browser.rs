use super::*;
use imgui::{StyleColor, StyleVar};

impl DesktopApp {
    pub(super) fn draw_browser_setup(&mut self, ui: &Ui) {
        let Some(mode) = self.controller.browser_mode.clone() else {
            return;
        };
        self.address_editing = false;
        self.focus_address = false;
        self.page_focused = false;
        self.page_input.reset();
        if ui.is_key_pressed(imgui::Key::Escape) {
            self.browser_resume_confirmation = None;
        }
        let display = ui.io().display_size;
        let width = (display[0] - 32.0).min(440.0);
        let switching = matches!(mode.state.as_str(), "opening" | "resuming" | "starting");
        if self
            .browser_resume_confirmation
            .is_some_and(|(revision, _)| revision != mode.revision)
        {
            self.browser_resume_confirmation = None;
        }
        ui.window("##browser-setup-state")
            .position([display[0] * 0.5, display[1] * 0.5], Condition::Always)
            .position_pivot([0.5, 0.5])
            .size_constraints([width, 0.0], [width, display[1] - 24.0])
            .flags(WindowFlags::NO_TITLE_BAR | WindowFlags::NO_RESIZE | WindowFlags::NO_MOVE
                | WindowFlags::NO_SAVED_SETTINGS | WindowFlags::ALWAYS_AUTO_RESIZE)
            .build(|| {
                self.hit_regions.push(window_rect(ui));
                let start = ui.cursor_pos();
                imgui::Image::new(self.assets.mark(), [42.0, 42.0]).build(ui);
                ui.set_cursor_pos([start[0] + 54.0, start[1] + 2.0]);
                {
                    let _font = ui.push_font(ui.fonts().fonts()[3]);
                    ui.text_wrapped(if switching { "Switching browser" } else { "Browser setup" });
                }
                ui.set_cursor_pos([start[0] + 54.0, ui.cursor_pos()[1]]);
                widgets::description(ui, &mode.host);
                ui.set_cursor_pos([start[0], ui.cursor_pos()[1].max(start[1] + 52.0)]);
                ui.separator();
                ui.spacing();
                ui.text_wrapped(&mode.message);
                ui.spacing();
                if let Some((revision, force)) = self.browser_resume_confirmation {
                    ui.text_wrapped(if force {
                        "Force close Surf's browser? Recent settings and unsaved work may be lost."
                    } else {
                        "Close the browser on the computer and resume here? Pages reload; unsaved work and active transfers may be lost."
                    });
                    ui.spacing();
                    if widgets::primary_button(ui, if force { "Force close and resume" } else { "Close and resume" }, ui.content_region_avail()[0]) {
                        self.controller.command(Command::BrowserResume { revision, force });
                        self.browser_resume_confirmation = None;
                    }
                    if ui.button_with_size("Cancel", [ui.content_region_avail()[0], 28.0]) { self.browser_resume_confirmation = None; }
                } else if !switching {
                    if widgets::primary_button(ui, "Resume here…", ui.content_region_avail()[0]) { self.browser_resume_confirmation = Some((mode.revision, false)); }
                    if mode.can_force && ui.button_with_size("Force close…", [ui.content_region_avail()[0], 28.0]) {
                        self.browser_resume_confirmation = Some((mode.revision, true));
                    }
                }
                ui.spacing();
                if ui.button_with_size("Disconnect", [ui.content_region_avail()[0], 28.0]) { self.controller.disconnect(); }
            });
    }

    pub(super) fn draw_chrome(&mut self, ui: &Ui) {
        let [_, y, width, height] = self.layout.rail;
        let density = self.layout.density;
        let target = if self.address_editing { 1.0 } else { 0.0 };
        self.address_expansion = if self.preferences.reduce_motion {
            target
        } else {
            self.address_expansion
                + (target - self.address_expansion)
                    .clamp(-ui.io().delta_time / 0.18, ui.io().delta_time / 0.18)
        };
        let show_tabs =
            density == Density::Wide && self.address_expansion < 0.001 && !self.address_editing;
        let palette = Palette::new(self.controller.dark_mode);
        let geometry = (
            self.observed_tab,
            self.controller.snapshot.tabs.len(),
            width as u32,
        );
        let reveal = self.tabs_geometry != Some(geometry);
        self.tabs_geometry = Some(geometry);
        let _bg = ui.push_style_color(StyleColor::WindowBg, palette.rail);
        let _padding = ui.push_style_var(StyleVar::WindowPadding([6.0, 6.0]));
        let _rounding = ui.push_style_var(StyleVar::WindowRounding(0.0));
        ui.window("##surf-chrome")
            .position([0.0, y], Condition::Always)
            .size([width, height], Condition::Always)
            .flags(
                WindowFlags::NO_DECORATION
                    | WindowFlags::NO_MOVE
                    | WindowFlags::NO_SAVED_SETTINGS
                    | WindowFlags::NO_BRING_TO_FRONT_ON_FOCUS,
            )
            .build(|| {
                self.hit_regions.push(window_rect(ui));
                {
                    let _disabled = ui.begin_disabled(!self.controller.snapshot.can_go_back);
                    if icon_button(
                        ui,
                        &self.assets.ui_icons,
                        "back",
                        icon::BACK,
                        "Back · Alt+Left",
                    ) {
                        self.controller.command(Command::Back {
                            causal: Causal::default(),
                        });
                    }
                }
                if density != Density::Minimal {
                    ui.same_line();
                    let _disabled = ui.begin_disabled(!self.controller.snapshot.can_go_forward);
                    if icon_button(
                        ui,
                        &self.assets.ui_icons,
                        "forward",
                        icon::FORWARD,
                        "Forward · Alt+Right",
                    ) {
                        self.controller.command(Command::Forward {
                            causal: Causal::default(),
                        });
                    }
                }
                ui.same_line();
                // The editor never changes its parent or ID when tabs yield their space.
                let utilities = match density {
                    Density::Wide => 68.0,
                    Density::Compact => 102.0,
                    Density::Minimal => 68.0,
                };
                let available = ui.content_region_avail()[0] - utilities;
                let address_width = if density == Density::Wide {
                    let compact = (width * 0.32).clamp(240.0, 420.0).min(available);
                    let t = self.address_expansion;
                    compact + (available - compact) * (t * t * (3.0 - 2.0 * t))
                } else {
                    available
                };
                ui.group(|| {
                    let origin = ui.cursor_screen_pos();
                    let end = [origin[0] + address_width, origin[1] + 30.0];
                    ui.get_window_draw_list()
                        .add_rect(origin, end, palette.surface)
                        .filled(true)
                        .rounding(6.0)
                        .build();
                    ui.get_window_draw_list()
                        .add_rect(origin, end, palette.border)
                        .rounding(6.0)
                        .build();
                    let _frame = ui.push_style_color(StyleColor::FrameBg, [0.0; 4]);
                    let _button = ui.push_style_color(StyleColor::Button, [0.0; 4]);
                    let _border = ui.push_style_var(StyleVar::FrameBorderSize(0.0));
                    ui.set_next_item_width((address_width - 34.0).max(80.0));
                    if self.focus_address {
                        ui.set_keyboard_focus_here();
                        self.focus_address = false;
                    }
                    let previous = self.address.clone();
                    let mut compact = compact_address(&self.controller.snapshot.current_url);
                    if self
                        .controller
                        .snapshot
                        .current_url
                        .starts_with("about:blank")
                    {
                        compact.clear();
                    }
                    let was_editing = self.address_editing;
                    let submitted = ui
                        .input_text(
                            "##omnibox",
                            if was_editing {
                                &mut self.address
                            } else {
                                &mut compact
                            },
                        )
                        .hint("Search or enter address")
                        .read_only(!was_editing)
                        .auto_select_all(true)
                        .enter_returns_true(true)
                        .build();
                    self.omnibox_rect = item_rect(ui);
                    if ui.is_item_clicked() && !was_editing {
                        self.edit_address();
                    }
                    if was_editing {
                        if previous != self.address {
                            self.suggestion_index = None;
                            self.controller.suggest(self.address.clone());
                        }
                        let count = self.controller.browser.suggestions.len().min(8);
                        if count > 0 {
                            if ui.is_key_pressed(ImKey::DownArrow) {
                                self.suggestion_index =
                                    Some(self.suggestion_index.map_or(0, |n| (n + 1) % count));
                            }
                            if ui.is_key_pressed(ImKey::UpArrow) {
                                self.suggestion_index = Some(
                                    self.suggestion_index
                                        .map_or(count - 1, |n| (n + count - 1) % count),
                                );
                            }
                        }
                        if submitted {
                            let target = self
                                .suggestion_index
                                .and_then(|i| self.controller.browser.suggestions.get(i))
                                .map(|item| item.url.clone())
                                .unwrap_or_else(|| self.address.clone());
                            self.navigate_from_ui(&target);
                        }
                    }
                    ui.same_line();
                    let glyph = if self.controller.snapshot.loading {
                        icon::STOP
                    } else {
                        icon::RELOAD
                    };
                    if icon_button(
                        ui,
                        &self.assets.ui_icons,
                        "reload-stop",
                        glyph,
                        "Reload / stop · Ctrl+R",
                    ) {
                        self.reload_or_stop();
                    }
                });
                if show_tabs {
                    ui.same_line();
                    let tab_width = (ui.content_region_avail()[0] - 102.0).max(40.0);
                    ui.child_window("##inline-tabs")
                        .size([tab_width, 30.0])
                        .scroll_bar(false)
                        .flags(WindowFlags::HORIZONTAL_SCROLLBAR | WindowFlags::NO_SCROLLBAR)
                        .build(|| {
                            let _font = ui.push_font(ui.fonts().fonts()[1]);
                            let tabs = self.controller.snapshot.tabs.clone();
                            for (i, tab) in tabs.iter().enumerate() {
                                if i > 0 {
                                    ui.same_line();
                                }
                                let _id = ui.push_id(format!("tab-{}", tab.id));
                                let _bg = ui.push_style_color(StyleColor::Button, [0.0; 4]);
                                let _border = ui.push_style_var(StyleVar::FrameBorderSize(0.0));
                                let title = if tab.title.trim().is_empty() {
                                    compact_address(&tab.url)
                                } else {
                                    tab.title.clone()
                                };
                                let item_width = (tab_width / tabs.len().max(1) as f32)
                                    .clamp(142.0, 190.0)
                                    - 30.0;
                                if tab.active {
                                    let start = ui.cursor_screen_pos();
                                    let end = [start[0] + item_width + 30.0, start[1] + 30.0];
                                    ui.get_window_draw_list()
                                        .add_rect(start, end, palette.surface)
                                        .filled(true)
                                        .rounding(6.0)
                                        .build();
                                    ui.get_window_draw_list()
                                        .add_rect(start, end, palette.border)
                                        .rounding(6.0)
                                        .build();
                                }
                                let texture = self.assets.icon(&tab.icon);
                                let inset = if texture.is_some() { 28.0 } else { 8.0 };
                                let label =
                                    widgets::ellipsize(ui, &title, item_width - inset - 8.0);
                                let clicked = ui.button_with_size("##activate", [item_width, 30.0]);
                                let origin = ui.item_rect_min();
                                let draw = ui.get_window_draw_list();
                                if let Some(texture) = texture {
                                    draw.add_image(
                                        texture,
                                        [origin[0] + 6.0, origin[1] + 7.0],
                                        [origin[0] + 22.0, origin[1] + 23.0],
                                    )
                                    .build();
                                }
                                draw.add_text(
                                    [origin[0] + inset, origin[1] + 8.0],
                                    palette.text,
                                    &label,
                                );
                                drop(draw);
                                if reveal && tab.active {
                                    ui.set_scroll_here_x();
                                }
                                if clicked {
                                    self.activate_tab(tab.id);
                                }
                                if ui.is_item_hovered() {
                                    ui.tooltip_text(&title);
                                }
                                ui.same_line_with_spacing(0.0, 0.0);
                                if icon_button(
                                    ui,
                                    &self.assets.ui_icons,
                                    "close",
                                    icon::CLOSE,
                                    "Close tab",
                                ) {
                                    self.close_tab(i32::try_from(tab.id).unwrap_or_default());
                                }
                            }
                        });
                } else if density != Density::Wide {
                    ui.same_line();
                    if icon_button(
                        ui,
                        &self.assets.ui_icons,
                        "tabs",
                        icon::TABS,
                        &format!("Tabs · {} open", self.controller.snapshot.tabs.len()),
                    ) {
                        self.panel = toggle(self.panel, Panel::Tabs);
                        self.page_focused = false;
                    }
                }
                if density != Density::Minimal {
                    if density == Density::Wide {
                        ui.set_cursor_pos([width - 6.0 - if show_tabs { 98.0 } else { 64.0 }, 6.0]);
                    } else {
                        ui.same_line();
                    }
                    if icon_button(
                        ui,
                        &self.assets.ui_icons,
                        "new-tab",
                        icon::PLUS,
                        "New tab · Ctrl+T",
                    ) {
                        self.new_tab();
                    }
                }
                if show_tabs {
                    ui.same_line();
                    if icon_button(ui, &self.assets.ui_icons, "library", icon::BOOK, "Library") {
                        self.open_library();
                    }
                }
                ui.set_cursor_pos([width - 36.0, 6.0]);
                if icon_button(
                    ui,
                    &self.assets.ui_icons,
                    "more",
                    icon::MORE,
                    "Browser tools",
                ) {
                    self.panel = toggle(self.panel, Panel::More);
                    self.page_focused = false;
                }
                let edge = if self.preferences.bottom {
                    y
                } else {
                    y + height - 1.0
                };
                ui.get_window_draw_list()
                    .add_line([0.0, edge], [width, edge], palette.border)
                    .build();
                if self.controller.snapshot.loading {
                    let t = if self.preferences.reduce_motion {
                        0.5
                    } else {
                        (ui.time() as f32 * 0.7).fract()
                    };
                    let length = width * 0.2;
                    let start = (width + length) * t - length;
                    ui.get_window_draw_list()
                        .add_line(
                            [start.max(0.0), edge],
                            [(start + length).min(width), edge],
                            palette.accent,
                        )
                        .thickness(2.0)
                        .build();
                }
            });
    }

    fn activate_tab(&mut self, id: i64) {
        self.focus_new_tab = false;
        self.finish_address_edit();
        self.controller.command(Command::Tab {
            action: "select".into(),
            id: i32::try_from(id).unwrap_or_default(),
            causal: Causal::default(),
        });
    }

    pub(super) fn finish_address_edit(&mut self) {
        // Public ImGui focus API: returning to the remote page relinquishes local
        // navigation focus as well as the host's editing flag.
        unsafe {
            imgui::sys::igSetWindowFocus_Str(std::ptr::null());
        }
        self.address_editing = false;
        self.focus_address = false;
        self.suggestion_index = None;
        self.address
            .clone_from(&self.controller.snapshot.current_url);
        self.controller.clear_suggestions();
        self.page_focused = true;
        self.ui_wants_keyboard = false;
    }

    pub(super) fn navigate_from_ui(&mut self, target: &str) {
        self.controller.navigate(target);
        self.finish_address_edit();
        self.panel = None;
    }

    pub(super) fn open_library(&mut self) {
        self.controller.command(Command::Library {
            causal: Causal::default(),
        });
        self.controller.command(Command::Downloads {
            causal: Causal::default(),
        });
        self.panel = Some(Panel::Library);
        self.page_focused = false;
    }

    pub(super) fn draw_tabs(&mut self, ui: &Ui) {
        let tabs = self.controller.snapshot.tabs.clone();
        let desired = [360.0, (tabs.len() as f32 * 48.0 + 62.0).min(500.0)];
        let (position, size) =
            crate::layout::anchored(ui.io().display_size, self.layout.rail, desired);
        ui.window("Tabs###tab-switcher")
            .position(position, Condition::Always)
            .size(size, Condition::Always)
            .flags(overlay_flags() | WindowFlags::NO_TITLE_BAR | WindowFlags::NO_RESIZE)
            .build(|| {
                self.hit_regions.push(window_rect(ui));
                ui.child_window("##tab-rows").size([0.0, -38.0]).build(|| {
                    for tab in &tabs {
                        let _id = ui.push_id(format!("switch-{}", tab.id));
                        let width = (ui.content_region_avail()[0] - 34.0).max(40.0);
                        if row(
                            ui,
                            "activate",
                            &tab.title,
                            &compact_address(&tab.url),
                            tab.active,
                            width,
                        ) {
                            self.activate_tab(tab.id);
                            self.panel = None;
                        }
                        ui.same_line();
                        if icon_button(ui, &self.assets.ui_icons, "close", icon::CLOSE, "Close tab")
                        {
                            self.close_tab(i32::try_from(tab.id).unwrap_or_default());
                        }
                    }
                });
                if ui.button_with_size("New tab", [ui.content_region_avail()[0], 30.0]) {
                    self.new_tab();
                    self.panel = None;
                }
            });
    }

    pub(super) fn draw_suggestions(&mut self, ui: &Ui) {
        if !self.address_editing || self.controller.browser.suggestions.is_empty() {
            return;
        }
        let suggestions = self.controller.browser.suggestions.clone();
        let a = self.omnibox_rect;
        let (position, size) = crate::layout::anchored(
            ui.io().display_size,
            [a[0], a[1], a[2] - a[0], a[3] - a[1]],
            [
                (a[2] - a[0]).max(260.0),
                (suggestions.len().min(8) as f32 * 48.0 + 24.0).min(360.0),
            ],
        );
        let mut selected = None;
        ui.window("##suggestions")
            .position(position, Condition::Always)
            .size(size, Condition::Always)
            .flags(
                overlay_flags()
                    | WindowFlags::NO_TITLE_BAR
                    | WindowFlags::NO_FOCUS_ON_APPEARING
                    | WindowFlags::NO_BRING_TO_FRONT_ON_FOCUS
                    | WindowFlags::NO_NAV,
            )
            .build(|| {
                self.hit_regions.push(window_rect(ui));
                for (index, item) in suggestions.iter().take(8).enumerate() {
                    let title = if item.title.trim().is_empty() {
                        compact_address(&item.url)
                    } else {
                        item.title.clone()
                    };
                    if row(
                        ui,
                        &format!("suggestion-{index}"),
                        &title,
                        &compact_address(&item.url),
                        self.suggestion_index == Some(index),
                        ui.content_region_avail()[0],
                    ) {
                        selected = Some(item.url.clone());
                    }
                }
            });
        if let Some(url) = selected {
            self.navigate_from_ui(&url);
        }
    }
}
