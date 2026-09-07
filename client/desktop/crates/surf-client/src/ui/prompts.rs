use super::*;

impl DesktopApp {
    pub(super) fn draw_files(&mut self, ui: &Ui) {
        let Some(picker) = &mut self.file_picker else {
            if let Some(_popup) = ui.begin_modal_popup("Choose files###file-picker") {
                ui.close_current_popup();
            }
            return;
        };
        if !picker.opened {
            ui.open_popup("Choose files###file-picker");
            picker.opened = true;
        }
        picker.refresh();
        let display = ui.io().display_size;
        let size = [
            (display[0] - 16.0).min(680.0),
            (display[1] - 16.0).min(520.0),
        ];
        widgets::place_next(
            [(display[0] - size[0]) * 0.5, (display[1] - size[1]) * 0.5],
            size,
        );
        let mut complete = None;
        ui.modal_popup_config("Choose files###file-picker")
            .flags(WindowFlags::NO_RESIZE | WindowFlags::NO_MOVE | WindowFlags::NO_TITLE_BAR)
            .build(|| {
                self.hit_regions.push(window_rect(ui));
                let mut open = true;
                widgets::sheet_header(ui, &self.assets.ui_icons, "Choose files", &mut open);
                if !open {
                    complete = Some(Vec::new());
                }
                ui.dummy([0.0, 6.0]);
                if let Some(dirs) = directories::UserDirs::new() {
                    if ui.button("Home") {
                        picker.directory = dirs.home_dir().to_owned();
                        picker.selected.clear();
                    }
                    if let Some(downloads) = dirs.download_dir() {
                        ui.same_line();
                        if ui.button("Downloads") {
                            picker.directory = downloads.to_owned();
                            picker.selected.clear();
                        }
                    }
                }
                ui.same_line();
                if ui.button("Up")
                    && let Some(parent) = picker.directory.parent()
                {
                    picker.directory = parent.to_owned();
                    picker.selected.clear();
                }
                ui.dummy([0.0, 4.0]);
                ui.text_disabled(widgets::ellipsize(
                    ui,
                    &picker.directory.display().to_string(),
                    ui.content_region_avail()[0],
                ));
                if ui.is_item_hovered() {
                    ui.tooltip_text(picker.directory.display().to_string());
                }
                ui.dummy([0.0, 6.0]);
                let list_height = (ui.content_region_avail()[1] - 68.0).max(40.0);
                widgets::inset(ui, "##files", list_height, || {
                    for entry in picker.entries.clone() {
                        let selected = picker.selected.contains(&entry.path);
                        if row(
                            ui,
                            &entry.path.to_string_lossy(),
                            &entry.name,
                            if entry.is_dir { "Folder" } else { "" },
                            selected,
                            ui.content_region_avail()[0],
                        ) {
                            if entry.is_dir {
                                picker.directory = entry.path;
                                picker.selected.clear();
                                break;
                            }
                            if !picker.multiple {
                                picker.selected.clear();
                            }
                            if !picker.selected.insert(entry.path.clone()) {
                                picker.selected.remove(&entry.path);
                            }
                        }
                    }
                    if let Some(error) = &picker.error {
                        ui.text_wrapped(error);
                    }
                });
                ui.dummy([0.0, 4.0]);
                ui.text_disabled(format!("{} selected", picker.selected.len()));
                {
                    let _disabled = ui.begin_disabled(picker.selected.is_empty());
                    if widgets::primary_button(
                        ui,
                        "Upload selected",
                        (ui.content_region_avail()[0] - 84.0).max(100.0),
                    ) {
                        complete = Some(picker.selected.iter().cloned().collect());
                    }
                }
                ui.same_line();
                if ui.button_with_size("Cancel", [80.0, 32.0]) || ui.is_key_pressed(ImKey::Escape) {
                    complete = Some(Vec::new());
                }
                if complete.is_some() {
                    ui.close_current_popup();
                }
            });
        if let Some(paths) = complete {
            self.file_picker = None;
            self.panel = None;
            self.controller.choose_files(paths);
        }
    }

