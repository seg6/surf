use gtk::gdk;
use surf_core::{CausalStamp, InputSample, InputState, monotonic_ns};
use surf_protocol::{Causal, Command, TouchPoint};

pub struct InputBridge {
    core: InputState,
    native_pointer: bool,
    buttons: i32,
    last_position: Option<(f64, f64)>,
}

impl InputBridge {
    pub fn new() -> Self {
        Self {
            core: InputState::new(),
            native_pointer: false,
            buttons: 0,
            last_position: None,
        }
    }

    pub fn configure(&mut self, native_pointer: bool, generation: u32) {
        self.native_pointer = native_pointer;
        self.core.set_surface(generation);
    }

    pub fn reset(&mut self) {
        self.core.set_surface(0);
        self.buttons = 0;
        self.last_position = None;
    }

    pub fn motion(
        &mut self,
        x: f64,
        y: f64,
        width: f64,
        height: f64,
        generation: u32,
        modifiers: gdk::ModifierType,
    ) -> Result<Option<Command>, String> {
        self.last_position = Some((x, y));
        if generation == 0 || !self.native_pointer {
            return Ok(None);
        }
        self.core.set_surface(generation);
        let sample = self
            .core
            .pointer((x, y), (width, height), monotonic_ns())
            .map_err(|error| error.to_string())?;
        Ok(Some(pointer_command(
            "move",
            "none",
            0,
            self.buttons,
            modifiers,
            sample,
        )))
    }

    #[allow(clippy::too_many_arguments)]
    pub fn button(
        &mut self,
        pressed: bool,
        button: u32,
        clicks: i32,
        x: f64,
        y: f64,
        width: f64,
        height: f64,
        generation: u32,
        modifiers: gdk::ModifierType,
    ) -> Result<Option<Command>, String> {
        if generation == 0 {
            return Ok(None);
        }
        let (name, mask) = button_mapping(button);
        if mask == 0 {
            return Ok(None);
        }
        self.last_position = Some((x, y));
        if pressed {
            self.buttons |= mask;
        } else {
            self.buttons &= !mask;
        }
        self.core.set_surface(generation);
        let sample = self
            .core
            .pointer((x, y), (width, height), monotonic_ns())
            .map_err(|error| error.to_string())?;
        if self.native_pointer {
            Ok(Some(pointer_command(
                if pressed { "down" } else { "up" },
                name,
                clicks,
                self.buttons,
                modifiers,
                sample,
            )))
        } else if button == 1 {
            Ok(Some(touch_command(
                if pressed { "start" } else { "end" },
                sample,
            )))
        } else {
            Ok(None)
        }
    }

    pub fn scroll(
        &mut self,
        dx: f64,
        dy: f64,
        width: f64,
        height: f64,
        generation: u32,
        modifiers: gdk::ModifierType,
    ) -> Result<Vec<Command>, String> {
        let Some((x, y)) = self.last_position else {
            return Ok(Vec::new());
        };
        if generation == 0 || (dx == 0.0 && dy == 0.0) {
            return Ok(Vec::new());
        }
        self.core.set_surface(generation);
        if self.native_pointer {
            let sample = self
                .core
                .wheel(
                    (x, y),
                    (dx * 50.0, dy * 50.0),
                    (width, height),
                    monotonic_ns(),
                )
                .map_err(|error| error.to_string())?;
            Ok(vec![Command::Wheel {
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
            }])
        } else {
            let start = self
                .core
                .pointer((x, y), (width, height), monotonic_ns())
                .map_err(|error| error.to_string())?;
            let moved = self
                .core
                .pointer(
                    (x - dx * 50.0, y - dy * 50.0),
                    (width, height),
                    monotonic_ns(),
                )
                .map_err(|error| error.to_string())?;
            let end = self
                .core
                .pointer(
                    (x - dx * 50.0, y - dy * 50.0),
                    (width, height),
                    monotonic_ns(),
                )
                .map_err(|error| error.to_string())?;
            Ok(vec![
                touch_command("start", start),
                touch_command("move", moved),
                touch_command("end", end),
            ])
        }
    }

