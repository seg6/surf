use super::*;

impl DesktopApp {
    pub(super) fn draw_start(&mut self, ui: &Ui) {
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
}
