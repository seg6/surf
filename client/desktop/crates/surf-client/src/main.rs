mod gtk_input;
mod gtk_video;

use std::cell::{Cell, RefCell};
use std::path::PathBuf;
use std::rc::Rc;
use std::time::Duration;

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
    let exit = application.run();
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
    dialog_visible: Cell<bool>,
    select_visible: Cell<bool>,
    file_visible: Cell<bool>,
    reader_visible: Cell<bool>,
    error_visible: Cell<bool>,
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
        loading.set_visible(false);
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
            dialog_visible: Cell::new(false),
            select_visible: Cell::new(false),
            file_visible: Cell::new(false),
            reader_visible: Cell::new(false),
            error_visible: Cell::new(false),
        });

        ui.install_actions();
        ui.connect_start_controls(&inspect_button);
        ui.connect_browser_controls(&new_tab);
        ui.connect_video();
        ui.connect_shortcuts();
        ui.update_view();

        let weak = Rc::downgrade(&ui);
        glib::timeout_add_local(Duration::from_millis(16), move || {
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
                self.show_message("Library", "History, bookmarks, and downloads are loading.");
            }
            "reader" => self.command(Command::Reader {
                causal: Causal::default(),
            }),
            "find" => self.show_find(),
            "media" => self.command(Command::MediaQuery {
                causal: Causal::default(),
            }),
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
        display.clipboard().read_text_async(
            gtk::gio::Cancellable::NONE,
            move |result| {
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
            },
        );
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
                HostEffect::ClearPagePresentation => self.suggestions.popdown(),
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
        self.loading.set_visible(client.snapshot.loading);
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
        let servers: Vec<_> = client
            .saved_servers
            .iter()
            .map(|server| (server.name.clone(), server.endpoint.clone()))
            .chain(
                client
                    .discovered_servers
                    .iter()
                    .map(|server| (server.name.clone(), server.endpoint.clone())),
            )
            .collect();
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
        }
        let select = self.controller.borrow().browser.select.clone();
        if let Some(select) = select {
            self.show_page_select(select);
        }
        let upload = self.controller.borrow().browser.upload_multiple;
        if let Some(multiple) = upload {
            self.show_file_chooser(multiple);
        }
        let reader = self.controller.borrow().browser.reader.clone();
        if let Some(reader) = reader {
            self.show_reader(reader);
        }
        let error = self.controller.borrow().browser.page_error.clone();
        if let Some(url) = error {
            self.show_page_error(&url);
        }
    }

    fn show_message(&self, title: &str, text: &str) {
        let dialog = gtk::MessageDialog::builder()
            .transient_for(&self.window)
            .modal(true)
            .text(title)
            .secondary_text(text)
            .buttons(gtk::ButtonsType::Close)
            .build();
        dialog.connect_response(|dialog, _| dialog.close());
        dialog.present();
    }

    fn show_find(self: &Rc<Self>) {
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
        popover.popup();
        entry.grab_focus();
    }

    fn show_settings(self: &Rc<Self>) {
        let dialog = gtk::Window::builder()
            .transient_for(&self.window)
            .modal(true)
            .title("Settings")
            .default_width(460)
            .resizable(false)
            .build();
        let content = gtk::Box::new(gtk::Orientation::Vertical, 14);
        content.set_margin_top(18);
        content.set_margin_bottom(18);
        content.set_margin_start(18);
        content.set_margin_end(18);
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
        disconnect.connect_clicked(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.controller.borrow_mut().disconnect();
            }
        });
        dialog.present();
    }

    fn show_performance(&self) {
        let client = self.controller.borrow();
        let report = client.latest_diagnostics.unwrap_or_default();
        let media = client.media_diagnostics();
        let text = format!(
            "Presented  {:.1} fps\nDecoded    {:.1} fps\nDropped    {:.1}%\n\nDecode     {} µs\nGPU upload {} µs\nFrame age  {} µs\nRound trip {} µs\nQueues     {} / {} / {}\n\nGaps {} · decode errors {}",
            report.presentation_fps,
            report.decode_fps,
            report.drop_percent,
            report.decode_us,
            report.upload_us,
            report.frame_age_us,
            report.rtt_us,
            report.encoded_video_depth,
            report.decoded_video_depth,
            report.audio_depth,
            report.sequence_gaps,
            media.decode_errors,
        );
        drop(client);
        self.show_message("Performance", &text);
    }

    fn show_page_dialog(self: &Rc<Self>, prompt: surf_client_app::DialogPrompt) {
        if self.dialog_visible.replace(true) {
            return;
        }
        let dialog = gtk::Dialog::builder()
            .transient_for(&self.window)
            .modal(true)
            .title("This page says")
            .build();
        dialog.add_button("Cancel", gtk::ResponseType::Cancel);
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
            if let Some(ui) = weak.upgrade() {
                ui.dialog_visible.set(false);
                ui.controller.borrow_mut().reply_dialog(
                    response == gtk::ResponseType::Ok,
                    response_input.text().to_string(),
                );
            }
            dialog.close();
        });
        dialog.present();
        if prompt.kind == "prompt" {
            input.grab_focus();
            input.select_region(0, -1);
        }
    }

    fn show_page_select(self: &Rc<Self>, prompt: surf_client_app::SelectPrompt) {
        if self.select_visible.replace(true) {
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
                        ui.select_visible.set(false);
                        ui.controller
                            .borrow_mut()
                            .reply_select(false, vec![index as i32]);
                        popover.popdown();
                    }
                });
            }
            content.append(&button);
        }
        if prompt.multiple {
            let choose = gtk::Button::with_label("Choose");
            content.append(&choose);
        }
        let weak = Rc::downgrade(self);
        popover.connect_closed(move |_| {
            if let Some(ui) = weak.upgrade()
                && ui.select_visible.replace(false)
            {
                ui.controller.borrow_mut().reply_select(true, Vec::new());
            }
        });
        popover.set_child(Some(&content));
        popover.popup();
    }

    fn show_file_chooser(self: &Rc<Self>, multiple: bool) {
        if self.file_visible.replace(true) {
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
            if let Some(ui) = weak.upgrade() {
                ui.file_visible.set(false);
                let paths = if response == gtk::ResponseType::Accept {
                    list_model_paths(&chooser.files())
                } else {
                    Vec::new()
                };
                ui.controller.borrow_mut().choose_files(paths);
            }
            chooser.destroy();
        });
        chooser.show();
    }

    fn show_reader(self: &Rc<Self>, reader: surf_client_app::ReaderDocument) {
        if self.reader_visible.replace(true) {
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
        window.set_child(Some(&scroll));
        let weak = Rc::downgrade(self);
        window.connect_close_request(move |_| {
            if let Some(ui) = weak.upgrade() {
                ui.reader_visible.set(false);
                ui.controller.borrow_mut().browser.reader = None;
            }
            Propagation::Proceed
        });
        window.present();
    }

    fn show_page_error(&self, url: &str) {
        if self.error_visible.replace(true) {
            return;
        }
        let dialog = gtk::MessageDialog::builder()
            .transient_for(&self.window)
            .modal(true)
            .text("This page is unavailable")
            .secondary_text(compact_address(url))
            .buttons(gtk::ButtonsType::Close)
            .build();
        let controller = Rc::clone(&self.controller);
        let error_visible = self.error_visible.clone();
        dialog.connect_response(move |dialog, _| {
            error_visible.set(false);
            controller.borrow_mut().browser.page_error = None;
            dialog.close();
        });
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

fn list_model_paths(model: &gtk::gio::ListModel) -> Vec<PathBuf> {
    (0..model.n_items())
        .filter_map(|index| model.item(index))
        .filter_map(|item| item.downcast::<gtk::gio::File>().ok())
        .filter_map(|file| file.path())
        .collect()
}
