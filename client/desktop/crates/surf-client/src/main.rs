mod gtk_input;
mod gtk_video;

use std::cell::{Cell, RefCell};
use std::path::PathBuf;
use std::rc::Rc;
use std::time::{Duration, Instant};

use gtk::gdk;
use gtk::glib::{self, ControlFlow, Propagation};
use gtk::prelude::*;
use surf_client_app::{ClientController, HostEffect, compact_address};
use surf_protocol::{Causal, Command};

use gtk_input::InputBridge;
use gtk_video::VideoSurface;

const APP_ID: &str = "space.seg6.surf.client";

fn main() -> glib::ExitCode {
    let application = gtk::Application::builder().application_id(APP_ID).build();
    let active_window: Rc<RefCell<Option<Rc<BrowserWindow>>>> = Rc::new(RefCell::new(None));
    application.connect_startup(|_| install_css());
    let window_slot = Rc::clone(&active_window);
    application.connect_activate(move |application| match ClientController::new() {
        Ok(controller) => {
            let window = BrowserWindow::new(application, controller);
            window.present();
            window_slot.replace(Some(window));
        }
        Err(error) => show_startup_error(application, &error),
    });
    // The controller reads the optional startup URL itself. Do not let
    // GApplication reinterpret it as a local file-open request.
    let exit = application.run_with_args(&["surf-client"]);
    active_window.borrow_mut().take();
    exit
}

