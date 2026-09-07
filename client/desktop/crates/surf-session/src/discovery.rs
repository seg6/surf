use std::net::Ipv4Addr;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, mpsc::SyncSender};
use std::thread;
use std::time::Duration;

use mdns_sd::{ResolvedService, ServiceDaemon, ServiceEvent};

use crate::SessionEvent;

const SERVICE_TYPE: &str = "_surf._tcp.local.";

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct DiscoveredServer {
    pub name: String,
    pub endpoint: String,
    pub server_id: String,
    pub protocol: String,
    pub compatibility_version: String,
}

pub(crate) struct DiscoveryWorker {
    stop: Arc<AtomicBool>,
    thread: Option<thread::JoinHandle<()>>,
}

impl DiscoveryWorker {
    pub(crate) fn spawn(events: SyncSender<SessionEvent>) -> Self {
        let stop = Arc::new(AtomicBool::new(false));
        let worker_stop = Arc::clone(&stop);
        let thread = thread::Builder::new()
            .name("surf-discovery".to_owned())
            .spawn(move || run(worker_stop, events))
            .ok();
        Self { stop, thread }
    }
}

impl Drop for DiscoveryWorker {
    fn drop(&mut self) {
        self.stop.store(true, Ordering::Release);
        if let Some(thread) = self.thread.take() {
            let _ = thread.join();
        }
    }
}

fn run(stop: Arc<AtomicBool>, events: SyncSender<SessionEvent>) {
    let daemon = match ServiceDaemon::new() {
        Ok(daemon) => daemon,
        Err(error) => {
            let _ = events.try_send(SessionEvent::DiscoveryUnavailable(error.to_string()));
            return;
        }
    };
    let receiver = match daemon.browse(SERVICE_TYPE) {
        Ok(receiver) => receiver,
        Err(error) => {
            let _ = events.try_send(SessionEvent::DiscoveryUnavailable(error.to_string()));
            let _ = daemon.shutdown();
            return;
        }
    };

    while !stop.load(Ordering::Acquire) {
        match receiver.recv_timeout(Duration::from_millis(250)) {
            Ok(ServiceEvent::ServiceResolved(service)) => {
                if let Some(server) = from_resolved(&service) {
                    let _ = events.try_send(SessionEvent::Discovered(server));
                }
            }
            Ok(_) => {}
            Err(_) => {}
        }
    }
    let _ = daemon.stop_browse(SERVICE_TYPE);
    let _ = daemon.shutdown();
}

fn from_resolved(service: &ResolvedService) -> Option<DiscoveredServer> {
    if service.get_property_val_str("api") != Some("v1")
        || service.get_property_val_str("proto") != Some("https")
    {
        return None;
    }
    let mut addresses: Vec<Ipv4Addr> = service.get_addresses_v4().into_iter().collect();
    addresses.sort_unstable_by_key(|address| (address.is_loopback(), u32::from(*address)));
    let endpoint = match addresses.first() {
        Some(address) => format!("{address}:{}", service.get_port()),
        None => format!(
            "{}:{}",
            service.get_hostname().trim_end_matches('.'),
            service.get_port()
        ),
    };
    let fallback_name = service
        .get_fullname()
        .strip_suffix(SERVICE_TYPE)
        .unwrap_or(service.get_fullname())
        .trim_end_matches('.')
        .to_owned();
    Some(DiscoveredServer {
        name: service
            .get_property_val_str("name")
            .unwrap_or(&fallback_name)
            .to_owned(),
        endpoint,
        server_id: service
            .get_property_val_str("id")
            .unwrap_or_default()
            .to_owned(),
        protocol: service
            .get_property_val_str("nv")
            .unwrap_or_default()
            .to_owned(),
        compatibility_version: service
            .get_property_val_str("cv")
            .unwrap_or_default()
            .to_owned(),
    })
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;

    use mdns_sd::ServiceInfo;

    use super::from_resolved;

    #[test]
    fn resolved_service_is_only_a_locator() {
        let properties = HashMap::from([
            ("api".to_owned(), "v1".to_owned()),
            ("proto".to_owned(), "https".to_owned()),
            ("name".to_owned(), "Studio".to_owned()),
            ("id".to_owned(), "untrusted-advertised-id".to_owned()),
            ("nv".to_owned(), "wire".to_owned()),
            ("cv".to_owned(), "1".to_owned()),
        ]);
        let service = ServiceInfo::new(
            "_surf._tcp.local.",
            "Studio",
            "studio.local.",
            "",
            18080,
            properties,
        )
        .unwrap()
        .as_resolved_service();
        let server = from_resolved(&service).unwrap();
        assert_eq!(server.name, "Studio");
        assert_eq!(server.endpoint, "studio.local:18080");
        assert_eq!(server.server_id, "untrusted-advertised-id");
    }
}
