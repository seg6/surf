//! Opt-in fixtures through the same production views. No second mock UI.
use super::*;

impl DesktopApp {
    pub(super) fn setup_gallery(&mut self) {
        let Ok(scene) = std::env::var("SURF_UI_GALLERY") else {
            return;
        };
        if std::env::var("SURF_UI_THEME").is_ok_and(|s| s == "light") {
            self.preferences.dark = false;
            self.controller.dark_mode = false;
            self.theme_request = Some(false);
        }
        if std::env::var("SURF_UI_CHROME").is_ok_and(|s| s == "top") {
            self.preferences.bottom = false;
        }
        if scene == "start" {
            return;
        }
        self.controller.connected = true;
        self.controller.connection_phase = surf_client_app::ConnectionPhase::Connected;
        self.controller.snapshot.tabs = vec![
            surf_core::Tab {
                id: 1,
                title: "seg6 — writing".into(),
                url: "https://seg6.space".into(),
                icon: String::new(),
                active: true,
            },
            surf_core::Tab {
                id: 2,
                title: "GitHub · Build things".into(),
                url: "https://github.com".into(),
                icon: String::new(),
                active: false,
            },
            surf_core::Tab {
                id: 3,
                title: "WebGL Aquarium".into(),
                url: "https://webglsamples.org/aquarium/aquarium.html".into(),
                icon: String::new(),
                active: false,
            },
        ];
        self.controller.snapshot.current_url = "https://seg6.space".into();
        self.controller.snapshot.active_title = "seg6 — writing".into();
        self.observed_tab = Some(1);
        self.controller.browser.bookmarks = vec![
            surf_protocol::LibraryEntry {
                title: "Surf — your browser, anywhere".into(),
                url: "https://github.com/seg6/surf".into(),
                ts: 1,
            },
            surf_protocol::LibraryEntry {
                title: "Writing about systems".into(),
                url: "https://seg6.space".into(),
                ts: 2,
            },
        ];
        self.controller.browser.history = self.controller.browser.bookmarks.clone();
        match scene.as_str() {
            "code"=>{
                self.controller.connected=false;
                self.controller.connection_phase=surf_client_app::ConnectionPhase::Code;
                self.controller.status="Enter the six-digit code shown on your computer.".into();
            }
            "words"=>{
                self.controller.connected=false;
                self.controller.connection_phase=surf_client_app::ConnectionPhase::Words;
                self.controller.pairing=Some(surf_session::PairingStatus{id:"gallery".into(),device_id:String::new(),
                    device_name:"Desktop".into(),phrase:"harbor paper silver cloud quiet river".into(),
                    requested_at:String::new(),client_confirmed:false,server_approved:false,paired:false});
            }
            "reader"=>self.controller.browser.reader=Some(surf_client_app::ReaderDocument{
                title:"A browser, wherever you are".into(),url:"https://seg6.space".into(),
                text:"Surf keeps the browser on your computer and brings the page to another screen.\n\nThe desktop client shares its portable core with the iPad client. Browsing stays familiar: open a page, switch a tab, or search for something new.".into()}),
            "dialog"=>{
                self.controller.browser.dialog_revision=1;
                self.controller.browser.dialog=Some(surf_client_app::DialogPrompt{
                    kind:"prompt".into(),text:"What would you like to call this document?".into(),input:"Untitled".into()});
            }
            "select"=>self.controller.browser.select=Some(surf_client_app::SelectPrompt{
                id:"gallery-select".into(),title:"Choose a quality".into(),multiple:false,
                options:["Automatic","High","Medium","Low"].into_iter().map(|label|surf_protocol::SelectOption{
                    label:label.into(),disabled:false,selected:false}).collect(),
                selected:vec![true,false,false,false],rect:Some([0.2,0.4,0.3,0.04])}),
            "media"=>{
                self.controller.browser.media.available=true;self.controller.browser.media.title="Page audio".into();
                self.controller.browser.media.duration=300.0;self.controller.browser.media.current_time=32.0;
                self.panel=Some(Panel::Media);
            }
            "error"=>self.controller.browser.page_error=Some("https://example.invalid".into()),
            "files"=>{
                self.controller.browser.upload_multiple=Some(true);
                let mut picker=FilePicker::new(true);
                picker.directory=PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../assets/fonts");
                self.file_picker=Some(picker);self.panel=Some(Panel::Files);
            }
            "settings" => self.panel = Some(Panel::Settings),
            "library" => self.panel = Some(Panel::Library),
            "tools" => self.panel = Some(Panel::More),
            "tabs" => self.panel = Some(Panel::Tabs),
            "performance" => self.performance_open = true,
            "find" => self.find_open = true,
            "new-tab" => self.controller.snapshot.current_url = "about:blank#surf-new".into(),
            "address" => {
                self.edit_address();
                self.address = "surf".into();
            }
            _ => {}
        }
    }
}
