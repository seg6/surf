use surf_core::{CausalStamp, InputSample, InputState, monotonic_ns};
use surf_protocol::{Causal, Command, TouchPoint};

const DOUBLE_CLICK_NS: u64 = 500_000_000;
const DOUBLE_CLICK_DISTANCE_SQ: f64 = 36.0;

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct Modifiers {
    pub alt: bool,
    pub control: bool,
    pub command: bool,
    pub shift: bool,
}

#[derive(Clone, Copy, Debug, Default, PartialEq)]
pub struct PageRect {
    pub x: f64,
    pub y: f64,
    pub width: f64,
    pub height: f64,
}

impl PageRect {
    pub fn contains(self, x: f64, y: f64) -> bool {
        x >= self.x && y >= self.y && x < self.x + self.width && y < self.y + self.height
    }
}

pub struct PageInput {
    core: InputState,
    native_pointer: bool,
    buttons: i32,
    pointer: Option<(f64, f64)>,
    last_click: Option<(u64, u32, f64, f64, i32)>,
    composition_active: bool,
}

impl PageInput {
    pub fn new() -> Self {
        Self {
            core: InputState::new(),
            native_pointer: false,
            buttons: 0,
            pointer: None,
            last_click: None,
            composition_active: false,
        }
    }

    pub fn configure(&mut self, native_pointer: bool, generation: u32) {
        self.native_pointer = native_pointer;
        self.core.set_surface(generation);
    }

    pub fn reset(&mut self) {
        self.core.set_surface(0);
        self.buttons = 0;
        self.pointer = None;
        self.last_click = None;
        self.composition_active = false;
    }

    pub fn motion(
        &mut self,
        x: f64,
        y: f64,
        rect: PageRect,
        generation: u32,
        modifiers: Modifiers,
    ) -> Result<Option<Command>, String> {
        self.pointer = Some((x, y));
        if generation == 0 || (!rect.contains(x, y) && self.buttons == 0) {
            return Ok(None);
        }
        let sample = self.sample(x, y, rect, generation)?;
        if self.native_pointer {
            Ok(Some(pointer_command(
                "move",
                "none",
                0,
                self.buttons,
                modifiers,
                sample,
            )))
        } else if self.buttons & 1 != 0 {
            Ok(Some(touch_command("move", sample)))
        } else {
            Ok(None)
        }
    }

