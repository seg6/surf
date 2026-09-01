//! Strict typed JSON contracts for Surf's authenticated control lane.
//!
//! The transport decodes here before data reaches the portable state core.
//! Unknown message kinds, unknown fields, trailing data, invalid types, and
//! values outside bounded client limits are rejected.

use serde::{Deserialize, Serialize};
use thiserror::Error;

pub const MAX_TEXT_BYTES: usize = 1024 * 1024;
pub const MAX_COLLECTION_ITEMS: usize = 4096;
pub const MAX_TOUCH_POINTS: usize = 16;

#[derive(Debug, Error)]
pub enum ProtocolError {
    #[error("invalid Surf control JSON: {0}")]
    Json(#[from] serde_json::Error),
    #[error("Surf control value exceeds {field} limit ({actual} > {limit})")]
    Limit {
        field: &'static str,
        actual: usize,
        limit: usize,
    },
    #[error("invalid Surf control value: {0}")]
    Invalid(&'static str),
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct Causal {
    #[serde(default, skip_serializing_if = "is_zero_u64")]
    pub iid: u64,
    #[serde(default, rename = "clientNs", skip_serializing_if = "is_zero_u64")]
    pub client_ns: u64,
}

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct TouchPoint {
    pub id: i32,
    pub x: f64,
    pub y: f64,
    #[serde(default, skip_serializing_if = "is_zero_f64")]
    pub rx: f64,
    #[serde(default, skip_serializing_if = "is_zero_f64")]
    pub ry: f64,
    #[serde(default, skip_serializing_if = "is_zero_f64")]
    pub force: f64,
}

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(tag = "t", deny_unknown_fields)]
pub enum Command {
    #[serde(rename = "size")]
    Size {
        w: i32,
        h: i32,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "clock")]
    Clock {
        c0: u64,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "tab")]
    Tab {
        action: String,
        id: i32,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "nav")]
    Navigate {
        url: String,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "opennew")]
    OpenNew {
        url: String,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "histdel")]
    HistoryDelete {
        url: String,
        ts: i64,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "bmdel")]
    BookmarkDelete {
        url: String,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "audio")]
    Audio {
        on: bool,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "mobile")]
    Mobile {
        on: bool,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "dark")]
    Dark {
        on: bool,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "fullscreen")]
    Fullscreen {
        on: bool,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "touch")]
    Touch {
        phase: String,
        seq: u64,
        surface: u32,
        ts: u64,
        points: Vec<TouchPoint>,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "key")]
    Key {
        down: bool,
        key: String,
        code: String,
        #[serde(rename = "keyCode")]
        key_code: i32,
        text: String,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "paste")]
    Paste {
        text: String,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "compose")]
    Compose {
        phase: String,
        text: String,
        #[serde(default, skip_serializing_if = "is_zero_i32")]
        start: i32,
        #[serde(default, skip_serializing_if = "is_zero_i32")]
        end: i32,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "suggest")]
    Suggest {
        q: String,
        offset: i32,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "history")]
    History {
        q: String,
        offset: i32,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "find")]
    Find {
        q: String,
        dir: i32,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "dldel")]
    DownloadDelete {
        name: String,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "clear")]
    Clear {
        what: String,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "dialogreply")]
    DialogReply {
        accept: bool,
        text: String,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "selectreply")]
    SelectReply {
        id: String,
        #[serde(default, skip_serializing_if = "is_false")]
        cancel: bool,
        #[serde(default, skip_serializing_if = "Vec::is_empty")]
        indices: Vec<i32>,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "media-stats")]
    MediaStats {
        fps: f64,
        #[serde(rename = "presentedFps")]
        presented_fps: f64,
        #[serde(rename = "decodeFps")]
        decode_fps: f64,
        #[serde(rename = "auRate")]
        au_rate: f64,
        renderer: String,
        #[serde(rename = "rendererFps")]
        renderer_fps: f64,
        #[serde(rename = "rendererMs")]
        renderer_ms: f64,
        #[serde(rename = "rendererBackpressure")]
        renderer_backpressure: i32,
        #[serde(rename = "rendererRecoveries")]
        renderer_recoveries: i32,
        #[serde(rename = "rendererFailures")]
        renderer_failures: i32,
        #[serde(rename = "callbackMs")]
        callback_ms: f64,
        #[serde(rename = "gapMs")]
        gap_ms: f64,
        #[serde(rename = "frameAgeMs")]
        frame_age_ms: f64,
        #[serde(rename = "windowMs")]
        window_ms: f64,
        #[serde(rename = "dropPct")]
        drop_pct: f64,
        queue: i32,
        #[serde(rename = "decodeErrors")]
        decode_errors: i32,
        #[serde(rename = "memoryWarn")]
        memory_warn: bool,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "media-volume")]
    MediaVolume {
        value: f64,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "clipboard-result")]
    ClipboardResult {
        id: String,
        ok: bool,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "clipboard-change")]
    ClipboardChange {
        text: String,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "log-record")]
    LogRecord {
        record: serde_json::Value,
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "log-cleared")]
    LogCleared {
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "back")]
    Back {
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "fwd")]
    Forward {
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "reload")]
    Reload {
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "stop")]
    Stop {
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "video-retry")]
    VideoRetry {
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "reqkeyframe")]
    RequestKeyframe {
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "hist")]
    Library {
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "bookmark")]
    Bookmark {
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "downloads")]
    Downloads {
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "reader")]
    Reader {
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "media-playpause")]
    MediaPlayPause {
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "media-mute")]
    MediaMute {
        #[serde(flatten)]
        causal: Causal,
    },
    #[serde(rename = "media-query")]
    MediaQuery {
        #[serde(flatten)]
        causal: Causal,
    },
}

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct TabInfo {
    pub id: i32,
    pub title: String,
    pub url: String,
    pub active: bool,
    #[serde(default)]
    pub icon: String,
}

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct LibraryEntry {
    pub url: String,
    pub title: String,
    pub ts: i64,
}

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct DownloadItem {
    pub name: String,
    pub size: i64,
    pub ts: i64,
}

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct SelectOption {
    pub label: String,
    #[serde(default)]
    pub disabled: bool,
    #[serde(default)]
    pub selected: bool,
}

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(tag = "t", deny_unknown_fields)]
pub enum Event {
    #[serde(rename = "hello")]
    Hello { vw: i32, vh: i32 },
    #[serde(rename = "tabs")]
    Tabs { tabs: Vec<TabInfo> },
    #[serde(rename = "video-config")]
    VideoConfig {
        state: String,
        #[serde(default)]
        reason: String,
        #[serde(default)]
        w: i32,
        #[serde(default)]
        h: i32,
        #[serde(default)]
        generation: u32,
        #[serde(default)]
        profile: String,
    },
    #[serde(rename = "audio-config")]
    AudioConfig {
        ok: bool,
        #[serde(default)]
        rate: i32,
        #[serde(default)]
        channels: i32,
    },
    #[serde(rename = "loading")]
    Loading { on: bool },
    #[serde(rename = "fullscreen")]
    Fullscreen { on: bool },
    #[serde(rename = "found")]
    Found { on: bool },
    #[serde(rename = "starred")]
    Starred { on: bool },
    #[serde(rename = "toast")]
    Toast { text: String },
    #[serde(rename = "clock")]
    Clock { c0: u64, s1: u64, s2: u64 },
    #[serde(rename = "url")]
    Url {
        url: String,
        starred: bool,
        #[serde(default)]
        security: String,
    },
    #[serde(rename = "histstate")]
    HistoryState { back: bool, fwd: bool },
    #[serde(rename = "download")]
    Download { name: String },
    #[serde(rename = "dlprogress")]
    DownloadProgress { name: String, pct: i32 },
    #[serde(rename = "suggest")]
    Suggest { items: Vec<LibraryEntry> },
    #[serde(rename = "hist")]
    Library {
        hist: Vec<LibraryEntry>,
        bookmarks: Vec<LibraryEntry>,
        starred: bool,
    },
    #[serde(rename = "history")]
    History {
        #[serde(default)]
        q: String,
        items: Vec<LibraryEntry>,
        offset: i32,
        total: i32,
    },
    #[serde(rename = "downloads")]
    Downloads { items: Vec<DownloadItem> },
    #[serde(rename = "dialog")]
    Dialog {
        kind: String,
        text: String,
        #[serde(rename = "def")]
        default: String,
    },
    #[serde(rename = "dialogdone")]
    DialogDone,
    #[serde(rename = "filechooser")]
    FileChooser { multiple: bool },
    #[serde(rename = "security")]
    Security { state: String },
    #[serde(rename = "pageerror")]
    PageError {
        url: String,
        starred: bool,
        #[serde(default)]
        security: String,
    },
    #[serde(rename = "reader")]
    Reader {
        ok: bool,
        #[serde(default)]
        title: String,
        #[serde(default)]
        html: String,
        #[serde(default)]
        url: String,
    },
    #[serde(rename = "editable")]
    Editable {
        on: bool,
        #[serde(default, rename = "show")]
        show_keyboard: bool,
        #[serde(default)]
        kind: String,
        #[serde(default)]
        rect: Vec<f64>,
    },
    #[serde(rename = "select")]
    Select {
        id: String,
        #[serde(default)]
        title: String,
        #[serde(default)]
        multiple: bool,
        options: Vec<SelectOption>,
        #[serde(default)]
        rect: Vec<f64>,
    },
    #[serde(rename = "media-state")]
    MediaState {
        available: bool,
        count: i32,
        paused: bool,
        muted: bool,
        volume: f64,
        #[serde(rename = "currentTime")]
        current_time: f64,
        duration: f64,
        #[serde(default)]
        title: String,
    },
    #[serde(rename = "pageframe")]
    PageFrame {
        #[serde(rename = "sourceSeq")]
        source_seq: u32,
    },
    #[serde(rename = "clipboard")]
    Clipboard {
        id: String,
        text: String,
        #[serde(default)]
        sync: bool,
    },
    #[serde(rename = "clipboard-sync")]
    ClipboardSync {
        enabled: bool,
        #[serde(default)]
        known: bool,
        text: String,
    },
    #[serde(rename = "log-request")]
    LogRequest,
    #[serde(rename = "log-clear")]
    LogClear,
}

