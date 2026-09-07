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
        Some("click") => {
            let x = args.get(1).ok_or("missing x")?.parse()?;
            let y = args.get(2).ok_or("missing y")?.parse()?;
            connection
                .xtest_fake_input(xproto::MOTION_NOTIFY_EVENT, 0, 0, root, x, y, 0)?
                .check()?;
            connection
                .xtest_fake_input(xproto::BUTTON_PRESS_EVENT, 1, 0, root, 0, 0, 0)?
                .check()?;
            connection
                .xtest_fake_input(xproto::BUTTON_RELEASE_EVENT, 1, 0, root, 0, 0, 0)?
                .check()?;
        }
        _ => return Err("Use type TEXT, key ctrl+l/Return/Escape, or click X Y".into()),
    }
    connection.flush()?;
    thread::sleep(Duration::from_millis(100));
    Ok(())
}
