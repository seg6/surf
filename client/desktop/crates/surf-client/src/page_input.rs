use eframe::egui::{self, ImeEvent, Key, Modifiers, MouseWheelUnit, PointerButton, Pos2, Rect};
use surf_core::{CausalStamp, Error, InputSample, InputState, monotonic_ns};
use surf_protocol::{Causal, Command, TouchPoint};

const DOUBLE_CLICK_NS: u64 = 500_000_000;
const DOUBLE_CLICK_DISTANCE_SQ: f32 = 36.0;

pub struct PageInput {
    core: InputState,
    native_pointer: bool,
    buttons: i32,
    pointer_inside: bool,
    last_pointer: Option<Pos2>,
    last_click: Option<(u64, PointerButton, Pos2, i32)>,
    click_counts: [i32; 5],
    composition_active: bool,
    modifiers: Modifiers,
}

impl PageInput {
    pub fn new() -> Self {
        Self {
            core: InputState::new(),
            native_pointer: false,
            buttons: 0,
            pointer_inside: false,
            last_pointer: None,
            last_click: None,
            click_counts: [1; 5],
            composition_active: false,
            modifiers: Modifiers::NONE,
        }
    }

    pub fn set_native_pointer(&mut self, available: bool) {
        self.native_pointer = available;
    }

    pub fn reset(&mut self) {
        self.core.set_surface(0);
        self.buttons = 0;
        self.pointer_inside = false;
        self.last_pointer = None;
        self.last_click = None;
        self.click_counts = [1; 5];
        self.composition_active = false;
        self.modifiers = Modifiers::NONE;
    }

    pub fn translate(
        &mut self,
        events: &[egui::Event],
        surface: Rect,
        generation: u32,
        keyboard_focused: bool,
    ) -> Result<Vec<Command>, Error> {
        self.core.set_surface(generation);
        let mut commands = Vec::new();
        for event in events {
            match event {
                egui::Event::ModifiersChanged(modifiers) => {
                    self.modifiers = *modifiers;
                }
                egui::Event::PointerMoved(position) => {
                    self.pointer_moved(*position, surface, generation, &mut commands)?;
                }
                egui::Event::PointerButton {
                    pos,
                    button,
                    pressed,
                    modifiers,
                } => {
                    self.modifiers = *modifiers;
                    self.pointer_button(
                        *pos,
                        *button,
                        *pressed,
                        *modifiers,
                        surface,
                        generation,
                        &mut commands,
                    )?;
                }
                egui::Event::PointerGone | egui::Event::WindowFocused(false) => {
                    self.pointer_gone(surface, generation, &mut commands)?;
                }
                egui::Event::MouseWheel {
                    unit,
                    delta,
                    modifiers,
                    ..
                } => {
                    self.modifiers = *modifiers;
                    self.wheel(
                        *unit,
                        *delta,
                        *modifiers,
                        surface,
                        generation,
                        &mut commands,
                    )?;
                }
                egui::Event::Key {
                    key,
                    physical_key,
                    pressed,
                    modifiers,
                    ..
                } if keyboard_focused => {
                    self.modifiers = *modifiers;
                    self.key(*key, *physical_key, *pressed, *modifiers, &mut commands)?;
                }
                egui::Event::Text(text) if keyboard_focused && !text.is_empty() => {
                    commands.push(Command::Key {
                        down: true,
                        key: String::new(),
                        code: String::new(),
                        key_code: 0,
                        text: text.clone(),
                        mods: 0,
                        causal: self.causal()?,
                    });
                }
                egui::Event::Paste(text) if keyboard_focused && !text.is_empty() => {
                    commands.push(Command::Paste {
                        text: text.clone(),
                        causal: self.causal()?,
                    });
                }
                egui::Event::Ime(event) if keyboard_focused => {
                    self.ime(event, &mut commands)?;
                }
                _ => {}
            }
        }
        Ok(commands)
    }

    fn pointer_moved(
        &mut self,
        position: Pos2,
        surface: Rect,
        generation: u32,
        commands: &mut Vec<Command>,
    ) -> Result<(), Error> {
        self.last_pointer = Some(position);
        let inside = surface.contains(position);
        if generation == 0 {
            self.pointer_inside = inside;
            return Ok(());
        }
        if inside || self.buttons != 0 {
            self.pointer_inside = inside;
            let sample = self.sample(position, surface)?;
            if self.native_pointer {
                commands.push(self.pointer_command("move", "none", 0, self.modifiers, sample));
            } else if self.buttons & 1 != 0 {
                commands.push(touch_command("move", sample));
            }
        } else if self.pointer_inside {
            self.pointer_inside = false;
            if self.native_pointer {
                let sample = self.sample(position, surface)?;
                commands.push(self.pointer_command("leave", "none", 0, self.modifiers, sample));
            }
        }
        Ok(())
    }

