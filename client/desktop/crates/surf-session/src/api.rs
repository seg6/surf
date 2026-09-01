use std::sync::Arc;
use std::time::Duration;

use reqwest::{Client, Method, StatusCode};
use serde::de::DeserializeOwned;
use serde::{Deserialize, Serialize};
use url::Url;

use crate::identity::DeviceIdentity;
use crate::storage::{SavedServer, Storage};
use crate::tls;
use crate::{Result, SessionError};

const REQUEST_TIMEOUT: Duration = Duration::from_secs(20);

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ServerInfo {
    pub api: String,
    pub name: String,
    #[serde(rename = "serverID")]
    pub server_id: String,
    pub fingerprint: String,
    pub protocol: String,
    #[serde(rename = "compatibilityVersion")]
    pub compatibility_version: i32,
    pub version: String,
    pub pairing: bool,
    #[serde(default)]
    pub transport: String,
}

#[derive(Clone, Debug)]
pub struct VerifiedEndpoint {
    pub endpoint: Url,
    pub info: ServerInfo,
    pub certificate: Vec<u8>,
    pub(crate) client: Client,
    pub(crate) tls: Arc<rustls::ClientConfig>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct PairingStatus {
    pub id: String,
    #[serde(rename = "deviceID")]
    pub device_id: String,
    #[serde(rename = "deviceName")]
    pub device_name: String,
    pub phrase: String,
    #[serde(rename = "requestedAt")]
    pub requested_at: String,
    #[serde(rename = "clientConfirmed")]
    pub client_confirmed: bool,
    #[serde(rename = "serverApproved")]
    pub server_approved: bool,
    pub paired: bool,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct NativeConfig {
    pub ticket: String,
    pub vw: i32,
    pub vh: i32,
    pub nv: String,
    #[serde(rename = "compatibilityVersion")]
    pub compatibility_version: i32,
    pub version: String,
    pub host: String,
    pub caps: Vec<String>,
    pub compatibility: String,
    #[serde(default, rename = "clientUpdate")]
    pub client_update: Option<serde_json::Value>,
}

#[derive(Debug, Serialize)]
struct PairRequest<'a> {
    #[serde(rename = "deviceName")]
    device_name: &'a str,
    #[serde(rename = "publicKey")]
    public_key: String,
    code: &'a str,
    #[serde(rename = "qrToken", skip_serializing_if = "str::is_empty")]
    qr_token: &'a str,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct Challenge {
    id: String,
    #[serde(rename = "deviceID")]
    device_id: String,
    nonce: String,
    #[serde(rename = "expiresAt")]
    expires_at: String,
}

#[derive(Debug, Serialize)]
struct ChallengeRequest<'a> {
    #[serde(rename = "deviceID")]
    device_id: &'a str,
}

#[derive(Debug, Serialize)]
struct ChallengeResponse<'a> {
    #[serde(rename = "challengeID")]
    challenge_id: &'a str,
    signature: &'a str,
}

pub async fn inspect_endpoint(value: &str) -> Result<VerifiedEndpoint> {
    let endpoint = normalize_endpoint(value)?;
    let probe_tls = Arc::new(tls::config(None)?);
    let probe = client(Arc::clone(&probe_tls))?;
    let response = probe.get(join(&endpoint, "/api/v1/server")?).send().await?;
    let status = response.status();
    let certificate = response
        .extensions()
        .get::<reqwest::tls::TlsInfo>()
        .and_then(reqwest::tls::TlsInfo::peer_certificate)
        .map(ToOwned::to_owned)
        .ok_or(SessionError::Tls(
            "server did not present a leaf certificate".to_owned(),
        ))?;
    let info: ServerInfo = decode_response(response, status).await?;
    if info.api != "v1" {
        return Err(SessionError::Protocol(
            "server does not offer Surf API v1".to_owned(),
        ));
    }
    if info.transport == "tunnel" {
        return Err(SessionError::Protocol(
            "tunneled Surf endpoints are not supported by this desktop preview yet".to_owned(),
        ));
    }
    let observed = tls::fingerprint(&certificate);
    let advertised = tls::parse_fingerprint(&info.fingerprint)?;
    if observed != advertised || info.server_id != info.fingerprint {
        return Err(SessionError::Tls(
            "advertised server identity does not match its TLS certificate".to_owned(),
        ));
    }
    let pinned_tls = Arc::new(tls::config(Some(observed))?);
    let client = client(Arc::clone(&pinned_tls))?;
    Ok(VerifiedEndpoint {
        endpoint,
        info,
        certificate,
        client,
        tls: pinned_tls,
    })
}

