use super::*;

impl DesktopApp {
    pub(super) fn draw_chrome(&mut self, ui: &Ui) {
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

    pub(super) fn draw_tabs(&mut self, ui: &Ui) {
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

    pub(super) fn draw_suggestions(&mut self, ui: &Ui) {
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
}
