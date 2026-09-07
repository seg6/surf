use super::*;

impl DesktopApp {
    pub(super) fn draw_panel(&mut self, ui: &Ui) {
        if self.panel != self.previous_panel {
            self.panel_started = Instant::now();
            if self.panel == Some(Panel::More) {
                ui.open_popup("##tools-popup");
            }
            self.previous_panel = self.panel;
        }
        let alpha = if self.preferences.reduce_motion {
            1.0
        } else {
            (self.panel_started.elapsed().as_secs_f32() / 0.10).min(1.0)
        };
        let _alpha = ui.push_style_var(imgui::StyleVar::Alpha(alpha));
        match self.panel {
            Some(Panel::Tabs) => self.draw_tabs(ui),
            Some(Panel::More) => self.draw_more(ui),
            Some(Panel::Library) => self.draw_library(ui),
            Some(Panel::Reader) => self.draw_reader(ui),
            Some(Panel::Media) => self.draw_media(ui),
            Some(Panel::Settings) => self.draw_settings(ui),
            Some(Panel::Files) => self.draw_files(ui),
            None => {}
        }
    }

    pub(super) fn draw_more(&mut self, ui: &Ui) {
        let (position, size) = crate::layout::anchored(
            ui.io().display_size,
            [
                ui.io().display_size[0] - 300.0,
                self.layout.rail[1],
                30.0,
                42.0,
            ],
            [292.0, self.more_height],
        );
        widgets::place_next(position, [size[0], 0.0]);
        // SAFETY: public ImGui sizing API on its owning thread. Long menus may
        // scroll on unusually short windows, but short menus keep their natural height.
        unsafe {
            imgui::sys::igSetNextWindowSizeConstraints(
                [size[0], 0.0].into(),
                [size[0], ui.io().display_size[1] - 16.0].into(),
                None,
                std::ptr::null_mut(),
            );
        }
        let Some(_popup) = ui.begin_popup("##tools-popup") else {
            self.panel = None;
            return;
        };
        if ui.window_size()[1] > 100.0 {
            self.more_height = ui.window_size()[1];
        }
        let mut next = None;
        if widgets::menu_row(ui, "new", icon::PLUS, "New tab", "Ctrl+T", false) {
            self.new_tab();
        }
        if widgets::menu_row(
            ui,
            "bookmark",
            icon::STAR,
            "Bookmark this page",
            "Ctrl+D",
            self.controller.snapshot.starred,
        ) {
            self.controller.command(Command::Bookmark {
                causal: Causal::default(),
            });
        }
        if widgets::menu_row(ui, "share", icon::SHARE, "Copy page link", "", false) {
            match arboard::Clipboard::new()
                .and_then(|mut c| c.set_text(self.controller.snapshot.current_url.clone()))
            {
                Ok(()) => self.controller.browser.toast("Link copied"),
                Err(e) => self
                    .controller
                    .browser
                    .toast(format!("Could not copy link: {e}")),
            }
        }
        ui.separator();
        if widgets::menu_row(ui, "library", icon::BOOK, "Library", "", false) {
            self.open_library();
            next = Some(Panel::Library);
        }
        if widgets::menu_row(
            ui,
            "find",
            icon::SEARCH,
            "Find on page",
            "Ctrl+F",
            self.find_open,
        ) {
            self.find_open = !self.find_open;
            self.focus_find = self.find_open;
            ui.close_current_popup();
            self.panel = None;
        }
        if widgets::menu_row(ui, "reader", icon::READER, "Reader", "", false) {
            self.controller.command(Command::Reader {
                causal: Causal::default(),
            });
            self.panel = None;
        }
        if widgets::menu_row(ui, "media", icon::MEDIA, "Page media", "", false) {
            self.controller.command(Command::MediaQuery {
                causal: Causal::default(),
            });
            next = Some(Panel::Media);
        }
        ui.separator();
        if widgets::menu_row(
            ui,
            "performance",
            icon::GAUGE,
            "Performance",
            "",
            self.performance_open,
        ) {
            self.performance_open = !self.performance_open;
            self.panel = None;
        }
        if widgets::menu_row(ui, "settings", icon::GEAR, "Settings", "", false) {
            next = Some(Panel::Settings);
        }
        if widgets::menu_row(
            ui,
            "fullscreen",
            icon::EXPAND,
            "Fullscreen",
            "F11",
            self.fullscreen,
        ) {
            self.set_fullscreen_command(!self.fullscreen);
            self.panel = None;
        }
        ui.separator();
        if widgets::menu_row(ui, "disconnect", icon::SERVER, "Disconnect", "", false) {
            self.controller.disconnect();
            self.panel = None;
        }
        if let Some(next) = next {
            self.panel = Some(next);
        }
        if self.panel != Some(Panel::More) {
            ui.close_current_popup();
        }
    }