fn install_css() {
    let provider = gtk::CssProvider::new();
    provider.load_from_data(
        r#"
        window { background: #17181a; color: #f2f3f5; }
        .surf-chrome { background: #202124; }
        .surf-tabs { min-height: 34px; padding: 3px 6px 0 6px; }
        .surf-tab { background: transparent; border: 0; border-radius: 5px 5px 0 0; padding: 5px 8px; }
        .surf-tab:hover { background: #292b2f; }
        .surf-tab.active { background: #34363a; color: #ffffff; }
        .surf-tab .surf-icon { opacity: 0.55; }
        .surf-tab:hover .surf-icon, .surf-tab.active .surf-icon { opacity: 1; }
        .surf-toolbar { min-height: 38px; padding: 5px 8px 7px 8px; border-bottom: 1px solid #3a3c40; }
        .surf-icon { min-width: 30px; min-height: 30px; padding: 0; border: 0; background: transparent; }
        .surf-icon:hover { background: #35373b; }
        .surf-omnibox { min-height: 30px; padding: 0 10px; border-radius: 7px; background: #303236; border: 1px solid #45484d; }
        .surf-omnibox:focus-within { border-color: #6bb8cf; box-shadow: 0 0 0 1px #6bb8cf; }
        .surf-omnibox entry { background: transparent; border: 0; box-shadow: none; padding: 0; }
        .surf-loading { min-height: 2px; padding: 0; }
        .surf-start { padding: 42px; }
        .surf-start-title { font-size: 28px; font-weight: 700; }
        .surf-start-copy { color: #aeb1b7; }
        .surf-server-row { padding: 10px 12px; border-radius: 7px; }
        .surf-server-row:hover { background: #292b2f; }
        .surf-status { color: #aeb1b7; }
        .surf-section { font-size: 11px; font-weight: 700; color: #9b9ea5; letter-spacing: 0.08em; }
        .surf-page { background: #000000; }
        .surf-popover { padding: 6px; }
        .surf-popover button { min-height: 34px; padding: 4px 10px; }
        .surf-toast { background: #303236; border: 1px solid #4a4d52; border-radius: 7px; padding: 8px 12px; }
        .surf-dialog-content { padding: 18px; }
        .surf-dialog-heading { font-size: 18px; font-weight: 700; }
        .surf-library-row { padding: 8px 10px; border-bottom: 1px solid #36383c; }
        .surf-library-row:hover { background: #292b2f; }
        .surf-metric { padding: 12px; border-radius: 7px; background: #292b2f; }
        .surf-metric-value { font-size: 20px; font-weight: 700; }
        .surf-monospace { font-family: monospace; }
        "#,
    );
    if let Some(display) = gdk::Display::default() {
        gtk::style_context_add_provider_for_display(
            &display,
            &provider,
            gtk::STYLE_PROVIDER_PRIORITY_APPLICATION,
        );
    }
    if let Some(settings) = gtk::Settings::default() {
        settings.set_gtk_application_prefer_dark_theme(true);
    }
}

fn show_startup_error(application: &gtk::Application, error: &str) {
    let window = gtk::ApplicationWindow::builder()
        .application(application)
        .title("Surf")
        .default_width(560)
        .default_height(220)
        .build();
    let content = gtk::Box::new(gtk::Orientation::Vertical, 12);
    content.set_margin_top(28);
    content.set_margin_bottom(28);
    content.set_margin_start(28);
    content.set_margin_end(28);
    content.append(
        &gtk::Label::builder()
            .label("Surf could not start")
            .xalign(0.0)
            .build(),
    );
    content.append(
        &gtk::Label::builder()
            .label(error)
            .wrap(true)
            .xalign(0.0)
            .build(),
    );
    window.set_child(Some(&content));
    window.present();
}

struct BrowserWindow {
    controller: Rc<RefCell<ClientController>>,
    video: Rc<RefCell<VideoSurface>>,
    input: Rc<RefCell<InputBridge>>,
    window: gtk::ApplicationWindow,
    view_stack: gtk::Stack,
    start_servers: gtk::Box,
    start_status: gtk::Label,
    endpoint_entry: gtk::Entry,
    pairing_box: gtk::Box,
    pairing_code: gtk::Entry,
    pairing_phrase: gtk::Label,
    pair_button: gtk::Button,
    confirm_button: gtk::Button,
    connect_button: gtk::Button,
    tab_box: gtk::Box,
    back_button: gtk::Button,
    forward_button: gtk::Button,
    reload_button: gtk::Button,
    reload_image: gtk::Image,
    star_button: gtk::Button,
    address_entry: gtk::Entry,
    suggestions: gtk::Popover,
    suggestions_box: gtk::Box,
    loading: gtk::ProgressBar,
    gl_area: gtk::GLArea,
    status_overlay: gtk::Label,
    toast: gtk::Label,
    address_editing: Cell<bool>,
    last_tab_revision: Cell<u64>,
    last_servers: RefCell<Vec<(String, String)>>,
    last_suggestions: RefCell<Vec<(String, String)>>,
    was_connected: Cell<bool>,
    library_view: RefCell<Option<LibraryView>>,
    media_view: RefCell<Option<MediaView>>,
    performance_view: RefCell<Option<PerformanceView>>,
    settings_window: RefCell<Option<gtk::Window>>,
    find_popover: RefCell<Option<gtk::Popover>>,
    page_dialog: RefCell<Option<gtk::Dialog>>,
    page_select: RefCell<Option<gtk::Popover>>,
    file_chooser: RefCell<Option<gtk::FileChooserNative>>,
    reader_window: RefCell<Option<gtk::Window>>,
    page_error: RefCell<Option<gtk::MessageDialog>>,
    smoke: RefCell<SmokeState>,
}

struct LibraryView {
    window: gtk::Window,
    history: gtk::Box,
    bookmarks: gtk::Box,
    downloads: gtk::Box,
    signature: RefCell<String>,
}

struct MediaView {
    window: gtk::Window,
    title: gtk::Label,
    time: gtk::Label,
    play_pause: gtk::Button,
    mute: gtk::Button,
    volume: gtk::Scale,
    unavailable: gtk::Label,
    controls: gtk::Box,
    updating: Cell<bool>,
}

struct PerformanceView {
    window: gtk::Window,
    presented: gtk::Label,
    decoded: gtk::Label,
    dropped: gtk::Label,
    details: gtk::Label,
    health: gtk::Label,
}

struct SmokeState {
    target: Option<u64>,
    started: Option<Instant>,
    last_heartbeat: Instant,
    interaction: bool,
    interaction_step: u8,
    stall_ms: Option<u64>,
    stalled: bool,
    reported: bool,
}

impl SmokeState {
    fn from_environment() -> Self {
        let target = std::env::var("SURF_SMOKE_EXIT_AFTER_FRAMES")
            .ok()
            .and_then(|value| value.parse::<u64>().ok())
            .filter(|value| *value > 0)
            .or_else(|| {
                std::env::var_os("SURF_SMOKE_EXIT_AFTER_FRAME")
                    .is_some()
                    .then_some(1)
            });
        Self {
            target,
            started: None,
            last_heartbeat: Instant::now(),
            interaction: std::env::var_os("SURF_SMOKE_INTERACTION").is_some(),
            interaction_step: 0,
            stall_ms: std::env::var("SURF_SMOKE_STALL_MS")
                .ok()
                .and_then(|value| value.parse::<u64>().ok())
                .filter(|value| *value > 0),
            stalled: false,
            reported: false,
        }
    }
}

impl BrowserWindow {
    fn new(application: &gtk::Application, controller: ClientController) -> Rc<Self> {
        let controller = Rc::new(RefCell::new(controller));
        let video = Rc::new(RefCell::new(VideoSurface::new()));
        let input = Rc::new(RefCell::new(InputBridge::new()));
        let window = gtk::ApplicationWindow::builder()
            .application(application)
            .title("Surf")
            .default_width(1180)
            .default_height(760)
            .build();
        window.set_size_request(720, 480);

        let view_stack = gtk::Stack::builder()
            .transition_type(gtk::StackTransitionType::Crossfade)
            .transition_duration(120)
            .hexpand(true)
            .vexpand(true)
            .build();

        let start = gtk::Box::new(gtk::Orientation::Vertical, 14);
        start.add_css_class("surf-start");
        start.set_halign(gtk::Align::Center);
        start.set_valign(gtk::Align::Center);
        start.set_size_request(520, -1);
        let logo = gtk::Image::from_file(concat!(
            env!("CARGO_MANIFEST_DIR"),
            "/../../../../backend/cmd/surf/surf-icon.png"
        ));
        logo.set_pixel_size(82);
        start.append(&logo);
        let title = gtk::Label::new(Some("Surf"));
        title.add_css_class("surf-start-title");
        start.append(&title);
        let copy = gtk::Label::new(Some("Choose the computer that will run your browser."));
        copy.add_css_class("surf-start-copy");
        start.append(&copy);
        let start_servers = gtk::Box::new(gtk::Orientation::Vertical, 4);
        start_servers.set_hexpand(true);
        start.append(&start_servers);

        let address_row = gtk::Box::new(gtk::Orientation::Horizontal, 6);
        address_row.set_hexpand(true);
        let endpoint_entry = gtk::Entry::builder()
            .hexpand(true)
            .placeholder_text("Computer address or host name")
            .activates_default(true)
            .build();
        let inspect_button = gtk::Button::with_label("Add server");
        address_row.append(&endpoint_entry);
        address_row.append(&inspect_button);
        start.append(&address_row);

        let pairing_box = gtk::Box::new(gtk::Orientation::Vertical, 8);
        pairing_box.set_visible(false);
        let pairing_code = gtk::Entry::builder()
            .placeholder_text("Six-digit pairing code")
            .input_purpose(gtk::InputPurpose::Digits)
            .max_length(6)
            .build();
        let pair_button = gtk::Button::with_label("Compare words");
        let pairing_phrase = gtk::Label::builder().wrap(true).selectable(true).build();
        let confirm_button = gtk::Button::with_label("Words match");
        confirm_button.set_visible(false);
        let connect_button = gtk::Button::with_label("Connect");
        connect_button.set_visible(false);
        pairing_box.append(&pairing_code);
        pairing_box.append(&pair_button);
        pairing_box.append(&pairing_phrase);
        pairing_box.append(&confirm_button);
        pairing_box.append(&connect_button);
        start.append(&pairing_box);
        let start_status = gtk::Label::builder().wrap(true).xalign(0.5).build();
        start_status.add_css_class("surf-status");
        start.append(&start_status);
        view_stack.add_named(&start, Some("start"));

        let browser = gtk::Box::new(gtk::Orientation::Vertical, 0);
        browser.add_css_class("surf-chrome");
        let tabs_row = gtk::Box::new(gtk::Orientation::Horizontal, 4);
        tabs_row.add_css_class("surf-tabs");
        let tab_scroll = gtk::ScrolledWindow::builder()
            .hexpand(true)
            .hscrollbar_policy(gtk::PolicyType::Never)
            .vscrollbar_policy(gtk::PolicyType::Never)
            .propagate_natural_height(true)
            .build();
        let tab_box = gtk::Box::new(gtk::Orientation::Horizontal, 2);
        tab_scroll.set_child(Some(&tab_box));
        let new_tab = icon_button("tab-new-symbolic", "New tab (Ctrl+T)");
        tabs_row.append(&tab_scroll);
        tabs_row.append(&new_tab);
        browser.append(&tabs_row);

        let toolbar = gtk::Box::new(gtk::Orientation::Horizontal, 4);
        toolbar.add_css_class("surf-toolbar");
        let back_button = icon_button("go-previous-symbolic", "Back (Alt+Left)");
        let forward_button = icon_button("go-next-symbolic", "Forward (Alt+Right)");
        let reload_image = gtk::Image::from_icon_name("view-refresh-symbolic");
        let reload_button = gtk::Button::builder()
            .child(&reload_image)
            .tooltip_text("Reload (Ctrl+R)")
            .build();
        reload_button.add_css_class("surf-icon");
        toolbar.append(&back_button);
        toolbar.append(&forward_button);
        toolbar.append(&reload_button);

        let omnibox = gtk::Box::new(gtk::Orientation::Horizontal, 6);
        omnibox.add_css_class("surf-omnibox");
        omnibox.set_hexpand(true);
        let security = gtk::Image::from_icon_name("channel-secure-symbolic");
        security.set_pixel_size(14);
        let address_entry = gtk::Entry::builder()
            .hexpand(true)
            .placeholder_text("Search or enter address")
            .build();
        omnibox.append(&security);
        omnibox.append(&address_entry);
        toolbar.append(&omnibox);
        let star_button = icon_button("non-starred-symbolic", "Bookmark (Ctrl+D)");
        toolbar.append(&star_button);

        let tools_button = gtk::MenuButton::builder()
            .icon_name("view-more-symbolic")
            .tooltip_text("Browser tools")
            .build();
        tools_button.add_css_class("surf-icon");
        let tools_popover = gtk::Popover::new();
        let tools_box = gtk::Box::new(gtk::Orientation::Vertical, 2);
        tools_box.add_css_class("surf-popover");
        for (label, action) in [
            ("Library", "library"),
            ("Reader", "reader"),
            ("Find on page", "find"),
            ("Media controls", "media"),
            ("Fullscreen", "fullscreen"),
            ("Performance", "performance"),
            ("Settings", "settings"),
        ] {
            let button = gtk::Button::with_label(label);
            button.set_halign(gtk::Align::Fill);
            button.set_action_name(Some(&format!("win.{action}")));
            tools_box.append(&button);
        }
        tools_popover.set_child(Some(&tools_box));
        tools_button.set_popover(Some(&tools_popover));
        toolbar.append(&tools_button);
        browser.append(&toolbar);
        let loading = gtk::ProgressBar::new();
        loading.add_css_class("surf-loading");
        loading.set_opacity(0.0);
        browser.append(&loading);

        let page_overlay = gtk::Overlay::new();
        page_overlay.set_hexpand(true);
        page_overlay.set_vexpand(true);
        page_overlay.add_css_class("surf-page");
        let gl_area = gtk::GLArea::builder()
            .hexpand(true)
            .vexpand(true)
            .auto_render(false)
            .has_depth_buffer(false)
            .has_stencil_buffer(false)
            .build();
        gl_area.set_focusable(true);
        page_overlay.set_child(Some(&gl_area));
        let status_overlay = gtk::Label::builder()
            .halign(gtk::Align::Start)
            .valign(gtk::Align::Start)
            .margin_top(14)
            .margin_start(14)
            .build();
        status_overlay.add_css_class("surf-status");
        page_overlay.add_overlay(&status_overlay);
        let toast = gtk::Label::builder()
            .halign(gtk::Align::Center)
            .valign(gtk::Align::End)
            .margin_bottom(18)
            .visible(false)
            .build();
        toast.add_css_class("surf-toast");
        page_overlay.add_overlay(&toast);
        browser.append(&page_overlay);
        view_stack.add_named(&browser, Some("browser"));
        window.set_child(Some(&view_stack));

        let suggestions = gtk::Popover::builder().has_arrow(false).build();
        let suggestions_box = gtk::Box::new(gtk::Orientation::Vertical, 1);
        suggestions_box.add_css_class("surf-popover");
        suggestions.set_child(Some(&suggestions_box));
        suggestions.set_parent(&omnibox);

        let ui = Rc::new(Self {
            controller,
            video,
            input,
            window,
            view_stack,
            start_servers,
            start_status,
            endpoint_entry,
            pairing_box,
            pairing_code,
            pairing_phrase,
            pair_button,
            confirm_button,
            connect_button,
            tab_box,
            back_button,
            forward_button,
            reload_button,
            reload_image,
            star_button,
            address_entry,
            suggestions,
            suggestions_box,
            loading,
            gl_area,
            status_overlay,
            toast,
            address_editing: Cell::new(false),
            last_tab_revision: Cell::new(0),
            last_servers: RefCell::new(Vec::new()),
            last_suggestions: RefCell::new(Vec::new()),
            was_connected: Cell::new(false),
            library_view: RefCell::new(None),
            media_view: RefCell::new(None),
            performance_view: RefCell::new(None),
            settings_window: RefCell::new(None),
            find_popover: RefCell::new(None),
            page_dialog: RefCell::new(None),
            page_select: RefCell::new(None),
            file_chooser: RefCell::new(None),
            reader_window: RefCell::new(None),
            page_error: RefCell::new(None),
            smoke: RefCell::new(SmokeState::from_environment()),
        });

        ui.install_actions();
        ui.connect_start_controls(&inspect_button);
        ui.connect_browser_controls(&new_tab);
        ui.connect_video();
        ui.connect_shortcuts();
        ui.update_view();

        let weak = Rc::downgrade(&ui);
        // Drain the bounded decoder output faster than one display interval;
        // GtkGLArea still presents on GTK's frame clock, while the newest
        // decoded frame is ready before each refresh instead of one tick late.
        glib::timeout_add_local(Duration::from_millis(8), move || {
            let Some(ui) = weak.upgrade() else {
                return ControlFlow::Break;
            };
            ui.tick();
            ControlFlow::Continue
        });
        ui
    }

    fn present(&self) {
        self.window.present();
        if self.controller.borrow().saved_servers.is_empty() {
            self.endpoint_entry.grab_focus();
        }
    }

    fn connect_start_controls(self: &Rc<Self>, inspect_button: &gtk::Button) {
        let weak = Rc::downgrade(self);
        inspect_button.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.inspect_entered_server();
            }
        });
        let weak = Rc::downgrade(self);
        self.endpoint_entry.connect_activate(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.inspect_entered_server();
            }
        });
        let weak = Rc::downgrade(self);
        self.pair_button.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                let code = ui.pairing_code.text().to_string();
                let device_name = glib::host_name().to_string();
                ui.controller.borrow_mut().pair(code, device_name);
            }
        });
        let weak = Rc::downgrade(self);
        self.confirm_button.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.controller.borrow_mut().confirm_pairing();
            }
        });
        let weak = Rc::downgrade(self);
        self.connect_button.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.controller.borrow_mut().connect();
            }
        });
    }

    fn inspect_entered_server(&self) {
        let endpoint = self.endpoint_entry.text().trim().to_owned();
        if !endpoint.is_empty() {
            self.controller.borrow_mut().inspect(endpoint, false);
        }
    }

    fn connect_browser_controls(self: &Rc<Self>, new_tab: &gtk::Button) {
        let weak = Rc::downgrade(self);
        self.back_button.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.command(Command::Back {
                    causal: Causal::default(),
                });
            }
        });
        let weak = Rc::downgrade(self);
        self.forward_button.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.command(Command::Forward {
                    causal: Causal::default(),
                });
            }
        });
        let weak = Rc::downgrade(self);
        self.reload_button.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                let loading = ui.controller.borrow().snapshot.loading;
                ui.command(if loading {
                    Command::Stop {
                        causal: Causal::default(),
                    }
                } else {
                    Command::Reload {
                        causal: Causal::default(),
                    }
                });
            }
        });
        let weak = Rc::downgrade(self);
        new_tab.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.command(Command::Tab {
                    action: "new".to_owned(),
                    id: 0,
                    causal: Causal::default(),
                });
            }
        });
        let weak = Rc::downgrade(self);
        self.star_button.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.command(Command::Bookmark {
                    causal: Causal::default(),
                });
            }
        });

        let weak = Rc::downgrade(self);
        self.address_entry.connect_has_focus_notify(move |entry| {
            let Some(ui) = weak.upgrade() else {
                return;
            };
            if entry.has_focus() {
                ui.address_editing.set(true);
                entry.set_text(&ui.controller.borrow().snapshot.current_url);
                entry.select_region(0, -1);
            } else {
                ui.address_editing.set(false);
                ui.suggestions.popdown();
                entry.set_text(&compact_address(
                    &ui.controller.borrow().snapshot.current_url,
                ));
            }
        });
        let weak = Rc::downgrade(self);
        self.address_entry.connect_changed(move |entry| {
            let Some(ui) = weak.upgrade() else {
                return;
            };
            if ui.address_editing.get() && entry.has_focus() {
                ui.command(Command::Suggest {
                    q: entry.text().to_string(),
                    offset: 0,
                    causal: Causal::default(),
                });
            }
        });
        let weak = Rc::downgrade(self);
        self.address_entry.connect_activate(move |entry| {
            if let Some(ui) = weak.upgrade() {
                ui.controller.borrow_mut().navigate(&entry.text());
                ui.address_editing.set(false);
                ui.suggestions.popdown();
                ui.gl_area.grab_focus();
            }
        });
    }

    fn install_actions(self: &Rc<Self>) {
        for name in [
            "library",
            "reader",
            "find",
            "media",
            "fullscreen",
            "performance",
            "settings",
        ] {
            let action = gtk::gio::SimpleAction::new(name, None);
            let weak = Rc::downgrade(self);
            action.connect_activate(move |action, _| {
                if let Some(ui) = weak.upgrade() {
                    ui.activate_tool(action.name().as_str());
                }
            });
            self.window.add_action(&action);
        }
    }

    fn activate_tool(self: &Rc<Self>, name: &str) {
        match name {
            "library" => {
                self.command(Command::Library {
                    causal: Causal::default(),
                });
                self.command(Command::Downloads {
                    causal: Causal::default(),
                });
                self.show_library();
            }
            "reader" => self.command(Command::Reader {
                causal: Causal::default(),
            }),
            "find" => self.show_find(),
            "media" => {
                self.command(Command::MediaQuery {
                    causal: Causal::default(),
                });
                self.show_media();
            }
            "fullscreen" => {
                let fullscreen = self.controller.borrow().snapshot.fullscreen;
                self.command(Command::Fullscreen {
                    on: !fullscreen,
                    causal: Causal::default(),
                });
            }
            "performance" => self.show_performance(),
            "settings" => self.show_settings(),
            _ => {}
        }
    }

    fn connect_video(self: &Rc<Self>) {
        let weak = Rc::downgrade(self);
        self.gl_area.connect_realize(move |area| {
            let Some(ui) = weak.upgrade() else {
                return;
            };
            area.make_current();
            if let Some(error) = area.error() {
                ui.controller
                    .borrow_mut()
                    .report_renderer_error(error.to_string());
                return;
            }
            if let Err(error) = ui.video.borrow_mut().realize() {
                ui.controller.borrow_mut().report_renderer_error(error);
            }
        });
        let weak = Rc::downgrade(self);
        self.gl_area.connect_unrealize(move |area| {
            if let Some(ui) = weak.upgrade() {
                area.make_current();
                ui.video.borrow_mut().unrealize();
            }
        });
        let weak = Rc::downgrade(self);
        self.gl_area.connect_render(move |area, _| {
            let Some(ui) = weak.upgrade() else {
                return Propagation::Proceed;
            };
            let scale = area.scale_factor();
            let presented = ui.video.borrow_mut().render(
                area.allocated_width() * scale,
                area.allocated_height() * scale,
            );
            if let Some(frame) = presented {
                ui.controller
                    .borrow_mut()
                    .report_presented_frame(frame.generation, frame.source_sequence);
            }
            Propagation::Stop
        });
        let weak = Rc::downgrade(self);
        self.gl_area.connect_resize(move |_, width, height| {
            if let Some(ui) = weak.upgrade() {
                ui.controller.borrow_mut().set_viewport(width, height);
            }
        });
        self.connect_pointer_input();
        self.connect_keyboard_input();
    }

    fn connect_pointer_input(self: &Rc<Self>) {
        let motion = gtk::EventControllerMotion::new();
        let weak = Rc::downgrade(self);
        motion.connect_motion(move |controller, x, y| {
            let Some(ui) = weak.upgrade() else {
                return;
            };
            let Some(widget) = controller.widget() else {
                return;
            };
            let (native, generation) = {
                let client = ui.controller.borrow();
                (
                    client.native_pointer,
                    ui.video.borrow().surface_generation(),
                )
            };
            let mut input = ui.input.borrow_mut();
            input.configure(native, generation);
            match input.motion(
                x,
                y,
                f64::from(widget.allocated_width()),
                f64::from(widget.allocated_height()),
                generation,
                current_modifiers(controller),
            ) {
                Ok(Some(command)) => ui.command(command),
                Ok(None) => {}
                Err(error) => ui.controller.borrow_mut().report_renderer_error(error),
            }
        });
        self.gl_area.add_controller(motion);

        let click = gtk::GestureClick::new();
        click.set_button(0);
        let weak = Rc::downgrade(self);
        click.connect_pressed(move |gesture, presses, x, y| {
            if let Some(ui) = weak.upgrade() {
                ui.gl_area.grab_focus();
                ui.send_pointer_button(gesture, true, presses, x, y);
            }
        });
        let weak = Rc::downgrade(self);
        click.connect_released(move |gesture, presses, x, y| {
            if let Some(ui) = weak.upgrade() {
                ui.send_pointer_button(gesture, false, presses, x, y);
            }
        });
        self.gl_area.add_controller(click);

        let scroll = gtk::EventControllerScroll::new(
            gtk::EventControllerScrollFlags::BOTH_AXES | gtk::EventControllerScrollFlags::KINETIC,
        );
        let weak = Rc::downgrade(self);
        scroll.connect_scroll(move |controller, dx, dy| {
            let Some(ui) = weak.upgrade() else {
                return Propagation::Proceed;
            };
            let Some(widget) = controller.widget() else {
                return Propagation::Proceed;
            };
            let native = ui.controller.borrow().native_pointer;
            let generation = ui.video.borrow().surface_generation();
            let mut input = ui.input.borrow_mut();
            input.configure(native, generation);
            match input.scroll(
                dx,
                dy,
                f64::from(widget.allocated_width()),
                f64::from(widget.allocated_height()),
                generation,
                current_modifiers(controller),
            ) {
                Ok(commands) => {
                    drop(input);
                    for command in commands {
                        ui.command(command);
                    }
                    Propagation::Stop
                }
                Err(error) => {
                    drop(input);
                    ui.controller.borrow_mut().report_renderer_error(error);
                    Propagation::Proceed
                }
            }
        });
        self.gl_area.add_controller(scroll);
    }

    fn send_pointer_button(
        &self,
        gesture: &gtk::GestureClick,
        pressed: bool,
        presses: i32,
        x: f64,
        y: f64,
    ) {
        let native = self.controller.borrow().native_pointer;
        let generation = self.video.borrow().surface_generation();
        let mut input = self.input.borrow_mut();
        input.configure(native, generation);
        let result = input.button(
            pressed,
            gesture.current_button(),
            presses,
            x,
            y,
            f64::from(self.gl_area.allocated_width()),
            f64::from(self.gl_area.allocated_height()),
            generation,
            current_modifiers(gesture),
        );
        drop(input);
        match result {
            Ok(Some(command)) => self.command(command),
            Ok(None) => {}
            Err(error) => self.controller.borrow_mut().report_renderer_error(error),
        }
    }

    fn connect_keyboard_input(self: &Rc<Self>) {
        let keys = gtk::EventControllerKey::new();
        let input_method = gtk::IMMulticontext::new();
        keys.set_im_context(Some(&input_method));
        let weak = Rc::downgrade(self);
        input_method.connect_commit(move |_, text| {
            if let Some(ui) = weak.upgrade() {
                ui.send_composition("commit", text, 0, 0);
            }
        });
        let weak = Rc::downgrade(self);
        input_method.connect_preedit_changed(move |context| {
            let Some(ui) = weak.upgrade() else {
                return;
            };
            let (text, _, cursor) = context.preedit_string();
            if text.is_empty() {
                ui.send_composition("cancel", "", 0, 0);
            } else {
                let offset = text
                    .chars()
                    .take(usize::try_from(cursor).unwrap_or_default())
                    .map(char::len_utf16)
                    .sum::<usize>();
                let offset = i32::try_from(offset).unwrap_or(i32::MAX);
                ui.send_composition("update", &text, offset, offset);
            }
        });
        let weak = Rc::downgrade(self);
        keys.connect_key_pressed(move |_, key, keycode, state| {
            let Some(ui) = weak.upgrade() else {
                return Propagation::Proceed;
            };
            ui.send_page_key(key, keycode, state, true)
        });
        let weak = Rc::downgrade(self);
        keys.connect_key_released(move |_, key, keycode, state| {
            if let Some(ui) = weak.upgrade() {
                let _ = ui.send_page_key(key, keycode, state, false);
            }
        });
        self.gl_area.add_controller(keys);
    }

    fn send_composition(&self, phase: &str, text: &str, start: i32, end: i32) {
        let generation = self.video.borrow().surface_generation();
        let native = self.controller.borrow().native_pointer;
        let mut input = self.input.borrow_mut();
        input.configure(native, generation);
        let command = input.compose(phase, text.to_owned(), start, end);
        drop(input);
        match command {
            Ok(command) => self.command(command),
            Err(error) => self.controller.borrow_mut().report_renderer_error(error),
        }
    }

    fn paste_into_page(self: &Rc<Self>) {
        let Some(display) = gdk::Display::default() else {
            return;
        };
        let weak = Rc::downgrade(self);
        display
            .clipboard()
            .read_text_async(gtk::gio::Cancellable::NONE, move |result| {
                let Some(ui) = weak.upgrade() else {
                    return;
                };
                let Ok(Some(text)) = result else {
                    return;
                };
                let generation = ui.video.borrow().surface_generation();
                let native = ui.controller.borrow().native_pointer;
                let mut input = ui.input.borrow_mut();
                input.configure(native, generation);
                let command = input.paste(text.to_string());
                drop(input);
                match command {
                    Ok(command) => ui.command(command),
                    Err(error) => ui.controller.borrow_mut().report_renderer_error(error),
                }
            });
    }

    fn send_page_key(
        &self,
        key: gdk::Key,
        keycode: u32,
        state: gdk::ModifierType,
        pressed: bool,
    ) -> Propagation {
        let generation = self.video.borrow().surface_generation();
        let native = self.controller.borrow().native_pointer;
        let mut input = self.input.borrow_mut();
        input.configure(native, generation);
        let result = input.key(key, keycode, state, pressed);
        drop(input);
        match result {
            Ok(Some(command)) => {
                self.command(command);
                Propagation::Stop
            }
            Ok(None) => Propagation::Proceed,
            Err(error) => {
                self.controller.borrow_mut().report_renderer_error(error);
                Propagation::Proceed
            }
        }
    }

    fn connect_shortcuts(self: &Rc<Self>) {
        let keys = gtk::EventControllerKey::new();
        keys.set_propagation_phase(gtk::PropagationPhase::Capture);
        let weak = Rc::downgrade(self);
        keys.connect_key_pressed(move |_, key, _, state| {
            let Some(ui) = weak.upgrade() else {
                return Propagation::Proceed;
            };
            let command = state.contains(gdk::ModifierType::CONTROL_MASK);
            let alt = state.contains(gdk::ModifierType::ALT_MASK);
            let name = key.name().map_or_else(String::new, |name| name.to_string());
            match (command, alt, name.as_str()) {
                (true, _, "v" | "V") if ui.gl_area.has_focus() => {
                    ui.paste_into_page();
                    Propagation::Stop
                }
                (true, _, "l" | "L") => {
                    ui.address_entry.grab_focus();
                    ui.address_entry.select_region(0, -1);
                    Propagation::Stop
                }
                (true, _, "t" | "T") => {
                    ui.command(Command::Tab {
                        action: "new".to_owned(),
                        id: 0,
                        causal: Causal::default(),
                    });
                    Propagation::Stop
                }
                (true, _, "w" | "W") => {
                    if let Some(id) = ui.controller.borrow().snapshot.active_tab_id {
                        ui.command(Command::Tab {
                            action: "close".to_owned(),
                            id: i32::try_from(id).unwrap_or_default(),
                            causal: Causal::default(),
                        });
                    }
                    Propagation::Stop
                }
                (true, _, "r" | "R") | (false, false, "F5") => {
                    let loading = ui.controller.borrow().snapshot.loading;
                    ui.command(if loading {
                        Command::Stop {
                            causal: Causal::default(),
                        }
                    } else {
                        Command::Reload {
                            causal: Causal::default(),
                        }
                    });
                    Propagation::Stop
                }
                (true, _, "d" | "D") => {
                    ui.command(Command::Bookmark {
                        causal: Causal::default(),
                    });
                    Propagation::Stop
                }
                (true, _, "f" | "F") => {
                    ui.show_find();
                    Propagation::Stop
                }
                (false, true, "Left") => {
                    ui.command(Command::Back {
                        causal: Causal::default(),
                    });
                    Propagation::Stop
                }
                (false, true, "Right") => {
                    ui.command(Command::Forward {
                        causal: Causal::default(),
                    });
                    Propagation::Stop
                }
                _ => Propagation::Proceed,
            }
        });
        self.window.add_controller(keys);
    }

    fn command(&self, command: Command) {
        self.controller.borrow_mut().command(command);
    }

    fn tick(self: &Rc<Self>) {
        let diagnostics = self.video.borrow_mut().diagnostics();
        self.controller
            .borrow_mut()
            .set_render_diagnostics(diagnostics);
        let tick = self.controller.borrow_mut().tick();
        for effect in tick.effects {
            match effect {
                HostEffect::ClearVideo => {
                    self.video.borrow_mut().clear();
                    self.input.borrow_mut().reset();
                    self.gl_area.queue_render();
                }
                HostEffect::ClearPagePresentation => self.clear_page_surfaces(),
                HostEffect::SetClipboard { request_id, text } => {
                    let ok = if let Some(display) = gdk::Display::default() {
                        display.clipboard().set_text(&text);
                        true
                    } else {
                        false
                    };
                    self.controller
                        .borrow_mut()
                        .complete_clipboard(request_id, ok);
                }
            }
        }
        if let Some(frame) = tick.frame {
            self.video.borrow_mut().submit(frame);
            self.gl_area.queue_render();
        }
        if let Some(error) = self.video.borrow_mut().take_error() {
            self.controller.borrow_mut().report_renderer_error(error);
        }
        self.update_view();
        self.handle_smoke();
    }

    fn handle_smoke(&self) {
        let presented = self.video.borrow_mut().diagnostics().presented;
        let mut smoke = self.smoke.borrow_mut();
        if smoke.target.is_none() {
            return;
        }
        if smoke.last_heartbeat.elapsed() >= Duration::from_secs(2) {
            smoke.last_heartbeat = Instant::now();
            let client = self.controller.borrow();
            let media = client.media_diagnostics();
            eprintln!(
                "SURF_SMOKE_HEARTBEAT presented={presented} decoded={} ingress={} encoded_depth={} decoded_depth={} connected={}",
                media.decoded_frames,
                media.ingress_frames,
                media.encoded_video_depth,
                media.decoded_video_depth,
                client.connected,
            );
        }
        if presented > 0 && smoke.started.is_none() {
            smoke.started = Some(Instant::now());
            eprintln!("SURF_SMOKE_STEP first-frame presented={presented}");
        }
        if smoke.interaction {
            if smoke.interaction_step == 0 && presented >= 45 {
                smoke.interaction_step = 1;
                eprintln!("SURF_SMOKE_STEP resize-small presented={presented}");
                self.window.set_default_size(940, 680);
            } else if smoke.interaction_step == 1 && presented >= 90 {
                smoke.interaction_step = 2;
                eprintln!("SURF_SMOKE_STEP edit-omnibox presented={presented}");
                self.address_entry.grab_focus();
                self.address_entry
                    .set_text("Editing the omnibox while video remains live");
                self.address_entry.select_region(0, -1);
            } else if smoke.interaction_step == 2 && presented >= 135 {
                smoke.interaction_step = 3;
                eprintln!("SURF_SMOKE_STEP resize-large presented={presented}");
                self.window.set_default_size(1260, 800);
                self.gl_area.grab_focus();
            }
        }
        if !smoke.stalled
            && presented >= 165
            && let Some(stall_ms) = smoke.stall_ms
        {
            smoke.stalled = true;
            eprintln!("SURF_SMOKE_STEP ui-stall presented={presented} ms={stall_ms}");
            std::thread::sleep(Duration::from_millis(stall_ms));
        }
        if smoke.reported || !smoke.target.is_some_and(|target| presented >= target) {
            return;
        }
        smoke.reported = true;
        let elapsed = smoke
            .started
            .map(|started| started.elapsed())
            .unwrap_or(Duration::ZERO);
        drop(smoke);
        let intervals = presented.saturating_sub(1);
        let fps = if elapsed.is_zero() {
            0.0
        } else {
            intervals as f64 / elapsed.as_secs_f64()
        };
        let surface = self.video.borrow_mut().diagnostics();
        let client = self.controller.borrow();
        let media = client.media_diagnostics();
        let diagnostics = client.latest_diagnostics.unwrap_or_default();
        println!(
            "SURF_SMOKE_RESULT presented={presented} elapsed_ms={} fps={fps:.2} decoded={} ingress_replaced={} output_replaced={} presentation_replaced={} gaps={} decode_errors={} encoded_depth={} decoded_depth={} upload_us={} rtt_us={} network_us={} clock_uncertainty_us={} frame_age_us={} timing_synchronized={} health={:?}",
            elapsed.as_millis(),
            media.decoded_frames,
            media.ingress_replaced,
            media.output_replaced,
            surface.replaced,
            media.gaps,
            media.decode_errors,
            media.encoded_video_depth,
            media.decoded_video_depth,
            surface.latest_upload_us,
            diagnostics.rtt_us,
            diagnostics.network_us,
            diagnostics.clock_uncertainty_us,
            diagnostics.frame_age_us,
            diagnostics.timing_synchronized,
            diagnostics.health,
        );
        drop(client);
        self.window.close();
    }

    fn update_view(self: &Rc<Self>) {
        let client = self.controller.borrow();
        let connected = client.connected;
        self.view_stack
            .set_visible_child_name(if connected { "browser" } else { "start" });
        self.start_status.set_text(&client.status);
        self.status_overlay
            .set_text(if connected && client.frames_received == 0 {
                "Waiting for the first frame…"
            } else {
                ""
            });
        self.status_overlay
            .set_visible(connected && client.frames_received == 0);
        self.back_button.set_sensitive(client.snapshot.can_go_back);
        self.forward_button
            .set_sensitive(client.snapshot.can_go_forward);
        self.reload_button.set_sensitive(connected);
        self.reload_image
            .set_icon_name(Some(if client.snapshot.loading {
                "process-stop-symbolic"
            } else {
                "view-refresh-symbolic"
            }));
        self.loading
            .set_opacity(if client.snapshot.loading { 1.0 } else { 0.0 });
        if client.snapshot.loading {
            self.loading.pulse();
        }
        self.star_button.set_icon_name(if client.snapshot.starred {
            "starred-symbolic"
        } else {
            "non-starred-symbolic"
        });
        let title = client.snapshot.active_title.trim();
        let window_title = if title.is_empty() {
            "Surf".to_owned()
        } else {
            format!("{title} — Surf")
        };
        self.window.set_title(Some(&window_title));
        if !self.address_editing.get() {
            let compact = compact_address(&client.snapshot.current_url);
            if self.address_entry.text().as_str() != compact {
                self.address_entry.set_text(&compact);
            }
        }
        self.pairing_box.set_visible(client.inspected.is_some());
        self.pair_button
            .set_visible(client.inspected.is_some() && !client.paired && client.pairing.is_none());
        self.pairing_code
            .set_visible(client.inspected.is_some() && !client.paired && client.pairing.is_none());
        self.confirm_button.set_visible(client.pairing.is_some());
        self.pairing_phrase.set_visible(client.pairing.is_some());
        self.pairing_phrase.set_text(
            &client
                .pairing
                .as_ref()
                .map_or_else(String::new, |pairing| pairing.phrase.clone()),
        );
        self.connect_button.set_visible(client.paired);
        self.toast.set_visible(client.browser.toast.is_some());
        self.toast.set_text(
            &client
                .browser
                .toast
                .as_ref()
                .map_or_else(String::new, |toast| toast.text.clone()),
        );
        let tab_revision = client.snapshot.revision;
        let mut servers: Vec<_> = client
            .saved_servers
            .iter()
            .map(|server| (server.name.clone(), server.endpoint.clone()))
            .collect();
        servers.extend(
            client
                .discovered_servers
                .iter()
                .filter(|discovered| {
                    !client.saved_servers.iter().any(|saved| {
                        (!discovered.server_id.is_empty()
                            && discovered.server_id == saved.server_id)
                            || discovered.endpoint == saved.endpoint
                    })
                })
                .map(|server| (server.name.clone(), server.endpoint.clone())),
        );
        let suggestions: Vec<_> = client
            .browser
            .suggestions
            .iter()
            .map(|item| (item.title.clone(), item.url.clone()))
            .collect();
        drop(client);

        if self.last_tab_revision.replace(tab_revision) != tab_revision {
            self.rebuild_tabs();
        }
        if *self.last_servers.borrow() != servers {
            *self.last_servers.borrow_mut() = servers;
            self.rebuild_servers();
        }
        if *self.last_suggestions.borrow() != suggestions {
            *self.last_suggestions.borrow_mut() = suggestions;
            self.rebuild_suggestions();
        }
        if connected && !self.was_connected.replace(true) {
            self.controller.borrow_mut().set_viewport(
                self.gl_area.allocated_width(),
                self.gl_area.allocated_height(),
            );
            self.gl_area.grab_focus();
        } else if !connected {
            self.was_connected.set(false);
        }
        self.reconcile_semantic_surfaces();
        self.refresh_auxiliary_windows();
    }

    fn rebuild_tabs(self: &Rc<Self>) {
        clear_box(&self.tab_box);
        let tabs = self.controller.borrow().snapshot.tabs.clone();
        for tab in tabs {
            let shell = gtk::Box::new(gtk::Orientation::Horizontal, 0);
            shell.add_css_class("surf-tab");
            if tab.active {
                shell.add_css_class("active");
            }
            let title = if tab.title.trim().is_empty() {
                compact_address(&tab.url)
            } else {
                tab.title.clone()
            };
            let select = gtk::Button::with_label(&title);
            select.set_tooltip_text(Some(&tab.url));
            select.set_size_request(150, 28);
            select.add_css_class("flat");
            let close = icon_button("window-close-symbolic", "Close tab (Ctrl+W)");
            let weak = Rc::downgrade(self);
            let id = tab.id;
            select.connect_clicked(move |_| {
                if let Some(ui) = weak.upgrade() {
                    ui.command(Command::Tab {
                        action: "select".to_owned(),
                        id: i32::try_from(id).unwrap_or_default(),
                        causal: Causal::default(),
                    });
                }
            });
            let weak = Rc::downgrade(self);
            let id = tab.id;
            close.connect_clicked(move |_| {
                if let Some(ui) = weak.upgrade() {
                    ui.command(Command::Tab {
                        action: "close".to_owned(),
                        id: i32::try_from(id).unwrap_or_default(),
                        causal: Causal::default(),
                    });
                }
            });
            shell.append(&select);
            shell.append(&close);
            self.tab_box.append(&shell);
        }
    }

    fn rebuild_servers(self: &Rc<Self>) {
        clear_box(&self.start_servers);
        let servers = self.last_servers.borrow().clone();
        if servers.is_empty() {
            return;
        }
        let section = gtk::Label::builder()
            .label("AVAILABLE COMPUTERS")
            .xalign(0.0)
            .build();
        section.add_css_class("surf-section");
        self.start_servers.append(&section);
        for (name, endpoint) in servers {
            let button = gtk::Button::new();
            button.add_css_class("surf-server-row");
            let row = gtk::Box::new(gtk::Orientation::Horizontal, 10);
            let labels = gtk::Box::new(gtk::Orientation::Vertical, 1);
            labels.set_hexpand(true);
            labels.append(&gtk::Label::builder().label(&name).xalign(0.0).build());
            labels.append(
                &gtk::Label::builder()
                    .label(endpoint.trim_start_matches("https://"))
                    .xalign(0.0)
                    .css_classes(["surf-status"])
                    .build(),
            );
            row.append(&labels);
            row.append(&gtk::Image::from_icon_name("go-next-symbolic"));
            button.set_child(Some(&row));
            let weak = Rc::downgrade(self);
            button.connect_clicked(move |_| {
                if let Some(ui) = weak.upgrade() {
                    ui.controller.borrow_mut().inspect(endpoint.clone(), true);
                }
            });
            self.start_servers.append(&button);
        }
    }

    fn rebuild_suggestions(self: &Rc<Self>) {
        clear_box(&self.suggestions_box);
        let items = self.last_suggestions.borrow().clone();
        for (title, url) in items.into_iter().take(8) {
            let label = if title.trim().is_empty() {
                compact_address(&url)
            } else {
                title
            };
            let button = gtk::Button::with_label(&label);
            button.set_tooltip_text(Some(&url));
            button.set_halign(gtk::Align::Fill);
            let weak = Rc::downgrade(self);
            button.connect_clicked(move |_| {
                if let Some(ui) = weak.upgrade() {
                    ui.address_entry.set_text(&url);
                    ui.controller.borrow_mut().navigate(&url);
                    ui.suggestions.popdown();
                    ui.gl_area.grab_focus();
                }
            });
            self.suggestions_box.append(&button);
        }
        if self.address_editing.get() && self.suggestions_box.first_child().is_some() {
            self.suggestions.popup();
        } else {
            self.suggestions.popdown();
        }
    }

    fn reconcile_semantic_surfaces(self: &Rc<Self>) {
        let dialog = self.controller.borrow().browser.dialog.clone();
        if let Some(dialog) = dialog {
            self.show_page_dialog(dialog);
        } else if let Some(window) = self.page_dialog.borrow_mut().take() {
            window.close();
        }
        let select = self.controller.borrow().browser.select.clone();
        if let Some(select) = select {
            self.show_page_select(select);
        } else if let Some(popover) = self.page_select.borrow_mut().take() {
            popover.popdown();
        }
        let upload = self.controller.borrow().browser.upload_multiple;
        if let Some(multiple) = upload {
            self.show_file_chooser(multiple);
        }
        let reader = self.controller.borrow().browser.reader.clone();
        if let Some(reader) = reader {
            self.show_reader(reader);
        } else if let Some(window) = self.reader_window.borrow_mut().take() {
            window.close();
        }
        let error = self.controller.borrow().browser.page_error.clone();
        if let Some(url) = error {
            self.show_page_error(&url);
        } else if let Some(dialog) = self.page_error.borrow_mut().take() {
            dialog.close();
        }
    }

    fn clear_page_surfaces(&self) {
        self.suggestions.popdown();
        if let Some(dialog) = self.page_dialog.borrow_mut().take() {
            dialog.close();
        }
        if let Some(popover) = self.page_select.borrow_mut().take() {
            popover.popdown();
        }
        if let Some(popover) = self.find_popover.borrow_mut().take() {
            popover.popdown();
        }
        if let Some(window) = self.reader_window.borrow_mut().take() {
            window.close();
        }
        if let Some(dialog) = self.page_error.borrow_mut().take() {
            dialog.close();
        }
        if let Some(chooser) = self.file_chooser.borrow_mut().take() {
            chooser.destroy();
        }
    }

    fn show_library(self: &Rc<Self>) {
        if let Some(view) = self.library_view.borrow().as_ref() {
            view.window.present();
            return;
        }
        let window = gtk::Window::builder()
            .transient_for(&self.window)
            .title("Library")
            .default_width(620)
            .default_height(520)
            .build();
        let content = gtk::Box::new(gtk::Orientation::Vertical, 10);
        content.add_css_class("surf-dialog-content");
        let heading = gtk::Label::builder().label("Library").xalign(0.0).build();
        heading.add_css_class("surf-dialog-heading");
        content.append(&heading);

        let stack = gtk::Stack::builder()
            .transition_type(gtk::StackTransitionType::Crossfade)
            .hexpand(true)
            .vexpand(true)
            .build();
        let switcher = gtk::StackSwitcher::builder()
            .stack(&stack)
            .halign(gtk::Align::Start)
            .build();
        content.append(&switcher);

        let history = gtk::Box::new(gtk::Orientation::Vertical, 0);
        let bookmarks = gtk::Box::new(gtk::Orientation::Vertical, 0);
        let downloads = gtk::Box::new(gtk::Orientation::Vertical, 0);
        for (name, title, list) in [
            ("history", "History", &history),
            ("bookmarks", "Bookmarks", &bookmarks),
            ("downloads", "Downloads", &downloads),
        ] {
            let scroll = gtk::ScrolledWindow::builder()
                .hscrollbar_policy(gtk::PolicyType::Never)
                .vexpand(true)
                .child(list)
                .build();
            stack.add_titled(&scroll, Some(name), title);
        }
        content.append(&stack);
        window.set_child(Some(&content));

        let weak = Rc::downgrade(self);
        window.connect_close_request(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.library_view.borrow_mut().take();
            }
            Propagation::Proceed
        });
        self.library_view.replace(Some(LibraryView {
            window: window.clone(),
            history,
            bookmarks,
            downloads,
            signature: RefCell::new(String::new()),
        }));
        self.refresh_library();
        window.present();
    }

    fn refresh_library(self: &Rc<Self>) {
        let (history, bookmarks, downloads, progress) = {
            let client = self.controller.borrow();
            (
                client.browser.history.clone(),
                client.browser.bookmarks.clone(),
                client.browser.downloads.clone(),
                client.browser.download_progress.clone(),
            )
        };
        let signature = format!("{history:?}|{bookmarks:?}|{downloads:?}|{progress:?}");
        let Some((history_box, bookmarks_box, downloads_box)) =
            self.library_view.borrow().as_ref().and_then(|view| {
                if *view.signature.borrow() == signature {
                    None
                } else {
                    view.signature.replace(signature);
                    Some((
                        view.history.clone(),
                        view.bookmarks.clone(),
                        view.downloads.clone(),
                    ))
                }
            })
        else {
            return;
        };

        clear_box(&history_box);
        clear_box(&bookmarks_box);
        clear_box(&downloads_box);
        if history.is_empty() {
            append_empty_state(&history_box, "No browsing history yet.");
        }
        for item in history {
            let row = library_entry_row(&item.title, &item.url, "Remove from history");
            let open = row.1;
            let remove = row.2;
            history_box.append(&row.0);
            let weak = Rc::downgrade(self);
            let url = item.url.clone();
            open.connect_clicked(move |_| {
                if let Some(ui) = weak.upgrade() {
                    ui.controller.borrow_mut().navigate(&url);
                }
            });
            let weak = Rc::downgrade(self);
            remove.connect_clicked(move |_| {
                if let Some(ui) = weak.upgrade() {
                    ui.command(Command::HistoryDelete {
                        url: item.url.clone(),
                        ts: item.ts,
                        causal: Causal::default(),
                    });
                    ui.command(Command::Library {
                        causal: Causal::default(),
                    });
                }
            });
        }
        if bookmarks.is_empty() {
            append_empty_state(&bookmarks_box, "Pages you bookmark appear here.");
        }
        for item in bookmarks {
            let row = library_entry_row(&item.title, &item.url, "Remove bookmark");
            let open = row.1;
            let remove = row.2;
            bookmarks_box.append(&row.0);
            let weak = Rc::downgrade(self);
            let url = item.url.clone();
            open.connect_clicked(move |_| {
                if let Some(ui) = weak.upgrade() {
                    ui.controller.borrow_mut().navigate(&url);
                }
            });
            let weak = Rc::downgrade(self);
            remove.connect_clicked(move |_| {
                if let Some(ui) = weak.upgrade() {
                    ui.command(Command::BookmarkDelete {
                        url: item.url.clone(),
                        causal: Causal::default(),
                    });
                    ui.command(Command::Library {
                        causal: Causal::default(),
                    });
                }
            });
        }
        if downloads.is_empty() {
            append_empty_state(&downloads_box, "Downloads from this server appear here.");
        }
        for item in downloads {
            let row = gtk::Box::new(gtk::Orientation::Horizontal, 8);
            row.add_css_class("surf-library-row");
            let labels = gtk::Box::new(gtk::Orientation::Vertical, 2);
            labels.set_hexpand(true);
            labels.append(
                &gtk::Label::builder()
                    .label(&item.name)
                    .xalign(0.0)
                    .ellipsize(gtk::pango::EllipsizeMode::Middle)
                    .build(),
            );
            let detail = progress.get(&item.name).map_or_else(
                || format_bytes(item.size),
                |percent| format!("{} · {percent}%", format_bytes(item.size)),
            );
            labels.append(
                &gtk::Label::builder()
                    .label(detail)
                    .xalign(0.0)
                    .css_classes(["surf-status"])
                    .build(),
            );
            let save = gtk::Button::with_label("Save");
            save.set_tooltip_text(Some("Save to your Downloads folder"));
            let remove = icon_button("user-trash-symbolic", "Remove download");
            row.append(&labels);
            row.append(&save);
            row.append(&remove);
            downloads_box.append(&row);
            let weak = Rc::downgrade(self);
            let name = item.name.clone();
            save.connect_clicked(move |_| {
                if let Some(ui) = weak.upgrade() {
                    ui.controller
                        .borrow_mut()
                        .request_download(name.clone(), download_destination(&name));
                }
            });
            let weak = Rc::downgrade(self);
            remove.connect_clicked(move |_| {
                if let Some(ui) = weak.upgrade() {
                    ui.command(Command::DownloadDelete {
                        name: item.name.clone(),
                        causal: Causal::default(),
                    });
                    ui.command(Command::Downloads {
                        causal: Causal::default(),
                    });
                }
            });
        }
    }

    fn show_media(self: &Rc<Self>) {
        if let Some(view) = self.media_view.borrow().as_ref() {
            view.window.present();
            return;
        }
        let window = gtk::Window::builder()
            .transient_for(&self.window)
            .title("Media")
            .default_width(380)
            .resizable(false)
            .build();
        let content = gtk::Box::new(gtk::Orientation::Vertical, 12);
        content.add_css_class("surf-dialog-content");
        let title = gtk::Label::builder()
            .xalign(0.0)
            .ellipsize(gtk::pango::EllipsizeMode::End)
            .build();
        title.add_css_class("surf-dialog-heading");
        let time = gtk::Label::builder()
            .xalign(0.0)
            .css_classes(["surf-status", "surf-monospace"])
            .build();
        let unavailable = gtk::Label::builder()
            .label("No controllable media on this page.")
            .xalign(0.0)
            .build();
        let controls = gtk::Box::new(gtk::Orientation::Vertical, 10);
        let buttons = gtk::Box::new(gtk::Orientation::Horizontal, 6);
        let play_pause = gtk::Button::with_label("Play");
        let mute = gtk::Button::with_label("Mute");
        buttons.append(&play_pause);
        buttons.append(&mute);
        let volume = gtk::Scale::with_range(gtk::Orientation::Horizontal, 0.0, 1.0, 0.01);
        volume.set_hexpand(true);
        volume.set_draw_value(false);
        volume.set_tooltip_text(Some("Volume"));
        controls.append(&buttons);
        controls.append(&volume);
        content.append(&title);
        content.append(&time);
        content.append(&unavailable);
        content.append(&controls);
        window.set_child(Some(&content));

        let weak = Rc::downgrade(self);
        play_pause.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.command(Command::MediaPlayPause {
                    causal: Causal::default(),
                });
            }
        });
        let weak = Rc::downgrade(self);
        mute.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.command(Command::MediaMute {
                    causal: Causal::default(),
                });
            }
        });
        let weak = Rc::downgrade(self);
        volume.connect_value_changed(move |scale| {
            let Some(ui) = weak.upgrade() else {
                return;
            };
            let updating = ui
                .media_view
                .borrow()
                .as_ref()
                .is_some_and(|view| view.updating.get());
            if !updating {
                ui.command(Command::MediaVolume {
                    value: scale.value(),
                    causal: Causal::default(),
                });
            }
        });
        let weak = Rc::downgrade(self);
        window.connect_close_request(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.media_view.borrow_mut().take();
            }
            Propagation::Proceed
        });
        self.media_view.replace(Some(MediaView {
            window: window.clone(),
            title,
            time,
            play_pause,
            mute,
            volume,
            unavailable,
            controls,
            updating: Cell::new(false),
        }));
        self.refresh_media();
        window.present();
    }

    fn refresh_media(&self) {
        let media = self.controller.borrow().browser.media.clone();
        let binding = self.media_view.borrow();
        let Some(view) = binding.as_ref() else {
            return;
        };
        view.unavailable.set_visible(!media.available);
        view.controls.set_visible(media.available);
        view.title.set_visible(media.available);
        view.time.set_visible(media.available);
        if !media.available {
            return;
        }
        let media_title = if media.title.is_empty() {
            format!("{} media element(s)", media.count)
        } else {
            media.title
        };
        view.title.set_text(&media_title);
        view.time.set_text(&format!(
            "{} / {}",
            format_time(media.current_time),
            format_time(media.duration)
        ));
        view.play_pause
            .set_label(if media.paused { "Play" } else { "Pause" });
        view.mute
            .set_label(if media.muted { "Unmute" } else { "Mute" });
        view.updating.set(true);
        view.volume.set_value(media.volume.clamp(0.0, 1.0));
        view.updating.set(false);
    }

    fn refresh_auxiliary_windows(self: &Rc<Self>) {
        self.refresh_library();
        self.refresh_media();
        self.refresh_performance();
    }

    fn show_find(self: &Rc<Self>) {
        if let Some(popover) = self.find_popover.borrow().as_ref() {
            popover.popup();
            return;
        }
        let popover = gtk::Popover::new();
        popover.set_parent(&self.gl_area);
        popover.set_halign(gtk::Align::End);
        popover.set_valign(gtk::Align::Start);
        let row = gtk::Box::new(gtk::Orientation::Horizontal, 4);
        row.add_css_class("surf-popover");
        let entry = gtk::Entry::builder()
            .placeholder_text("Find on page")
            .build();
        let previous = icon_button("go-up-symbolic", "Previous match");
        let next = icon_button("go-down-symbolic", "Next match");
        row.append(&entry);
        row.append(&previous);
        row.append(&next);
        popover.set_child(Some(&row));
        let weak = Rc::downgrade(self);
        entry.connect_changed(move |entry| {
            if let Some(ui) = weak.upgrade() {
                ui.command(Command::Find {
                    q: entry.text().to_string(),
                    dir: 1,
                    causal: Causal::default(),
                });
            }
        });
        let weak = Rc::downgrade(self);
        let entry_previous = entry.clone();
        previous.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.command(Command::Find {
                    q: entry_previous.text().to_string(),
                    dir: -1,
                    causal: Causal::default(),
                });
            }
        });
        let weak = Rc::downgrade(self);
        let entry_next = entry.clone();
        next.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.command(Command::Find {
                    q: entry_next.text().to_string(),
                    dir: 1,
                    causal: Causal::default(),
                });
            }
        });
        let weak = Rc::downgrade(self);
        popover.connect_closed(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.find_popover.borrow_mut().take();
            }
        });
        self.find_popover.replace(Some(popover.clone()));
        popover.popup();
        entry.grab_focus();
    }

    fn show_settings(self: &Rc<Self>) {
        if let Some(window) = self.settings_window.borrow().as_ref() {
            window.present();
            return;
        }
        let dialog = gtk::Window::builder()
            .transient_for(&self.window)
            .modal(true)
            .title("Settings")
            .default_width(500)
            .resizable(false)
            .build();
        let content = gtk::Box::new(gtk::Orientation::Vertical, 14);
        content.add_css_class("surf-dialog-content");
        let heading = gtk::Label::builder().label("Settings").xalign(0.0).build();
        heading.add_css_class("surf-dialog-heading");
        content.append(&heading);
        content.append(&section_label("APPEARANCE"));
        let dark = gtk::Switch::builder()
            .active(self.controller.borrow().dark_mode)
            .build();
        content.append(&setting_row(
            "Dark appearance",
            "Apply the same preference to Surf and websites.",
            &dark,
        ));
        let mobile = gtk::Switch::builder()
            .active(self.controller.borrow().mobile_mode)
            .build();
        content.append(&setting_row(
            "Mobile websites",
            "Request compact layouts from Chromium.",
            &mobile,
        ));
        content.append(&gtk::Separator::new(gtk::Orientation::Horizontal));
        content.append(&section_label("PRIVACY"));
        let clear_history = gtk::Button::with_label("Clear browsing history");
        clear_history.set_halign(gtk::Align::Fill);
        content.append(&clear_history);
        content.append(&gtk::Separator::new(gtk::Orientation::Horizontal));
        content.append(&section_label("CONNECTION"));
        let connection = gtk::Label::builder()
            .label(&self.controller.borrow().status)
            .wrap(true)
            .xalign(0.0)
            .css_classes(["surf-status"])
            .build();
        content.append(&connection);
        for server in self.controller.borrow().saved_servers.clone() {
            let row = gtk::Box::new(gtk::Orientation::Horizontal, 8);
            let labels = gtk::Box::new(gtk::Orientation::Vertical, 1);
            labels.set_hexpand(true);
            labels.append(
                &gtk::Label::builder()
                    .label(&server.name)
                    .xalign(0.0)
                    .build(),
            );
            labels.append(
                &gtk::Label::builder()
                    .label(server.endpoint.trim_start_matches("https://"))
                    .xalign(0.0)
                    .css_classes(["surf-status"])
                    .build(),
            );
            let forget = gtk::Button::with_label("Forget");
            row.append(&labels);
            row.append(&forget);
            content.append(&row);
            let weak = Rc::downgrade(self);
            let dialog_weak = dialog.downgrade();
            forget.connect_clicked(move |_| {
                if let (Some(ui), Some(parent)) = (weak.upgrade(), dialog_weak.upgrade()) {
                    ui.confirm_forget_server(&parent, server.clone());
                }
            });
        }
        let disconnect = gtk::Button::with_label("Disconnect");
        content.append(&disconnect);
        content.append(
            &gtk::Label::builder()
                .label(format!(
                    "Surf {} · portable core",
                    include_str!("../../../../../VERSION").trim()
                ))
                .xalign(0.0)
                .css_classes(["surf-status"])
                .build(),
        );
        dialog.set_child(Some(&content));
        let weak = Rc::downgrade(self);
        dark.connect_active_notify(move |toggle| {
            if let Some(ui) = weak.upgrade() {
                ui.controller.borrow_mut().set_dark_mode(toggle.is_active());
                if let Some(settings) = gtk::Settings::default() {
                    settings.set_gtk_application_prefer_dark_theme(toggle.is_active());
                }
            }
        });
        let weak = Rc::downgrade(self);
        mobile.connect_active_notify(move |toggle| {
            if let Some(ui) = weak.upgrade() {
                ui.controller
                    .borrow_mut()
                    .set_mobile_mode(toggle.is_active());
            }
        });
        let weak = Rc::downgrade(self);
        clear_history.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.command(Command::Clear {
                    what: "history".to_owned(),
                    causal: Causal::default(),
                });
                ui.controller
                    .borrow_mut()
                    .browser
                    .toast("Browsing history cleared");
            }
        });
        let weak = Rc::downgrade(self);
        disconnect.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.controller.borrow_mut().disconnect();
            }
        });
        let weak = Rc::downgrade(self);
        dialog.connect_close_request(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.settings_window.borrow_mut().take();
            }
            Propagation::Proceed
        });
        self.settings_window.replace(Some(dialog.clone()));
        dialog.present();
    }

    fn confirm_forget_server(
        self: &Rc<Self>,
        parent: &gtk::Window,
        server: surf_session::SavedServer,
    ) {
        let dialog = gtk::Dialog::builder()
            .transient_for(parent)
            .modal(true)
            .title("Forget server?")
            .build();
        dialog.add_button("Cancel", gtk::ResponseType::Cancel);
        dialog.add_button("Forget", gtk::ResponseType::Accept);
        let content = dialog.content_area();
        content.add_css_class("surf-dialog-content");
        content.append(
            &gtk::Label::builder()
                .label(format!(
                    "Surf will remove {} and this computer's pairing key for it.",
                    server.name
                ))
                .wrap(true)
                .xalign(0.0)
                .build(),
        );
        let weak = Rc::downgrade(self);
        dialog.connect_response(move |dialog, response| {
            if response == gtk::ResponseType::Accept
                && let Some(ui) = weak.upgrade()
            {
                match ui.controller.borrow_mut().forget_server(&server.server_id) {
                    Ok(()) => ui
                        .controller
                        .borrow_mut()
                        .browser
                        .toast(format!("Forgot {}", server.name)),
                    Err(error) => ui.controller.borrow_mut().browser.toast(error),
                }
            }
            dialog.close();
        });
        dialog.present();
    }

    fn show_performance(self: &Rc<Self>) {
        if let Some(view) = self.performance_view.borrow().as_ref() {
            view.window.present();
            return;
        }
        let window = gtk::Window::builder()
            .transient_for(&self.window)
            .title("Performance")
            .default_width(430)
            .resizable(false)
            .build();
        let content = gtk::Box::new(gtk::Orientation::Vertical, 12);
        content.add_css_class("surf-dialog-content");
        let heading = gtk::Label::builder()
            .label("Streaming performance")
            .xalign(0.0)
            .build();
        heading.add_css_class("surf-dialog-heading");
        content.append(&heading);
        let metrics = gtk::Box::new(gtk::Orientation::Horizontal, 8);
        let (presented_card, presented) = metric_card("Presented");
        let (decoded_card, decoded) = metric_card("Decoded");
        let (dropped_card, dropped) = metric_card("Dropped");
        metrics.append(&presented_card);
        metrics.append(&decoded_card);
        metrics.append(&dropped_card);
        content.append(&metrics);
        let details = gtk::Label::builder()
            .xalign(0.0)
            .selectable(true)
            .css_classes(["surf-monospace"])
            .build();
        content.append(&details);
        let health = gtk::Label::builder()
            .xalign(0.0)
            .wrap(true)
            .css_classes(["surf-status"])
            .build();
        content.append(&health);
        window.set_child(Some(&content));
        let weak = Rc::downgrade(self);
        window.connect_close_request(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.performance_view.borrow_mut().take();
            }
            Propagation::Proceed
        });
        self.performance_view.replace(Some(PerformanceView {
            window: window.clone(),
            presented,
            decoded,
            dropped,
            details,
            health,
        }));
        self.refresh_performance();
        window.present();
    }

    fn refresh_performance(&self) {
        let (report, media) = {
            let client = self.controller.borrow();
            (
                client.latest_diagnostics.unwrap_or_default(),
                client.media_diagnostics(),
            )
        };
        let binding = self.performance_view.borrow();
        let Some(view) = binding.as_ref() else {
            return;
        };
        view.presented
            .set_text(&format!("{:.1} fps", report.presentation_fps));
        view.decoded
            .set_text(&format!("{:.1} fps", report.decode_fps));
        view.dropped
            .set_text(&format!("{:.1}%", report.drop_percent));
        view.details.set_text(&format!(
            "Decode       {:>8} µs\nGPU upload   {:>8} µs\nFrame age    {:>8} µs\nNetwork      {:>8} µs\nRound trip   {:>8} µs\nQueues        {} / {} / {}",
            report.decode_us,
            report.upload_us,
            report.frame_age_us,
            report.network_us,
            report.rtt_us,
            report.encoded_video_depth,
            report.decoded_video_depth,
            report.audio_depth,
        ));
        view.health.set_text(&format!(
            "{:?} · {} sequence gaps · {} decode errors · {} frames received",
            report.health, report.sequence_gaps, report.decode_errors, media.ingress_frames
        ));
    }

    fn show_page_dialog(self: &Rc<Self>, prompt: surf_client_app::DialogPrompt) {
        if self.page_dialog.borrow().is_some() {
            return;
        }
        let dialog = gtk::Dialog::builder()
            .transient_for(&self.window)
            .modal(true)
            .title("This page says")
            .build();
        if prompt.kind != "alert" {
            dialog.add_button("Cancel", gtk::ResponseType::Cancel);
        }
        dialog.add_button("OK", gtk::ResponseType::Ok);
        let content = dialog.content_area();
        content.set_spacing(10);
        content.set_margin_top(14);
        content.set_margin_bottom(14);
        content.set_margin_start(14);
        content.set_margin_end(14);
        content.append(
            &gtk::Label::builder()
                .label(&prompt.text)
                .wrap(true)
                .xalign(0.0)
                .build(),
        );
        let input = gtk::Entry::new();
        input.set_text(&prompt.input);
        input.set_visible(prompt.kind == "prompt");
        content.append(&input);
        let weak = Rc::downgrade(self);
        let response_input = input.clone();
        dialog.connect_response(move |dialog, response| {
            if let Some(ui) = weak.upgrade()
                && ui.page_dialog.borrow_mut().take().is_some()
            {
                ui.controller.borrow_mut().reply_dialog(
                    response == gtk::ResponseType::Ok,
                    response_input.text().to_string(),
                );
            }
            dialog.close();
        });
        self.page_dialog.replace(Some(dialog.clone()));
        dialog.present();
        if prompt.kind == "prompt" {
            input.grab_focus();
            input.select_region(0, -1);
        }
    }

    fn show_page_select(self: &Rc<Self>, prompt: surf_client_app::SelectPrompt) {
        if self.page_select.borrow().is_some() {
            return;
        }
        let popover = gtk::Popover::new();
        popover.set_parent(&self.gl_area);
        if let Some(rect) = prompt.rect {
            let width = f64::from(self.gl_area.allocated_width());
            let height = f64::from(self.gl_area.allocated_height());
            popover.set_pointing_to(Some(&gdk::Rectangle::new(
                (rect[0] * width).round() as i32,
                (rect[1] * height).round() as i32,
                (rect[2] * width).round().max(1.0) as i32,
                (rect[3] * height).round().max(1.0) as i32,
            )));
        }
        let content = gtk::Box::new(gtk::Orientation::Vertical, 2);
        content.add_css_class("surf-popover");
        if !prompt.title.is_empty() {
            content.append(&gtk::Label::new(Some(&prompt.title)));
        }
        let choices = Rc::new(RefCell::new(Vec::with_capacity(prompt.options.len())));
        for (index, option) in prompt.options.iter().enumerate() {
            let button = gtk::CheckButton::with_label(&option.label);
            button.set_active(prompt.selected[index]);
            button.set_sensitive(!option.disabled);
            if !prompt.multiple {
                let weak = Rc::downgrade(self);
                let popover = popover.clone();
                button.connect_toggled(move |button| {
                    if button.is_active()
                        && let Some(ui) = weak.upgrade()
                    {
                        ui.page_select.borrow_mut().take();
                        ui.controller
                            .borrow_mut()
                            .reply_select(false, vec![index as i32]);
                        popover.popdown();
                    }
                });
            }
            choices.borrow_mut().push(button.clone());
            content.append(&button);
        }
        if prompt.multiple {
            let choose = gtk::Button::with_label("Choose");
            let weak = Rc::downgrade(self);
            let popover_clone = popover.clone();
            let choices = Rc::clone(&choices);
            choose.connect_clicked(move |_| {
                if let Some(ui) = weak.upgrade() {
                    let indices = choices
                        .borrow()
                        .iter()
                        .enumerate()
                        .filter_map(|(index, choice)| {
                            choice
                                .is_active()
                                .then_some(i32::try_from(index).unwrap_or(i32::MAX))
                        })
                        .collect();
                    ui.page_select.borrow_mut().take();
                    ui.controller.borrow_mut().reply_select(false, indices);
                    popover_clone.popdown();
                }
            });
            content.append(&choose);
        }
        let weak = Rc::downgrade(self);
        popover.connect_closed(move |_| {
            if let Some(ui) = weak.upgrade()
                && ui.page_select.borrow_mut().take().is_some()
            {
                ui.controller.borrow_mut().reply_select(true, Vec::new());
            }
        });
        popover.set_child(Some(&content));
        self.page_select.replace(Some(popover.clone()));
        popover.popup();
    }

    fn show_file_chooser(self: &Rc<Self>, multiple: bool) {
        if self.file_chooser.borrow().is_some() {
            return;
        }
        let chooser = gtk::FileChooserNative::builder()
            .title("Choose file")
            .transient_for(&self.window)
            .modal(true)
            .action(gtk::FileChooserAction::Open)
            .accept_label("Upload")
            .cancel_label("Cancel")
            .select_multiple(multiple)
            .build();
        let weak = Rc::downgrade(self);
        chooser.connect_response(move |chooser, response| {
            if let Some(ui) = weak.upgrade()
                && ui.file_chooser.borrow_mut().take().is_some()
            {
                let paths = if response == gtk::ResponseType::Accept {
                    list_model_paths(&chooser.files())
                } else {
                    Vec::new()
                };
                ui.controller.borrow_mut().choose_files(paths);
            }
            chooser.destroy();
        });
        self.file_chooser.replace(Some(chooser.clone()));
        chooser.show();
    }

    fn show_reader(self: &Rc<Self>, reader: surf_client_app::ReaderDocument) {
        if self.reader_window.borrow().is_some() {
            return;
        }
        let window = gtk::Window::builder()
            .transient_for(&self.window)
            .title(if reader.title.is_empty() {
                "Reader"
            } else {
                &reader.title
            })
            .default_width(680)
            .default_height(620)
            .build();
        let content = gtk::Box::new(gtk::Orientation::Vertical, 0);
        let toolbar = gtk::Box::new(gtk::Orientation::Horizontal, 8);
        toolbar.add_css_class("surf-toolbar");
        let address = gtk::Label::builder()
            .label(compact_address(&reader.url))
            .xalign(0.0)
            .hexpand(true)
            .ellipsize(gtk::pango::EllipsizeMode::Middle)
            .css_classes(["surf-status"])
            .build();
        let open = gtk::Button::with_label("Open page");
        toolbar.append(&address);
        toolbar.append(&open);
        let scroll = gtk::ScrolledWindow::new();
        let text = gtk::Label::builder()
            .label(&reader.text)
            .wrap(true)
            .selectable(true)
            .xalign(0.0)
            .yalign(0.0)
            .margin_top(24)
            .margin_bottom(24)
            .margin_start(32)
            .margin_end(32)
            .build();
        scroll.set_child(Some(&text));
        content.append(&toolbar);
        content.append(&scroll);
        window.set_child(Some(&content));
        let weak = Rc::downgrade(self);
        let url = reader.url.clone();
        open.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.controller.borrow_mut().navigate(&url);
                if let Some(window) = ui.reader_window.borrow_mut().take() {
                    window.close();
                }
                ui.controller.borrow_mut().browser.reader = None;
            }
        });
        let weak = Rc::downgrade(self);
        window.connect_close_request(move |_| {
            if let Some(ui) = weak.upgrade()
                && ui.reader_window.borrow_mut().take().is_some()
            {
                ui.controller.borrow_mut().browser.reader = None;
            }
            Propagation::Proceed
        });
        self.reader_window.replace(Some(window.clone()));
        window.present();
    }

    fn show_page_error(self: &Rc<Self>, url: &str) {
        if self.page_error.borrow().is_some() {
            return;
        }
        let dialog = gtk::MessageDialog::builder()
            .transient_for(&self.window)
            .modal(true)
            .text("This page is unavailable")
            .secondary_text(compact_address(url))
            .buttons(gtk::ButtonsType::Close)
            .build();
        let weak = Rc::downgrade(self);
        dialog.connect_response(move |dialog, _| {
            if let Some(ui) = weak.upgrade()
                && ui.page_error.borrow_mut().take().is_some()
            {
                ui.controller.borrow_mut().browser.page_error = None;
            }
            dialog.close();
        });
        self.page_error.replace(Some(dialog.clone()));
        dialog.present();
    }
}

