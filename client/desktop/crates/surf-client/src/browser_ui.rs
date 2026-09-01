use std::collections::BTreeMap;
use std::time::{Duration, Instant};

use surf_protocol::{DownloadItem, LibraryEntry, SelectOption};

#[derive(Clone, Debug)]
pub struct DialogPrompt {
    pub kind: String,
    pub text: String,
    pub input: String,
}

#[derive(Clone, Debug)]
pub struct SelectPrompt {
    pub id: String,
    pub title: String,
    pub multiple: bool,
    pub options: Vec<SelectOption>,
    pub selected: Vec<bool>,
}

#[derive(Clone, Debug, Default)]
pub struct ReaderDocument {
    pub title: String,
    pub url: String,
    pub text: String,
}

#[derive(Clone, Debug)]
pub struct MediaState {
    pub available: bool,
    pub count: i32,
    pub paused: bool,
    pub muted: bool,
    pub volume: f64,
    pub current_time: f64,
    pub duration: f64,
    pub title: String,
}

impl Default for MediaState {
    fn default() -> Self {
        Self {
            available: false,
            count: 0,
            paused: true,
            muted: false,
            volume: 1.0,
            current_time: 0.0,
            duration: 0.0,
            title: String::new(),
        }
    }
}

#[derive(Clone, Debug)]
pub struct Toast {
    pub text: String,
    pub expires: Instant,
}

#[derive(Default)]
pub struct BrowserUi {
    pub menu_open: bool,
    pub find_open: bool,
    pub find_query: String,
    pub find_found: Option<bool>,
    pub library_open: bool,
    pub library_section: usize,
    pub reader_open: bool,
    pub media_open: bool,
    pub settings_open: bool,
    pub diagnostics_open: bool,
    pub history: Vec<LibraryEntry>,
    pub bookmarks: Vec<LibraryEntry>,
    pub downloads: Vec<DownloadItem>,
    pub download_progress: BTreeMap<String, i32>,
    pub suggestions: Vec<LibraryEntry>,
    pub dialog: Option<DialogPrompt>,
    pub select: Option<SelectPrompt>,
    pub reader: Option<ReaderDocument>,
    pub upload_multiple: Option<bool>,
    pub upload_paths: String,
    pub page_error: Option<String>,
    pub media: MediaState,
    pub clipboard_sync: bool,
    pub clipboard_known: bool,
    pub clipboard_text: String,
    pub pending_clipboard: Option<(String, String)>,
    pub pending_downloads: Vec<String>,
    pub toast: Option<Toast>,
}

impl BrowserUi {
    pub fn reset_connection(&mut self) {
        self.menu_open = false;
        self.find_open = false;
        self.find_query.clear();
        self.find_found = None;
        self.library_open = false;
        self.reader_open = false;
        self.media_open = false;
        self.history.clear();
        self.bookmarks.clear();
        self.downloads.clear();
        self.download_progress.clear();
        self.suggestions.clear();
        self.dialog = None;
        self.select = None;
        self.reader = None;
        self.upload_multiple = None;
        self.upload_paths.clear();
        self.page_error = None;
        self.media = MediaState::default();
        self.clipboard_sync = false;
        self.clipboard_known = false;
        self.clipboard_text.clear();
        self.pending_clipboard = None;
        self.pending_downloads.clear();
        self.toast = None;
    }

    pub fn toast(&mut self, text: impl Into<String>) {
        self.toast = Some(Toast {
            text: text.into(),
            expires: Instant::now() + Duration::from_secs(4),
        });
    }

    pub fn prune(&mut self) {
        if self
            .toast
            .as_ref()
            .is_some_and(|toast| toast.expires <= Instant::now())
        {
            self.toast = None;
        }
    }

    pub fn open_select(
        &mut self,
        id: String,
        title: String,
        multiple: bool,
        options: Vec<SelectOption>,
    ) {
        let selected = options.iter().map(|option| option.selected).collect();
        self.select = Some(SelectPrompt {
            id,
            title,
            multiple,
            options,
            selected,
        });
    }

    pub fn close_transient_overlays(&mut self) -> bool {
        if self.find_open {
            self.find_open = false;
        } else if self.menu_open {
            self.menu_open = false;
        } else {
            return false;
        }
        true
    }
}

pub fn reader_text(html: &str) -> String {
    let mut out = String::with_capacity(html.len());
    let mut in_tag = false;
    let mut entity = String::new();
    let mut in_entity = false;
    for ch in html.chars() {
        if in_tag {
            if ch == '>' {
                in_tag = false;
                if !out.ends_with([' ', '\n']) {
                    out.push(' ');
                }
            }
        } else if in_entity {
            if ch == ';' {
                out.push_str(match entity.as_str() {
                    "amp" => "&",
                    "lt" => "<",
                    "gt" => ">",
                    "quot" => "\"",
                    "apos" | "#39" => "'",
                    "nbsp" => " ",
                    _ => " ",
                });
                entity.clear();
                in_entity = false;
            } else if entity.len() < 12 {
                entity.push(ch);
            } else {
                entity.clear();
                in_entity = false;
            }
        } else if ch == '<' {
            in_tag = true;
        } else if ch == '&' {
            in_entity = true;
        } else {
            out.push(ch);
        }
    }
    out.split_whitespace().collect::<Vec<_>>().join(" ")
}

#[cfg(test)]
mod tests {
    use super::{BrowserUi, reader_text};
    use surf_protocol::SelectOption;

    #[test]
    fn reader_fallback_removes_markup_and_decodes_common_entities() {
        assert_eq!(
            reader_text("<article><h1>A &amp; B</h1><p>Hello&nbsp;world.</p></article>"),
            "A & B Hello world."
        );
    }

    #[test]
    fn selects_preserve_server_selection() {
        let mut state = BrowserUi::default();
        state.open_select(
            "request".to_owned(),
            String::new(),
            true,
            vec![SelectOption {
                label: "One".to_owned(),
                disabled: false,
                selected: true,
            }],
        );
        assert_eq!(state.select.as_ref().unwrap().selected, vec![true]);
    }
}
