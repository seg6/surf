import { useState } from 'preact/hooks';
import { useDashboard } from '../state';
import { Switch } from '../components/Switch';
import { clipboardMessage } from '../lib/view';

export function Clipboard() {
  const { clipboard, clipboardPending, setClipboardSync, sendClipboard } = useDashboard();
  const [text, setText] = useState('');
  const [sending, setSending] = useState(false);
  const enabled = Boolean(clipboard?.enabled);

  const send = async () => {
    const value = text;
    setText(''); // the text is never kept
    setSending(true);
    try {
      await sendClipboard(value);
    } finally {
      setSending(false);
    }
  };

  return (
    <>
      <div class="panel panel-pad row-flex">
        <span class="row-text">
          <span class="row-title" id="sync-label">Two-way sync</span>
          <span class="row-note">
            Text copied here appears on connected devices, and text copied on a device appears here.
          </span>
        </span>
        <Switch
          checked={enabled}
          labelledBy="sync-label"
          disabled={clipboardPending || clipboard === null}
          onChange={(next) => void setClipboardSync(next)}
        />
      </div>
      <p class="status-line" role="status" aria-live="polite">
        <span class={`dot${enabled ? ' dot-ok' : ''}`} aria-hidden="true" />
        {clipboardMessage(clipboard)}
      </p>

      {!enabled && (
        <>
          <h2 class="section-title">Send once</h2>
          <div class="panel panel-pad stack">
            <label class="visually-hidden" htmlFor="send-once">Text to send</label>
            <textarea
              id="send-once"
              class="control textarea"
              rows={4}
              autocomplete="off"
              spellcheck={false}
              placeholder="Paste a password or other text to send to connected devices"
              value={text}
              onInput={(event) => setText(event.currentTarget.value)}
            />
            <div class="row-flex">
              <span class="row-note grow">
                Text is never saved. The device clears it after two minutes unless its clipboard changes.
              </span>
              <button type="button" class="btn" disabled={sending} onClick={() => void send()}>
                Send to Devices
              </button>
            </div>
          </div>
        </>
      )}
    </>
  );
}
