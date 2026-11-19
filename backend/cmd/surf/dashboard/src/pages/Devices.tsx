import { useDashboard } from '../state';
import { deviceName, deviceStatus, formatDateTime } from '../lib/devices';

function DeviceGlyph() {
  return (
    <span class="device-glyph" aria-hidden="true">
      <span />
    </span>
  );
}

export function Devices() {
  const {
    devices, devicesError, onlineDevices, revokeDevice, openPairing, session, go, candidatesError, mode,
  } = useDashboard();

  return (
    <>
      <p class="lede">
        Devices paired with this computer. A revoked device has to pair again before it can connect.
      </p>

      {session?.active && (
        <div class="notice notice-info">
          <p>A pairing invitation is open.</p>
          <button type="button" class="btn" onClick={() => go('pair')}>
            Show Invitation
          </button>
        </div>
      )}
      {candidatesError && <p class="inline-error">Could not check for pairing requests: {candidatesError}</p>}

      {devicesError && !devices ? (
        <p class="panel panel-pad muted">Could not load devices: {devicesError}</p>
      ) : devices === null ? (
        <p class="panel panel-pad muted">Loading devices…</p>
      ) : devices.length === 0 ? (
        <p class="panel panel-pad muted">No paired devices.</p>
      ) : (
        <ul class="panel list device-list">
          {devices.map((device) => {
            const status = deviceStatus(device, Boolean(onlineDevices[device.id]));
            const paired = formatDateTime(device.pairedAt);
            return (
              <li key={device.id} class="device-row">
                <DeviceGlyph />
                <div class="device-main">
                  <p class="device-name">{deviceName(device)}</p>
                  {paired && <p class="muted">Paired {paired}</p>}
                </div>
                <span class={`state${status.online ? ' state-ok' : ' muted'}`}>
                  {status.online && <span class="dot dot-ok" aria-hidden="true" />}
                  {status.text}
                </span>
                <button type="button" class="btn btn-danger" onClick={() => void revokeDevice(device)}>
                  Revoke
                </button>
              </li>
            );
          })}
        </ul>
      )}
      {devicesError && devices && <p class="inline-error">Could not refresh devices: {devicesError}</p>}

      <button
        type="button"
        class="btn btn-primary btn-large spaced"
        disabled={Boolean(mode?.standalone)}
        onClick={() => void openPairing()}
      >
        Pair a New Device…
      </button>
    </>
  );
}
