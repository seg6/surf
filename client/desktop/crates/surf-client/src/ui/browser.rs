use super::*;
use imgui::{StyleColor, StyleVar};

impl DesktopApp {
    pub(super) fn draw_chrome(&mut self, ui: &Ui) {
        let [_, y, width, height] = self.layout.rail;
        let density = self.layout.density;
        let palette = Palette::new(self.controller.dark_mode);
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
                if icon_button(ui, "back", icon::BACK, "Back · Alt+Left") {
                    self.controller.command(Command::Back {
                        causal: Causal::default(),
                    });
                }
                if density != Density::Minimal {
                    ui.same_line();
                    if icon_button(ui, "forward", icon::FORWARD, "Forward · Alt+Right") {
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
                let address_width = if density == Density::Wide && !self.address_editing {
                    (width * 0.32).clamp(240.0, 420.0).min(available)
                } else {
                    available
                };
                ui.group(|| {
                    let _frame = ui.push_style_color(StyleColor::FrameBg, palette.surface);
                    let _border = ui.push_style_var(StyleVar::FrameBorderSize(1.0));
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
                    if icon_button(ui, "reload-stop", glyph, "Reload / stop · Ctrl+R") {
                        self.reload_or_stop();
                    }
                });
                if density == Density::Wide && !self.address_editing {
                    ui.same_line();
                    let tab_width = (ui.content_region_avail()[0] - 102.0).max(40.0);
                    ui.child_window("##inline-tabs")
                        .size([tab_width, 30.0])
                        .scroll_bar(false)
                        .flags(WindowFlags::HORIZONTAL_SCROLLBAR)
                        .build(|| {
                            let tabs = self.controller.snapshot.tabs.clone();
                            for (i, tab) in tabs.iter().enumerate() {
                                if i > 0 {
                                    ui.same_line();
                                }
                                let _id = ui.push_id(format!("tab-{}", tab.id));
                                let _bg = ui.push_style_color(
                                    StyleColor::Button,
                                    if tab.active {
                                        palette.surface
                                    } else {
                                        palette.rail
                                    },
                                );
                                let _border =
                                    ui.push_style_var(StyleVar::FrameBorderSize(if tab.active {
                                        1.0
                                    } else {
                                        0.0
                                    }));
                                let title = if tab.title.trim().is_empty() {
                                    compact_address(&tab.url)
                                } else {
                                    tab.title.clone()
                                };
                                let label = widgets::ellipsize(ui, &title, 100.0);
                                if ui.button_with_size(format!("{label}###activate"), [114.0, 30.0])
                                {
                                    self.activate_tab(tab.id);
                                }
                                if ui.is_item_hovered() {
                                    ui.tooltip_text(&title);
                                }
                                ui.same_line_with_spacing(0.0, 0.0);
                                if icon_button(ui, "close", icon::CLOSE, "Close tab") {
                                    self.close_tab(i32::try_from(tab.id).unwrap_or_default());
                                }
                            }
                        });
                } else if density != Density::Wide {
                    ui.same_line();
                    if icon_button(
                        ui,
                        "tabs",
                        icon::TABS,
                        &format!("Tabs · {} open", self.controller.snapshot.tabs.len()),
                    ) {
                        self.panel = toggle(self.panel, Panel::Tabs);
                        self.page_focused = false;
                    }
                }
                if density != Density::Minimal {
                    ui.same_line();
                    if icon_button(ui, "new-tab", icon::PLUS, "New tab · Ctrl+T") {
                        self.new_tab();
                    }
                }
                if density == Density::Wide && !self.address_editing {
                    ui.same_line();
                    if icon_button(ui, "library", icon::BOOK, "Library") {
                        self.open_library();
                    }
                }
                ui.same_line();
                if icon_button(ui, "more", icon::MORE, "Browser tools") {
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
                    let t = (ui.time() as f32 * 0.7).fract();
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
        self.finish_address_edit();
        self.controller.command(Command::Tab {
            action: "select".into(),
            id: i32::try_from(id).unwrap_or_default(),
            causal: Causal::default(),
        });
    }

    pub(super) fn finish_address_edit(&mut self) {
        self.address_editing = false;
        self.focus_address = false;
        self.suggestion_index = None;
        self.address
            .clone_from(&self.controller.snapshot.current_url);
        self.controller.clear_suggestions();
        self.page_focused = true;
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
                        if icon_button(ui, "close", icon::CLOSE, "Close tab") {
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