    #[allow(clippy::too_many_arguments)]
    fn pointer_button(
        &mut self,
        position: Pos2,
        button: PointerButton,
        pressed: bool,
        modifiers: Modifiers,
        surface: Rect,
        generation: u32,
        commands: &mut Vec<Command>,
    ) -> Result<(), Error> {
        self.last_pointer = Some(position);
        let mask = pointer_mask(button);
        let was_pressed = self.buttons & mask != 0;
        if pressed {
            if generation == 0 || !surface.contains(position) || was_pressed {
                return Ok(());
            }
            self.buttons |= mask;
            self.pointer_inside = true;
        } else {
            if generation == 0 || !was_pressed {
                return Ok(());
            }
            self.buttons &= !mask;
            self.pointer_inside = surface.contains(position);
        }
        let sample = self.sample(position, surface)?;
        let clicks = self.click_count(button, position, pressed, sample.event_ns);
        if self.native_pointer {
            commands.push(self.pointer_command(
                if pressed { "down" } else { "up" },
                pointer_name(button),
                clicks,
                modifiers,
                sample,
            ));
        } else if button == PointerButton::Primary {
            commands.push(touch_command(if pressed { "start" } else { "end" }, sample));
        }
        Ok(())
    }

    fn pointer_gone(
        &mut self,
        surface: Rect,
        generation: u32,
        commands: &mut Vec<Command>,
    ) -> Result<(), Error> {
        if generation == 0 {
            self.buttons = 0;
            self.pointer_inside = false;
            return Ok(());
        }
        let position = self.last_pointer.unwrap_or(surface.center());
        if self.native_pointer {
            for button in [
                PointerButton::Primary,
                PointerButton::Secondary,
                PointerButton::Middle,
                PointerButton::Extra1,
                PointerButton::Extra2,
            ] {
                let mask = pointer_mask(button);
                if self.buttons & mask == 0 {
                    continue;
                }
                self.buttons &= !mask;
                let sample = self.sample(position, surface)?;
                commands.push(self.pointer_command(
                    "up",
                    pointer_name(button),
                    self.click_counts[button as usize],
                    self.modifiers,
                    sample,
                ));
            }
            if self.pointer_inside {
                let sample = self.sample(position, surface)?;
                commands.push(self.pointer_command("leave", "none", 0, self.modifiers, sample));
            }
        } else if self.buttons & 1 != 0 {
            let sample = self.sample(position, surface)?;
            commands.push(touch_command("cancel", sample));
        }
        self.buttons = 0;
        self.pointer_inside = false;
        Ok(())
    }

    fn wheel(
        &mut self,
        unit: MouseWheelUnit,
        delta: egui::Vec2,
        modifiers: Modifiers,
        surface: Rect,
        generation: u32,
        commands: &mut Vec<Command>,
    ) -> Result<(), Error> {
        let Some(position) = self
            .last_pointer
            .filter(|position| surface.contains(*position))
        else {
            return Ok(());
        };
        if generation == 0 || delta == egui::Vec2::ZERO {
            return Ok(());
        }
        let pixels = match unit {
            MouseWheelUnit::Point => delta,
            MouseWheelUnit::Line => delta * 50.0,
            MouseWheelUnit::Page => {
                egui::vec2(delta.x * surface.width(), delta.y * surface.height())
            }
        };
        if self.native_pointer {
            // egui deltas describe content movement, whereas DOM WheelEvent
            // deltas describe the wheel/gesture direction.
            let local = local_position(position, surface);
            let sample = self.core.wheel(
                (f64::from(local.x), f64::from(local.y)),
                (-f64::from(pixels.x), -f64::from(pixels.y)),
                (f64::from(surface.width()), f64::from(surface.height())),
                monotonic_ns(),
            )?;
            commands.push(Command::Wheel {
                seq: sample.sequence,
                surface: sample.surface_generation,
                ts: sample.event_ns,
                x: sample.x,
                y: sample.y,
                dx: sample.delta_x,
                dy: sample.delta_y,
                buttons: self.buttons,
                mods: modifier_mask(modifiers),
                causal: causal_from_sample(sample),
            });
        } else {
            // Older servers only understand direct manipulation. Turn one
            // wheel sample into a bounded one-contact swipe.
            let start = self.sample(position, surface)?;
            let moved = self.sample(position + pixels, surface)?;
            let end = self.sample(position + pixels, surface)?;
            commands.push(touch_command("start", start));
            commands.push(touch_command("move", moved));
            commands.push(touch_command("end", end));
        }
        Ok(())
    }

