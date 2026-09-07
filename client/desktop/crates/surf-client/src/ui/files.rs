use super::*;

#[derive(Clone)]
pub(super) struct FileEntry {
    pub path: PathBuf,
    pub name: String,
    pub is_dir: bool,
}
type Listing = Result<Vec<FileEntry>, String>;

pub(super) struct FilePicker {
    pub directory: PathBuf,
    pub selected: BTreeSet<PathBuf>,
    pub multiple: bool,
    pub error: Option<String>,
    pub entries: Vec<FileEntry>,
    pub opened: bool,
    loaded: Option<PathBuf>,
    pending: Option<(PathBuf, std::sync::mpsc::Receiver<Listing>)>,
}

impl FilePicker {
    pub fn new(multiple: bool) -> Self {
        Self {
            directory: directories::UserDirs::new()
                .map(|d| d.home_dir().to_owned())
                .or_else(|| std::env::current_dir().ok())
                .unwrap_or_else(|| PathBuf::from(".")),
            selected: BTreeSet::new(),
            multiple,
            error: None,
            entries: Vec::new(),
            opened: false,
            loaded: None,
            pending: None,
        }
    }
    pub fn refresh(&mut self) {
        if let Some((path, rx)) = &self.pending
            && let Ok(result) = rx.try_recv()
        {
            if *path == self.directory {
                match result {
                    Ok(entries) => {
                        self.entries = entries;
                        self.error = None;
                    }
                    Err(e) => self.error = Some(e),
                }
                self.loaded = Some(path.clone());
            }
            self.pending = None;
        }
        if self.pending.is_none() && self.loaded.as_ref() != Some(&self.directory) {
            self.entries.clear();
            let path = self.directory.clone();
            let worker_path = path.clone();
            let (tx, rx) = std::sync::mpsc::sync_channel(1);
            match std::thread::Builder::new()
                .name("surf-file-list".into())
                .spawn(move || {
                    let result =
                        fs::read_dir(worker_path)
                            .map_err(|e| e.to_string())
                            .map(|entries| {
                                let mut entries: Vec<_> = entries
                                    .flatten()
                                    .take(10_000)
                                    .map(|e| FileEntry {
                                        is_dir: e.path().is_dir(),
                                        name: e.file_name().to_string_lossy().into_owned(),
                                        path: e.path(),
                                    })
                                    .collect();
                                entries.sort_by_key(|e| (!e.is_dir, e.name.to_lowercase()));
                                entries
                            });
                    let _ = tx.send(result);
                }) {
                Ok(_) => self.pending = Some((path, rx)),
                Err(e) => {
                    self.error = Some(e.to_string());
                    self.loaded = Some(path);
                }
            }
        }
    }
}
