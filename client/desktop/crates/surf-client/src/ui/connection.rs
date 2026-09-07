use super::*;
use surf_client_app::ConnectionPhase;

impl DesktopApp {
    pub(super) fn draw_start(&mut self, ui: &Ui) {
        let display = ui.io().display_size;
        let width = (display[0] - 32.0).min(440.0);
        let phase = self.controller.connection_phase.clone();
        let mut actions = Vec::new();
        let mut cancel = false;
        let title = match phase {
            ConnectionPhase::Choose => "Connect to Surf",
            ConnectionPhase::Code | ConnectionPhase::Pairing => "Enter the pairing code",
            ConnectionPhase::Words => "Check the words",
            ConnectionPhase::Failed(_) => "Couldn't connect",
            _ => "Connecting",
        };
        ui.window("##connection")
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
                    ui.text_wrapped(title);
                }
                ui.set_cursor_pos([start[0] + 54.0, ui.cursor_pos()[1]]);
                ui.text_disabled(if phase == ConnectionPhase::Choose { "Your browser, on another screen." } else { "Secure connection" });
                ui.set_cursor_pos([start[0], (ui.cursor_pos()[1]).max(start[1] + 52.0)]);
                ui.separator();
                match &phase {
                    ConnectionPhase::Choose => {
                        section(ui, "Choose a computer");
                        let mut servers: Vec<(String, String)> = self.controller.saved_servers.iter()
                            .map(|s| (s.name.clone(), s.endpoint.clone())).collect();
                        for s in &self.controller.discovered_servers {
                            if !servers.iter().any(|(_, endpoint)| endpoint == &s.endpoint) {
                                servers.push((s.name.clone(), s.endpoint.clone()));
                            }
                        }
                        let list_height = if servers.is_empty() { 76.0 } else { (servers.len().min(3) as f32 * 48.0) + 20.0 };
                        widgets::inset(ui, "##computers", list_height, || {
                            if servers.is_empty() {
                                ui.text("Looking for nearby computers…");
                                widgets::description(ui, "Open Surf on your computer and use the same network.");
                            }
                            for (name, endpoint) in servers {
                                if row(ui, &endpoint, &name, endpoint.trim_start_matches("https://"), false, ui.content_region_avail()[0]) {
                                    actions.push(Action::Inspect(endpoint, true));
                                }
                            }
                        });
                        section(ui, "Connect by address");
                        ui.set_next_item_width(-1.0);
                        let enter = ui.input_text("##server-address", &mut self.endpoint)
                            .hint("Computer address or host name").enter_returns_true(true).build();
                        let valid = !self.endpoint.trim().is_empty();
                        let clicked = {
                            let _disabled = ui.begin_disabled(!valid);
                            widgets::primary_button(ui, "Continue", ui.content_region_avail()[0])
                        };
                        if valid && (clicked || enter) { actions.push(Action::Inspect(self.endpoint.trim().into(), true)); }
                        if let Some(note) = &self.controller.discovery_note { widgets::description(ui, note); }
                    }
                    ConnectionPhase::Code | ConnectionPhase::Pairing => {
                        section(ui, "Your computer");
                        ui.text_wrapped(self.controller.inspected.as_ref().map_or(self.endpoint.as_str(), |s| s.name.as_str()));
                        widgets::description(ui, &self.controller.status);
                        section(ui, "Six-digit code");
                        ui.set_next_item_width(-1.0);
                        let enter = ui.input_text("##pairing-code", &mut self.pairing_code).hint("000000")
                            .chars_decimal(true).enter_returns_true(true).build();
                        self.pairing_code.retain(|c| c.is_ascii_digit());
                        self.pairing_code.truncate(6);
                        let ready = self.pairing_code.len() == 6 && phase != ConnectionPhase::Pairing;
                        let clicked = {
                            let _disabled = ui.begin_disabled(!ready);
                            widgets::primary_button(ui, if phase == ConnectionPhase::Pairing { "Checking code…" } else { "Continue" }, ui.content_region_avail()[0])
                        };
                        if ready && (clicked || enter) { actions.push(Action::Pair(self.pairing_code.clone())); }
                    }
                    ConnectionPhase::Words => {
                        section(ui, "Confirm it's your computer");
                        widgets::description(ui, "The words below must match the ones shown on your computer, in the same order.");
                        ui.dummy([0.0, 8.0]);
                        if let Some(pairing) = &self.controller.pairing {
                            widgets::inset(ui, "##verification", 84.0, || {
                                let _font = ui.push_font(ui.fonts().fonts()[3]);
                                ui.text_wrapped(&pairing.phrase);
                            });
                        }
                        ui.dummy([0.0, 8.0]);
                        if widgets::primary_button(ui, "The words match", ui.content_region_avail()[0]) {
                            actions.push(Action::ConfirmPairing);
                        }
                        if ui.button("They don't match — cancel") { cancel = true; }
                    }
                    ConnectionPhase::Failed(failure) => {
                        section(ui, "Connection interrupted");
                        widgets::description(ui, &failure.message);
                        ui.dummy([0.0, 12.0]);
                        if widgets::primary_button(ui, "Try again", ui.content_region_avail()[0]) {
                            cancel = true;
                            actions.push(Action::Inspect(self.endpoint.clone(), true));
                        }
                    }
                    _ => { section(ui, "Please wait"); widgets::description(ui, &self.controller.status); }
                }
                ui.dummy([0.0, 12.0]);
                ui.separator();
                if phase != ConnectionPhase::Choose && ui.button("Back to computers") { cancel = true; }
                let _font = ui.push_font(ui.fonts().fonts()[2]);
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
        let height = (p.height as f32 - 24.0).min(380.0);
        let _bg = ui.push_style_color(
            imgui::StyleColor::WindowBg,
            Palette::new(self.controller.dark_mode).canvas,
        );
        let _border = ui.push_style_var(imgui::StyleVar::WindowBorderSize(0.0));
        ui.get_background_draw_list()
            .add_rect(
                [p.x as f32, p.y as f32],
                [(p.x + p.width) as f32, (p.y + p.height) as f32],
                Palette::new(self.controller.dark_mode).canvas,
            )
            .filled(true)
            .build();
        ui.window("##new-tab")
            .position(
                [
                    p.x as f32 + p.width as f32 * 0.5,
                    p.y as f32 + p.height as f32 * 0.45,
                ],
                Condition::Always,
            )
            .position_pivot([0.5, 0.5])
            .size([width, height], Condition::Always)
            .flags(
                WindowFlags::NO_TITLE_BAR
                    | WindowFlags::NO_RESIZE
                    | WindowFlags::NO_MOVE
                    | WindowFlags::NO_SAVED_SETTINGS
                    | WindowFlags::NO_FOCUS_ON_APPEARING,
            )
            .build(|| {
                self.hit_regions.push(window_rect(ui));
                let start = ui.cursor_pos();
                imgui::Image::new(self.assets.mark(), [48.0, 48.0]).build(ui);
                ui.set_cursor_pos([start[0] + 60.0, start[1] + 4.0]);
                {
                    let _font = ui.push_font(ui.fonts().fonts()[4]);
                    ui.text("New tab");
                }
                ui.set_cursor_pos([start[0], start[1] + 64.0]);
                ui.set_next_item_width(-1.0);
                let submitted = ui
                    .input_text("##new-tab-search", &mut self.new_tab_query)
                    .hint("Search or enter address")
                    .enter_returns_true(true)
                    .build();
                if ui.is_item_active() {
                    self.page_focused = false;
                }
                if submitted && !self.new_tab_query.trim().is_empty() {
                    let query = std::mem::take(&mut self.new_tab_query);
                    self.navigate_from_ui(&query);
                }
                {
                    let _font = ui.push_font(ui.fonts().fonts()[2]);
                    ui.text_disabled("Enter to browse  ·  Ctrl+L to use the address bar");
                }
                section(ui, "Bookmarks");
                let bookmarks = self.controller.browser.bookmarks.clone();
                if bookmarks.is_empty() {
                    widgets::description(
                        ui,
                        "Keep useful pages here by bookmarking them from Browser tools.",
                    );
                } else {
                    for item in bookmarks.iter().take(4) {
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
                        ui.separator();
                    }
                    if ui.button("View library") {
                        self.open_library();
                        self.library_section = LibrarySection::Bookmarks;
                    }
                }
            });
    }
}
