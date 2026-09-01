use std::{
    error::Error as StdError,
    ffi::{CStr, c_int},
    fmt,
    marker::PhantomData,
    ptr::{self, NonNull},
    rc::Rc,
    slice, str,
};

use surf_core_sys as sys;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Error {
    code: c_int,
    message: String,
}

impl Error {
    fn from_code(code: c_int) -> Self {
        let message = unsafe {
            let pointer = sys::surf_core_result_string(code);
            if pointer.is_null() {
                "unknown core error".to_owned()
            } else {
                CStr::from_ptr(pointer).to_string_lossy().into_owned()
            }
        };
        Self { code, message }
    }

    fn invalid_utf8() -> Self {
        Self {
            code: -1,
            message: "core snapshot contains invalid UTF-8".to_owned(),
        }
    }

    pub fn code(&self) -> i32 {
        self.code
    }
}

impl fmt::Display for Error {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&self.message)
    }
}

impl StdError for Error {}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Tab {
    pub id: i64,
    pub title: String,
    pub url: String,
    pub icon: String,
    pub active: bool,
}

#[derive(Debug, Clone, PartialEq)]
pub enum Event {
    Reset,
    Tabs(Vec<Tab>),
    Url {
        url: String,
        security: String,
        starred: bool,
    },
    History {
        can_go_back: bool,
        can_go_forward: bool,
    },
    Loading(bool),
    Editable {
        on: bool,
        show_keyboard: bool,
        kind: String,
        rect: Option<[f64; 4]>,
    },
    Fullscreen(bool),
    Security(String),
    Starred(bool),
    PageFrame(u32),
    FramePresented(u32),
    KeyboardVisible(bool),
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Effect {
    RequestLibrary,
    ShowKeyboard,
    HideKeyboard,
    Unknown(i32),
}

#[derive(Debug, Clone, PartialEq)]
pub struct Snapshot {
    pub revision: u64,
    pub tabs: Vec<Tab>,
    pub active_tab_id: Option<i64>,
    pub active_title: String,
    pub current_url: String,
    pub security: String,
    pub editable_kind: String,
    pub editable_rect: Option<[f64; 4]>,
    pub awaited_source_sequence: Option<u32>,
    pub show_start_page: bool,
    pub loading: bool,
    pub can_go_back: bool,
    pub can_go_forward: bool,
    pub starred: bool,
    pub fullscreen: bool,
    pub editable: bool,
    pub keyboard_visible: bool,
    pub awaiting_page_frame: bool,
}

pub struct Core {
    raw: NonNull<sys::surf_core_t>,
    _single_owner: PhantomData<Rc<()>>,
}

impl Core {
    pub fn new() -> Result<Self, Error> {
        let mut config = std::mem::MaybeUninit::<sys::surf_core_config_t>::uninit();
        let mut raw = ptr::null_mut();
        let result = unsafe {
            sys::surf_core_config_init(config.as_mut_ptr());
            sys::surf_core_create(config.as_ptr(), &mut raw)
        };
        check(result)?;
        let raw = NonNull::new(raw).ok_or_else(|| Error {
            code: -1,
            message: "core returned a null handle".to_owned(),
        })?;
        Ok(Self {
            raw,
            _single_owner: PhantomData,
        })
    }

