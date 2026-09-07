use super::*;

// Event ownership lives here. Every event still reaches ImGui; remote presses
// retain their release owner, and page hits use current surface geometry.
impl DesktopApp {
    pub(super) fn release_page_input(&mut self) {
        for (_, (key, code, key_code)) in std::mem::take(&mut self.remote_keys) {
            if let Ok(command) = self.page_input.key(
                false,
                key,
                code,
                key_code,
                String::new(),
                Modifiers::default(),
            ) {
                self.controller.command(command);
            }
        }
        for command in self
            .page_input
            .release_buttons(self.page_rect, self.video.surface_generation())
        {
            self.controller.command(command);
        }
        if self.page_input.composition_active()
            && let Ok(command) = self.page_input.compose("cancel", String::new(), 0, 0)
        {
            self.controller.command(command);
        }
        self.page_focused = false;
        self.local_preedit.clear();
    }

    // WinitPlatform currently does not forward IME events. This narrow host adapter
    // commits local text once; preedit is presentation-only, never an InputText draft.

    pub fn handle_local_ime(&mut self, io: &mut imgui::Io, event: &winit::event::Event<()>) {
        if self.page_accepts_keyboard() {
            return;
        }
        if let winit::event::Event::WindowEvent {
            event: WindowEvent::Ime(ime),
            ..
        } = event
        {
            apply_local_ime(&mut self.local_preedit, io, ime);
        }
    }

    pub fn handle_window_event(&mut self, window: &Window, event: &WindowEvent) {
        match event {
            WindowEvent::ModifiersChanged(modifiers) => {
                self.modifiers = modifiers_from_winit(*modifiers);
            }
            WindowEvent::CursorMoved { position, .. } => {
                let position: LogicalPosition<f64> = position.to_logical(window.scale_factor());
                self.cursor = Some((position.x, position.y));
                if self.page_accepts_pointer(position.x, position.y) || self.page_input.dragging() {
                    match self.page_input.motion(
                        position.x,
                        position.y,
                        self.input_rect,
                        self.video.surface_generation(),
                        self.modifiers,
                    ) {
                        Ok(Some(command)) => self.controller.command(command),
                        Ok(None) => {}
                        Err(error) => self.report_host_error(format!("pointer input: {error}")),
                    }
                }
            }
            WindowEvent::MouseInput { state, button, .. } => {
                let Some((x, y)) = self.cursor else {
                    return;
                };
                let pressed = *state == ElementState::Pressed;
                if pressed {
                    // Dismissal is a local click, not an accidental click through to a page.
                    if self.panel.is_some()
                        && !self.hit_regions.iter().any(|r| r.contains(x, y))
                        && !self.native_popup_open
                    {
                        if self.panel == Some(Panel::Reader) {
                            self.controller.browser.reader = None;
                        }
                        self.panel = None;
                        self.page_focused = false;
                        return;
                    }
                    let page_hit = self.page_accepts_pointer(x, y);
                    let a = self.omnibox_rect;
                    if !self.native_popup_open
                        && self.panel.is_none()
                        && !self.address_editing
                        && x >= f64::from(a[0])
                        && x < f64::from(a[2])
                        && y >= f64::from(a[1])
                        && y < f64::from(a[3])
                    {
                        self.edit_address();
                    }
                    self.page_focused = page_hit;
                    if page_hit {
                        self.finish_address_edit();
                    }
                }
                if (self.page_accepts_pointer(x, y) || !pressed)
                    && let Some(number) = mouse_button_number(*button)
                {
                    match self.page_input.button(
                        pressed,
                        number,
                        x,
                        y,
                        self.input_rect,
                        self.video.surface_generation(),
                        self.modifiers,
                    ) {
                        Ok(Some(command)) => self.controller.command(command),
                        Ok(None) => {}
                        Err(error) => self.report_host_error(format!("pointer input: {error}")),
                    }
                }
            }
            WindowEvent::MouseWheel { delta, .. } => {
                let (dx, dy) = match delta {
                    MouseScrollDelta::LineDelta(x, y) => {
                        (f64::from(*x) * 50.0, f64::from(*y) * 50.0)
                    }
                    MouseScrollDelta::PixelDelta(position) => {
                        let position: LogicalPosition<f64> =
                            position.to_logical(window.scale_factor());
                        (position.x, position.y)
                    }
                };
                if self
                    .cursor
                    .is_some_and(|(x, y)| self.page_accepts_pointer(x, y))
                {
                    match self.page_input.wheel(
                        dx,
                        -dy,
                        self.input_rect,
                        self.video.surface_generation(),
                        self.modifiers,
                    ) {
                        Ok(commands) => {
                            for command in commands {
                                self.controller.command(command);
                            }
                        }
                        Err(error) => self.report_host_error(format!("wheel input: {error}")),
                    }
                }
            }
            WindowEvent::KeyboardInput { event, .. } => self.handle_key(event),
            WindowEvent::Ime(ime) if self.page_accepts_keyboard() => {
                let command = match ime {
                    Ime::Preedit(text, cursor) if text.is_empty() => self
                        .page_input
                        .composition_active()
                        .then(|| self.page_input.compose("cancel", String::new(), 0, 0)),
                    Ime::Preedit(text, cursor) => {
                        let (start, end) = cursor.unwrap_or((text.len(), text.len()));
                        Some(self.page_input.compose(
                            "update",
                            text.clone(),
                            utf16_offset(text, start),
                            utf16_offset(text, end),
                        ))
                    }
                    Ime::Commit(text) => Some(self.page_input.compose(
                        if text.is_empty() { "cancel" } else { "commit" },
                        text.clone(),
                        0,
                        0,
                    )),
                    Ime::Enabled | Ime::Disabled => None,
                };
                if let Some(command) = command {
                    match command {
                        Ok(command) => self.controller.command(command),
                        Err(error) => self.report_host_error(format!("composition input: {error}")),
                    }
                }
            }
            WindowEvent::Focused(false) => self.release_page_input(),
            _ => {}
        }
    }