impl Command {
    pub fn decode(data: &[u8]) -> Result<Self, ProtocolError> {
        let command: Self = serde_json::from_slice(data)?;
        command.validate()?;
        Ok(command)
    }

    pub fn encode(&self) -> Result<Vec<u8>, ProtocolError> {
        self.validate()?;
        Ok(serde_json::to_vec(self)?)
    }

    fn validate(&self) -> Result<(), ProtocolError> {
        if let Self::Touch { phase, points, .. } = self {
            if !matches!(phase.as_str(), "start" | "move" | "end" | "cancel") {
                return Err(ProtocolError::Invalid("touch phase"));
            }
            check_collection("touch points", points.len(), MAX_TOUCH_POINTS)?;
            for point in points {
                if !point.x.is_finite()
                    || !point.y.is_finite()
                    || !point.rx.is_finite()
                    || !point.ry.is_finite()
                    || !point.force.is_finite()
                {
                    return Err(ProtocolError::Invalid("finite touch coordinate"));
                }
            }
        }
        check_text_lengths(serde_json::to_value(self)?)
    }
}

impl Event {
    pub fn decode(data: &[u8]) -> Result<Self, ProtocolError> {
        let event: Self = serde_json::from_slice(data)?;
        event.validate()?;
        Ok(event)
    }

