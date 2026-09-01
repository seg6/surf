//! Developer smoke probe for the genuine desktop session path.
//!
//! Usage: `cargo run -p surf-session --example probe -- ENDPOINT [CODE --confirm] [--expect-reconnect]`

use std::env;
use std::thread;
use std::time::{Duration, Instant};

use base64::Engine as _;
use surf_core::{InputSample, InputState, monotonic_ns};
use surf_protocol::{Causal, Command, Event};
use surf_session::{SessionAction, SessionClient, SessionEvent, Storage};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = env::args().skip(1).collect();
    let endpoint = args
        .first()
        .ok_or("usage: probe ENDPOINT [CODE --confirm]")?;
    let code = args.get(1).cloned();
    let confirm = args.iter().any(|argument| argument == "--confirm");
    let expect_reconnect = args.iter().any(|argument| argument == "--expect-reconnect");
    let expect_input = args.iter().any(|argument| argument == "--expect-input");
    let storage = env::var_os("SURF_CLIENT_HOME")
        .map(Storage::at)
        .unwrap_or(Storage::system()?);
    let client = SessionClient::spawn(storage)?;
    client.send(SessionAction::Inspect(endpoint.clone()))?;
    let deadline = Instant::now() + Duration::from_secs(40);
    let mut connections = 0_u8;
    let mut input = InputState::new();
    let mut input_generation = 0_u32;
    let mut input_stage = 0_u8;
    let mut input_ready = false;
    let mut restore_title = false;
    let mut restore_page_frame = false;
    let mut input_size = (768.0, 934.0);
    while Instant::now() < deadline {
        while let Some(event) = client.try_recv() {
            println!("{event:?}");
            match event {
                SessionEvent::Inspected { paired: true, .. } => {
                    client.send(SessionAction::Connect)?;
                }
                SessionEvent::Inspected { paired: false, .. } => match &code {
                    Some(code) => client.send(SessionAction::Pair {
                        code: code.clone(),
                        device_name: "Surf integration probe".to_owned(),
                    })?,
                    None => return Ok(()),
                },
                SessionEvent::PairingPhrase(_) if confirm => {
                    client.send(SessionAction::ConfirmPairing)?;
                }
                SessionEvent::PairingPhrase(_) => return Ok(()),
                SessionEvent::Failure(failure) => return Err(failure.message.into()),
                SessionEvent::Connected { config, .. } => {
                    connections = connections.saturating_add(1);
                    if expect_input {
                        if !config
                            .caps
                            .iter()
                            .any(|capability| capability == "pointer-input")
                        {
                            return Err("server did not advertise pointer-input".into());
                        }
                        input_size = (f64::from(config.vw), f64::from(config.vh));
                        client.send(SessionAction::Send(Command::Navigate {
                            url: input_test_url(),
                            causal: Causal::default(),
                        }))?;
                    }
                }
                SessionEvent::Control(Event::VideoConfig {
                    state, generation, ..
                }) if expect_input && state == "ready" => {
                    input_generation = generation;
                    input.set_surface(generation);
                    if input_ready && input_stage == 0 {
                        send_pointer_setup(&client, &mut input, input_size)?;
                        input_stage = 1;
                    }
                }
                SessionEvent::Control(Event::Tabs { tabs }) if expect_input => {
                    let title = tabs
                        .iter()
                        .find(|tab| tab.active)
                        .map(|tab| tab.title.as_str())
                        .unwrap_or("");
                    if title == "INPUT_READY" {
                        input_ready = true;
                        if input_stage == 0 && input_generation != 0 {
                            send_pointer_setup(&client, &mut input, input_size)?;
                            input_stage = 1;
                        }
                    } else if title == "ANIMATION_READY" && input_stage == 3 {
                        restore_title = true;
                    }
                }
                SessionEvent::Control(Event::PageFrame { .. })
                    if expect_input && input_stage == 3 =>
                {
                    restore_page_frame = true;
                }
                SessionEvent::Control(Event::Dialog { text, .. })
                    if expect_input && input_stage == 2 =>
                {
                    if text == "P1W1K1C1F1" {
                        println!("verified pointer, wheel, keyboard, and IME input");
                        client.send(SessionAction::Send(Command::Navigate {
                            url: animated_test_url(),
                            causal: Causal::default(),
                        }))?;
                        input_stage = 3;
                        continue;
                    }
                    return Err(format!("browser input state was {text}, want P1W1K1C1F1").into());
                }
                SessionEvent::Control(Event::Editable { on: true, .. })
                    if expect_input && input_stage == 1 =>
                {
                    send_keyboard_input(&client, &mut input)?;
                    input_stage = 2;
                }
                _ => {}
            }
        }
        if let Some(frame) = client.take_latest_frame() {
            if !expect_input {
                println!("received validated binary frame ({} bytes)", frame.len());
            }
            if (!expect_reconnect || connections >= 2) && !expect_input {
                return Ok(());
            }
            if expect_input && input_stage == 3 && restore_title && restore_page_frame {
                println!("restored animated page for the next integration phase");
                return Ok(());
            }
        }
        thread::sleep(Duration::from_millis(10));
    }
    Err("session probe timed out".into())
}

