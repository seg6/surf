import { useDashboard } from '../state';
import { SettingsForm } from '../components/SettingsForm';
import { Switch } from '../components/Switch';

export function Connection() {
  const { config, draft, updateDraft, copy } = useDashboard();
  const address = config ? config.lanURL || `Port ${config.port}, no network address detected` : 'Loading…';

  return (
    <>
      <div class="panel rows">
        <div class="row">
          <span class="row-text">
            <span class="row-title">Server address</span>
            <span class="row-note">Enter this on a device to connect to this computer.</span>
          </span>
          <span class="copy-row">
            <span class="mono value">{address}</span>
            {config?.lanURL && (
              <button type="button" class="btn btn-small" onClick={() => void copy(config.lanURL ?? '')}>
                Copy
              </button>
            )}
          </span>
        </div>
      </div>

      <h2 class="section-title">Settings</h2>
      <SettingsForm>
        {draft && (
          <div class="panel rows">
            <label class="row">
              <span class="row-title">Server name</span>
              <input
                class="control"
                required
                maxLength={80}
                value={draft.serverName}
                onInput={(event) => updateDraft({ serverName: event.currentTarget.value })}
              />
            </label>
            <label class="row">
              <span class="row-title">Port</span>
              <input
                class="control mono"
                type="number"
                required
                min="1"
                max="65535"
                value={draft.port}
                onInput={(event) => updateDraft({ port: event.currentTarget.value })}
              />
            </label>
            <label class="row">
              <span class="row-text">
                <span class="row-title">Public address</span>
                <span class="row-note">For devices that connect from outside your network.</span>
              </span>
              <input
                class="control mono"
                placeholder="surf.example.net:18080"
                value={draft.publicAddress}
                onInput={(event) => updateDraft({ publicAddress: event.currentTarget.value })}
              />
            </label>
            <div class="row">
              <span class="row-title" id="login-label">Start Surf when I sign in</span>
              <Switch
                checked={draft.startAtLogin}
                labelledBy="login-label"
                onChange={(startAtLogin) => updateDraft({ startAtLogin })}
              />
            </div>
          </div>
        )}
      </SettingsForm>
    </>
  );
}
