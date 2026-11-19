import { useDashboard } from '../state';
import { SettingsForm } from '../components/SettingsForm';
import { CUSTOM_SEARCH, SEARCH_PRESETS } from '../lib/search';
import { browserControls, browserModeCopy } from '../lib/view';

export function Browser() {
  const { mode, browserBusy, browserAction, draft, updateDraft } = useDashboard();
  const controls = browserControls(mode);

  return (
    <>
      <section class="panel panel-pad setup" aria-labelledby="setup-title">
        <h2 id="setup-title" class="visually-hidden">Browser on this computer</h2>
        <p class="setup-copy" role="status" aria-live="polite">{browserModeCopy(mode)}</p>
        <div class="actions">
          <button
            type="button"
            class="btn btn-primary"
            disabled={controls.openDisabled || browserBusy}
            onClick={() => void browserAction('open')}
          >
            {controls.openLabel}
          </button>
          {controls.showResume && (
            <button
              type="button"
              class="btn btn-primary"
              disabled={controls.resumeDisabled || browserBusy}
              onClick={() => void browserAction('resume')}
            >
              Resume on Devices
            </button>
          )}
          {controls.showForce && (
            <button type="button" class="btn btn-danger" disabled={browserBusy} onClick={() => void browserAction('resume', true)}>
              Force Close and Resume
            </button>
          )}
        </div>
      </section>

      <h2 class="section-title">Browsing</h2>
      <SettingsForm>
        {draft && (
          <div class="panel rows">
            <label class="row">
              <span class="row-text">
                <span class="row-title">Address bar search</span>
              </span>
              <select
                class="control"
                value={draft.searchPreset}
                onChange={(event) => {
                  const value = event.currentTarget.value;
                  updateDraft(
                    value === CUSTOM_SEARCH ? { searchPreset: value } : { searchPreset: value, searchURL: value },
                  );
                }}
              >
                {SEARCH_PRESETS.map((preset) => (
                  <option key={preset.label} value={preset.value}>{preset.label}</option>
                ))}
                <option value={CUSTOM_SEARCH}>Custom…</option>
              </select>
            </label>
            {draft.searchPreset === CUSTOM_SEARCH && (
              <label class="row">
                <span class="row-text">
                  <span class="row-title">Search address</span>
                  <span class="row-note">Use {'{searchTerms}'} where the search words go.</span>
                </span>
                <input
                  class="control control-wide"
                  value={draft.searchURL}
                  placeholder="https://example.com/search?q={searchTerms}"
                  onInput={(event) => updateDraft({ searchURL: event.currentTarget.value })}
                />
              </label>
            )}
            <label class="row">
              <span class="row-text">
                <span class="row-title">Page width scale</span>
                <span class="row-note">Above 1, pages are laid out wider than the device screen.</span>
              </span>
              <input
                class="control control-narrow"
                type="number"
                min="1"
                max="3"
                step="0.1"
                placeholder="1"
                value={draft.viewportScale}
                onInput={(event) => updateDraft({ viewportScale: event.currentTarget.value })}
              />
            </label>
            <label class="row">
              <span class="row-text">
                <span class="row-title">Audio quality</span>
                <span class="row-note">Standard uses less bandwidth, for slow or remote connections.</span>
              </span>
              <select
                class="control"
                value={draft.audioQuality}
                onChange={(event) => updateDraft({ audioQuality: event.currentTarget.value })}
              >
                <option value="high">High (48 kHz stereo)</option>
                <option value="standard">Standard (16 kHz mono)</option>
              </select>
            </label>
            <label class="row row-stack">
              <span class="row-title">Custom user agent</span>
              <input
                class="control"
                maxLength={512}
                placeholder={'Leave empty to use Surf’s default'}
                value={draft.userAgent}
                onInput={(event) => updateDraft({ userAgent: event.currentTarget.value })}
              />
            </label>
          </div>
        )}
      </SettingsForm>
    </>
  );
}
