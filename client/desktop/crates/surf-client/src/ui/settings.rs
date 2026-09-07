use super::*;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(super) enum SettingsCategory {
    Appearance,
    Browsing,
    Computers,
    Testing,
    About,
}

impl SettingsCategory {
    const ALL: [(Self, &'static str); 5] = [
        (Self::Appearance, "Appearance"),
        (Self::Browsing, "Browsing"),
        (Self::Computers, "Computers"),
        (Self::Testing, "Device testing"),
        (Self::About, "About"),
    ];
    fn label(self) -> &'static str {
        Self::ALL
            .iter()
            .find(|(category, _)| *category == self)
            .unwrap()
            .1
    }
}

impl DesktopApp {
    pub(super) fn draw_settings(&mut self, ui: &Ui) {
        let display = ui.io().display_size;
        let page = self.layout.page;
        let available_height = if self.controller.connected {
            page.height as f32
        } else {
            display[1]
        };
        let top = if self.controller.connected {
            page.y as f32
        } else {
            0.0
        };
        let size = [
            (display[0] - 16.0).min(660.0),
            (available_height - 16.0).min(490.0),
        ];
        let mut open = true;
        let mut actions = Vec::new();
        ui.window("Settings###settings")
            .position(
                [display[0] * 0.5, top + available_height * 0.5],
                Condition::Always,
            )
            .position_pivot([0.5, 0.5])
            .size(size, Condition::Always)
            .flags(
                overlay_flags()
                    | WindowFlags::NO_RESIZE
                    | WindowFlags::NO_TITLE_BAR
                    | WindowFlags::NO_MOVE,
            )
            .build(|| {
                self.hit_regions.push(window_rect(ui));
                widgets::sheet_header(ui, &self.assets.ui_icons, "Settings", &mut open);
                ui.dummy([0.0, 6.0]);
                let wide = size[0] >= 560.0;
                if wide {
                    let _bg = ui.push_style_color(
                        imgui::StyleColor::ChildBg,
                        Palette::new(self.controller.dark_mode).rail,
                    );
                    let _pad = ui.push_style_var(imgui::StyleVar::WindowPadding([8.0, 8.0]));
                    ui.child_window("##settings-nav")
                        .size([144.0, -28.0])
                        .flags(WindowFlags::ALWAYS_USE_WINDOW_PADDING)
                        .build(|| {
                            for (category, label) in SettingsCategory::ALL {
                                if row(
                                    ui,
                                    label,
                                    label,
                                    "",
                                    self.settings_category == category,
                                    ui.content_region_avail()[0],
                                ) {
                                    self.settings_category = category;
                                }
                            }
                        });
                    ui.same_line_with_spacing(0.0, 16.0);
                } else {
                    ui.set_next_item_width(-1.0);
                    if let Some(_combo) =
                        ui.begin_combo("##settings-category", self.settings_category.label())
                    {
                        for (category, label) in SettingsCategory::ALL {
                            if ui
                                .selectable_config(label)
                                .selected(self.settings_category == category)
                                .build()
                            {
                                self.settings_category = category;
                            }
                        }
                    }
                    ui.dummy([0.0, 8.0]);
                }
                ui.child_window(format!("##settings-{}", self.settings_category.label()))
                    .size([0.0, -28.0])
                    .build(|| {
                        if wide {
                            let _font = ui.push_font(ui.fonts().fonts()[3]);
                            ui.text(self.settings_category.label());
                        }
                        ui.dummy([0.0, 8.0]);
                        self.draw_settings_category(ui, &mut actions);
                    });
                ui.separator();
                let _font = ui.push_font(ui.fonts().fonts()[2]);
                ui.text_disabled(format!(
                    "Surf {}  ·  Preferences are saved automatically",
                    SURF_VERSION.trim()
                ));
            });
        if !open {
            self.panel = None;
        }
        self.apply_actions(actions);
    }