    #[allow(clippy::too_many_arguments)]
    pub fn button(
        &mut self,
        pressed: bool,
        button: u32,
        x: f64,
        y: f64,
        rect: PageRect,
        generation: u32,
        modifiers: Modifiers,
    ) -> Result<Option<Command>, String> {
        let (name, mask) = button_mapping(button);
        if generation == 0 || mask == 0 || (pressed && !rect.contains(x, y)) {
            return Ok(None);
        }
        self.pointer = Some((x, y));
        if pressed {
            self.buttons |= mask;
        } else if self.buttons & mask == 0 {
            return Ok(None);
        } else {
            self.buttons &= !mask;
        }
        let sample = self.sample(x, y, rect, generation)?;
        let clicks = self.click_count(button, x, y, pressed, sample.event_ns);
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

    pub fn wheel(
        &mut self,
        dx: f64,
        dy: f64,
        rect: PageRect,
        generation: u32,
        modifiers: Modifiers,
    ) -> Result<Vec<Command>, String> {
        let Some((x, y)) = self.pointer.filter(|(x, y)| rect.contains(*x, *y)) else {
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
                    (x - rect.x, y - rect.y),
                    (dx, dy),
                    (rect.width, rect.height),
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
            let start = self.sample(x, y, rect, generation)?;
            let moved = self.sample(x - dx, y - dy, rect, generation)?;
            let end = self.sample(x - dx, y - dy, rect, generation)?;
            Ok(vec![
                touch_command("start", start),
                touch_command("move", moved),
                touch_command("end", end),
            ])
        }
    }

    #[allow(clippy::too_many_arguments)]
    pub fn key(
        &mut self,
        down: bool,
        key: String,
        code: String,
        key_code: i32,
        text: String,
        modifiers: Modifiers,
    ) -> Result<Command, String> {
        Ok(Command::Key {
            down,
            key,
            code,
            key_code,
            text,
            mods: if self.native_pointer {
                modifier_mask(modifiers)
            } else {
                0
            },
            causal: self.causal()?,
        })
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
        self.composition_active = phase == "update";
        Ok(Command::Compose {
            phase: phase.to_owned(),
            text,
            start,
            end,
            causal: self.causal()?,
        })
    }

    pub fn composition_active(&self) -> bool {
        self.composition_active
    }

    fn sample(
        &mut self,
        x: f64,
        y: f64,
        rect: PageRect,
        generation: u32,
    ) -> Result<InputSample, String> {
        self.core.set_surface(generation);
        self.core
            .pointer(
                (x - rect.x, y - rect.y),
                (rect.width, rect.height),
                monotonic_ns(),
            )
            .map_err(|error| error.to_string())
    }

    fn causal(&mut self) -> Result<Causal, String> {
        self.core
            .causal(monotonic_ns())
            .map(causal_from_stamp)
            .map_err(|error| error.to_string())
    }

    fn click_count(&mut self, button: u32, x: f64, y: f64, pressed: bool, now: u64) -> i32 {
        if !pressed {
            return self
                .last_click
                .filter(|(_, previous, _, _, _)| *previous == button)
                .map_or(1, |(_, _, _, _, count)| count);
        }
        let count = self
            .last_click
            .map_or(1, |(when, previous, px, py, count)| {
                let distance = (px - x).powi(2) + (py - y).powi(2);
                if previous == button
                    && now.saturating_sub(when) <= DOUBLE_CLICK_NS
                    && distance <= DOUBLE_CLICK_DISTANCE_SQ
                {
                    (count + 1).min(3)
                } else {
                    1
                }
            });
        self.last_click = Some((now, button, x, y, count));
        count
    }
}

impl Default for PageInput {
    fn default() -> Self {
        Self::new()
    }
}

fn pointer_command(
    phase: &str,
    button: &str,
    clicks: i32,
    buttons: i32,
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

fn modifier_mask(modifiers: Modifiers) -> i32 {
    i32::from(modifiers.alt)
        | (i32::from(modifiers.control) << 1)
        | (i32::from(modifiers.command) << 2)
        | (i32::from(modifiers.shift) << 3)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn pointer_is_normalized_inside_the_page_not_the_window() {
        let mut input = PageInput::new();
        input.configure(true, 7);
        let command = input
            .button(
                true,
                3,
                110.0,
                85.0,
                PageRect {
                    x: 10.0,
                    y: 60.0,
                    width: 200.0,
                    height: 100.0,
                },
                7,
                Modifiers {
                    shift: true,
                    ..Modifiers::default()
                },
            )
            .unwrap()
            .unwrap();
        match command {
            Command::Pointer {
                x,
                y,
                button,
                buttons,
                mods,
                ..
            } => {
                assert_eq!((x, y), (0.5, 0.25));
                assert_eq!(button, "right");
                assert_eq!((buttons, mods), (2, 8));
            }
            command => panic!("unexpected command {command:?}"),
        }
    }

    #[test]
    fn old_servers_get_touch_instead_of_pointer_input() {
        let mut input = PageInput::new();
        input.configure(false, 3);
        let command = input
            .button(
                true,
                1,
                50.0,
                50.0,
                PageRect {
                    x: 0.0,
                    y: 0.0,
                    width: 100.0,
                    height: 100.0,
                },
                3,
                Modifiers::default(),
            )
            .unwrap()
            .unwrap();
        assert!(matches!(command, Command::Touch { phase, .. } if phase == "start"));
    }
}
