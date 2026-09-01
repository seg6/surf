//! End-to-end probe for secure transport, bounded media handoff, and FFmpeg.
//!
//! Usage: `cargo run -p surf-media --example probe_decode -- ENDPOINT [CODE --confirm]`

use std::env;
use std::thread;
use std::time::{Duration, Instant};

use surf_media::{MediaEvent, MediaPipeline};
use surf_protocol::{Causal, Command};
use surf_session::{SessionAction, SessionClient, SessionEvent, Storage};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = env::args().skip(1).collect();
    let endpoint = args
        .first()
        .ok_or("usage: probe_decode ENDPOINT [CODE --confirm]")?;
    let code = args.get(1).cloned();
    let confirm = args.iter().any(|argument| argument == "--confirm");
    let expect_audio = args.iter().any(|argument| argument == "--expect-audio");
    let storage = env::var_os("SURF_CLIENT_HOME")
        .map(Storage::at)
        .unwrap_or(Storage::system()?);
    let media = MediaPipeline::spawn()?;
    let client = SessionClient::spawn_with_frame_sink(storage, media.frame_sink())?;
    client.send(SessionAction::Inspect(endpoint.clone()))?;
    let deadline = Instant::now() + Duration::from_secs(45);
    let mut connected = false;
    let mut audio_ready = false;
    let mut audio_requested = false;
    let mut decoded_video = false;
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
                    None => return Err("server is not paired".into()),
                },
                SessionEvent::PairingPhrase(_) if confirm => {
                    client.send(SessionAction::ConfirmPairing)?;
                }
                SessionEvent::PairingPhrase(_) => {
                    return Err("pairing confirmation required".into());
                }
                SessionEvent::Connected { .. } => connected = true,
                SessionEvent::Failure(failure) => return Err(failure.message.into()),
                _ => {}
            }
        }
        while let Some(event) = media.try_recv_event() {
            println!("media: {event:?}");
            match event {
                MediaEvent::RequestKeyframe => {
                    client.send(SessionAction::Send(Command::RequestKeyframe {
                        causal: Causal::default(),
                    }))?
                }
                MediaEvent::DecoderError(message) => eprintln!("decoder: {message}"),
                MediaEvent::AudioReady { .. } => audio_ready = true,
                MediaEvent::AudioUnavailable(message) | MediaEvent::AudioError(message) => {
                    eprintln!("audio: {message}");
                }
            }
        }
        if connected && audio_ready && expect_audio && !audio_requested {
            client.send(SessionAction::Send(Command::Audio {
                on: true,
                causal: Causal::default(),
            }))?;
            audio_requested = true;
        }
        if let Some(frame) = media.take_latest_frame() {
            let diagnostics = media.diagnostics();
            println!(
                "decoded YUV420 frame {}x{} generation {} sequence {} in {:.3} ms; {diagnostics:?}",
                frame.width,
                frame.height,
                frame.generation,
                frame.sequence,
                frame.decode_time.as_secs_f64() * 1_000.0,
            );
            if frame.y().len() != usize::try_from(frame.width * frame.height)? {
                return Err("decoded luma plane has the wrong size".into());
            }
            decoded_video = true;
        }
        let diagnostics = media.diagnostics();
        if decoded_video && (!expect_audio || diagnostics.audio_packets >= 3) {
            if expect_audio {
                println!(
                    "received {} bounded PCM packets with {} underruns",
                    diagnostics.audio_packets, diagnostics.audio_underruns
                );
            }
            return Ok(());
        }
        thread::sleep(Duration::from_millis(5));
    }
    Err(format!(
        "media probe timed out; diagnostics: {:?}",
        media.diagnostics()
    )
    .into())
}