    fn key(
        &mut self,
        key: Key,
        physical_key: Option<Key>,
        pressed: bool,
        modifiers: Modifiers,
        commands: &mut Vec<Command>,
    ) -> Result<(), Error> {
        if modifiers.command && matches!(key, Key::L | Key::T | Key::W | Key::R | Key::V) {
            return Ok(());
        }
        if is_printable(key) && !modifiers.ctrl && !modifiers.alt && !modifiers.mac_cmd {
            return Ok(());
        }
        let mapping = key_mapping(key, physical_key.unwrap_or(key), modifiers.shift);
        commands.push(Command::Key {
            down: pressed,
            key: mapping.key,
            code: mapping.code,
            key_code: mapping.key_code,
            text: String::new(),
            mods: if self.native_pointer {
                modifier_mask(modifiers)
            } else {
                0
            },
            causal: self.causal()?,
        });
        Ok(())
    }

    fn ime(&mut self, event: &ImeEvent, commands: &mut Vec<Command>) -> Result<(), Error> {
        match event {
            ImeEvent::Preedit {
                text,
                active_range_chars,
            } if text.is_empty() => {
                if self.composition_active {
                    commands.push(Command::Compose {
                        phase: "cancel".to_owned(),
                        text: String::new(),
                        start: 0,
                        end: 0,
                        causal: self.causal()?,
                    });
                    self.composition_active = false;
                }
            }
            ImeEvent::Preedit {
                text,
                active_range_chars,
            } => {
                let range = active_range_chars.clone().unwrap_or_else(|| {
                    let count = text.chars().count();
                    count..count
                });
                commands.push(Command::Compose {
                    phase: "update".to_owned(),
                    text: text.clone(),
                    start: utf16_offset(text, range.start),
                    end: utf16_offset(text, range.end),
                    causal: self.causal()?,
                });
                self.composition_active = true;
            }
            ImeEvent::Commit(text) => {
                commands.push(Command::Compose {
                    phase: if text.is_empty() { "cancel" } else { "commit" }.to_owned(),
                    text: text.clone(),
                    start: 0,
                    end: 0,
                    causal: self.causal()?,
                });
                self.composition_active = false;
            }
            ImeEvent::DeleteSurrounding {
                before_chars,
                after_chars,
            } => {
                for _ in 0..(*before_chars).min(64) {
                    self.key(Key::Backspace, None, true, Modifiers::NONE, commands)?;
                    self.key(Key::Backspace, None, false, Modifiers::NONE, commands)?;
                }
                for _ in 0..(*after_chars).min(64) {
                    self.key(Key::Delete, None, true, Modifiers::NONE, commands)?;
                    self.key(Key::Delete, None, false, Modifiers::NONE, commands)?;
                }
            }
            #[allow(deprecated)]
            ImeEvent::Enabled | ImeEvent::Disabled => {}
        }
        Ok(())
    }

    fn sample(&mut self, position: Pos2, surface: Rect) -> Result<InputSample, Error> {
        let local = local_position(position, surface);
        self.core.pointer(
            (f64::from(local.x), f64::from(local.y)),
            (f64::from(surface.width()), f64::from(surface.height())),
            monotonic_ns(),
        )
    }

    fn pointer_command(
        &self,
        phase: &str,
        button: &str,
        clicks: i32,
        modifiers: Modifiers,
        sample: InputSample,
    ) -> Command {
        Command::Pointer {
            phase: phase.to_owned(),
            seq: sample.sequence,
            surface: sample.surface_generation,
            ts: sample.event_ns,
            x: sample.x,
            y: sample.y,
            button: button.to_owned(),
            buttons: self.buttons,
            mods: modifier_mask(modifiers),
            clicks,
            causal: causal_from_sample(sample),
        }
    }

    fn causal(&mut self) -> Result<Causal, Error> {
        Ok(causal_from_stamp(self.core.causal(monotonic_ns())?))
    }