    pub(super) fn handle_key(&mut self, event: &KeyEvent) {
        let pressed = event.state == ElementState::Pressed;
        if !pressed
            && let Some((key, code, key_code)) = self.remote_keys.remove(&event.physical_key)
        {
            if let Ok(command) =
                self.page_input
                    .key(false, key, code, key_code, String::new(), self.modifiers)
            {
                self.controller.command(command);
            }
            return;
        }
        if self.controller.browser.dialog.is_none()
            && self.controller.browser.select.is_none()
            && self.panel != Some(Panel::Files)
            && self.handle_shortcut(event, pressed)
        {
            return;
        }
        if !self.page_accepts_keyboard() {
            return;
        }
        if pressed
            && (self.modifiers.control || self.modifiers.command)
            && logical_character(&event.logical_key)
                .is_some_and(|value| value.eq_ignore_ascii_case("v"))
        {
            if let Ok(text) = arboard::Clipboard::new().and_then(|mut value| value.get_text())
                && let Ok(command) = self.page_input.paste(text)
            {
                self.controller.command(command);
            }
            return;
        }
        let plain_text = plain_key_text(
            &event.logical_key,
            event.text.as_deref(),
            self.modifiers,
            pressed,
            self.page_input.composition_active(),
        );
        if let Some(text) = plain_text {
            match self
                .page_input
                .key(true, String::new(), String::new(), 0, text, self.modifiers)
            {
                Ok(command) => self.controller.command(command),
                Err(error) => self.report_host_error(format!("text input: {error}")),
            }
            return;
        }
        let (key, code, key_code, _) = dom_key(event);
        if pressed {
            self.remote_keys
                .insert(event.physical_key, (key.clone(), code.clone(), key_code));
        }
        let textual_key = logical_character(&event.logical_key).is_some()
            || matches!(event.logical_key, Key::Named(NamedKey::Space));
        if !pressed && textual_key && !self.modifiers.control && !self.modifiers.alt {
            return;
        }
        match self
            .page_input
            .key(pressed, key, code, key_code, String::new(), self.modifiers)
        {
            Ok(command) => self.controller.command(command),
            Err(error) => self.report_host_error(format!("keyboard input: {error}")),
        }
    }