pub(crate) async fn request_pairing(
    verified: &VerifiedEndpoint,
    storage: &Storage,
    device_name: &str,
    code: &str,
) -> Result<PairingStatus> {
    if code.len() != 6 || !code.bytes().all(|byte| byte.is_ascii_digit()) {
        return Err(SessionError::Pairing(
            "enter the six-digit code shown by the Surf server".to_owned(),
        ));
    }
    let identity = DeviceIdentity::load_or_create(storage.root(), &verified.info.server_id)?;
    let body = PairRequest {
        device_name,
        public_key: identity.public_key(),
        code,
        qr_token: "",
    };
    let status: PairingStatus = request_json(
        &verified.client,
        Method::POST,
        join(&verified.endpoint, "/api/v1/pairing/request")?,
        Some(&body),
    )
    .await?;
    let expected = identity.pairing_phrase(&verified.info.server_id);
    if status.phrase != expected || status.device_id != identity.device_id() {
        return Err(SessionError::Pairing(
            "pairing response is not bound to this device and server".to_owned(),
        ));
    }
    Ok(status)
}

pub(crate) async fn confirm_pairing(
    verified: &VerifiedEndpoint,
    storage: &Storage,
    status: &PairingStatus,
) -> Result<PairingStatus> {
    let path = format!("/api/v1/pairing/confirm/{}", status.id);
    let confirmed: PairingStatus = request_json::<(), _>(
        &verified.client,
        Method::POST,
        join(&verified.endpoint, &path)?,
        None,
    )
    .await?;
    if !confirmed.paired || !confirmed.client_confirmed || !confirmed.server_approved {
        return Err(SessionError::Pairing(
            "the server did not finish pairing".to_owned(),
        ));
    }
    storage.save_server(SavedServer {
        server_id: verified.info.server_id.clone(),
        fingerprint: verified.info.fingerprint.clone(),
        name: verified.info.name.clone(),
        endpoint: verified.endpoint.as_str().trim_end_matches('/').to_owned(),
        transport: verified.info.transport.clone(),
    })?;
    let ack = format!("/api/v1/pairing/ack/{}", status.id);
    request_empty(
        &verified.client,
        Method::POST,
        join(&verified.endpoint, &ack)?,
    )
    .await?;
    Ok(confirmed)
}

pub(crate) async fn authenticate(
    verified: &VerifiedEndpoint,
    storage: &Storage,
) -> Result<NativeConfig> {
    let saved = storage
        .server(&verified.info.server_id)?
        .ok_or_else(|| SessionError::Pairing("this server is not paired".to_owned()))?;
    if saved.fingerprint != verified.info.fingerprint {
        return Err(SessionError::Tls(
            "saved server fingerprint does not match the endpoint".to_owned(),
        ));
    }
    let identity = DeviceIdentity::load(storage.root(), &verified.info.server_id)?
        .ok_or_else(|| SessionError::Identity("this server's device key is missing".to_owned()))?;
    let device_id = identity.device_id();
    let challenge: Challenge = request_json(
        &verified.client,
        Method::POST,
        join(&verified.endpoint, "/api/v1/auth/challenge")?,
        Some(&ChallengeRequest {
            device_id: &device_id,
        }),
    )
    .await?;
    if challenge.device_id != device_id || challenge.id.is_empty() || challenge.nonce.is_empty() {
        return Err(SessionError::Protocol(
            "authentication challenge is not bound to this device".to_owned(),
        ));
    }
    let _ = &challenge.expires_at;
    let signature =
        identity.sign_authentication(&verified.info.server_id, &challenge.id, &challenge.nonce)?;
    request_empty_json(
        &verified.client,
        join(&verified.endpoint, "/api/v1/auth/complete")?,
        &ChallengeResponse {
            challenge_id: &challenge.id,
            signature: &signature,
        },
    )
    .await?;

    let mut url = join(&verified.endpoint, "/api/v1/config")?;
    url.query_pairs_mut()
        .append_pair("av", app_version())
        .append_pair("cv", compatibility_version())
        .append_pair("nv", wire_compatibility_version());
    let config: NativeConfig =
        request_json::<(), _>(&verified.client, Method::GET, url, None).await?;
    if config.compatibility != "compatible"
        || config.compatibility_version.to_string() != compatibility_version()
    {
        return Err(SessionError::Compatibility {
            decision: config.compatibility.clone(),
            server_version: config.version.clone(),
        });
    }
    if config.ticket.is_empty() {
        return Err(SessionError::Protocol(
            "server configuration omitted the WebSocket ticket".to_owned(),
        ));
    }
    Ok(config)
}

