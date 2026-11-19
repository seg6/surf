import { useDashboard } from '../state';
import { engineStatus } from '../lib/view';

export function Header() {
  const { config, update, online, mode, restart, openPairing } = useDashboard();
  const status = engineStatus(online, mode);
  const address = (config?.lanURL || config?.baseURL || '').replace(/^https?:\/\//, '');
  const version = update?.current ? `Surf ${update.current}` : 'Surf';

  return (
    <header class="toolbar">
      <div class="brand">
        <img src="/icon.png" alt="" width="28" height="28" />
        <div>
          <p class="brand-name">{config?.serverName || 'Surf'}</p>
          <p class="brand-sub">{address ? `${version}, running at ${address}` : version}</p>
        </div>
      </div>
      <div class={`toolbar-status tone-${status.tone}`} role="status">
        <span class="dot" aria-hidden="true" />
        {status.text}
      </div>
      <div class="toolbar-actions">
        <button type="button" class="tb-btn" onClick={() => void restart()}>
          Restart Engine
        </button>
        <button
          type="button"
          class="tb-btn tb-primary"
          disabled={Boolean(mode?.standalone)}
          onClick={() => void openPairing()}
        >
          Pair Device
        </button>
      </div>
    </header>
  );
}