    fn click_count(
        &mut self,
        button: PointerButton,
        position: Pos2,
        pressed: bool,
        now_ns: u64,
    ) -> i32 {
        let index = button as usize;
        if pressed {
            let clicks = self
                .last_click
                .filter(|(previous_ns, previous_button, previous_position, _)| {
                    *previous_button == button
                        && now_ns.saturating_sub(*previous_ns) <= DOUBLE_CLICK_NS
                        && previous_position.distance_sq(position) <= DOUBLE_CLICK_DISTANCE_SQ
                })
                .map_or(1, |(_, _, _, previous_clicks)| (previous_clicks + 1).min(3));
            self.last_click = Some((now_ns, button, position, clicks));
            self.click_counts[index] = clicks;
        }
        self.click_counts[index]
    }
}

impl Default for PageInput {
    fn default() -> Self {
        Self::new()
    }
}

fn local_position(position: Pos2, surface: Rect) -> egui::Vec2 {
    position - surface.min
}

fn touch_command(phase: &str, sample: InputSample) -> Command {
    Command::Touch {
        phase: phase.to_owned(),
        seq: sample.sequence,
        surface: sample.surface_generation,
        ts: sample.event_ns,
        points: if phase == "cancel" {
            Vec::new()
        } else {
            vec![TouchPoint {
                id: 1,
                x: sample.x,
                y: sample.y,
                rx: 0.0,
                ry: 0.0,
                force: 0.5,
            }]
        },
        causal: causal_from_sample(sample),
    }
}

fn causal_from_sample(sample: InputSample) -> Causal {
    Causal {
        iid: sample.interaction_id,
        client_ns: sample.client_ns,
    }
}

fn causal_from_stamp(stamp: CausalStamp) -> Causal {
    Causal {
        iid: stamp.interaction_id,
        client_ns: stamp.client_ns,
    }
}

fn pointer_mask(button: PointerButton) -> i32 {
    match button {
        PointerButton::Primary => 1,
        PointerButton::Secondary => 2,
        PointerButton::Middle => 4,
        PointerButton::Extra1 => 8,
        PointerButton::Extra2 => 16,
    }
}

fn pointer_name(button: PointerButton) -> &'static str {
    match button {
        PointerButton::Primary => "left",
        PointerButton::Secondary => "right",
        PointerButton::Middle => "middle",
        PointerButton::Extra1 => "back",
        PointerButton::Extra2 => "forward",
    }
}

fn modifier_mask(modifiers: Modifiers) -> i32 {
    i32::from(modifiers.alt)
        | (i32::from(modifiers.ctrl) << 1)
        | (i32::from(modifiers.mac_cmd) << 2)
        | (i32::from(modifiers.shift) << 3)
}

fn is_printable(key: Key) -> bool {
    matches!(
        key,
        Key::Space
            | Key::Colon
            | Key::Comma
            | Key::Backslash
            | Key::Slash
            | Key::Pipe
            | Key::Questionmark
            | Key::Exclamationmark
            | Key::OpenBracket
            | Key::CloseBracket
            | Key::OpenCurlyBracket
            | Key::CloseCurlyBracket
            | Key::Backtick
            | Key::Minus
            | Key::Period
            | Key::Plus
            | Key::Equals
            | Key::Semicolon
            | Key::Quote
            | Key::Num0
            | Key::Num1
            | Key::Num2
            | Key::Num3
            | Key::Num4
            | Key::Num5
            | Key::Num6
            | Key::Num7
            | Key::Num8
            | Key::Num9
            | Key::A
            | Key::B
            | Key::C
            | Key::D
            | Key::E
            | Key::F
            | Key::G
            | Key::H
            | Key::I
            | Key::J
            | Key::K
            | Key::L
            | Key::M
            | Key::N
            | Key::O
            | Key::P
            | Key::Q
            | Key::R
            | Key::S
            | Key::T
            | Key::U
            | Key::V
            | Key::W
            | Key::X
            | Key::Y
            | Key::Z
    )
}

struct KeyMapping {
    key: String,
    code: String,
    key_code: i32,
}