fn icon_button(icon: &str, tooltip: &str) -> gtk::Button {
    let button = gtk::Button::builder()
        .icon_name(icon)
        .tooltip_text(tooltip)
        .build();
    button.add_css_class("surf-icon");
    button
}

fn clear_box(container: &gtk::Box) {
    while let Some(child) = container.first_child() {
        container.remove(&child);
    }
}

fn current_modifiers(controller: &impl IsA<gtk::EventController>) -> gdk::ModifierType {
    controller.as_ref().current_event_state()
}

fn setting_row(title: &str, detail: &str, toggle: &gtk::Switch) -> gtk::Box {
    let row = gtk::Box::new(gtk::Orientation::Horizontal, 12);
    let labels = gtk::Box::new(gtk::Orientation::Vertical, 2);
    labels.set_hexpand(true);
    labels.append(&gtk::Label::builder().label(title).xalign(0.0).build());
    labels.append(
        &gtk::Label::builder()
            .label(detail)
            .xalign(0.0)
            .wrap(true)
            .css_classes(["surf-status"])
            .build(),
    );
    row.append(&labels);
    row.append(toggle);
    row
}

fn section_label(text: &str) -> gtk::Label {
    let label = gtk::Label::builder().label(text).xalign(0.0).build();
    label.add_css_class("surf-section");
    label
}

