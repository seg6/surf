use super::*;

impl DesktopApp {
    pub(super) fn draw_panel(&mut self, ui: &Ui) {
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

    pub(super) fn draw_more(&mut self, ui: &Ui) {
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

    pub(super) fn draw_library(&mut self, ui: &Ui) {
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

    pub(super) fn draw_find(&mut self, ui: &Ui) {
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

    pub(super) fn draw_reader(&mut self, ui: &Ui) {
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

    pub(super) fn draw_media(&mut self, ui: &Ui) {
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

    pub(super) fn draw_settings(&mut self, ui: &Ui) {
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

    pub(super) fn draw_performance(&mut self, ui: &Ui) {
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
}
