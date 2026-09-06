mod api;
mod discovery;
mod identity;
mod storage;
mod tls;

use std::fs::{self, OpenOptions};
use std::io::Write as _;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::sync::Mutex;
use std::sync::mpsc::{self, Receiver, SyncSender, TrySendError};
use std::thread;
use std::time::{Duration, Instant};

use bytes::Bytes;
use futures_util::{SinkExt as _, StreamExt as _};
use surf_core::{Frame, ReconnectPolicy, RetryDecision};
use surf_protocol::{Causal, Command, Event};
use thiserror::Error;
use tokio::sync::mpsc::{Receiver as ActionReceiver, Sender as ActionSender, channel};
use tokio_tungstenite::Connector;
use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::tungstenite::client::IntoClientRequest as _;
use tokio_tungstenite::tungstenite::http::HeaderValue;

pub use api::{
    NativeConfig, PairingStatus, ServerInfo, VerifiedEndpoint, app_version, compatibility_version,
    inspect_endpoint, normalize_endpoint, wire_compatibility_version,
};
pub use discovery::DiscoveredServer;
pub use storage::{SavedServer, Storage};

pub type Result<T> = std::result::Result<T, SessionError>;

#[derive(Debug, Error)]
pub enum SessionError {
    #[error("invalid Surf endpoint: {0}")]
    Endpoint(String),
    #[error("Surf TLS verification failed: {0}")]
    Tls(String),
    #[error("Surf device identity failed: {0}")]
    Identity(String),
    #[error("Surf pairing failed: {0}")]
    Pairing(String),
    #[error("Surf protocol failed: {0}")]
    Protocol(String),
    #[error("Surf storage failed: {0}")]
    Storage(String),
    #[error("Surf server returned HTTP {status}: {message}")]
    HttpStatus { status: u16, message: String },
    #[error("Surf compatibility decision {decision}; server version {server_version}")]
    Compatibility {
        decision: String,
        server_version: String,
    },
    #[error("Surf network request failed: {0}")]
    Http(#[from] reqwest::Error),
    #[error("Surf WebSocket failed: {0}")]
    WebSocket(#[from] tokio_tungstenite::tungstenite::Error),
    #[error("Surf transport failed: {0}")]
    Transport(String),
    #[error("Surf file operation failed: {0}")]
    Io(#[from] std::io::Error),
}

impl SessionError {
    fn retryable(&self) -> bool {
        match self {
            Self::Http(error) => error.is_connect() || error.is_timeout() || error.is_request(),
            Self::HttpStatus { status, .. } => matches!(*status, 408 | 425 | 429) || *status >= 500,
            Self::WebSocket(_) | Self::Transport(_) => true,
            Self::Endpoint(_)
            | Self::Tls(_)
            | Self::Identity(_)
            | Self::Pairing(_)
            | Self::Protocol(_)
            | Self::Storage(_)
            | Self::Compatibility { .. }
            | Self::Io(_) => false,
        }
    }

    fn kind(&self) -> FailureKind {
        match self {
            Self::Endpoint(_) => FailureKind::Endpoint,
            Self::Tls(_) => FailureKind::Trust,
            Self::Identity(_) => FailureKind::Identity,
            Self::Pairing(_) => FailureKind::Pairing,
            Self::Compatibility { .. } => FailureKind::Compatibility,
            Self::Storage(_) | Self::Io(_) => FailureKind::Storage,
            Self::Protocol(_) => FailureKind::Protocol,
            Self::HttpStatus {
                status: 401 | 403, ..
            } => FailureKind::Authentication,
            Self::HttpStatus { .. } | Self::Http(_) | Self::WebSocket(_) | Self::Transport(_) => {
                FailureKind::Transport
            }
        }
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum FailureKind {
    Endpoint,
    Trust,
    Identity,
    Pairing,
    Authentication,
    Compatibility,
    Storage,
    Protocol,
    Transport,
    Internal,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct SessionFailure {
    pub kind: FailureKind,
    pub message: String,
    pub retryable: bool,
}

impl From<SessionError> for SessionFailure {
    fn from(error: SessionError) -> Self {
        Self {
            kind: error.kind(),
            retryable: error.retryable(),
            message: error.to_string(),
        }
    }
}

#[derive(Clone, Debug)]
pub enum SessionAction {
    Inspect(String),
    Pair { code: String, device_name: String },
    ConfirmPairing,
    Connect,
    Send(Command),
    Upload(Vec<PathBuf>),
    CancelUpload,
    Download { name: String, destination: PathBuf },
    Disconnect,
    Shutdown,
}

#[derive(Clone, Debug)]
pub enum SessionEvent {
    Status {
        phase: &'static str,
        message: String,
    },
    Discovered(DiscoveredServer),
    DiscoveryUnavailable(String),
    Inspected {
        info: ServerInfo,
        endpoint: String,
        saved_pairing: bool,
    },
    PairingPhrase(PairingStatus),
    Paired(SavedServer),
    Connected {
        info: ServerInfo,
        config: NativeConfig,
    },
    Reconnecting {
        attempt: u8,
        maximum: u8,
        delay: Duration,
        reason: String,
    },
    Control(Event),
    Transfer {
        kind: TransferKind,
        name: String,
        ok: bool,
        message: String,
    },
    Disconnected(String),
    Failure(SessionFailure),
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum TransferKind {
    Upload,
    Download,
}

/// Receives validated wire frames without making the network task wait for
/// decoding or presentation. Implementations must keep `submit` and `clear`
/// bounded and non-blocking.
pub trait FrameSink: Send + Sync + 'static {
    fn submit(&self, frame: Bytes);
    fn clear(&self);
}

#[derive(Default)]
struct LatestFrameSink {
    slot: Mutex<Option<Bytes>>,
}

impl FrameSink for LatestFrameSink {
    fn submit(&self, frame: Bytes) {
        if let Ok(mut slot) = self.slot.try_lock() {
            *slot = Some(frame);
        }
    }

    fn clear(&self) {
        if let Ok(mut slot) = self.slot.try_lock() {
            *slot = None;
        }
    }
}

pub struct SessionClient {
    cancel: Arc<tokio::sync::Notify>,
    actions: ActionSender<SessionAction>,
    events: Receiver<SessionEvent>,
    latest_frame: Arc<LatestFrameSink>,
    discovery: Option<discovery::DiscoveryWorker>,
    thread: Option<thread::JoinHandle<()>>,
}

impl SessionClient {
    pub fn spawn(storage: Storage) -> Result<Self> {
        let latest_frame = Arc::new(LatestFrameSink::default());
        let frame_sink: Arc<dyn FrameSink> = latest_frame.clone();
        Self::spawn_inner(storage, frame_sink, latest_frame)
    }

    pub fn spawn_with_frame_sink(storage: Storage, frame_sink: Arc<dyn FrameSink>) -> Result<Self> {
        Self::spawn_inner(storage, frame_sink, Arc::new(LatestFrameSink::default()))
    }

    fn spawn_inner(
        storage: Storage,
        frame_sink: Arc<dyn FrameSink>,
        latest_frame: Arc<LatestFrameSink>,
    ) -> Result<Self> {
        let (actions, action_rx) = channel(256);
        let (event_tx, events) = mpsc::sync_channel(256);
        let discovery_events = event_tx.clone();
        let cancel = Arc::new(tokio::sync::Notify::new());
        let worker_cancel = cancel.clone();
        let thread = thread::Builder::new()
            .name("surf-session".to_owned())
            .spawn(move || {
                let runtime = tokio::runtime::Builder::new_multi_thread()
                    .enable_all()
                    .worker_threads(2)
                    .thread_name("surf-net")
                    .build();
                match runtime {
                    Ok(runtime) => runtime.block_on(async {
                        tokio::select! {
                            _=worker_cancel.notified()=>{},
                            _=driver(storage,action_rx,event_tx.clone(),frame_sink.clone())=>{},
                        }
                        frame_sink.clear();
                    }),
                    Err(error) => {
                        let _ = event_tx.try_send(SessionEvent::Failure(SessionFailure {
                            kind: FailureKind::Internal,
                            message: format!("could not start networking: {error}"),
                            retryable: false,
                        }));
                    }
                }
            })?;
        let discovery = Some(discovery::DiscoveryWorker::spawn(discovery_events));
        Ok(Self {
            cancel,
            actions,
            events,
            latest_frame,
            discovery,
            thread: Some(thread),
        })
    }

    pub fn send(&self, action: SessionAction) -> Result<()> {
        self.actions.try_send(action).map_err(|error| match error {
            tokio::sync::mpsc::error::TrySendError::Full(_) => {
                SessionError::Protocol("session command buffer is full".to_owned())
            }
            tokio::sync::mpsc::error::TrySendError::Closed(_) => {
                SessionError::Protocol("session worker stopped".to_owned())
            }
        })
    }

    pub fn try_recv(&self) -> Option<SessionEvent> {
        self.events.try_recv().ok()
    }

    pub fn take_latest_frame(&self) -> Option<Bytes> {
        self.latest_frame.slot.lock().ok()?.take()
    }
}

impl Drop for SessionClient {
    fn drop(&mut self) {
        self.cancel.notify_one();
        self.discovery.take();
        let _ = self.actions.try_send(SessionAction::Shutdown);
        if let Some(thread) = self.thread.take()
            && thread.is_finished()
        {
            let _ = thread.join();
        }
    }
}

async fn driver(
    storage: Storage,
    mut actions: ActionReceiver<SessionAction>,
    events: SyncSender<SessionEvent>,
    frame_sink: Arc<dyn FrameSink>,
) {
    let mut verified: Option<VerifiedEndpoint> = None;
    let mut pairing: Option<PairingStatus> = None;
    let mut deferred = None;

    loop {
        let action = match deferred.take() {
            Some(action) => action,
            None => match actions.recv().await {
                Some(action) => action,
                None => return,
            },
        };
        let outcome: Result<Option<SessionAction>> = async {
            match action {
                SessionAction::Inspect(endpoint) => {
                    emit_status(&events, "verification", "Verifying server identity");
                    match api::inspect_endpoint(&endpoint).await {
                        Ok(result) => {
                            let saved_pairing = storage
                                .server(&result.info.server_id)
                                .ok()
                                .flatten()
                                .is_some();
                            emit(
                                &events,
                                SessionEvent::Inspected {
                                    info: result.info.clone(),
                                    endpoint: result
                                        .endpoint
                                        .as_str()
                                        .trim_end_matches('/')
                                        .to_owned(),
                                    saved_pairing,
                                },
                            )?;
                            verified = Some(result);
                            pairing = None;
                            Ok(None)
                        }
                        Err(error) => Err(error),
                    }
                }
                SessionAction::Pair { code, device_name } => match verified.as_ref() {
                    Some(server) => {
                        emit_status(&events, "pairing", "Requesting encrypted pairing");
                        let status =
                            api::request_pairing(server, &storage, device_name.trim(), code.trim())
                                .await?;
                        pairing = Some(status.clone());
                        emit(&events, SessionEvent::PairingPhrase(status))?;
                        Ok(None)
                    }
                    None => Err(SessionError::Pairing(
                        "verify a server before pairing".to_owned(),
                    )),
                },
                SessionAction::ConfirmPairing => match (verified.as_ref(), pairing.as_ref()) {
                    (Some(server), Some(status)) => {
                        emit_status(&events, "pairing", "Confirming the comparison phrase");
                        api::confirm_pairing(server, &storage, status).await?;
                        let saved = storage.server(&server.info.server_id)?.ok_or_else(|| {
                            SessionError::Storage("paired server was not saved".to_owned())
                        })?;
                        emit(&events, SessionEvent::Paired(saved))?;
                        pairing = None;
                        Ok(Some(SessionAction::Connect))
                    }
                    _ => Err(SessionError::Pairing(
                        "there is no pairing phrase to confirm".to_owned(),
                    )),
                },
                SessionAction::Connect => match verified.as_ref() {
                    Some(server) => {
                        run_connection(server, &storage, &mut actions, &events, &frame_sink).await
                    }
                    None => Err(SessionError::Endpoint(
                        "verify a server before connecting".to_owned(),
                    )),
                },
                SessionAction::Send(_)
                | SessionAction::Upload(_)
                | SessionAction::CancelUpload
                | SessionAction::Download { .. } => Err(SessionError::Protocol(
                    "browser command sent while disconnected".to_owned(),
                )),
                SessionAction::Disconnect => {
                    frame_sink.clear();
                    emit(
                        &events,
                        SessionEvent::Disconnected("Disconnected".to_owned()),
                    )?;
                    Ok(None)
                }
                SessionAction::Shutdown => Ok(Some(SessionAction::Shutdown)),
            }
        }
        .await;
        match outcome {
            Ok(Some(SessionAction::Shutdown)) => return,
            Ok(next) => deferred = next,
            Err(error) => {
                let _ = events.try_send(SessionEvent::Failure(error.into()));
            }
        }
    }
}

async fn run_connection(
    server: &VerifiedEndpoint,
    storage: &Storage,
    actions: &mut ActionReceiver<SessionAction>,
    events: &SyncSender<SessionEvent>,
    frame_sink: &Arc<dyn FrameSink>,
) -> Result<Option<SessionAction>> {
    let mut reconnect_policy = ReconnectPolicy::new();
    let mut pending_retry: Option<RetryDecision> = None;
    let mut last_failure = String::new();
    loop {
        if let Some(retry) = pending_retry.take() {
            emit(
                events,
                SessionEvent::Reconnecting {
                    attempt: retry.attempt,
                    maximum: retry.maximum,
                    delay: retry.delay,
                    reason: last_failure.clone(),
                },
            )?;
            tokio::select! {
                () = tokio::time::sleep(retry.delay) => {}
                action = actions.recv() => return handle_interrupted_connection(action, events, frame_sink),
            }
        }

        frame_sink.clear();
        emit_status(events, "authentication", "Authenticating this device");
        let config = tokio::select! {
            result = api::authenticate(server, storage) => result,
            action = actions.recv() => return handle_interrupted_connection(action, events, frame_sink),
        };
        let (attempt, connected_for) = match config {
            Ok(config) => {
                emit(
                    events,
                    SessionEvent::Connected {
                        info: server.info.clone(),
                        config: config.clone(),
                    },
                )?;
                let connected_at = Instant::now();
                (
                    run_socket(server, &config, actions, events, frame_sink).await,
                    connected_at.elapsed(),
                )
            }
            Err(error) => (Err(error), Duration::ZERO),
        };

        match attempt {
            Ok(next) => return Ok(next),
            Err(error) => {
                let retry = reconnect_policy
                    .failure(error.retryable(), connected_for)
                    .map_err(|policy_error| SessionError::Protocol(policy_error.to_string()))?;
                if let Some(retry) = retry {
                    pending_retry = Some(retry);
                    last_failure = error.to_string();
                    continue;
                }
                frame_sink.clear();
                let _ = events.try_send(SessionEvent::Disconnected(
                    "The secure session ended".to_owned(),
                ));
                return Err(error);
            }
        }
    }
}

fn handle_interrupted_connection(
    action: Option<SessionAction>,
    events: &SyncSender<SessionEvent>,
    frame_sink: &Arc<dyn FrameSink>,
) -> Result<Option<SessionAction>> {
    frame_sink.clear();
    match action {
        Some(SessionAction::Disconnect) => {
            emit(
                events,
                SessionEvent::Disconnected("Disconnected".to_owned()),
            )?;
            Ok(None)
        }
        Some(SessionAction::Shutdown) | None => Ok(Some(SessionAction::Shutdown)),
        Some(other) => Ok(Some(other)),
    }
}

async fn run_socket(
    server: &VerifiedEndpoint,
    config: &NativeConfig,
    actions: &mut ActionReceiver<SessionAction>,
    events: &SyncSender<SessionEvent>,
    frame_sink: &Arc<dyn FrameSink>,
) -> Result<Option<SessionAction>> {
    emit_status(
        events,
        "websocket",
        "Opening the authenticated browser stream",
    );
    let mut url = server.endpoint.clone();
    url.set_scheme("wss")
        .map_err(|()| SessionError::Endpoint("could not create WSS endpoint".to_owned()))?;
    url.set_path("/api/v1/ws");
    url.query_pairs_mut()
        .append_pair("ticket", &config.ticket)
        .append_pair("cv", api::compatibility_version())
        .append_pair("nv", api::wire_compatibility_version());
    let mut request = url.as_str().into_client_request()?;
    request.headers_mut().insert(
        "User-Agent",
        HeaderValue::from_str(&format!("Surf Desktop/{}", api::app_version()))
            .map_err(|error| SessionError::Protocol(error.to_string()))?,
    );
    let (socket, _) = tokio_tungstenite::connect_async_tls_with_config(
        request,
        None,
        false,
        Some(Connector::Rustls(Arc::clone(&server.tls))),
    )
    .await?;
    let (mut writer, mut reader) = socket.split();
    send_command(
        &mut writer,
        &Command::Size {
            w: config.vw & !1,
            h: config.vh & !1,
            causal: Causal::default(),
        },
    )
    .await?;

    loop {
        tokio::select! {
            action = actions.recv() => match action {
                Some(SessionAction::Send(command)) => send_command(&mut writer, &command).await?,
                Some(SessionAction::Upload(paths)) => {
                    spawn_upload(server.clone(), paths, events.clone());
                }
                Some(SessionAction::CancelUpload) => {
                    spawn_upload(server.clone(), Vec::new(), events.clone());
                }
                Some(SessionAction::Download { name, destination }) => {
                    spawn_download(server.clone(), name, destination, events.clone());
                }
                Some(SessionAction::Disconnect) => {
                    writer.send(Message::Close(None)).await?;
                    frame_sink.clear();
                    emit(events, SessionEvent::Disconnected("Disconnected".to_owned()))?;
                    return Ok(None);
                }
                Some(SessionAction::Shutdown) | None => {
                    let _ = writer.send(Message::Close(None)).await;
                    return Ok(Some(SessionAction::Shutdown));
                }
                Some(other) => {
                    let _ = writer.send(Message::Close(None)).await;
                    return Ok(Some(other));
                }
            },
            message = reader.next() => match message {
                Some(Ok(Message::Text(text))) => {
                    match Event::decode(text.as_bytes()) {
                        Ok(event) => emit(events, SessionEvent::Control(event))?,
                        Err(error) => return Err(SessionError::Protocol(error.to_string())),
                    }
                }
                Some(Ok(Message::Binary(data))) => {
                    Frame::parse(&data).map_err(|error| SessionError::Protocol(error.to_string()))?;
                    frame_sink.submit(data);
                }
                Some(Ok(Message::Ping(data))) => writer.send(Message::Pong(data)).await?,
                Some(Ok(Message::Pong(_))) | Some(Ok(Message::Frame(_))) => {}
                Some(Ok(Message::Close(frame))) => {
                    let message = frame.map(|frame| frame.reason.to_string()).unwrap_or_else(|| "Server closed the stream".to_owned());
                    return Err(SessionError::Transport(message));
                }
                Some(Err(error)) => return Err(error.into()),
                None => {
                    return Err(SessionError::Transport("Server closed the stream".to_owned()));
                }
            }
        }
    }
}

fn spawn_upload(server: VerifiedEndpoint, paths: Vec<PathBuf>, events: SyncSender<SessionEvent>) {
    tokio::spawn(async move {
        let label = if paths.is_empty() {
            "File selection".to_owned()
        } else if paths.len() == 1 {
            paths[0]
                .file_name()
                .map(|name| name.to_string_lossy().into_owned())
                .unwrap_or_else(|| "File".to_owned())
        } else {
            format!("{} files", paths.len())
        };
        let outcome = api::upload_files(&server, &paths).await;
        let _ = events.try_send(SessionEvent::Transfer {
            kind: TransferKind::Upload,
            name: label,
            ok: outcome.is_ok(),
            message: match outcome {
                Ok(0) => "File selection cancelled".to_owned(),
                Ok(count) => format!("Attached {count} file(s)"),
                Err(error) => error.to_string(),
            },
        });
    });
}

fn spawn_download(
    server: VerifiedEndpoint,
    name: String,
    destination: PathBuf,
    events: SyncSender<SessionEvent>,
) {
    tokio::spawn(async move {
        let outcome = api::download_file(&server, &name, &destination).await;
        let _ = events.try_send(SessionEvent::Transfer {
            kind: TransferKind::Download,
            name,
            ok: outcome.is_ok(),
            message: match outcome {
                Ok(()) => format!("Saved to {}", destination.display()),
                Err(error) => error.to_string(),
            },
        });
    });
}

async fn send_command<S>(writer: &mut S, command: &Command) -> Result<()>
where
    S: futures_util::Sink<Message, Error = tokio_tungstenite::tungstenite::Error> + Unpin,
{
    let json = command
        .encode()
        .map_err(|error| SessionError::Protocol(error.to_string()))?;
    let text = String::from_utf8(json)
        .map_err(|_| SessionError::Protocol("command encoder returned non-UTF-8".to_owned()))?;
    writer.send(Message::Text(text.into())).await?;
    Ok(())
}

fn emit_status(events: &SyncSender<SessionEvent>, phase: &'static str, message: &str) {
    let _ = events.try_send(SessionEvent::Status {
        phase,
        message: message.to_owned(),
    });
}

fn emit(events: &SyncSender<SessionEvent>, event: SessionEvent) -> Result<()> {
    events.try_send(event).map_err(|error| match error {
        TrySendError::Full(_) => {
            SessionError::Protocol("session control-event buffer overflow".to_owned())
        }
        TrySendError::Disconnected(_) => {
            SessionError::Protocol("session event consumer stopped".to_owned())
        }
    })
}

pub fn atomic_write_private(path: &Path, data: &[u8]) -> Result<()> {
    let parent = path
        .parent()
        .ok_or_else(|| SessionError::Storage("private file has no parent".to_owned()))?;
    fs::create_dir_all(parent)?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt as _;
        fs::set_permissions(parent, fs::Permissions::from_mode(0o700))?;
    }
    let temporary = path.with_extension("tmp");
    let mut options = OpenOptions::new();
    options.write(true).create(true).truncate(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt as _;
        options.mode(0o600);
    }
    let mut file = options.open(&temporary)?;
    file.write_all(data)?;
    file.sync_all()?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt as _;
        file.set_permissions(fs::Permissions::from_mode(0o600))?;
    }
    drop(file);
    fs::rename(&temporary, path)?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::{FailureKind, SessionError};

    #[test]
    fn trust_and_compatibility_failures_never_retry() {
        let trust = SessionError::Tls("changed".to_owned());
        assert_eq!(trust.kind(), FailureKind::Trust);
        assert!(!trust.retryable());

        let compatibility = SessionError::Compatibility {
            decision: "client-too-old".to_owned(),
            server_version: "1.0.0".to_owned(),
        };
        assert_eq!(compatibility.kind(), FailureKind::Compatibility);
        assert!(!compatibility.retryable());
    }

    #[test]
    fn transient_server_failures_retry_but_authentication_does_not() {
        let unavailable = SessionError::HttpStatus {
            status: 503,
            message: "restarting".to_owned(),
        };
        assert_eq!(unavailable.kind(), FailureKind::Transport);
        assert!(unavailable.retryable());

        let unauthorized = SessionError::HttpStatus {
            status: 401,
            message: "revoked".to_owned(),
        };
        assert_eq!(unauthorized.kind(), FailureKind::Authentication);
        assert!(!unauthorized.retryable());
    }
}