fn library_entry_row(
    title: &str,
    url: &str,
    remove_tooltip: &str,
) -> (gtk::Box, gtk::Button, gtk::Button) {
    let row = gtk::Box::new(gtk::Orientation::Horizontal, 4);
    row.add_css_class("surf-library-row");
    let open = gtk::Button::new();
    open.add_css_class("flat");
    open.set_hexpand(true);
    let labels = gtk::Box::new(gtk::Orientation::Vertical, 2);
    let title = if title.trim().is_empty() {
        compact_address(url)
    } else {
        title.to_owned()
    };
    labels.append(
        &gtk::Label::builder()
            .label(title)
            .xalign(0.0)
            .ellipsize(gtk::pango::EllipsizeMode::End)
            .build(),
    );
    labels.append(
        &gtk::Label::builder()
            .label(compact_address(url))
            .xalign(0.0)
            .ellipsize(gtk::pango::EllipsizeMode::Middle)
            .css_classes(["surf-status"])
            .build(),
    );
    open.set_child(Some(&labels));
    let remove = icon_button("user-trash-symbolic", remove_tooltip);
    row.append(&open);
    row.append(&remove);
    (row, open, remove)
}

fn append_empty_state(container: &gtk::Box, text: &str) {
    container.append(
        &gtk::Label::builder()
            .label(text)
            .xalign(0.0)
            .margin_top(18)
            .margin_bottom(18)
            .margin_start(10)
            .margin_end(10)
            .css_classes(["surf-status"])
            .build(),
    );
}