    pub(super) fn handle_shortcut(&mut self, event: &KeyEvent, pressed: bool) -> bool {
        if self.native_popup_open {
            return false;
        }
        let character = logical_character(&event.logical_key).map(str::to_ascii_lowercase);
        let command = self.modifiers.control || self.modifiers.command;
        if command {
            match character {
                Some(ref value) if value == "l" => {
                    if pressed {
                        self.edit_address();
                    }
                    return true;
                }
                Some(ref value) if value == "t" => {
                    if pressed {
                        self.new_tab();
                    }
                    return true;
                }
                Some(ref value) if value == "w" => {
                    if pressed {
                        self.close_active_tab();
                    }
                    return true;
                }
                Some(ref value) if value == "r" => {
                    if pressed {
                        self.reload_or_stop();
                    }
                    return true;
                }
                Some(ref value) if value == "d" => {
                    if pressed {
                        self.controller.command(Command::Bookmark {
                            causal: Causal::default(),
                        });
                    }
                    return true;
                }
                Some(ref value) if value == "f" => {
                    if pressed {
                        self.find_open = true;
                        self.focus_find = true;
                        self.page_focused = false;
                    }
                    return true;
                }
                _ => {}
            }
        }
        if self.modifiers.alt {
            match event.logical_key {
                Key::Named(NamedKey::ArrowLeft) => {
                    if pressed {
                        self.controller.command(Command::Back {
                            causal: Causal::default(),
                        });
                    }
                    return true;
                }
                Key::Named(NamedKey::ArrowRight) => {
                    if pressed {
                        self.controller.command(Command::Forward {
                            causal: Causal::default(),
                        });
                    }
                    return true;
                }
                _ => {}
            }
        }
        match event.logical_key {
            Key::Named(NamedKey::F5) => {
                if pressed {
                    self.reload_or_stop();
                }
                true
            }
            Key::Named(NamedKey::F11) => {
                if pressed {
                    let on = !self.fullscreen;
                    self.set_fullscreen_command(on);
                }
                true
            }
            Key::Named(NamedKey::Escape) if pressed && self.panel.is_some() => {
                if self.panel == Some(Panel::Reader) {
                    self.controller.browser.reader = None;
                }
                if self.panel == Some(Panel::Files) {
                    self.controller.choose_files(Vec::new());
                    self.file_picker = None;
                }
                self.panel = None;
                self.page_focused = true;
                true
            }
            Key::Named(NamedKey::Escape) if pressed && self.address_editing => {
                self.finish_address_edit();
                true
            }
            Key::Named(NamedKey::Escape) if pressed && self.find_open => {
                self.find_open = false;
                self.controller.command(Command::Find {
                    q: String::new(),
                    dir: 0,
                    causal: Causal::default(),
                });
                true
            }
            _ => false,
        }
    }

    pub(super) fn page_accepts_pointer(&self, x: f64, y: f64) -> bool {
        self.controller.connected
            && !self.hit_regions.iter().any(|r| r.contains(x, y))
            && !self.native_popup_open
            && self.controller.browser.dialog.is_none()
            && self.controller.browser.select.is_none()
            && self.controller.browser.page_error.is_none()
            && self.input_rect.contains(x, y)
    }

    pub(super) fn page_accepts_keyboard(&self) -> bool {
        self.controller.connected
            && self.page_focused
            && !self.address_editing
            && !self.ui_wants_keyboard
            && self.controller.browser.dialog.is_none()
            && self.controller.browser.select.is_none()
            && self.controller.browser.page_error.is_none()
    }
}

fn apply_local_ime(preedit: &mut String, io: &mut imgui::Io, ime: &Ime) {
    match ime {
        Ime::Preedit(text, _) => preedit.clone_from(text),
        Ime::Commit(text) => {
            preedit.clear();
            if io.want_text_input {
                for c in text.chars() {
                    io.add_input_character(c);
                }
            }
        }
        Ime::Disabled => preedit.clear(),
        Ime::Enabled => {}
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn local_ime_preedit_is_visual_and_commit_reaches_input_text_once() {
        let mut context = imgui::Context::create();
        context.set_ini_filename(None);
        context.io_mut().display_size = [400.0, 100.0];
        context.io_mut().delta_time = 1.0 / 60.0;
        context.fonts().build_rgba32_texture();
        let mut text = String::new();
        let mut preedit = String::new();
        let frame = |context: &mut imgui::Context, text: &mut String, focus: bool| {
            let ui = context.frame();
            ui.window("ime-test")
                .position([0.0, 0.0], Condition::Always)
                .size([400.0, 100.0], Condition::Always)
                .build(|| {
                    if focus {
                        ui.set_keyboard_focus_here();
                    }
                    ui.input_text("##field", text).build();
                });
            context.render();
        };
        frame(&mut context, &mut text, true);
        frame(&mut context, &mut text, false);
        for value in ["に", "日本"] {
            apply_local_ime(
                &mut preedit,
                context.io_mut(),
                &Ime::Preedit(value.into(), None),
            );
            frame(&mut context, &mut text, false);
            assert!(text.is_empty());
            assert_eq!(preedit, value);
        }
        apply_local_ime(&mut preedit, context.io_mut(), &Ime::Commit("日本".into()));
        frame(&mut context, &mut text, false);
        assert_eq!(text, "日本");
        assert!(preedit.is_empty());
        frame(&mut context, &mut text, false);
        assert_eq!(text, "日本");
    }
}
