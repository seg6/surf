use std::fs;
use std::path::{Path, PathBuf};

use base64::Engine as _;
use base64::engine::general_purpose::URL_SAFE_NO_PAD;
use rand::rngs::OsRng;
use rsa::pkcs1::{DecodeRsaPrivateKey, EncodeRsaPrivateKey, EncodeRsaPublicKey};
use rsa::traits::PublicKeyParts as _;
use rsa::{Pkcs1v15Sign, RsaPrivateKey};
use sha2::{Digest, Sha256};

use crate::{Result, SessionError, atomic_write_private};

const PHRASE_WORDS: [&str; 64] = [
    "amber", "anchor", "apple", "april", "arrow", "atlas", "bamboo", "beacon", "birch", "bloom",
    "blue", "bridge", "brook", "canyon", "cedar", "cloud", "coral", "crane", "dawn", "delta",
    "drift", "eagle", "ember", "fern", "field", "finch", "forest", "frost", "garden", "glade",
    "gold", "harbor", "hazel", "heron", "island", "jade", "lake", "leaf", "lilac", "luna", "maple",
    "meadow", "mist", "moon", "north", "ocean", "olive", "orchid", "pearl", "pine", "quartz",
    "rain", "reed", "river", "robin", "sage", "shore", "silver", "sky", "stone", "sun", "tide",
    "willow", "wren",
];

#[derive(Debug)]
pub(crate) struct DeviceIdentity {
    key: RsaPrivateKey,
    public_der: Vec<u8>,
    #[cfg(test)]
    path: PathBuf,
}

impl DeviceIdentity {
    pub(crate) fn load(root: &Path, server_id: &str) -> Result<Option<Self>> {
        validate_server_id(server_id)?;
        let path = identity_path(root, server_id);
        let pem = match fs::read_to_string(&path) {
            Ok(pem) => pem,
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(None),
            Err(error) => return Err(error.into()),
        };
        let key = RsaPrivateKey::from_pkcs1_pem(&pem)
            .map_err(|error| SessionError::Identity(error.to_string()))?;
        Self::from_key(key, path).map(Some)
    }

    pub(crate) fn load_or_create(root: &Path, server_id: &str) -> Result<Self> {
        if let Some(identity) = Self::load(root, server_id)? {
            return Ok(identity);
        }
        let path = identity_path(root, server_id);
        let key = RsaPrivateKey::new(&mut OsRng, 2048)
            .map_err(|error| SessionError::Identity(error.to_string()))?;
        let pem = key
            .to_pkcs1_pem(rsa::pkcs1::LineEnding::LF)
            .map_err(|error| SessionError::Identity(error.to_string()))?;
        atomic_write_private(&path, pem.as_bytes())?;
        Self::from_key(key, path)
    }

    fn from_key(key: RsaPrivateKey, path: PathBuf) -> Result<Self> {
        if key.size() != 256 {
            return Err(SessionError::Identity(
                "device identity is not RSA-2048".to_owned(),
            ));
        }
        let public_der = key
            .to_public_key()
            .to_pkcs1_der()
            .map_err(|error| SessionError::Identity(error.to_string()))?
            .as_bytes()
            .to_vec();
        #[cfg(not(test))]
        let _ = path;
        Ok(Self {
            key,
            public_der,
            #[cfg(test)]
            path,
        })
    }

    pub(crate) fn public_key(&self) -> String {
        URL_SAFE_NO_PAD.encode(&self.public_der)
    }

    pub(crate) fn device_id(&self) -> String {
        hex::encode(Sha256::digest(&self.public_der))
    }

    pub(crate) fn pairing_phrase(&self, server_id: &str) -> String {
        let mut hash = Sha256::new();
        hash.update(b"SURF-PAIR-V1\0");
        hash.update(server_id.as_bytes());
        hash.update(&self.public_der);
        let digest = hash.finalize();
        let indices = [
            digest[0] >> 2,
            ((digest[0] & 3) << 4) | (digest[1] >> 4),
            ((digest[1] & 15) << 2) | (digest[2] >> 6),
            digest[2] & 63,
            digest[3] >> 2,
            ((digest[3] & 3) << 4) | (digest[4] >> 4),
        ];
        indices
            .iter()
            .map(|index| PHRASE_WORDS[usize::from(*index)])
            .collect::<Vec<_>>()
            .join(" ")
    }

    pub(crate) fn sign_authentication(
        &self,
        server_id: &str,
        challenge_id: &str,
        nonce: &str,
    ) -> Result<String> {
        let device_id = self.device_id();
        let value = format!("SURF-AUTH-V1\0{server_id}\0{device_id}\0{challenge_id}\0{nonce}");
        let digest = Sha256::digest(value.as_bytes());
        let signature = self
            .key
            .sign(Pkcs1v15Sign::new::<Sha256>(), &digest)
            .map_err(|error| SessionError::Identity(error.to_string()))?;
        Ok(URL_SAFE_NO_PAD.encode(signature))
    }

    #[cfg(test)]
    pub(crate) fn path(&self) -> &Path {
        &self.path
    }
}

fn identity_path(root: &Path, server_id: &str) -> PathBuf {
    root.join("identities").join(format!("{server_id}.pem"))
}

fn validate_server_id(server_id: &str) -> Result<()> {
    if server_id.len() != 64
        || !server_id
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
    {
        return Err(SessionError::Identity(
            "server identity must be a lowercase SHA-256 fingerprint".to_owned(),
        ));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use std::fs;

    use super::DeviceIdentity;

    const SERVER_ID: &str = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";

    #[test]
    fn identity_is_stable_and_private() {
        let temp = tempfile::tempdir().unwrap();
        let first = DeviceIdentity::load_or_create(temp.path(), SERVER_ID).unwrap();
        let id = first.device_id();
        let key = first.public_key();
        let phrase = first.pairing_phrase(SERVER_ID);
        let signature = first
            .sign_authentication(SERVER_ID, "challenge", "nonce")
            .unwrap();
        assert_eq!(phrase.split_whitespace().count(), 6);
        assert!(!signature.is_empty());

        let second = DeviceIdentity::load_or_create(temp.path(), SERVER_ID).unwrap();
        assert_eq!(second.device_id(), id);
        assert_eq!(second.public_key(), key);

        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt as _;
            assert_eq!(
                fs::metadata(first.path()).unwrap().permissions().mode() & 0o777,
                0o600
            );
        }
    }
}
