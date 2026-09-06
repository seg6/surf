use serde::{Deserialize, Serialize};
use std::path::PathBuf;

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct Preferences {
    pub dark: bool,
    pub bottom: bool,
    pub reduce_motion: bool,
    pub mobile: bool,
    pub device_preset: usize,
    pub landscape: bool,
}

impl Default for Preferences {
    fn default() -> Self {
        Self {
            dark: true,
            bottom: true,
            reduce_motion: false,
            mobile: false,
            device_preset: 5,
            landscape: false,
        }
    }
}

impl Preferences {
    fn path() -> Result<PathBuf, String> {
        let storage = std::env::var_os("SURF_CLIENT_HOME")
            .map(surf_session::Storage::at)
            .map_or_else(surf_session::Storage::system, Ok)
            .map_err(|e| e.to_string())?;
        Ok(storage.root().join("desktop-preferences.json"))
    }
    pub fn load() -> Self {
        Self::path()
            .ok()
            .and_then(|p| std::fs::read(p).ok())
            .and_then(|b| serde_json::from_slice(&b).ok())
            .unwrap_or_default()
    }
    pub fn save(&self) -> Result<(), String> {
        let path = Self::path()?;
        let data = serde_json::to_vec_pretty(self).map_err(|e| e.to_string())?;
        surf_session::atomic_write_private(&path, &data).map_err(|e| e.to_string())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn missing_fields_use_compatible_defaults() {
        let p: Preferences = serde_json::from_str("{\"dark\":false}").unwrap();
        assert!(!p.dark);
        assert!(p.bottom);
        assert_eq!(p.device_preset, 5);
        assert_eq!(
            serde_json::from_str::<Preferences>(&serde_json::to_string(&p).unwrap()).unwrap(),
            p
        );
    }
}