    pub fn key(
        &mut self,
        keyval: gdk::Key,
        keycode: u32,
        modifiers: gdk::ModifierType,
        pressed: bool,
    ) -> Result<Option<Command>, String> {
        let name = keyval
            .name()
            .map_or_else(String::new, |name| name.to_string());
        if pressed
            && !modifiers.intersects(
                gdk::ModifierType::CONTROL_MASK
                    | gdk::ModifierType::ALT_MASK
                    | gdk::ModifierType::META_MASK,
            )
            && let Some(character) = keyval.to_unicode()
            && !character.is_control()
        {
            return Ok(Some(Command::Key {
                down: true,
                key: String::new(),
                code: String::new(),
                key_code: 0,
                text: character.to_string(),
                mods: 0,
                causal: self.causal()?,
            }));
        }
        let (key, dom_code, legacy) = key_mapping(&name, keycode);
        Ok(Some(Command::Key {
            down: pressed,
            key,
            code: dom_code,
            key_code: legacy,
            text: String::new(),
            mods: if self.native_pointer {
                modifier_mask(modifiers)
            } else {
                0
            },
            causal: self.causal()?,
        }))
    }

    pub fn paste(&mut self, text: String) -> Result<Command, String> {
        Ok(Command::Paste {
            text,
            causal: self.causal()?,
        })
    }

    pub fn compose(
        &mut self,
        phase: &str,
        text: String,
        start: i32,
        end: i32,
    ) -> Result<Command, String> {
        Ok(Command::Compose {
            phase: phase.to_owned(),
            text,
            start,
            end,
            causal: self.causal()?,
        })
    }

    fn causal(&mut self) -> Result<Causal, String> {
        self.core
            .causal(monotonic_ns())
            .map(causal_from_stamp)
            .map_err(|error| error.to_string())
    }
}

fn pointer_command(
    phase: &str,
    button: &str,
    clicks: i32,
    buttons: i32,
    modifiers: gdk::ModifierType,
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
        buttons,
        mods: modifier_mask(modifiers),
        clicks,
        causal: causal_from_sample(sample),
    }
}

fn touch_command(phase: &str, sample: InputSample) -> Command {
    Command::Touch {
        phase: phase.to_owned(),
        seq: sample.sequence,
        surface: sample.surface_generation,
        ts: sample.event_ns,
        points: vec![TouchPoint {
            id: 1,
            x: sample.x,
            y: sample.y,
            rx: 0.0,
            ry: 0.0,
            force: 0.5,
        }],
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

fn button_mapping(button: u32) -> (&'static str, i32) {
    match button {
        1 => ("left", 1),
        2 => ("middle", 4),
        3 => ("right", 2),
        8 => ("back", 8),
        9 => ("forward", 16),
        _ => ("none", 0),
    }
}

fn modifier_mask(modifiers: gdk::ModifierType) -> i32 {
    i32::from(modifiers.contains(gdk::ModifierType::ALT_MASK))
        | (i32::from(modifiers.contains(gdk::ModifierType::CONTROL_MASK)) << 1)
        | (i32::from(modifiers.contains(gdk::ModifierType::META_MASK)) << 2)
        | (i32::from(modifiers.contains(gdk::ModifierType::SHIFT_MASK)) << 3)
}

fn key_mapping(name: &str, keycode: u32) -> (String, String, i32) {
    let (key, legacy) = match name {
        "Down" => ("ArrowDown", 40),
        "Left" => ("ArrowLeft", 37),
        "Right" => ("ArrowRight", 39),
        "Up" => ("ArrowUp", 38),
        "Escape" => ("Escape", 27),
        "Tab" => ("Tab", 9),
        "BackSpace" => ("Backspace", 8),
        "Return" | "KP_Enter" => ("Enter", 13),
        "space" => (" ", 32),
        "Insert" => ("Insert", 45),
        "Delete" => ("Delete", 46),
        "Home" => ("Home", 36),
        "End" => ("End", 35),
        "Page_Up" => ("PageUp", 33),
        "Page_Down" => ("PageDown", 34),
        _ => (name, 0),
    };
    let code = if name.len() == 1 {
        let character = name.as_bytes()[0];
        if character.is_ascii_digit() {
            format!("Digit{name}")
        } else {
            format!("Key{}", name.to_ascii_uppercase())
        }
    } else if keycode != 0 {
        format!("Unidentified{keycode}")
    } else {
        key.to_owned()
    };
    (key.to_owned(), code, legacy)
}
