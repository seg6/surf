use super::*;
use chrono::{DateTime, Local, NaiveDate};

fn date_heading(date: NaiveDate, today: NaiveDate) -> String {
    if date == today {
        "Today".into()
    } else if Some(date) == today.pred_opt() {
        "Yesterday".into()
    } else {
        date.format("%d %b %Y").to_string()
    }
}

fn entry_date(ts: i64, today: NaiveDate) -> (String, String) {
    match DateTime::from_timestamp(ts, 0) {
        Some(date) => {
            let local = date.with_timezone(&Local);
            (
                date_heading(local.date_naive(), today),
                local.format("%H:%M").to_string(),
            )
        }
        None => ("Unknown date".into(), String::new()),
    }
}

impl DesktopApp {
    pub(super) fn draw_library(&mut self, ui: &Ui) {
        let display = ui.io().display_size;
        let page = self.layout.page;
        let height = if self.controller.connected {
            page.height as f32
        } else {
            display[1]
        };
        let top = if self.controller.connected {
            page.y as f32
        } else {
            0.0
        };
        let size = [(display[0] - 16.0).min(680.0), (height - 16.0).min(560.0)];
        let mut open = true;
        let mut actions = Vec::new();
        ui.window("Library###library")
            .position([display[0] * 0.5, top + height * 0.5], Condition::Always)
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
                widgets::sheet_header(ui, &self.assets.ui_icons, "Library", &mut open);
                ui.dummy([0.0, 6.0]);
                let width = (ui.content_region_avail()[0] - 8.0) / 3.0;
                let palette = Palette::new(self.controller.dark_mode);
                for (section, label) in [
                    (LibrarySection::History, "History"),
                    (LibrarySection::Bookmarks, "Bookmarks"),
                    (LibrarySection::Downloads, "Downloads"),
                ] {
                    if section != LibrarySection::History {
                        ui.same_line();
                    }
                    let active = self.library_section == section;
                    let _bg = ui.push_style_color(
                        imgui::StyleColor::Button,
                        if active {
                            palette.hover
                        } else {
                            palette.surface
                        },
                    );
                    let _text = ui.push_style_color(
                        imgui::StyleColor::Text,
                        if active {
                            palette.accent
                        } else {
                            palette.muted
                        },
                    );
                    if ui.button_with_size(label, [width, 32.0]) {
                        self.library_section = section;
                    }
                }
                ui.dummy([0.0, 6.0]);
                ui.set_next_item_width(-1.0);
                let hint = match self.library_section {
                    LibrarySection::History => "Search history",
                    LibrarySection::Bookmarks => "Search bookmarks",
                    LibrarySection::Downloads => "Search downloads",
                };
                ui.input_text("##library-filter", &mut self.library_filter)
                    .hint(hint)
                    .build();
                ui.dummy([0.0, 8.0]);
                let filter = self.library_filter.to_lowercase();
                let list_height = (ui.content_region_avail()[1] - 32.0).max(60.0);
                widgets::inset(ui, "##library-list", list_height, || {
                    if self.library_section == LibrarySection::Downloads {
                        self.draw_download_rows(ui, &filter, &mut actions);
                    } else {
                        self.draw_page_rows(ui, &filter, &mut actions);
                    }
                });
                ui.dummy([0.0, 4.0]);
                let _font = ui.push_font(ui.fonts().fonts()[2]);
                ui.text_disabled(match self.library_section {
                    LibrarySection::History => "Pages visited on your Surf computer",
                    LibrarySection::Bookmarks => "Saved pages · use the row menu to manage",
                    LibrarySection::Downloads => "Files on your Surf computer · save a local copy",
                });
            });
        if !open {
            self.panel = None;
        }
        self.apply_actions(actions);
    }

    fn draw_page_rows(&mut self, ui: &Ui, filter: &str, actions: &mut Vec<Action>) {
        let history = self.library_section == LibrarySection::History;
        let mut items = if history {
            self.controller.browser.history.clone()
        } else {
            self.controller.browser.bookmarks.clone()
        };
        if history {
            items.sort_by_key(|item| std::cmp::Reverse(item.ts));
        }
        let mut matches = 0;
        let today = Local::now().date_naive();
        let mut group = String::new();
        for item in items.iter().filter(|i| {
            format!("{} {}", i.title, i.url)
                .to_lowercase()
                .contains(filter)
        }) {
            matches += 1;
            let (date, time) = entry_date(item.ts, today);
            if history && date != group {
                section(ui, &date);
                group = date;
            }
            let _id = ui.push_id(format!("{}-{}", item.url, item.ts));
            let start = ui.cursor_pos();
            let width = ui.content_region_avail()[0];
            let subtitle = if history {
                format!("{}  ·  {time}", compact_address(&item.url))
            } else {
                compact_address(&item.url)
            };
            let title = if item.title.is_empty() {
                &item.url
            } else {
                &item.title
            };
            if row(ui, "open", title, &subtitle, false, width - 36.0) {
                actions.push(Action::Navigate(item.url.clone()));
            }
            let bottom = ui.cursor_pos()[1];
            ui.set_cursor_pos([start[0] + width - 30.0, start[1] + 7.0]);
            {
                let _bg = ui.push_style_color(imgui::StyleColor::Button, [0.0; 4]);
                if icon_button(
                    ui,
                    &self.assets.ui_icons,
                    "manage",
                    icon::MORE,
                    "Page actions",
                ) {
                    ui.open_popup("##page-actions");
                }
            }
            if let Some(_popup) = ui.begin_popup("##page-actions") {
                if ui.menu_item("Open page") {
                    actions.push(Action::Navigate(item.url.clone()));
                }
                if ui.menu_item("Copy link") {
                    ui.set_clipboard_text(&item.url);
                }
                ui.separator();
                if widgets::confirm_button(
                    ui,
                    "Remove…",
                    if history {
                        "Remove this entry from your browsing history?"
                    } else {
                        "Remove this bookmark?"
                    },
                ) {
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
                    ui.close_current_popup();
                }
            }
            ui.set_cursor_pos([start[0], bottom]);
            ui.separator();
        }
        if matches == 0 {
            section(
                ui,
                if items.is_empty() {
                    if history {
                        "No history yet"
                    } else {
                        "No bookmarks yet"
                    }
                } else {
                    "No matching pages"
                },
            );
            widgets::description(
                ui,
                if !filter.is_empty() {
                    "Try a different title or website address."
                } else if history {
                    "Pages you visit will appear here, grouped by date."
                } else {
                    "Bookmark a page from Browser tools to keep it here."
                },
            );
        }
        if history && !items.is_empty() {
            ui.dummy([0.0, 12.0]);
            if ui.button("Load older history") {
                actions.push(Action::Command(Command::History {
                    q: String::new(),
                    offset: items.len() as i32,
                    causal: Causal::default(),
                }));
            }
        }
    }

    fn draw_download_rows(&mut self, ui: &Ui, filter: &str, actions: &mut Vec<Action>) {
        let items = self.controller.browser.downloads.clone();
        let mut matches = 0;
        for item in items
            .iter()
            .filter(|i| i.name.to_lowercase().contains(filter))
        {
            matches += 1;
            let _id = ui.push_id(&item.name);
            let progress = self
                .controller
                .browser
                .download_progress
                .get(&item.name)
                .copied();
            let detail = progress.map_or_else(
                || format!("{}  ·  On your computer", format_bytes(item.size)),
                |p| format!("Saving local copy · {p}%"),
            );
            let start = ui.cursor_pos();
            let width = ui.content_region_avail()[0];
            if row(ui, "file", &item.name, &detail, false, width - 36.0) {
                ui.open_popup("##download-actions");
            }
            let bottom = ui.cursor_pos()[1];
            ui.set_cursor_pos([start[0] + width - 30.0, start[1] + 7.0]);
            {
                let _bg = ui.push_style_color(imgui::StyleColor::Button, [0.0; 4]);
                if icon_button(
                    ui,
                    &self.assets.ui_icons,
                    "manage",
                    icon::MORE,
                    "File actions",
                ) {
                    ui.open_popup("##download-actions");
                }
            }
            if let Some(_popup) = ui.begin_popup("##download-actions") {
                if ui.menu_item("Save to Downloads") {
                    actions.push(Action::Download(item.name.clone()));
                }
                ui.separator();
                if widgets::confirm_button(
                    ui,
                    "Remove file…",
                    "Delete this file from your Surf computer? This cannot be undone.",
                ) {
                    actions.push(Action::Command(Command::DownloadDelete {
                        name: item.name.clone(),
                        causal: Causal::default(),
                    }));
                    actions.push(Action::Command(Command::Downloads {
                        causal: Causal::default(),
                    }));
                    ui.close_current_popup();
                }
            }
            ui.set_cursor_pos([start[0], bottom]);
            if let Some(progress) = progress {
                imgui::ProgressBar::new(progress as f32 / 100.0)
                    .size([width, 3.0])
                    .overlay_text("")
                    .build(ui);
            }
            ui.separator();
        }
        if matches == 0 {
            section(
                ui,
                if items.is_empty() {
                    "No downloads yet"
                } else {
                    "No matching files"
                },
            );
            widgets::description(
                ui,
                if items.is_empty() {
                    "Files downloaded in the browser appear here. Save a copy to use them on this device."
                } else {
                    "Try a different file name."
                },
            );
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn history_groups_follow_calendar_days() {
        let today = NaiveDate::from_ymd_opt(2026, 1, 1).unwrap();
        assert_eq!(date_heading(today, today), "Today");
        assert_eq!(date_heading(today.pred_opt().unwrap(), today), "Yesterday");
        assert_eq!(
            date_heading(NaiveDate::from_ymd_opt(2025, 12, 30).unwrap(), today),
            "30 Dec 2025"
        );
        assert_eq!(entry_date(i64::MAX, today).0, "Unknown date");
    }
}