    pub fn encode(&self) -> Result<Vec<u8>, ProtocolError> {
        self.validate()?;
        Ok(serde_json::to_vec(self)?)
    }

    fn validate(&self) -> Result<(), ProtocolError> {
        match self {
            Self::Tabs { tabs } => check_collection("tabs", tabs.len(), MAX_COLLECTION_ITEMS)?,
            Self::Suggest { items } | Self::History { items, .. } => {
                check_collection("library items", items.len(), MAX_COLLECTION_ITEMS)?
            }
            Self::Library {
                hist, bookmarks, ..
            } => {
                check_collection("history", hist.len(), MAX_COLLECTION_ITEMS)?;
                check_collection("bookmarks", bookmarks.len(), MAX_COLLECTION_ITEMS)?;
            }
            Self::Downloads { items } => {
                check_collection("downloads", items.len(), MAX_COLLECTION_ITEMS)?
            }
            Self::Select { options, rect, .. } => {
                check_collection("select options", options.len(), MAX_COLLECTION_ITEMS)?;
                validate_rect(rect, "select rect")?;
            }
            Self::Editable { rect, .. } => validate_rect(rect, "editable rect")?,
            _ => {}
        }
        check_text_lengths(serde_json::to_value(self)?)
    }
}

fn validate_rect(rect: &[f64], label: &'static str) -> Result<(), ProtocolError> {
    if (!rect.is_empty() && rect.len() != 4) || rect.iter().any(|value| !value.is_finite()) {
        return Err(ProtocolError::Invalid(label));
    }
    Ok(())
}

