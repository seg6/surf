import { useDashboard } from '../state';
import { totalDrops } from '../lib/metrics';
import { updateView } from '../lib/view';

function Tile({ label, value, unit }: { label: string; value: string; unit?: string }) {
  return (
    <div class="tile">
      <p class="tile-label">{label}</p>
      <p class="tile-value">
        {value}
        {unit && <span class="tile-unit"> {unit}</span>}
      </p>
    </div>
  );
}

export function Overview() {
  const { stats, fps, online, mode, go, update, runUpdate } = useDashboard();
  const view = updateView(update);
  const known = online !== null && stats !== null;
  const count = (value: number | undefined) => (known ? String(value ?? 0) : '—');
  const capture = stats?.casting;

  return (
    <>
      {mode && mode.state !== 'streaming' && (
        <div class="notice notice-info">
          <p>
            {mode.standalone
              ? 'Standalone browser setup is active, so devices cannot connect.'
              : 'Surf’s browser is open on this computer, so connected devices are paused.'}
          </p>
          <button type="button" class="btn" onClick={() => go('browser')}>
            Open Browser Settings
          </button>
        </div>
      )}

      <div class="tiles">
        <Tile label="Frame rate" value={fps === null ? '—' : fps.toFixed(1)} unit={fps === null ? undefined : 'fps'} />
        <Tile label="Connected" value={count(stats?.clients)} />
        <Tile label="Open tabs" value={count(stats?.tabs)} />
        <Tile label="Dropped frames" value={known && stats ? String(totalDrops(stats)) : '—'} />
      </div>

      <h2 class="section-title">Media</h2>
      <ul class="panel list">
        <li class="list-row">
          <span>Chromium capture</span>
          <span class="state">
            <span class={`dot${capture ? ' dot-ok' : ''}`} aria-hidden="true" />
            {known ? (capture ? 'Active' : 'Idle') : '—'}
          </span>
        </li>
        <li class="list-row">
          <span>H.264 subscribers</span>
          <strong>{count(stats?.videoSubs)}</strong>
        </li>
        <li class="list-row">
          <span>Audio subscribers</span>
          <strong>{count(stats?.audioSubs)}</strong>
        </li>
      </ul>

      <h2 class="section-title">Updates</h2>
      <div class="panel panel-pad row-flex" role="status" aria-live="polite">
        <span>{view.text}</span>
        <button type="button" class="btn" disabled={view.disabled} onClick={() => void runUpdate(view.action)}>
          {view.label}
        </button>
      </div>
    </>
  );
}