fn client(config: Arc<rustls::ClientConfig>) -> Result<Client> {
    Ok(Client::builder()
        .tls_backend_preconfigured((*config).clone())
        .tls_info(true)
        .cookie_store(true)
        .https_only(true)
        .timeout(REQUEST_TIMEOUT)
        .user_agent(format!("Surf Desktop/{}", app_version()))
        .build()?)
}

async fn request_json<B: Serialize + ?Sized, T: DeserializeOwned>(
    client: &Client,
    method: Method,
    url: Url,
    body: Option<&B>,
) -> Result<T> {
    let mut request = client.request(method, url);
    if let Some(body) = body {
        request = request.json(body);
    }
    let response = request.send().await?;
    let status = response.status();
    decode_response(response, status).await
}

async fn request_empty(client: &Client, method: Method, url: Url) -> Result<()> {
    let response = client.request(method, url).send().await?;
    check_status(response).await
}

async fn request_empty_json<T: Serialize + ?Sized>(
    client: &Client,
    url: Url,
    body: &T,
) -> Result<()> {
    let response = client.post(url).json(body).send().await?;
    check_status(response).await
}

async fn check_status(response: reqwest::Response) -> Result<()> {
    let status = response.status();
    if status.is_success() {
        return Ok(());
    }
    Err(status_error(response, status).await)
}

async fn decode_response<T: DeserializeOwned>(
    response: reqwest::Response,
    status: StatusCode,
) -> Result<T> {
    if !status.is_success() {
        return Err(status_error(response, status).await);
    }
    response
        .json()
        .await
        .map_err(|error| SessionError::Protocol(error.to_string()))
}

async fn status_error(response: reqwest::Response, status: StatusCode) -> SessionError {
    let message = response.text().await.unwrap_or_default();
    SessionError::HttpStatus {
        status: status.as_u16(),
        message: message.trim().to_owned(),
    }
}

fn join(endpoint: &Url, path: &str) -> Result<Url> {
    endpoint
        .join(path)
        .map_err(|error| SessionError::Endpoint(error.to_string()))
}

pub fn normalize_endpoint(value: &str) -> Result<Url> {
    let value = value.trim();
    let value = if value.contains("://") {
        value.to_owned()
    } else {
        format!("https://{value}")
    };
    let parsed = Url::parse(&value).map_err(|error| SessionError::Endpoint(error.to_string()))?;
    if parsed.scheme() != "https" || parsed.host().is_none() {
        return Err(SessionError::Endpoint(
            "Surf server endpoints must be HTTPS addresses".to_owned(),
        ));
    }
    let host = parsed
        .host()
        .ok_or_else(|| SessionError::Endpoint("endpoint has no host".to_owned()))?;
    let mut normalized = Url::parse(&format!("https://{host}"))
        .map_err(|error| SessionError::Endpoint(error.to_string()))?;
    if let Some(port) = parsed.port() {
        normalized
            .set_port(Some(port))
            .map_err(|()| SessionError::Endpoint("invalid endpoint port".to_owned()))?;
    }
    Ok(normalized)
}

pub fn app_version() -> &'static str {
    include_str!("../../../../../VERSION").trim()
}

pub fn compatibility_version() -> &'static str {
    include_str!("../../../../../COMPATIBILITY_VERSION").trim()
}

pub fn wire_compatibility_version() -> &'static str {
    match compatibility_version() {
        "1" => "20260831-1",
        value => value,
    }
}

#[cfg(test)]
mod tests {
    use super::{
        app_version, compatibility_version, normalize_endpoint, wire_compatibility_version,
    };

    #[test]
    fn endpoint_normalization_matches_native_client() {
        assert_eq!(
            normalize_endpoint(" 192.0.2.1:18080/path ")
                .unwrap()
                .as_str(),
            "https://192.0.2.1:18080/"
        );
        assert!(normalize_endpoint("http://example.com").is_err());
    }

    #[test]
    fn versions_are_sourced_from_release_files() {
        assert!(!app_version().is_empty());
        assert!(!compatibility_version().is_empty());
        assert!(!wire_compatibility_version().is_empty());
    }
}
