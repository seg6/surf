import type { ComponentChildren } from 'preact';
import { useDashboard } from '../state';

/**
 * Wraps settings fields that share one draft and one save action. Browser and
 * Connection both use it, because the host saves every setting in one request.
 */
export function SettingsForm({ children }: { children: ComponentChildren }) {
  const { draft, dirty, saving, saveSettings } = useDashboard();

  if (!draft) return <p class="panel panel-pad muted">Loading settings…</p>;

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        void saveSettings();
      }}
    >
      {children}
      <div class="save-bar">
        <span class="muted" role="status">
          {dirty ? 'You have unsaved changes.' : 'Changes take effect after Surf restarts.'}
        </span>
        <button type="submit" class="btn btn-primary btn-large" disabled={saving}>
          Save and Restart
        </button>
      </div>
    </form>
  );
}