    fn draw_settings_category(&mut self, ui: &Ui, actions: &mut Vec<Action>) {
        use SettingsCategory::*;
        match self.settings_category {
            Appearance => {
                widgets::description(ui, "Make Surf comfortable on this screen.");
                section(ui, "Interface");
                let mut dark = self.controller.dark_mode;
                if widgets::setting_toggle(
                    ui,
                    "dark",
                    "Dark appearance",
                    "Use dark colors in Surf and request dark websites.",
                    &mut dark,
                ) {
                    actions.push(Action::Dark(dark));
                }
                widgets::setting_toggle(
                    ui,
                    "bottom",
                    "Controls at the bottom",
                    "Keep navigation and tabs along the lower edge.",
                    &mut self.preferences.bottom,
                );
                widgets::setting_toggle(
                    ui,
                    "motion",
                    "Reduce motion",
                    "Show panels and address changes without animation.",
                    &mut self.preferences.reduce_motion,
                );
            }
            Browsing => {
                widgets::description(ui, "Website layout and browsing data on your computer.");
                section(ui, "Websites");
                let mut mobile = self.controller.mobile_mode;
                if widgets::setting_toggle(
                    ui,
                    "mobile",
                    "Request mobile websites",
                    "Ask websites for their mobile layout.",
                    &mut mobile,
                ) {
                    actions.push(Action::Mobile(mobile));
                }
                section(ui, "Browsing history");
                widgets::description(
                    ui,
                    "Remove the history saved by Surf on the connected computer. Bookmarks are kept.",
                );
                ui.dummy([0.0, 8.0]);
                if widgets::confirm_button(
                    ui,
                    "Clear history…",
                    "Clear all Surf browsing history on this computer? This cannot be undone.",
                ) {
                    actions.push(Action::Command(Command::Clear {
                        what: "history".into(),
                        causal: Causal::default(),
                    }));
                }
            }
            Computers => {
                widgets::description(ui, "Manage the computers you have paired with.");
                section(ui, "Current connection");
                ui.text_wrapped(if self.endpoint.is_empty() {
                    "Surf server"
                } else {
                    &self.endpoint
                });
                if self.controller.connected && ui.button("Disconnect") {
                    actions.push(Action::Disconnect);
                }
                section(ui, "Saved computers");
                let servers = self.controller.saved_servers.clone();
                if servers.is_empty() {
                    widgets::description(ui, "Paired computers will appear here.");
                }
                for server in servers {
                    let _id = ui.push_id(&server.server_id);
                    ui.text_wrapped(&server.name);
                    widgets::description(ui, &server.endpoint);
                    if widgets::confirm_button(
                        ui,
                        "Forget computer…",
                        "Forget this computer? You will need to pair again to reconnect.",
                    ) {
                        actions.push(Action::Forget(server.server_id.clone()));
                    }
                    ui.separator();
                    ui.dummy([0.0, 8.0]);
                }
            }
            Testing => self.draw_device_settings(ui),
            About => {
                imgui::Image::new(self.assets.mark(), [48.0, 48.0]).build(ui);
                section(ui, "Surf");
                widgets::key_value(ui, "Client", SURF_VERSION.trim());
                let server = self
                    .controller
                    .inspected
                    .as_ref()
                    .map_or("Not connected", |s| s.version.as_str());
                widgets::key_value(ui, "Server", server);
                section(ui, "Desktop client");
                widgets::description(ui, "A shared C99 core, with a compact desktop interface.");
                section(ui, "Built with");
                widgets::description(ui, "Inter · Lucide · Dear ImGui");
            }
        }
    }

    fn draw_device_settings(&mut self, ui: &Ui) {
        widgets::description(
            ui,
            "Test an iPhone or iPad viewport. Presets resize the window; they do not stretch the interface.",
        );
        section(ui, "Device");
        let preset = DEVICE_PRESETS[self.device_preset.min(DEVICE_PRESETS.len() - 1)];
        ui.set_next_item_width(-1.0);
        if let Some(_combo) = ui.begin_combo("##device-preset", preset.label) {
            for (index, candidate) in DEVICE_PRESETS.iter().enumerate() {
                let selected = index == self.device_preset;
                if ui
                    .selectable_config(candidate.label)
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
        section(ui, "Orientation");
        let width = (ui.content_region_avail()[0] - 4.0) * 0.5;
        for (landscape, label) in [(false, "Portrait"), (true, "Landscape")] {
            if landscape {
                ui.same_line();
            }
            let p = Palette::new(self.controller.dark_mode);
            let _color = ui.push_style_color(
                imgui::StyleColor::Button,
                if self.device_landscape == landscape {
                    p.hover
                } else {
                    p.rail
                },
            );
            if ui.button_with_size(label, [width, 30.0]) {
                self.device_landscape = landscape;
            }
        }
        let preset = DEVICE_PRESETS[self.device_preset.min(DEVICE_PRESETS.len() - 1)];
        let size = preset.size(self.device_landscape);
        section(ui, "Viewport");
        widgets::key_value(ui, "Requested", &format!("{} × {} pt", size[0], size[1]));
        widgets::key_value(
            ui,
            "Current window",
            &format!(
                "{:.0} × {:.0} pt",
                ui.io().display_size[0],
                ui.io().display_size[1]
            ),
        );
        if let Some((w, h)) = self.controller.video_dimensions {
            widgets::key_value(ui, "Browser video", &format!("{w} × {h} px"));
        }
        ui.dummy([0.0, 12.0]);
        if widgets::primary_button(ui, "Apply window size", ui.content_region_avail()[0]) {
            self.window_size_request = Some(WindowSizeRequest {
                label: preset.label,
                size,
            });
        }
        if self.pending_window_size.is_some() {
            widgets::description(ui, "Applying viewport…");
        }
    }
}
