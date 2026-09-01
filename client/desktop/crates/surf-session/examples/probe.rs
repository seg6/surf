//! Developer smoke probe for the genuine desktop session path.
//!
//! Usage: `cargo run -p surf-session --example probe -- ENDPOINT [CODE --confirm] [--expect-reconnect]`

use std::env;
use std::thread;
use std::time::{Duration, Instant};

use surf_session::{SessionAction, SessionClient, SessionEvent, Storage};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = env::args().skip(1).collect();
    let endpoint = args
        .first()
        .ok_or("usage: probe ENDPOINT [CODE --confirm]")?;
    let code = args.get(1).cloned();
    let confirm = args.iter().any(|argument| argument == "--confirm");
    let expect_reconnect = args.iter().any(|argument| argument == "--expect-reconnect");
    let storage = env::var_os("SURF_CLIENT_HOME")
        .map(Storage::at)
        .unwrap_or(Storage::system()?);
    let client = SessionClient::spawn(storage)?;
    client.send(SessionAction::Inspect(endpoint.clone()))?;
    let deadline = Instant::now() + Duration::from_secs(40);
    let mut connections = 0_u8;
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
                SessionEvent::Connected { .. } => connections = connections.saturating_add(1),
                _ => {}
            }
        }
        if let Some(frame) = client.take_latest_frame() {
            println!("received validated binary frame ({} bytes)", frame.len());
            if !expect_reconnect || connections >= 2 {
                return Ok(());
            }
        }
        thread::sleep(Duration::from_millis(10));
    }
    Err("session probe timed out".into())
}