    pub fn dispatch(&mut self, event: &Event) -> Result<Vec<Effect>, Error> {
        match event {
            Event::Tabs(tabs) => self.dispatch_tabs(tabs)?,
            Event::Url {
                url,
                security,
                starred,
            } => {
                let raw = sys::surf_event_t {
                    kind: sys::SURF_EVENT_URL,
                    data: sys::surf_event_data_t {
                        url: sys::surf_url_event_t {
                            url: string_view(url),
                            security: string_view(security),
                            starred: int(*starred),
                        },
                    },
                };
                self.dispatch_raw(&raw)?;
            }
            Event::History {
                can_go_back,
                can_go_forward,
            } => {
                let raw = sys::surf_event_t {
                    kind: sys::SURF_EVENT_HISTORY_STATE,
                    data: sys::surf_event_data_t {
                        history: sys::surf_history_event_t {
                            can_go_back: int(*can_go_back),
                            can_go_forward: int(*can_go_forward),
                        },
                    },
                };
                self.dispatch_raw(&raw)?;
            }
            Event::Editable {
                on,
                show_keyboard,
                kind,
                rect,
            } => {
                let raw = sys::surf_event_t {
                    kind: sys::SURF_EVENT_EDITABLE,
                    data: sys::surf_event_data_t {
                        editable: sys::surf_editable_event_t {
                            on: int(*on),
                            show_keyboard: int(*show_keyboard),
                            kind: string_view(kind),
                            has_rect: int(rect.is_some()),
                            rect: rect.unwrap_or_default(),
                        },
                    },
                };
                self.dispatch_raw(&raw)?;
            }
            Event::Security(state) => {
                let raw = sys::surf_event_t {
                    kind: sys::SURF_EVENT_SECURITY,
                    data: sys::surf_event_data_t {
                        security: sys::surf_security_event_t {
                            state: string_view(state),
                        },
                    },
                };
                self.dispatch_raw(&raw)?;
            }
            Event::PageFrame(sequence) | Event::FramePresented(sequence) => {
                let kind = if matches!(event, Event::PageFrame(_)) {
                    sys::SURF_EVENT_PAGE_FRAME
                } else {
                    sys::SURF_EVENT_FRAME_PRESENTED
                };
                let raw = sys::surf_event_t {
                    kind,
                    data: sys::surf_event_data_t {
                        page_frame: sys::surf_page_frame_event_t {
                            source_sequence: *sequence,
                        },
                    },
                };
                self.dispatch_raw(&raw)?;
            }
            Event::Reset
            | Event::Loading(_)
            | Event::Fullscreen(_)
            | Event::Starred(_)
            | Event::KeyboardVisible(_) => {
                let (kind, value) = match event {
                    Event::Reset => (sys::SURF_EVENT_RESET, false),
                    Event::Loading(value) => (sys::SURF_EVENT_LOADING, *value),
                    Event::Fullscreen(value) => (sys::SURF_EVENT_FULLSCREEN, *value),
                    Event::Starred(value) => (sys::SURF_EVENT_STARRED, *value),
                    Event::KeyboardVisible(value) => (sys::SURF_EVENT_KEYBOARD_VISIBILITY, *value),
                    _ => unreachable!(),
                };
                let raw = sys::surf_event_t {
                    kind,
                    data: sys::surf_event_data_t {
                        boolean: sys::surf_boolean_event_t { on: int(value) },
                    },
                };
                self.dispatch_raw(&raw)?;
            }
        }
        Ok(self.drain_effects())
    }

    fn dispatch_tabs(&mut self, tabs: &[Tab]) -> Result<(), Error> {
        let raw_tabs: Vec<_> = tabs
            .iter()
            .map(|tab| sys::surf_tab_event_t {
                id: tab.id,
                title: string_view(&tab.title),
                url: string_view(&tab.url),
                icon: string_view(&tab.icon),
                active: int(tab.active),
            })
            .collect();
        let raw = sys::surf_event_t {
            kind: sys::SURF_EVENT_TABS,
            data: sys::surf_event_data_t {
                tabs: sys::surf_tabs_event_t {
                    items: if raw_tabs.is_empty() {
                        ptr::null()
                    } else {
                        raw_tabs.as_ptr()
                    },
                    count: raw_tabs.len(),
                },
            },
        };
        self.dispatch_raw(&raw)
    }

    fn dispatch_raw(&mut self, event: &sys::surf_event_t) -> Result<(), Error> {
        let result = unsafe { sys::surf_core_dispatch(self.raw.as_ptr(), event) };
        check(result)
    }