fn input_test_url() -> String {
    let page = r#"<!doctype html><meta charset=utf-8><style>
html,body{margin:0;min-height:3000px;background:#17191d;color:white;font:24px sans-serif}
button,input{position:fixed;left:80px;width:220px;height:54px;font:22px sans-serif}
button{top:75px}input{top:175px}
</style><button id=b>Pointer target</button><input id=i value="">
<script>
document.title='INPUT_READY';
const state={pointer:false,wheel:false,key:false,compose:false,focus:false,reported:false};
const flags=()=>'P'+(+state.pointer)+'W'+(+state.wheel)+'K'+(+state.key)+'C'+(+state.compose)+'F'+(+state.focus);
const check=()=>{};
b.addEventListener('pointerdown',()=>{state.pointer=true;check()});
addEventListener('wheel',()=>{state.wheel=true;check()},{passive:true});
i.addEventListener('focus',()=>{state.focus=true;setTimeout(()=>{if(!state.reported){state.reported=true;alert(flags())}},500)});
i.addEventListener('input',()=>{state.key=i.value.includes('x');state.compose=i.value.includes('é');check()});
</script>"#;
    format!(
        "data:text/html;base64,{}",
        base64::engine::general_purpose::STANDARD.encode(page)
    )
}

fn animated_test_url() -> String {
    let page = r#"<!doctype html><title>ANIMATION_READY</title><style>
html,body{margin:0;height:100%;overflow:hidden;background:#111}.box{width:35%;height:35%;
background:linear-gradient(135deg,#39d,#f45);animation:surf 1s linear infinite}
@keyframes surf{0%{transform:translate(0,0);filter:hue-rotate(0deg)}50%{transform:translate(180%,180%);
filter:hue-rotate(180deg)}100%{transform:translate(0,0);filter:hue-rotate(360deg)}}
</style><div class=box></div>"#;
    format!(
        "data:text/html;base64,{}",
        base64::engine::general_purpose::STANDARD.encode(page)
    )
}

fn causal(sample: InputSample) -> Causal {
    Causal {
        iid: sample.interaction_id,
        client_ns: sample.client_ns,
    }
}

fn pointer(
    input: &mut InputState,
    position: (f64, f64),
    size: (f64, f64),
) -> Result<InputSample, Box<dyn std::error::Error>> {
    Ok(input.pointer(position, size, monotonic_ns())?)
}

fn send_pointer_setup(
    client: &SessionClient,
    input: &mut InputState,
    size: (f64, f64),
) -> Result<(), Box<dyn std::error::Error>> {
    let button = (150.0, 100.0);
    let down = pointer(input, button, size)?;
    client.send(SessionAction::Send(Command::Pointer {
        phase: "down".to_owned(),
        seq: down.sequence,
        surface: down.surface_generation,
        ts: down.event_ns,
        x: down.x,
        y: down.y,
        button: "left".to_owned(),
        buttons: 1,
        mods: 0,
        clicks: 1,
        causal: causal(down),
    }))?;
    let up = pointer(input, button, size)?;
    client.send(SessionAction::Send(Command::Pointer {
        phase: "up".to_owned(),
        seq: up.sequence,
        surface: up.surface_generation,
        ts: up.event_ns,
        x: up.x,
        y: up.y,
        button: "left".to_owned(),
        buttons: 0,
        mods: 0,
        clicks: 1,
        causal: causal(up),
    }))?;
    let wheel = input.wheel(button, (0.0, 80.0), size, monotonic_ns())?;
    client.send(SessionAction::Send(Command::Wheel {
        seq: wheel.sequence,
        surface: wheel.surface_generation,
        ts: wheel.event_ns,
        x: wheel.x,
        y: wheel.y,
        dx: wheel.delta_x,
        dy: wheel.delta_y,
        buttons: 0,
        mods: 0,
        causal: causal(wheel),
    }))?;
    let field = (150.0, 200.0);
    let down = pointer(input, field, size)?;
    client.send(SessionAction::Send(Command::Pointer {
        phase: "down".to_owned(),
        seq: down.sequence,
        surface: down.surface_generation,
        ts: down.event_ns,
        x: down.x,
        y: down.y,
        button: "left".to_owned(),
        buttons: 1,
        mods: 0,
        clicks: 1,
        causal: causal(down),
    }))?;
    let up = pointer(input, field, size)?;
    client.send(SessionAction::Send(Command::Pointer {
        phase: "up".to_owned(),
        seq: up.sequence,
        surface: up.surface_generation,
        ts: up.event_ns,
        x: up.x,
        y: up.y,
        button: "left".to_owned(),
        buttons: 0,
        mods: 0,
        clicks: 1,
        causal: causal(up),
    }))?;
    Ok(())
}

fn send_keyboard_input(
    client: &SessionClient,
    input: &mut InputState,
) -> Result<(), Box<dyn std::error::Error>> {
    let stamp = input.causal(monotonic_ns())?;
    client.send(SessionAction::Send(Command::Key {
        down: true,
        key: String::new(),
        code: String::new(),
        key_code: 0,
        text: "x".to_owned(),
        mods: 0,
        causal: Causal {
            iid: stamp.interaction_id,
            client_ns: stamp.client_ns,
        },
    }))?;
    let stamp = input.causal(monotonic_ns())?;
    client.send(SessionAction::Send(Command::Compose {
        phase: "commit".to_owned(),
        text: "é".to_owned(),
        start: 0,
        end: 0,
        causal: Causal {
            iid: stamp.interaction_id,
            client_ns: stamp.client_ns,
        },
    }))?;
    Ok(())
}