    pub(super) fn draw_library(&mut self, ui: &Ui) {
        let mut display = ui.io().display_size;
        let top = if self.controller.connected {
            self.layout.page.y as f32
        } else {
            0.0
        };
        if self.controller.connected {
            display[1] = self.layout.page.height as f32;
        }
        let size = [
            (display[0] - 16.0).min(680.0),
            (display[1] - 16.0).min(610.0),
        ];
        let mut open = true;
        let mut actions = Vec::new();
        ui.window("Library###library")
            .position(
                [display[0] * 0.5, top + display[1] * 0.5],
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
                widgets::sheet_header(ui, "Library", &mut open);
                let segment_width = (ui.content_region_avail()[0] - 8.0) / 3.0;
                for (section, label) in [
                    (LibrarySection::History, "History"),
                    (LibrarySection::Bookmarks, "Bookmarks"),
                    (LibrarySection::Downloads, "Downloads"),
                ] {
                    if section != LibrarySection::History {
                        ui.same_line();
                    }
                    let p = Palette::new(self.controller.dark_mode);
                    let _bg = ui.push_style_color(
                        imgui::StyleColor::Button,
                        if self.library_section == section {
                            p.hover
                        } else {
                            p.rail
                        },
                    );
                    if ui.button_with_size(label, [segment_width, 30.0]) {
                        self.library_section = section;
                    }
                }
                ui.set_next_item_width(-1.0);
                ui.input_text("##library-filter", &mut self.library_filter)
                    .hint("Filter library")
                    .build();
                let filter = self.library_filter.to_lowercase();
                ui.child_window("##library-list")
                    .size([0.0, 0.0])
                    .build(|| {
                        if self.library_section == LibrarySection::Downloads {
                            let items = self.controller.browser.downloads.clone();
                            if items.is_empty() {
                                ui.text_disabled("No downloads yet.");
                            }
                            for item in items
                                .iter()
                                .filter(|i| i.name.to_lowercase().contains(&filter))
                            {
                                let _id = ui.push_id(&item.name);
                                let progress = self
                                    .controller
                                    .browser
                                    .download_progress
                                    .get(&item.name)
                                    .map(|p| format!("{p}%"))
                                    .unwrap_or_else(|| format_bytes(item.size));
                                row(
                                    ui,
                                    "file",
                                    &item.name,
                                    &progress,
                                    false,
                                    ui.content_region_avail()[0],
                                );
                                if ui.button("Save to Downloads") {
                                    actions.push(Action::Download(item.name.clone()));
                                }
                                ui.same_line();
                                if ui.button("Remove") {
                                    actions.push(Action::Command(Command::DownloadDelete {
                                        name: item.name.clone(),
                                        causal: Causal::default(),
                                    }));
                                    actions.push(Action::Command(Command::Downloads {
                                        causal: Causal::default(),
                                    }));
                                }
                                ui.separator();
                            }
                        } else {
                            let history = self.library_section == LibrarySection::History;
                            let items = if history {
                                self.controller.browser.history.clone()
                            } else {
                                self.controller.browser.bookmarks.clone()
                            };
                            if items.is_empty() {
                                ui.text_disabled(if history {
                                    "No browsing history yet."
                                } else {
                                    "Bookmark a page to find it here."
                                });
                            }
                            let mut matches = 0;
                            for item in items.iter().filter(|i| {
                                format!("{} {}", i.title, i.url)
                                    .to_lowercase()
                                    .contains(&filter)
                            }) {
                                matches += 1;
                                let _id = ui.push_id(format!("{}-{}", item.url, item.ts));
                                let width = ui.content_region_avail()[0] - 34.0;
                                if row(
                                    ui,
                                    "open",
                                    &item.title,
                                    &compact_address(&item.url),
                                    false,
                                    width,
                                ) {
                                    actions.push(Action::Navigate(item.url.clone()));
                                }
                                ui.same_line();
                                if icon_button(ui, "remove", icon::CLOSE, "Remove") {
                                    actions.push(Action::Command(if history {
                                        Command::HistoryDelete {
                                            url: item.url.clone(),
                                            ts: item.ts,
                                            causal: Causal::default(),
                                        }
                                    } else {
                                        Command::BookmarkDelete {
                                            url: item.url.clone(),
                                            causal: Causal::default(),
                                        }
                                    }));
                                    actions.push(Action::Command(Command::Library {
                                        causal: Causal::default(),
                                    }));
                                }
                            }
                            if matches == 0 && !items.is_empty() {
                                ui.text_disabled("No matching pages.");
                            }
                            if history && !items.is_empty() && ui.button("Load older history") {
                                actions.push(Action::Command(Command::History {
                                    q: String::new(),
                                    offset: items.len() as i32,
                                    causal: Causal::default(),
                                }));
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
        let [x, y, width, height] = self.layout.find;
        let mut direction = None;
        let _padding = ui.push_style_var(imgui::StyleVar::WindowPadding([6.0, 6.0]));
        ui.window("##find-rail")
            .position([x, y], Condition::Always)
            .size([width, height], Condition::Always)
            .flags(
                WindowFlags::NO_DECORATION | WindowFlags::NO_MOVE | WindowFlags::NO_SAVED_SETTINGS,
            )
            .build(|| {
                self.hit_regions.push(window_rect(ui));
                let old = self.controller.browser.find_query.clone();
                if self.focus_find {
                    ui.set_keyboard_focus_here();
                    self.focus_find = false;
                }
                ui.set_next_item_width((width - 114.0).max(80.0));
                if ui
                    .input_text("##find", &mut self.controller.browser.find_query)
                    .hint("Find on page")
                    .enter_returns_true(true)
                    .build()
                {
                    direction = Some(1);
                }
                if old != self.controller.browser.find_query {
                    direction = Some(0);
                }
                ui.same_line();
                if icon_button(ui, "previous", icon::BACK, "Previous match") {
                    direction = Some(-1);
                }
                ui.same_line();
                if icon_button(ui, "next", icon::FORWARD, "Next match") {
                    direction = Some(1);
                }
                ui.same_line();
                if icon_button(ui, "close-find", icon::CLOSE, "Close find") {
                    self.find_open = false;
                    self.controller.command(Command::Find {
                        q: String::new(),
                        dir: 0,
                        causal: Causal::default(),
                    });
                }
            });
        if let Some(dir) = direction {
            self.controller.command(Command::Find {
                q: self.controller.browser.find_query.clone(),
                dir,
                causal: Causal::default(),
            });
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
        ui.window("Reader###reader")
            .position([display[0] * 0.5, display[1] * 0.5], Condition::Appearing)
            .position_pivot([0.5, 0.5])
            .size([width, height], Condition::Appearing)
            .flags(overlay_flags() | WindowFlags::NO_TITLE_BAR | WindowFlags::NO_MOVE)
            .build(|| {
                self.hit_regions.push(window_rect(ui));
                widgets::sheet_header(ui, "Reader", &mut open);
                ui.text_wrapped(&reader.title);
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
                self.hit_regions.push(window_rect(ui));
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
        let mut display = ui.io().display_size;
        let top = if self.controller.connected {
            self.layout.page.y as f32
        } else {
            0.0
        };
        if self.controller.connected {
            display[1] = self.layout.page.height as f32;
        }
        let size = [
            (display[0] - 16.0).min(440.0),
            (display[1] - 16.0).min(610.0),
        ];
        let mut open = true;
        let mut actions = Vec::new();
        ui.window("Settings###settings").position([display[0]*0.5,top+display[1]*0.5],Condition::Always)
            .position_pivot([0.5,0.5]).size(size,Condition::Always)
            .flags(overlay_flags()|WindowFlags::NO_RESIZE | WindowFlags::NO_TITLE_BAR | WindowFlags::NO_MOVE).build(|| {
                self.hit_regions.push(window_rect(ui));
                widgets::sheet_header(ui,"Settings",&mut open);
                ui.child_window("##settings-body").size([0.0,0.0]).build(||{
                    section(ui,"Appearance");
                    let mut dark=self.controller.dark_mode;
                    if ui.checkbox("Dark interface and websites",&mut dark){actions.push(Action::Dark(dark));}
                    ui.checkbox("Browser controls at the bottom",&mut self.preferences.bottom);
                    ui.checkbox("Reduce motion",&mut self.preferences.reduce_motion);
                    section(ui,"Browsing");
                    let mut mobile=self.controller.mobile_mode;
                    if ui.checkbox("Request mobile websites",&mut mobile){actions.push(Action::Mobile(mobile));}
                    if ui.button("Clear browsing history"){
                        actions.push(Action::Command(Command::Clear{what:"history".into(),causal:Causal::default()}));
                    }
                    section(ui,"Computers");
                    let servers=self.controller.saved_servers.clone();
                    if servers.is_empty(){ui.text_disabled("No saved computers");}
                    for server in servers {
                        ui.text_wrapped(&server.name);ui.text_disabled(&server.endpoint);
                        let _id=ui.push_id(&server.server_id);
                        if ui.button("Forget"){actions.push(Action::Forget(server.server_id.clone()));}
                    }
                    if self.controller.connected && ui.button("Disconnect"){actions.push(Action::Disconnect);}
                    section(ui,"Device testing");
                    ui.text_wrapped("Resize the client to an iPhone or iPad layout. This changes the browser viewport, not the UI scale.");
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
                    ui.io().display_size[0], ui.io().display_size[1]
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

                    section(ui,"About");
                    let server=self.controller.inspected.as_ref().map_or("—",|s|s.version.as_str());
                    ui.text(format!("Surf client {}",SURF_VERSION.trim()));
                    ui.text_disabled(format!("Server {server}"));
                    ui.text_wrapped("Inter · Lucide · Dear ImGui");
                    ui.text_wrapped("Device sizes use logical points. Window scaling follows your desktop display.");
                });
            });
        if !open {
            self.panel = None;
        }
        self.apply_actions(actions);
    }

    pub(super) fn draw_performance(&mut self, ui: &Ui) {
        let report = self.controller.latest_diagnostics.unwrap_or_default();
        let width = (ui.io().display_size[0] - 16.0).min(300.0);
        let p = self.layout.page;
        let position = [p.width as f32 - width - 8.0, p.y as f32 + 8.0];
        let mut open = true;
        ui.window("Performance###performance")
            .position(position, Condition::Always)
            .size([width, 0.0], Condition::Always)
            .opened(&mut open)
            .flags(
                overlay_flags()
                    | WindowFlags::ALWAYS_AUTO_RESIZE
                    | WindowFlags::NO_RESIZE
                    | WindowFlags::NO_SCROLLBAR,
            )
            .bg_alpha(0.90)
            .build(|| {
                self.hit_regions.push(window_rect(ui));
                metric(ui, "FPS", format!("{:.1}", report.presentation_fps));
                ui.same_line();
                metric(
                    ui,
                    "Decode",
                    format!("{:.1} ms", report.decode_us as f64 / 1000.0),
                );
                ui.same_line();
                metric(
                    ui,
                    "Upload",
                    format!("{:.1} ms", report.upload_us as f64 / 1000.0),
                );
                ui.separator();
                ui.text(format!(
                    "Frame age  {:.1} ms",
                    report.frame_age_us as f64 / 1000.0
                ));
                ui.text(format!(
                    "Dropped  {:.1}%   ·   Queue  {}/{}",
                    report.drop_percent, report.encoded_video_depth, report.decoded_video_depth
                ));
                if let Some((w, h)) = self.controller.video_dimensions {
                    ui.text_disabled(format!("{w} × {h}"));
                }
                let server = self
                    .controller
                    .inspected
                    .as_ref()
                    .map_or("—", |s| s.version.as_str());
                ui.text_disabled(format!("Client {} · Server {server}", SURF_VERSION.trim()));
            });
        self.performance_open = open;
    }
}
