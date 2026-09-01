use std::fs;
use std::path::{Path, PathBuf};

use directories::ProjectDirs;
use serde::{Deserialize, Serialize};

use crate::{Result, SessionError, atomic_write_private};

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct SavedServer {
    #[serde(rename = "serverID")]
    pub server_id: String,
    pub fingerprint: String,
    pub name: String,
    pub endpoint: String,
    #[serde(default)]
    pub transport: String,
}

#[derive(Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
struct ServerFile {
    version: u32,
    servers: Vec<SavedServer>,
}

#[derive(Clone, Debug)]
pub struct Storage {
    root: PathBuf,
}

impl Storage {
    pub fn system() -> Result<Self> {
        let dirs = ProjectDirs::from("space", "seg6", "Surf").ok_or_else(|| {
            SessionError::Storage("could not determine the Surf data directory".to_owned())
        })?;
        Ok(Self {
            root: dirs.data_local_dir().to_owned(),
        })
    }

    pub fn at(root: impl Into<PathBuf>) -> Self {
        Self { root: root.into() }
    }

    pub fn root(&self) -> &Path {
        &self.root
    }

    pub fn servers(&self) -> Result<Vec<SavedServer>> {
        let path = self.root.join("servers.json");
        let data = match fs::read(&path) {
            Ok(data) => data,
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(Vec::new()),
            Err(error) => return Err(error.into()),
        };
        let file: ServerFile = serde_json::from_slice(&data)
            .map_err(|error| SessionError::Storage(error.to_string()))?;
        if file.version != 1 {
            return Err(SessionError::Storage(format!(
                "unsupported saved-server format {}",
                file.version
            )));
        }
        Ok(file.servers)
    }

    pub(crate) fn save_server(&self, server: SavedServer) -> Result<()> {
        let mut servers = self.servers()?;
        servers.retain(|saved| saved.server_id != server.server_id);
        servers.push(server);
        servers.sort_by(|left, right| left.name.cmp(&right.name));
        let mut data = serde_json::to_vec_pretty(&ServerFile {
            version: 1,
            servers,
        })
        .map_err(|error| SessionError::Storage(error.to_string()))?;
        data.push(b'\n');
        atomic_write_private(&self.root.join("servers.json"), &data)
    }

    pub(crate) fn server(&self, server_id: &str) -> Result<Option<SavedServer>> {
        Ok(self
            .servers()?
            .into_iter()
            .find(|server| server.server_id == server_id))
    }
}

#[cfg(test)]
mod tests {
    use super::{SavedServer, Storage};

    #[test]
    fn server_updates_are_atomic_and_deduplicated() {
        let temp = tempfile::tempdir().unwrap();
        let storage = Storage::at(temp.path());
        let mut server = SavedServer {
            server_id: "id".to_owned(),
            fingerprint: "fingerprint".to_owned(),
            name: "One".to_owned(),
            endpoint: "https://127.0.0.1:18080".to_owned(),
            transport: String::new(),
        };
        storage.save_server(server.clone()).unwrap();
        server.name = "Updated".to_owned();
        storage.save_server(server.clone()).unwrap();
        assert_eq!(storage.servers().unwrap(), vec![server]);
    }
}