    pub fn snapshot(&self) -> Result<Snapshot, Error> {
        let mut raw = std::mem::MaybeUninit::<sys::surf_snapshot_t>::uninit();
        check(unsafe { sys::surf_core_snapshot(self.raw.as_ptr(), raw.as_mut_ptr()) })?;
        let raw = unsafe { raw.assume_init() };
        let raw_tabs = if raw.tab_count == 0 {
            &[][..]
        } else {
            if raw.tabs.is_null() {
                return Err(Error {
                    code: -1,
                    message: "core returned null tabs with a nonzero count".to_owned(),
                });
            }
            unsafe { slice::from_raw_parts(raw.tabs, raw.tab_count) }
        };
        let tabs = raw_tabs
            .iter()
            .map(|tab| {
                Ok(Tab {
                    id: tab.id,
                    title: owned_string(tab.title)?,
                    url: owned_string(tab.url)?,
                    icon: owned_string(tab.icon)?,
                    active: tab.active != 0,
                })
            })
            .collect::<Result<Vec<_>, Error>>()?;
        Ok(Snapshot {
            revision: raw.revision,
            tabs,
            active_tab_id: (raw.has_active_tab != 0).then_some(raw.active_tab_id),
            active_title: owned_string(raw.active_title)?,
            current_url: owned_string(raw.current_url)?,
            security: owned_string(raw.security)?,
            editable_kind: owned_string(raw.editable_kind)?,
            editable_rect: (raw.editable_has_rect != 0).then_some(raw.editable_rect),
            awaited_source_sequence: (raw.awaiting_page_frame != 0)
                .then_some(raw.awaited_source_sequence),
            show_start_page: raw.show_start_page != 0,
            loading: raw.loading != 0,
            can_go_back: raw.can_go_back != 0,
            can_go_forward: raw.can_go_forward != 0,
            starred: raw.starred != 0,
            fullscreen: raw.fullscreen != 0,
            editable: raw.editable != 0,
            keyboard_visible: raw.keyboard_visible != 0,
            awaiting_page_frame: raw.awaiting_page_frame != 0,
        })
    }

    fn drain_effects(&mut self) -> Vec<Effect> {
        let mut effects = Vec::new();
        loop {
            let mut raw = sys::surf_effect_t { kind: 0 };
            if unsafe { sys::surf_core_next_effect(self.raw.as_ptr(), &mut raw) } == 0 {
                break;
            }
            effects.push(match raw.kind {
                1 => Effect::RequestLibrary,
                2 => Effect::ShowKeyboard,
                3 => Effect::HideKeyboard,
                other => Effect::Unknown(other),
            });
        }
        effects
    }
}

impl Drop for Core {
    fn drop(&mut self) {
        unsafe { sys::surf_core_destroy(self.raw.as_ptr()) };
    }
}

fn check(code: c_int) -> Result<(), Error> {
    if code == sys::SURF_CORE_OK {
        Ok(())
    } else {
        Err(Error::from_code(code))
    }
}

fn int(value: bool) -> c_int {
    i32::from(value)
}

fn string_view(value: &str) -> sys::surf_string_view_t {
    sys::surf_string_view_t {
        data: value.as_ptr().cast(),
        length: value.len(),
    }
}

fn owned_string(value: sys::surf_string_view_t) -> Result<String, Error> {
    if value.length == 0 {
        return Ok(String::new());
    }
    if value.data.is_null() {
        return Err(Error {
            code: -1,
            message: "core returned a null string with nonzero length".to_owned(),
        });
    }
    let bytes = unsafe { slice::from_raw_parts(value.data.cast::<u8>(), value.length) };
    Ok(str::from_utf8(bytes)
        .map_err(|_| Error::invalid_utf8())?
        .to_owned())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rust_drives_the_c_browser_model() {
        let mut core = Core::new().unwrap();
        assert!(core.snapshot().unwrap().show_start_page);
        core.dispatch(&Event::Tabs(vec![Tab {
            id: 7,
            title: "https://surf.test/page".to_owned(),
            url: "https://surf.test/page".to_owned(),
            icon: String::new(),
            active: true,
        }]))
        .unwrap();
        let snapshot = core.snapshot().unwrap();
        assert_eq!(snapshot.active_tab_id, Some(7));
        assert_eq!(snapshot.active_title, "surf.test");

        core.dispatch(&Event::Loading(true)).unwrap();
        core.dispatch(&Event::Editable {
            on: true,
            show_keyboard: true,
            kind: "text".to_owned(),
            rect: Some([0.1, 0.2, 0.3, 0.4]),
        })
        .unwrap();
        let snapshot = core.snapshot().unwrap();
        assert!(snapshot.loading && snapshot.editable);
        assert_eq!(snapshot.editable_rect, Some([0.1, 0.2, 0.3, 0.4]));
    }

    #[test]
    fn event_strings_are_copied_by_the_core() {
        let mut core = Core::new().unwrap();
        let mut title = "Original".to_owned();
        core.dispatch(&Event::Tabs(vec![Tab {
            id: 1,
            title: title.clone(),
            url: "https://example.test".to_owned(),
            icon: String::new(),
            active: true,
        }]))
        .unwrap();
        title.replace_range(.., "Changed!");
        assert_eq!(core.snapshot().unwrap().active_title, "Original");
    }
}