fn download_destination(name: &str) -> PathBuf {
    directories::UserDirs::new()
        .and_then(|directories| directories.download_dir().map(PathBuf::from))
        .unwrap_or_else(std::env::temp_dir)
        .join(name)
}

fn format_bytes(bytes: i64) -> String {
    let bytes = bytes.max(0) as f64;
    if bytes >= 1024.0 * 1024.0 * 1024.0 {
        format!("{:.1} GB", bytes / (1024.0 * 1024.0 * 1024.0))
    } else if bytes >= 1024.0 * 1024.0 {
        format!("{:.1} MB", bytes / (1024.0 * 1024.0))
    } else if bytes >= 1024.0 {
        format!("{:.1} KB", bytes / 1024.0)
    } else {
        format!("{} B", bytes as i64)
    }
}

fn format_time(seconds: f64) -> String {
    if !seconds.is_finite() || seconds < 0.0 {
        return "0:00".to_owned();
    }
    let seconds = seconds.round() as u64;
    format!("{}:{:02}", seconds / 60, seconds % 60)
}

fn metric_card(name: &str) -> (gtk::Box, gtk::Label) {
    let card = gtk::Box::new(gtk::Orientation::Vertical, 2);
    card.set_hexpand(true);
    card.add_css_class("surf-metric");
    let value = gtk::Label::new(Some("—"));
    value.add_css_class("surf-metric-value");
    card.append(&value);
    card.append(
        &gtk::Label::builder()
            .label(name)
            .css_classes(["surf-status"])
            .build(),
    );
    (card, value)
}

fn list_model_paths(model: &gtk::gio::ListModel) -> Vec<PathBuf> {
    (0..model.n_items())
        .filter_map(|index| model.item(index))
        .filter_map(|item| item.downcast::<gtk::gio::File>().ok())
        .filter_map(|file| file.path())
        .collect()
}