fn key_mapping(key: Key, physical: Key, shifted: bool) -> KeyMapping {
    let logical_name = key.name();
    let (key_name, key_code) = match key {
        Key::ArrowDown => ("ArrowDown".to_owned(), 40),
        Key::ArrowLeft => ("ArrowLeft".to_owned(), 37),
        Key::ArrowRight => ("ArrowRight".to_owned(), 39),
        Key::ArrowUp => ("ArrowUp".to_owned(), 38),
        Key::Escape => ("Escape".to_owned(), 27),
        Key::Tab => ("Tab".to_owned(), 9),
        Key::Backspace => ("Backspace".to_owned(), 8),
        Key::Enter => ("Enter".to_owned(), 13),
        Key::Space => (" ".to_owned(), 32),
        Key::Insert => ("Insert".to_owned(), 45),
        Key::Delete => ("Delete".to_owned(), 46),
        Key::Home => ("Home".to_owned(), 36),
        Key::End => ("End".to_owned(), 35),
        Key::PageUp => ("PageUp".to_owned(), 33),
        Key::PageDown => ("PageDown".to_owned(), 34),
        _ if logical_name.len() == 1 => (
            if shifted {
                logical_name.to_owned()
            } else {
                logical_name.to_ascii_lowercase()
            },
            i32::from(logical_name.as_bytes()[0]),
        ),
        _ => (logical_name.to_owned(), function_key_code(key)),
    };
    let physical_name = physical.name();
    let code = if physical_name.len() == 1 {
        if physical_name.as_bytes()[0].is_ascii_digit() {
            format!("Digit{physical_name}")
        } else {
            format!("Key{physical_name}")
        }
    } else {
        match physical {
            Key::ArrowDown => "ArrowDown".to_owned(),
            Key::ArrowLeft => "ArrowLeft".to_owned(),
            Key::ArrowRight => "ArrowRight".to_owned(),
            Key::ArrowUp => "ArrowUp".to_owned(),
            Key::Space => "Space".to_owned(),
            _ => physical_name.to_owned(),
        }
    };
    KeyMapping {
        key: key_name,
        code,
        key_code,
    }
}

fn function_key_code(key: Key) -> i32 {
    match key {
        Key::F1 => 112,
        Key::F2 => 113,
        Key::F3 => 114,
        Key::F4 => 115,
        Key::F5 => 116,
        Key::F6 => 117,
        Key::F7 => 118,
        Key::F8 => 119,
        Key::F9 => 120,
        Key::F10 => 121,
        Key::F11 => 122,
        Key::F12 => 123,
        _ => 0,
    }
}

fn utf16_offset(text: &str, characters: usize) -> i32 {
    let units = text
        .chars()
        .take(characters)
        .map(char::len_utf16)
        .sum::<usize>();
    i32::try_from(units).unwrap_or(i32::MAX)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn surface() -> Rect {
        Rect::from_min_size(Pos2::new(10.0, 20.0), egui::vec2(200.0, 100.0))
    }

    #[test]
    fn native_pointer_preserves_buttons_and_normalized_coordinates() {
        let mut input = PageInput::new();
        input.set_native_pointer(true);
        let commands = input
            .translate(
                &[egui::Event::PointerButton {
                    pos: Pos2::new(110.0, 45.0),
                    button: PointerButton::Secondary,
                    pressed: true,
                    modifiers: Modifiers::SHIFT,
                }],
                surface(),
                7,
                true,
            )
            .unwrap();
        match &commands[0] {
            Command::Pointer {
                phase,
                x,
                y,
                button,
                buttons,
                mods,
                ..
            } => {
                assert_eq!(phase, "down");
                assert_eq!((*x, *y), (0.5, 0.25));
                assert_eq!(button, "right");
                assert_eq!((*buttons, *mods), (2, 8));
            }
            command => panic!("unexpected command {command:?}"),
        }
    }

    #[test]
    fn older_server_gets_primary_touch_fallback() {
        let mut input = PageInput::new();
        let commands = input
            .translate(
                &[egui::Event::PointerButton {
                    pos: Pos2::new(110.0, 45.0),
                    button: PointerButton::Primary,
                    pressed: true,
                    modifiers: Modifiers::NONE,
                }],
                surface(),
                3,
                true,
            )
            .unwrap();
        assert!(matches!(&commands[0], Command::Touch { phase, .. } if phase == "start"));
    }

    #[test]
    fn text_key_and_ime_are_distinct() {
        let mut input = PageInput::new();
        let commands = input
            .translate(
                &[
                    egui::Event::Text("a".to_owned()),
                    egui::Event::Key {
                        key: Key::Backspace,
                        physical_key: Some(Key::Backspace),
                        pressed: true,
                        repeat: false,
                        modifiers: Modifiers::NONE,
                    },
                    egui::Event::Ime(ImeEvent::Preedit {
                        text: "🙂a".to_owned(),
                        active_range_chars: Some(1..2),
                    }),
                ],
                surface(),
                1,
                true,
            )
            .unwrap();
        assert!(matches!(&commands[0], Command::Key { text, .. } if text == "a"));
        assert!(matches!(&commands[1], Command::Key { key, .. } if key == "Backspace"));
        assert!(matches!(
            &commands[2],
            Command::Compose {
                start: 2,
                end: 3,
                ..
            }
        ));
    }
}