    pub(super) fn draw_page_dialog(&mut self, ui: &Ui) {
        let Some(prompt) = self.controller.browser.dialog.clone() else {
            if !self.dialog_signature.is_empty() {
                if let Some(_popup) = ui.begin_modal_popup("This page says###page-dialog") {
                    ui.close_current_popup();
                }
                self.dialog_signature.clear();
            }
            return;
        };
        let signature = self.controller.browser.dialog_revision.to_string();
        if self.dialog_signature != signature {
            self.dialog_signature = signature;
            self.dialog_input.clone_from(&prompt.input);
            self.release_page_input();
            ui.open_popup("This page says###page-dialog");
        }
        let display = ui.io().display_size;
        let width = (display[0] - 32.0).min(460.0);
        widgets::place_next(
            [display[0] * 0.5 - width * 0.5, display[1] * 0.3],
            [width, 0.0],
        );
        let mut reply = None;
        ui.modal_popup_config("This page says###page-dialog")
            .flags(WindowFlags::ALWAYS_AUTO_RESIZE | WindowFlags::NO_MOVE | WindowFlags::NO_RESIZE)
            .build(|| {
                self.hit_regions.push(window_rect(ui));
                ui.text_wrapped(&prompt.text);
                if prompt.kind == "prompt" {
                    ui.set_next_item_width(-1.0);
                    if ui
                        .input_text("##dialog-input", &mut self.dialog_input)
                        .enter_returns_true(true)
                        .build()
                    {
                        reply = Some(true);
                    }
                }
                if prompt.kind != "alert" {
                    if ui.button("Cancel") {
                        reply = Some(false);
                    }
                    ui.same_line();
                }
                if ui.button("OK") {
                    reply = Some(true);
                }
                if ui.is_key_pressed(ImKey::Escape) {
                    reply = Some(false);
                }
                if reply.is_some() {
                    ui.close_current_popup();
                }
            });
        if let Some(accept) = reply {
            self.controller
                .reply_dialog(accept, self.dialog_input.clone());
        }
    }

    pub(super) fn draw_page_select(&mut self, ui: &Ui) {
        let Some(prompt) = self.controller.browser.select.clone() else {
            if !self.select_signature.is_empty() {
                if let Some(_popup) = ui.begin_popup("##page-select") {
                    self.hit_regions.push(window_rect(ui));
                    ui.close_current_popup();
                }
                self.select_signature.clear();
                self.select_values.clear();
            }
            return;
        };
        if self.select_signature != prompt.id {
            self.select_signature.clone_from(&prompt.id);
            self.select_values.clone_from(&prompt.selected);
            self.release_page_input();
            ui.open_popup("##page-select");
        }
        let display = ui.io().display_size;
        let p = self.input_rect;
        let anchor = prompt
            .rect
            .map(|r| {
                [
                    (p.x + r[0] * p.width) as f32,
                    (p.y + r[1] * p.height) as f32,
                    (r[2] * p.width) as f32,
                    (r[3] * p.height) as f32,
                ]
            })
            .unwrap_or([display[0] * 0.5 - 120.0, display[1] * 0.25, 240.0, 30.0]);
        let (position, size) = crate::layout::anchored(
            display,
            anchor,
            [
                anchor[2].clamp(240.0, 420.0),
                (prompt.options.len() as f32 * 34.0 + if prompt.multiple { 86.0 } else { 48.0 })
                    .min(display[1] * 0.6),
            ],
        );
        widgets::place_next(position, size);
        let mut reply = None;
        let mut cancelled = false;
        if let Some(_popup) = ui.begin_popup("##page-select") {
            if !prompt.title.is_empty() {
                ui.text_wrapped(&prompt.title);
                ui.separator();
            }
            ui.child_window("##options")
                .size([0.0, if prompt.multiple { -38.0 } else { 0.0 }])
                .build(|| {
                    for (index, option) in prompt.options.iter().enumerate() {
                        let selected = self.select_values.get(index).copied().unwrap_or(false);
                        if ui
                            .selectable_config(format!("{}###option-{index}", option.label))
                            .selected(selected)
                            .disabled(option.disabled)
                            .close_popups(false)
                            .size([0.0, 30.0])
                            .build()
                        {
                            if prompt.multiple {
                                if let Some(value) = self.select_values.get_mut(index) {
                                    *value = !*value;
                                }
                            } else {
                                reply = Some(vec![index as i32]);
                            }
                        }
                    }
                });
            if prompt.multiple && ui.button("Done") {
                reply = Some(
                    self.select_values
                        .iter()
                        .enumerate()
                        .filter_map(|(i, v)| v.then_some(i as i32))
                        .collect(),
                );
            }
            if ui.is_key_pressed(ImKey::Escape) {
                cancelled = true;
                reply = Some(Vec::new());
            }
            if reply.is_some() {
                ui.close_current_popup();
            }
        } else {
            cancelled = true;
            reply = Some(Vec::new());
        }
        if let Some(indices) = reply {
            self.controller.reply_select(cancelled, indices);
        }
    }

    pub(super) fn draw_page_error(&mut self, ui: &Ui) {
        let Some(url) = self.controller.browser.page_error.clone() else {
            return;
        };
        let display = ui.io().display_size;
        let mut close = false;
        ui.window("Page unavailable")
            .position([display[0] * 0.5, display[1] * 0.5], Condition::Always)
            .position_pivot([0.5, 0.5])
            .flags(overlay_flags() | WindowFlags::ALWAYS_AUTO_RESIZE)
            .build(|| {
                self.hit_regions.push(window_rect(ui));
                ui.text_disabled(compact_address(&url));
                if ui.button("Close") {
                    close = true;
                }
            });
        if close {
            self.controller.browser.page_error = None;
        }
    }

    pub(super) fn draw_toast(&self, ui: &Ui) {
        let Some(toast) = &self.controller.browser.toast else {
            return;
        };
        let display = ui.io().display_size;
        small_overlay(ui, [display[0] * 0.5, display[1] - 34.0], &toast.text);
    }
}
