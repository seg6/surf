use super::*;

impl DesktopApp {
    pub(super) fn draw_files(&mut self, ui: &Ui) {
        let Some(picker) = &mut self.file_picker else {
            self.panel = None;
            return;
        };
        let display = ui.io().display_size;
        let width = (display[0] * 0.72)
            .clamp(280.0, 800.0)
            .min((display[0] - 16.0).max(280.0));
        let height = (display[1] * 0.72)
            .clamp(220.0, 600.0)
            .min((display[1] - 16.0).max(220.0));
        let mut open = true;
        let mut complete: Option<Vec<PathBuf>> = None;
        ui.window("Choose file")
            .position([display[0] * 0.5, display[1] * 0.5], Condition::Appearing)
            .position_pivot([0.5, 0.5])
            .size([width, height], Condition::Appearing)
            .opened(&mut open)
            .flags(overlay_flags())
            .build(|| {
                if ui.small_button("Up")
                    && let Some(parent) = picker.directory.parent()
                {
                    picker.directory = parent.to_owned();
                    picker.selected.clear();
                }
                ui.same_line();
                ui.text_disabled(picker.directory.display().to_string());
                ui.separator();
                ui.child_window("##files").size([0.0, -31.0]).build(|| {
                    let mut entries = match fs::read_dir(&picker.directory) {
                        Ok(entries) => entries.flatten().collect::<Vec<_>>(),
                        Err(error) => {
                            picker.error = Some(error.to_string());
                            Vec::new()
                        }
                    };
                    entries.sort_by_key(|entry| {
                        let is_file = entry.file_type().map_or(true, |kind| kind.is_file());
                        (is_file, entry.file_name().to_string_lossy().to_lowercase())
                    });
                    for entry in entries {
                        let path = entry.path();
                        let is_dir = entry.file_type().is_ok_and(|kind| kind.is_dir());
                        let selected = picker.selected.contains(&path);
                        let prefix = if is_dir { "[dir] " } else { "" };
                        let clicked = ui
                            .selectable_config(format!(
                                "{prefix}{}##{}",
                                entry.file_name().to_string_lossy(),
                                path.display()
                            ))
                            .selected(selected)
                            .allow_double_click(true)
                            .build();
                        if !clicked {
                            continue;
                        }
                        if is_dir && ui.is_mouse_double_clicked(ImMouseButton::Left) {
                            picker.directory = path;
                            picker.selected.clear();
                            break;
                        }
                        if !is_dir {
                            if !picker.multiple {
                                picker.selected.clear();
                            }
                            if !picker.selected.insert(path.clone()) {
                                picker.selected.remove(&path);
                            }
                        }
                    }
                    if let Some(error) = &picker.error {
                        ui.text_disabled(error);
                    }
                });
                let choose_label = if picker.multiple {
                    "Upload selected"
                } else {
                    "Upload"
                };
                if ui.button(choose_label) && !picker.selected.is_empty() {
                    complete = Some(picker.selected.iter().cloned().collect());
                }
                ui.same_line();
                if ui.button("Cancel") {
                    complete = Some(Vec::new());
                }
            });
        if !open && complete.is_none() {
            complete = Some(Vec::new());
        }
        if let Some(paths) = complete {
            self.file_picker = None;
            self.panel = None;
            self.controller.choose_files(paths);
        }
    }

    pub(super) fn draw_page_dialog(&mut self, ui: &Ui) {
        let Some(prompt) = self.controller.browser.dialog.clone() else {
            self.dialog_signature.clear();
            return;
        };
        let signature = format!("{}|{}|{}", prompt.kind, prompt.text, prompt.input);
        if self.dialog_signature != signature {
            self.dialog_signature = signature;
            self.dialog_input.clone_from(&prompt.input);
        }
        let display = ui.io().display_size;
        let width = (display[0] - 32.0).clamp(260.0, 520.0);
        let mut action = None;
        ui.window("This page says")
            .position([display[0] * 0.5, display[1] * 0.5], Condition::Always)
            .position_pivot([0.5, 0.5])
            .size_constraints(
                [width.min(360.0), 0.0],
                [width, (display[1] - 32.0).max(180.0)],
            )
            .flags(overlay_flags() | WindowFlags::ALWAYS_AUTO_RESIZE)
            .build(|| {
                ui.text_wrapped(&prompt.text);
                if prompt.kind == "prompt" {
                    ui.set_next_item_width(-1.0);
                    ui.input_text("##dialog-input", &mut self.dialog_input)
                        .enter_returns_true(true)
                        .build();
                }
                if prompt.kind != "alert" && ui.button("Cancel") {
                    action = Some(false);
                }
                if prompt.kind != "alert" {
                    ui.same_line();
                }
                if ui.button("OK") {
                    action = Some(true);
                }
            });
        if let Some(accept) = action {
            self.controller
                .reply_dialog(accept, self.dialog_input.clone());
        }
    }

    pub(super) fn draw_page_select(&mut self, ui: &Ui) {
        let Some(prompt) = self.controller.browser.select.clone() else {
            self.select_signature.clear();
            self.select_values.clear();
            return;
        };
        if self.select_signature != prompt.id {
            self.select_signature.clone_from(&prompt.id);
            self.select_values.clone_from(&prompt.selected);
        }
        let display = ui.io().display_size;
        let position = prompt
            .rect
            .map_or([display[0] * 0.5, display[1] * 0.5], |rect| {
                [
                    (self.page_rect.x + rect[0] * self.page_rect.width) as f32,
                    (self.page_rect.y + (rect[1] + rect[3]) * self.page_rect.height) as f32,
                ]
            });
        let mut reply = None;
        ui.window("Choose##page-select")
            .position(position, Condition::Always)
            .position_pivot(if prompt.rect.is_some() {
                [0.0, 0.0]
            } else {
                [0.5, 0.5]
            })
            .size_constraints([220.0, 0.0], [420.0, display[1] * 0.6])
            .flags(overlay_flags() | WindowFlags::ALWAYS_AUTO_RESIZE)
            .build(|| {
                if !prompt.title.is_empty() {
                    ui.text_wrapped(&prompt.title);
                    ui.separator();
                }
                for (index, option) in prompt.options.iter().enumerate() {
                    let selected = self.select_values.get(index).copied().unwrap_or(false);
                    if ui
                        .selectable_config(format!("{}##select-{index}", option.label))
                        .selected(selected)
                        .disabled(option.disabled)
                        .build()
                    {
                        if prompt.multiple {
                            if let Some(value) = self.select_values.get_mut(index) {
                                *value = !*value;
                            }
                        } else {
                            reply = Some(vec![i32::try_from(index).unwrap_or(i32::MAX)]);
                        }
                    }
                }
                if prompt.multiple && ui.button("Choose") {
                    reply = Some(
                        self.select_values
                            .iter()
                            .enumerate()
                            .filter_map(|(index, selected)| {
                                selected.then_some(i32::try_from(index).unwrap_or(i32::MAX))
                            })
                            .collect(),
                    );
                }
                ui.same_line();
                if ui.button("Cancel") {
                    self.controller.reply_select(true, Vec::new());
                }
            });
        if let Some(indices) = reply {
            self.controller.reply_select(false, indices);
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
