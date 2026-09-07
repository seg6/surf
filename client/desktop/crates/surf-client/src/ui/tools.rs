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
        let mut chosen = None;
        let items = [
            ("new", icon::PLUS, "New tab", "Ctrl+T", false),
            ("library", icon::BOOK, "Library", "", false),
            (
                "fullscreen",
                icon::EXPAND,
                "Fullscreen",
                "F11",
                self.fullscreen,
            ),
            (
                "bookmark",
                icon::STAR,
                "Bookmark this page",
                "Ctrl+D",
                self.controller.snapshot.starred,
            ),
            ("share", icon::SHARE, "Copy page link", "", false),
            (
                "find",
                icon::SEARCH,
                "Find on page",
                "Ctrl+F",
                self.find_open,
            ),
            ("reader", icon::READER, "Reader", "", false),
            ("media", icon::MEDIA, "Page media", "", false),
            ("settings", icon::GEAR, "Settings", "", false),
            (
                "performance",
                icon::GAUGE,
                "Performance",
                "",
                self.performance_open,
            ),
            ("disconnect", icon::SERVER, "Disconnect", "", false),
        ];
        for (index, (id, glyph, label, shortcut, checked)) in items.into_iter().enumerate() {
            if index == 3 || index == 8 {
                ui.separator();
            }
            if widgets::menu_row(
                ui,
                &self.assets.ui_icons,
                id,
                glyph,
                label,
                shortcut,
                checked,
            ) {
                chosen = Some(id);
            }
        }
        if let Some(chosen) = chosen {
            self.panel = None;
            match chosen {
                "new" => self.new_tab(),
                "library" => self.open_library(),
                "fullscreen" => self.set_fullscreen_command(!self.fullscreen),
                "bookmark" => self.controller.command(Command::Bookmark {
                    causal: Causal::default(),
                }),
                "share" => {
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
                "find" => {
                    self.find_open = !self.find_open;
                    self.focus_find = self.find_open;
                }
                "reader" => self.controller.command(Command::Reader {
                    causal: Causal::default(),
                }),
                "media" => {
                    self.controller.command(Command::MediaQuery {
                        causal: Causal::default(),
                    });
                    self.panel = Some(Panel::Media);
                }
                "settings" => self.panel = Some(Panel::Settings),
                "performance" => self.performance_open = !self.performance_open,
                "disconnect" => self.controller.disconnect(),
                _ => unreachable!(),
            }
            ui.close_current_popup();
        }
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
                if icon_button(
                    ui,
                    &self.assets.ui_icons,
                    "previous",
                    icon::BACK,
                    "Previous match",
                ) {
                    direction = Some(-1);
                }
                ui.same_line();
                if icon_button(
                    ui,
                    &self.assets.ui_icons,
                    "next",
                    icon::FORWARD,
                    "Next match",
                ) {
                    direction = Some(1);
                }
                ui.same_line();
                if icon_button(
                    ui,
                    &self.assets.ui_icons,
                    "close-find",
                    icon::CLOSE,
                    "Close find",
                ) {
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
        let page = self.layout.page;
        let width = (display[0] - 16.0).min(720.0);
        let height = (page.height as f32 - 16.0).min(620.0);
        let mut open = true;
        let mut navigate = false;
        ui.window("Reader###reader")
            .position(
                [display[0] * 0.5, page.y as f32 + page.height as f32 * 0.5],
                Condition::Always,
            )
            .position_pivot([0.5, 0.5])
            .size([width, height], Condition::Always)
            .flags(
                overlay_flags()
                    | WindowFlags::NO_TITLE_BAR
                    | WindowFlags::NO_MOVE
                    | WindowFlags::NO_RESIZE,
            )
            .build(|| {
                self.hit_regions.push(window_rect(ui));
                widgets::sheet_header(ui, &self.assets.ui_icons, "Reader", &mut open);
                ui.dummy([0.0, 8.0]);
                let body_height = (ui.content_region_avail()[1] - 44.0).max(40.0);
                widgets::inset(ui, "##reader-text", body_height, || {
                    {
                        let _font = ui.push_font(ui.fonts().fonts()[4]);
                        ui.text_wrapped(&reader.title);
                    }
                    widgets::description(ui, &compact_address(&reader.url));
                    ui.dummy([0.0, 12.0]);
                    ui.text_wrapped(&reader.text);
                });
                ui.dummy([0.0, 4.0]);
                if ui.button_with_size("Open original page", [0.0, 32.0]) {
                    navigate = true;
                }
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
        let media = self.controller.browser.media.clone();
        let (position, size) = crate::layout::anchored(
            ui.io().display_size,
            [
                ui.io().display_size[0] - 38.0,
                self.layout.rail[1],
                30.0,
                self.layout.rail[3],
            ],
            [330.0, if media.available { 244.0 } else { 130.0 }],
        );
        let mut open = true;
        let mut actions = Vec::new();
        ui.window("Media###media")
            .position(position, Condition::Always)
            .size(size, Condition::Always)
            .flags(
                overlay_flags()
                    | WindowFlags::NO_TITLE_BAR
                    | WindowFlags::NO_RESIZE
                    | WindowFlags::NO_MOVE,
            )
            .build(|| {
                self.hit_regions.push(window_rect(ui));
                widgets::sheet_header(ui, &self.assets.ui_icons, "Page media", &mut open);
                ui.dummy([0.0, 6.0]);
                if !media.available {
                    widgets::description(
                        ui,
                        "Play audio or video on this page to control it here.",
                    );
                    return;
                }
                let title = if media.title.is_empty() {
                    "Audio or video"
                } else {
                    &media.title
                };
                ui.text(widgets::ellipsize(ui, title, ui.content_region_avail()[0]));
                widgets::key_value(
                    ui,
                    &format_time(media.current_time),
                    &format_time(media.duration),
                );
                let fraction = if media.duration > 0.0 {
                    (media.current_time / media.duration).clamp(0.0, 1.0) as f32
                } else {
                    0.0
                };
                imgui::ProgressBar::new(fraction)
                    .size([ui.content_region_avail()[0], 3.0])
                    .overlay_text("")
                    .build(ui);
                ui.dummy([0.0, 4.0]);
                let half = (ui.content_region_avail()[0] - 4.0) * 0.5;
                if widgets::primary_button(ui, if media.paused { "Play" } else { "Pause" }, half) {
                    actions.push(Action::Command(Command::MediaPlayPause {
                        causal: Causal::default(),
                    }));
                }
                ui.same_line();
                if ui.button_with_size(if media.muted { "Unmute" } else { "Mute" }, [half, 32.0]) {
                    actions.push(Action::Command(Command::MediaMute {
                        causal: Causal::default(),
                    }));
                }
                ui.dummy([0.0, 6.0]);
                ui.separator();
                ui.dummy([0.0, 6.0]);
                let mut volume = media.volume.clamp(0.0, 1.0) as f32 * 100.0;
                widgets::key_value(ui, "Volume", &format!("{volume:.0}%"));
                ui.set_next_item_width(-1.0);
                if ui
                    .slider_config("##volume", 0.0, 100.0)
                    .display_format("")
                    .build(&mut volume)
                {
                    actions.push(Action::Command(Command::MediaVolume {
                        value: f64::from(volume) / 100.0,
                        causal: Causal::default(),
                    }));
                }
            });
        if !open {
            self.panel = None;
        }
        self.apply_actions(actions);
    }

    pub(super) fn draw_performance(&mut self, ui: &Ui) {
        let report = self.controller.latest_diagnostics;
        let width = (ui.io().display_size[0] - 16.0).min(280.0);
        let p = self.layout.page;
        let mut open = true;
        ui.window("Performance###performance")
            .position(
                [p.x as f32 + p.width as f32 - width - 8.0, p.y as f32 + 8.0],
                Condition::Always,
            )
            .size([width, 0.0], Condition::Always)
            .flags(
                overlay_flags()
                    | WindowFlags::ALWAYS_AUTO_RESIZE
                    | WindowFlags::NO_TITLE_BAR
                    | WindowFlags::NO_MOVE
                    | WindowFlags::NO_RESIZE
                    | WindowFlags::NO_SCROLLBAR,
            )
            .bg_alpha(0.92)
            .build(|| {
                self.hit_regions.push(window_rect(ui));
                widgets::sheet_header(ui, &self.assets.ui_icons, "Performance", &mut open);
                let fps = report.map_or("—".into(), |r| format!("{:.1}", r.presentation_fps));
                {
                    let _font = ui.push_font(ui.fonts().fonts()[4]);
                    ui.text(&fps);
                }
                ui.same_line();
                ui.text_disabled("FPS");
                if let Some(report) = report {
                    widgets::key_value(
                        ui,
                        "Decode",
                        &format!("{:.1} ms", report.decode_us as f64 / 1000.0),
                    );
                    widgets::key_value(
                        ui,
                        "Upload",
                        &format!("{:.1} ms", report.upload_us as f64 / 1000.0),
                    );
                    widgets::key_value(
                        ui,
                        "Frame age",
                        &format!("{:.1} ms", report.frame_age_us as f64 / 1000.0),
                    );
                    widgets::key_value(ui, "Dropped", &format!("{:.1}%", report.drop_percent));
                    widgets::key_value(
                        ui,
                        "Queue · encoded / decoded",
                        &format!(
                            "{} / {}",
                            report.encoded_video_depth, report.decoded_video_depth
                        ),
                    );
                } else {
                    widgets::description(ui, "Waiting for stream measurements…");
                }
                if let Some((w, h)) = self.controller.video_dimensions {
                    widgets::key_value(ui, "Video", &format!("{w} × {h}"));
                }
                ui.dummy([0.0, 4.0]);
                ui.separator();
                let server = self
                    .controller
                    .inspected
                    .as_ref()
                    .map_or("—", |s| s.version.as_str());
                let _font = ui.push_font(ui.fonts().fonts()[2]);
                ui.text_disabled(format!("Client {} · Server {server}", SURF_VERSION.trim()));
            });
        self.performance_open = open;
    }
}
