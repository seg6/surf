//! X11/XTest driver: exercise the real Winit -> ImGui input path in an isolated Xvfb.
use std::{error::Error, thread, time::Duration};
use x11rb::{
    connection::Connection,
    protocol::{
        xproto::{self, AtomEnum, ConnectionExt as _, InputFocus},
        xtest::ConnectionExt as _,
    },
};

fn main() -> Result<(), Box<dyn Error>> {
    let (connection, screen) = x11rb::connect(None)?;
    let root = connection.setup().roots[screen].root;
    let windows = connection.query_tree(root)?.reply()?.children;
    let window = windows
        .into_iter()
        .find(|id| {
            connection
                .get_property(false, *id, AtomEnum::WM_NAME, AtomEnum::STRING, 0, 1024)
                .ok()
                .and_then(|cookie| cookie.reply().ok())
                .is_some_and(|p| String::from_utf8_lossy(&p.value).contains("Surf"))
        })
        .ok_or("Surf test window not found")?;
    connection.set_input_focus(InputFocus::PARENT, window, x11rb::CURRENT_TIME)?;
    let setup = connection.setup();
    let map = connection
        .get_keyboard_mapping(setup.min_keycode, setup.max_keycode - setup.min_keycode + 1)?
        .reply()?;
    let code = |symbol: u32| -> Result<u8, Box<dyn Error>> {
        map.keysyms
            .chunks(map.keysyms_per_keycode as usize)
            .position(|row| row.contains(&symbol))
            .map(|index| setup.min_keycode + index as u8)
            .ok_or_else(|| format!("No keycode for {symbol}").into())
    };
    let press = |symbol: u32, down: bool| -> Result<(), Box<dyn Error>> {
        connection
            .xtest_fake_input(
                if down {
                    xproto::KEY_PRESS_EVENT
                } else {
                    xproto::KEY_RELEASE_EVENT
                },
                code(symbol)?,
                x11rb::CURRENT_TIME,
                root,
                0,
                0,
                0,
            )?
            .check()?;
        Ok(())
    };
    let args: Vec<_> = std::env::args().skip(1).collect();
    match args.first().map(String::as_str) {
        Some("type" | "focus-type") => {
            if args[0] == "focus-type" {
                press(0xffe3, true)?;
                press('l' as u32, true)?;
                press('l' as u32, false)?;
                press(0xffe3, false)?;
            }
            for c in args.get(1).ok_or("missing text")?.chars() {
                press(c as u32, true)?;
                press(c as u32, false)?;
                connection.flush()?;
                thread::sleep(Duration::from_millis(8));
            }
        }
        Some("key") => {
            let key = args.get(1).ok_or("missing key")?;
            let (control, key) = key
                .strip_prefix("ctrl+")
                .map_or((false, key.as_str()), |s| (true, s));
            let symbol = match key {
                "Escape" => 0xff1b,
                "Return" => 0xff0d,
                "Down" => 0xff54,
                "Up" => 0xff52,
                "Tab" => 0xff09,
                "F11" => 0xffc8,
                _ => key.chars().next().ok_or("empty key")? as u32,
            };
            if control {
                press(0xffe3, true)?;
            }
            press(symbol, true)?;
            press(symbol, false)?;
            if control {
                press(0xffe3, false)?;
            }
        }
        Some("paste") => {
            let mut clipboard = arboard::Clipboard::new()?;
            clipboard.set_text(args.get(1).ok_or("missing clipboard text")?.clone())?;
            press(0xffe3, true)?;
            press('v' as u32, true)?;
            press('v' as u32, false)?;
            press(0xffe3, false)?;
            connection.flush()?;
            thread::sleep(Duration::from_millis(500));
        }
        Some("resize") => {
            connection
                .configure_window(
                    window,
                    &xproto::ConfigureWindowAux::new()
                        .width(args.get(1).ok_or("missing width")?.parse::<u32>()?)
                        .height(args.get(2).ok_or("missing height")?.parse::<u32>()?),
                )?
                .check()?;
        }
        Some("move") => {
            let x: i16 = args.get(1).ok_or("missing x")?.parse()?;
            let y: i16 = args.get(2).ok_or("missing y")?.parse()?;
            let duration: u64 = args.get(3).map_or(Ok(400), |s| s.parse())?;
            let duration = duration.min(10_000);
            let pointer = connection.query_pointer(root)?.reply()?;
            let steps = (duration / 16).max(1);
            for step in 1..=steps {
                let t = step as f64 / steps as f64;
                let eased = t * t * (3.0 - 2.0 * t);
                let interpolate = |a: i16, b: i16| {
                    (f64::from(a) + (f64::from(b) - f64::from(a)) * eased).round() as i16
                };
                connection.xtest_fake_input(
                    xproto::MOTION_NOTIFY_EVENT, 0, 0, root,
                    interpolate(pointer.root_x, x), interpolate(pointer.root_y, y), 0,
                )?.check()?;
                connection.flush()?;
                thread::sleep(Duration::from_millis(duration / steps));
            }
        }
        Some("swipe") => {
            let x1: i16 = args.get(1).ok_or("missing start x")?.parse()?;
            let y1: i16 = args.get(2).ok_or("missing start y")?.parse()?;
            let x2: i16 = args.get(3).ok_or("missing end x")?.parse()?;
            let y2: i16 = args.get(4).ok_or("missing end y")?.parse()?;
            let duration: u64 = args.get(5).map_or(Ok(400), |s| s.parse())?;
            if !(32..=5000).contains(&duration) {
                return Err("swipe duration must be 32–5000 ms".into());
            }
            connection.xtest_fake_input(xproto::MOTION_NOTIFY_EVENT, 0, 0, root, x1, y1, 0)?.check()?;
            connection.xtest_fake_input(xproto::BUTTON_PRESS_EVENT, 1, 0, root, 0, 0, 0)?.check()?;
            connection.flush()?;
            // Constant velocity through release lets Chromium recognize a fling.
            // Unlike the presentation cursor's move command, do not ease to zero.
            let started = std::time::Instant::now();
            let steps = (duration / 16).max(2);
            let gesture = (|| -> Result<(), Box<dyn Error>> {
                for step in 1..=steps {
                    let deadline = Duration::from_millis(duration * step / steps);
                    thread::sleep(deadline.saturating_sub(started.elapsed()));
                    let t = step as f64 / steps as f64;
                    let interpolate = |a: i16, b: i16| {
                        (f64::from(a) + (f64::from(b) - f64::from(a)) * t).round() as i16
                    };
                    connection.xtest_fake_input(xproto::MOTION_NOTIFY_EVENT, 0, 0, root,
                        interpolate(x1, x2), interpolate(y1, y2), 0)?.check()?;
                    connection.flush()?;
                }
                thread::sleep(Duration::from_millis(16));
                Ok(())
            })();
            // Always attempt release, including when an intermediate move fails.
            let release = (|| -> Result<(), Box<dyn Error>> {
                connection.xtest_fake_input(xproto::BUTTON_RELEASE_EVENT, 1, 0, root, 0, 0, 0)?.check()?;
                connection.flush()?;
                Ok(())
            })();
            gesture?;
            release?;
        }
        Some("scroll") => {
            let steps: i32 = args.get(1).ok_or("missing scroll steps")?.parse()?;
            let interval: u64 = args.get(2).map_or(Ok(100), |s| s.parse())?;
            if steps.unsigned_abs() > 100 || interval > 1000 {
                return Err("scroll accepts at most 100 steps and 1000 ms between steps".into());
            }
            let button = if steps < 0 { 4 } else { 5 };
            for _ in 0..steps.unsigned_abs() {
                connection.xtest_fake_input(xproto::BUTTON_PRESS_EVENT, button, 0, root, 0, 0, 0)?.check()?;
                connection.xtest_fake_input(xproto::BUTTON_RELEASE_EVENT, button, 0, root, 0, 0, 0)?.check()?;
                connection.flush()?;
                thread::sleep(Duration::from_millis(interval));
            }
        }
        Some("click") => {
            let x = args.get(1).ok_or("missing x")?.parse()?;
            let y = args.get(2).ok_or("missing y")?.parse()?;
            connection
                .xtest_fake_input(xproto::MOTION_NOTIFY_EVENT, 0, 0, root, x, y, 0)?
                .check()?;
            connection
                .xtest_fake_input(xproto::BUTTON_PRESS_EVENT, 1, 0, root, 0, 0, 0)?
                .check()?;
            connection.flush()?;
            thread::sleep(Duration::from_millis(80));
            connection
                .xtest_fake_input(xproto::BUTTON_RELEASE_EVENT, 1, 0, root, 0, 0, 0)?
                .check()?;
        }
        _ => return Err("Use type TEXT, paste TEXT, key KEY, click X Y, move X Y [MS], swipe X1 Y1 X2 Y2 [MS], scroll STEPS [MS], or resize W H".into()),
    }
    connection.flush()?;
    thread::sleep(Duration::from_millis(100));
    Ok(())
}