fn check_text_lengths(value: serde_json::Value) -> Result<(), ProtocolError> {
    match value {
        serde_json::Value::String(value) => {
            check_collection("text bytes", value.len(), MAX_TEXT_BYTES)
        }
        serde_json::Value::Array(values) => {
            check_collection("array", values.len(), MAX_COLLECTION_ITEMS)?;
            for value in values {
                check_text_lengths(value)?;
            }
            Ok(())
        }
        serde_json::Value::Object(values) => {
            for value in values.into_values() {
                check_text_lengths(value)?;
            }
            Ok(())
        }
        _ => Ok(()),
    }
}

fn check_collection(field: &'static str, actual: usize, limit: usize) -> Result<(), ProtocolError> {
    if actual > limit {
        return Err(ProtocolError::Limit {
            field,
            actual,
            limit,
        });
    }
    Ok(())
}

const fn is_zero_u64(value: &u64) -> bool {
    *value == 0
}

const fn is_zero_i32(value: &i32) -> bool {
    *value == 0
}

fn is_zero_f64(value: &f64) -> bool {
    *value == 0.0
}

const fn is_false(value: &bool) -> bool {
    !*value
}

#[cfg(test)]
mod tests {
    use super::{Command, Event};

    #[test]
    fn canonical_commands_are_strict_and_round_trip() {
        let fixtures: Vec<serde_json::Value> = serde_json::from_str(include_str!(
            "../../../../../protocol/fixtures/commands.json"
        ))
        .unwrap();
        for fixture in fixtures {
            let data = serde_json::to_vec(&fixture).unwrap();
            let decoded = Command::decode(&data).unwrap_or_else(|error| {
                panic!("command fixture {fixture} did not decode: {error}")
            });
            Command::decode(&decoded.encode().unwrap()).unwrap();
        }
    }

    #[test]
    fn canonical_events_are_strict_and_round_trip() {
        let fixtures: Vec<serde_json::Value> =
            serde_json::from_str(include_str!("../../../../../protocol/fixtures/events.json"))
                .unwrap();
        for fixture in fixtures {
            let data = serde_json::to_vec(&fixture).unwrap();
            let decoded = Event::decode(&data)
                .unwrap_or_else(|error| panic!("event fixture {fixture} did not decode: {error}"));
            Event::decode(&decoded.encode().unwrap()).unwrap();
        }
    }

    #[test]
    fn rejects_unknown_trailing_and_oversized_messages() {
        assert!(Command::decode(br#"{"t":"back","wat":1}"#).is_err());
        assert!(Event::decode(br#"{"t":"loading","on":true} {}"#).is_err());
        let huge = "x".repeat(super::MAX_TEXT_BYTES + 1);
        assert!(Event::Toast { text: huge }.encode().is_err());
    }
}
