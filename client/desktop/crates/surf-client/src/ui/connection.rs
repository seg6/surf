use super::*;
use surf_client_app::ConnectionPhase;

impl DesktopApp {
    pub(super) fn draw_start(&mut self, ui: &Ui) {
        let display = ui.io().display_size;
        let width = (display[0] - 32.0).min(420.0);
        let height = (display[1] - 32.0).min(560.0);
        let phase = self.controller.connection_phase.clone();
        let mut actions = Vec::new();
        let mut cancel = false;
        let _bg = ui.push_style_color(
            imgui::StyleColor::WindowBg,
            Palette::new(self.controller.dark_mode).canvas,
        );
        ui.window("##connection")
            .position([display[0] * 0.5, display[1] * 0.5], Condition::Always)
            .position_pivot([0.5, 0.5])
            .size([width, height], Condition::Always)
            .flags(
                WindowFlags::NO_DECORATION | WindowFlags::NO_MOVE | WindowFlags::NO_SAVED_SETTINGS,
            )
            .build(|| {
                ui.set_cursor_pos([width * 0.5 - 34.0, 16.0]);
                imgui::Image::new(self.assets.mark(), [68.0, 68.0]).build(ui);
                ui.set_cursor_pos([12.0, 100.0]);
                let title = match phase {
                    ConnectionPhase::Choose => "Surf",
                    ConnectionPhase::Code | ConnectionPhase::Pairing => "Pair with Surf",
                    ConnectionPhase::Words => "Do these words match?",
                    ConnectionPhase::Failed(_) => "Couldn't connect",
                    _ => "Connecting",
                };
                {
                    let _font = ui.push_font(ui.fonts().fonts()[4]);
                    ui.text(title);
                }
                ui.dummy([0.0, 8.0]);
                match &phase {
                    ConnectionPhase::Choose => {
                        ui.text_wrapped("Choose the computer that runs your browser.");
                        section(ui, "Computers");
                        let mut servers: Vec<(String, String)> = self
                            .controller
                            .saved_servers
                            .iter()
                            .map(|s| (s.name.clone(), s.endpoint.clone()))
                            .collect();
                        for s in &self.controller.discovered_servers {
                            if !servers.iter().any(|(_, endpoint)| endpoint == &s.endpoint) {
                                servers.push((s.name.clone(), s.endpoint.clone()));
                            }
                        }
                        ui.child_window("##computers")
                            .size([0.0, (height - 288.0).max(68.0)])
                            .build(|| {
                                if servers.is_empty() {
                                    ui.text_disabled("Looking for nearby computers…");
                                }
                                for (name, endpoint) in servers {
                                    if row(
                                        ui,
                                        &endpoint,
                                        &name,
                                        endpoint.trim_start_matches("https://"),
                                        false,
                                        ui.content_region_avail()[0],
                                    ) {
                                        actions.push(Action::Inspect(endpoint, true));
                                    }
                                }
                            });
                        section(ui, "Or enter an address");
                        ui.set_next_item_width(-1.0);
                        let enter = ui
                            .input_text("##server-address", &mut self.endpoint)
                            .hint("Computer address or host name")
                            .enter_returns_true(true)
                            .build();
                        if (ui.button_with_size("Continue", [ui.content_region_avail()[0], 30.0])
                            || enter)
                            && !self.endpoint.trim().is_empty()
                        {
                            actions.push(Action::Inspect(self.endpoint.trim().into(), true));
                        }
                        if let Some(note) = &self.controller.discovery_note {
                            ui.text_wrapped(note);
                        }
                    }
                    ConnectionPhase::Code | ConnectionPhase::Pairing => {
                        ui.text_wrapped(&self.controller.status);
                        ui.dummy([0.0, 12.0]);
                        ui.set_next_item_width(-1.0);
                        let enter = ui
                            .input_text("##pairing-code", &mut self.pairing_code)
                            .hint("Six-digit code")
                            .chars_decimal(true)
                            .enter_returns_true(true)
                            .build();
                        self.pairing_code.retain(|c| c.is_ascii_digit());
                        self.pairing_code.truncate(6);
                        let _disabled = ui.begin_disabled(
                            self.pairing_code.len() != 6 || phase == ConnectionPhase::Pairing,
                        );
                        if ui.button_with_size("Continue", [ui.content_region_avail()[0], 30.0])
                            || enter
                        {
                            actions.push(Action::Pair(self.pairing_code.clone()));
                        }
                    }
                    ConnectionPhase::Words => {
                        ui.text_wrapped("Compare these words with the ones on your computer.");
                        ui.dummy([0.0, 16.0]);
                        if let Some(pairing) = &self.controller.pairing {
                            let _font = ui.push_font(ui.fonts().fonts()[3]);
                            ui.text_wrapped(&pairing.phrase);
                        }
                        ui.dummy([0.0, 16.0]);
                        if ui.button_with_size(
                            "The words match",
                            [ui.content_region_avail()[0], 30.0],
                        ) {
                            actions.push(Action::ConfirmPairing);
                        }
                        if ui.button("They don't match") {
                            cancel = true;
                        }
                    }
                    ConnectionPhase::Failed(failure) => {
                        ui.text_wrapped(&failure.message);
                        ui.dummy([0.0, 12.0]);
                        if ui.button("Try again") {
                            cancel = true;
                            actions.push(Action::Inspect(self.endpoint.clone(), true));
                        }
                    }
                    _ => {
                        ui.text_wrapped(&self.controller.status);
                    }
                }
                if phase != ConnectionPhase::Choose {
                    ui.dummy([0.0, 16.0]);
                    if ui.button("Back to computers") {
                        cancel = true;
                    }
                }
                ui.dummy([0.0, 12.0]);
                ui.text_disabled(format!("Surf {}", SURF_VERSION.trim()));
            });
        if cancel {
            self.controller.cancel_connection();
            self.pairing_code.clear();
        }
        self.apply_actions(actions);
    }

    pub(super) fn draw_new_tab(&mut self, ui: &Ui) {
        let p = self.layout.page;
        let width = (p.width as f32 - 32.0).min(520.0);
        let position = [
            p.x as f32 + p.width as f32 * 0.5,
            p.y as f32 + p.height as f32 * 0.40,
        ];
        let _bg = ui.push_style_color(
            imgui::StyleColor::WindowBg,
            Palette::new(self.controller.dark_mode).canvas,
        );
        ui.window("##new-tab")
            .position(position, Condition::Always)
            .position_pivot([0.5, 0.5])
            .size([width, 240.0], Condition::Always)
            .flags(
                WindowFlags::NO_DECORATION
                    | WindowFlags::NO_MOVE
                    | WindowFlags::NO_SAVED_SETTINGS
                    | WindowFlags::NO_FOCUS_ON_APPEARING,
            )
            .build(|| {
                ui.set_cursor_pos([width * 0.5 - 32.0, 8.0]);
                imgui::Image::new(self.assets.mark(), [64.0, 64.0]).build(ui);
                ui.set_cursor_pos([12.0, 94.0]);
                if ui.button_with_size(
                    "Search or enter address",
                    [ui.content_region_avail()[0], 38.0],
                ) {
                    self.edit_address();
                }
                ui.text_disabled("Ctrl+L to start typing");
                let bookmarks = self.controller.browser.bookmarks.clone();
                for item in bookmarks.iter().take(2) {
                    if row(
                        ui,
                        &item.url,
                        &item.title,
                        &compact_address(&item.url),
                        false,
                        ui.content_region_avail()[0],
                    ) {
                        self.navigate_from_ui(&item.url);
                    }
                }
            });
    }
}
