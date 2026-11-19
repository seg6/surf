import { afterEach, describe, expect, it, vi } from 'vitest';
import { interruptionHeaders, request } from './api';
import { parseAppearance, resolveTheme } from './appearance';
import { deviceStatus, phraseWords, validDate } from './devices';
import {
  applyLogUpdate, buildLogSources, filterLogRecords, formatLogTime, logQuery, logRecordParts,
} from './logs';
import { computeFps, totalDrops } from './metrics';
import { CUSTOM_SEARCH, presetForURL, SEARCH_PRESETS } from './search';
import { draftFromConfig, draftsEqual, settingsFormData } from './settings';
import { browserControls, browserModeCopy, clipboardMessage, engineStatus, updateView } from './view';

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubFetch(handler: (path: string) => Response | Promise<Response>) {
  vi.stubGlobal('fetch', vi.fn((path: string) => Promise.resolve(handler(path))));
}

const json = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } });

describe('search presets', () => {
  it('matches known templates and treats everything else as custom', () => {
    expect(presetForURL('')).toBe('');
    expect(presetForURL(undefined)).toBe('');
    expect(presetForURL(SEARCH_PRESETS[2]?.value)).toBe(SEARCH_PRESETS[2]?.value);
    expect(presetForURL('https://example.com/?q={searchTerms}')).toBe(CUSTOM_SEARCH);
  });
});

describe('log helpers', () => {
  const records = [
    { ts: '2026-11-19T21:42:06Z', level: 'info', component: 'video', message: 'client subscribed', fields: { id: 'd1' } },
    { ts: '2026-11-19T21:42:07Z', level: 'warn', component: 'audio', message: 'dropped', fields: { count: 3 } },
  ];

  it('filters by any part of the record, ignoring case and outer spaces', () => {
    expect(filterLogRecords(records, '  AUDIO ')).toEqual([records[1]]);
    expect(filterLogRecords(records, 'd1')).toEqual([records[0]]);
    expect(filterLogRecords(records, '')).toBe(records);
    expect(filterLogRecords(records, 'nothing')).toEqual([]);
  });

  it('splits a record and hides fields already present in the message', () => {
    expect(logRecordParts(records[1])).toMatchObject({ level: 'WARN', component: 'audio', fields: 'count=3' });
    expect(logRecordParts({ message: 'id=d1 seen', fields: { id: 'd1', b: { x: 1 } } }).fields).toBe('b={"x":1}');
    expect(logRecordParts(undefined)).toMatchObject({ level: 'INFO', component: 'app', message: '' });
  });

  it('formats times and leaves bad input alone', () => {
    expect(formatLogTime('not a time')).toBe('not a time');
    expect(formatLogTime('')).toBe('');
    expect(formatLogTime('2026-11-19T21:42:06Z')).toMatch(/^\d\d:\d\d:\d\d$/);
  });

  it('builds sources with device online state and stream parameters', () => {
    const sources = buildLogSources({ devices: [{ id: 'a', name: 'iPad' }, { id: 'b' }], online: { a: true } });
    expect(sources.map((source) => source.label)).toEqual([
      'Server', 'Desktop', 'iPad \u00b7 online', 'Device \u00b7 offline',
    ]);
    expect(logQuery(sources[0]!).toString()).toBe('source=server');
    expect(logQuery(sources[2]!).toString()).toBe('source=device&deviceID=a');
  });

  it('replaces the buffer on reset and appends otherwise', () => {
    expect(applyLogUpdate(records, { reset: true, records: [records[0]!] })).toEqual([records[0]]);
    expect(applyLogUpdate([records[0]!], { records: [records[1]!] })).toEqual(records);
    expect(applyLogUpdate(records, { reset: true })).toEqual([]);
  });
});

describe('metrics', () => {
  it('computes frames per second from two samples', () => {
    expect(computeFps(null, { accessUnits: 10, at: 0 })).toBeNull();
    expect(computeFps({ accessUnits: 100, at: 0 }, { accessUnits: 220, at: 2000 })).toBe(60);
    expect(computeFps({ accessUnits: 100, at: 0 }, { accessUnits: 50, at: 2000 })).toBeNull();
    expect(computeFps({ accessUnits: 1, at: 5 }, { accessUnits: 2, at: 5 })).toBeNull();
  });

  it('adds video and audio drops', () => {
    expect(totalDrops({ transport_video_drops: 2, transport_audio_drops: 3 })).toBe(5);
    expect(totalDrops({})).toBe(0);
  });
});

describe('settings draft', () => {
  const config = {
    baseURL: 'https://127.0.0.1:18080', port: 18080, serverName: 'desk', startAtLogin: true,
    searchURL: 'https://example.com/?q={searchTerms}', viewportScale: '1.5',
  };

  it('starts from the saved configuration', () => {
    const draft = draftFromConfig(config);
    expect(draft).toMatchObject({ serverName: 'desk', port: '18080', searchPreset: CUSTOM_SEARCH, viewportScale: '1.5' });
    expect(draftFromConfig({ ...config, serverName: '' }).serverName).toBe('Surf');
    expect(draftsEqual(draft, draftFromConfig(config))).toBe(true);
    expect(draftsEqual(draft, { ...draft, port: '1' })).toBe(false);
  });

  it('encodes the form the way /settings expects', () => {
    const form = settingsFormData(draftFromConfig(config));
    expect(form.get('serverName')).toBe('desk');
    expect(form.get('startAtLogin')).toBe('on');
    expect(settingsFormData({ ...draftFromConfig(config), startAtLogin: false }).has('startAtLogin')).toBe(false);
  });
});

describe('devices', () => {
  it('treats Go zero times as missing', () => {
    expect(validDate('0001-01-01T00:00:00Z')).toBeNull();
    expect(validDate('')).toBeNull();
    expect(validDate('2026-01-02T03:04:05Z')).toBeInstanceOf(Date);
  });

  it('describes connection state', () => {
    expect(deviceStatus({ id: 'a' }, true)).toEqual({ online: true, text: 'Connected' });
    expect(deviceStatus({ id: 'a' }, false).text).toBe('Not seen yet');
    expect(deviceStatus({ id: 'a', lastSeen: '2026-01-02T03:04:05Z' }, false).text).toMatch(/^Last seen /);
  });

  it('splits the verification phrase into words', () => {
    expect(phraseWords('harbor maple  tide')).toEqual(['harbor', 'maple', 'tide']);
    expect(phraseWords(undefined)).toEqual([]);
  });
});

describe('appearance', () => {
  it('falls back to system and resolves the theme', () => {
    expect(parseAppearance('dark')).toBe('dark');
    expect(parseAppearance('purple')).toBe('system');
    expect(resolveTheme('system', true)).toBe('dark');
    expect(resolveTheme('light', true)).toBe('light');
  });
});

describe('view text', () => {
  it('reports engine status', () => {
    expect(engineStatus(null, null).text).toBe('Connecting\u2026');
    expect(engineStatus(false, null).tone).toBe('bad');
    expect(engineStatus(true, { state: 'setup', revision: 1 }).text).toBe('Browser setup, devices paused');
    expect(engineStatus(true, { state: 'opening', revision: 1 }).text).toBe('Switching browser\u2026');
    expect(engineStatus(true, { state: 'streaming', revision: 1 }).text).toBe('Online and ready');
  });

  it('chooses browser setup controls', () => {
    expect(browserControls({ state: 'streaming', revision: 1 })).toMatchObject({ openDisabled: false, showResume: false });
    expect(browserControls({ state: 'setup', revision: 1 })).toMatchObject({ openLabel: 'Show Setup Browser', showResume: true });
    expect(browserControls({ state: 'setup', revision: 1, canForce: true })).toMatchObject({ openDisabled: true, showForce: true });
    expect(browserControls({ state: 'setup', revision: 1, standalone: true })).toMatchObject({ showResume: false, showForce: false });
    expect(browserControls({ state: 'resuming', revision: 1 })).toMatchObject({ openDisabled: true });
  });

  it('describes browser mode', () => {
    expect(browserModeCopy({ state: 'setup', revision: 1, host: 'desk', message: 'Close it.' })).toBe('Browser on desk. Close it.');
    expect(browserModeCopy({ state: 'streaming', revision: 1, message: 'Custom.' })).toBe('Custom.');
  });

  it('describes updates', () => {
    expect(updateView({ state: 'available', version: '0.20.0' })).toMatchObject({ action: 'apply', label: 'Install 0.20.0' });
    expect(updateView({ state: 'checking' })).toMatchObject({ disabled: true });
    expect(updateView({ state: 'error', message: 'offline' }).text).toBe('Could not check for updates. offline');
    expect(updateView({ state: 'idle', current: '0.19.0' }).text).toBe('Surf 0.19.0 is installed.');
  });

  it('describes clipboard sync', () => {
    expect(clipboardMessage(null)).toBe('Checking clipboard support\u2026');
    expect(clipboardMessage({ enabled: false })).toContain('off');
    expect(clipboardMessage({ enabled: true, available: true, system: 'wl-clipboard' })).toBe(
      'Two-way sync is live through wl-clipboard.',
    );
    expect(clipboardMessage({ enabled: true, error: 'denied', system: 'x' })).toContain('denied');
    expect(clipboardMessage({ enabled: true })).toContain('no system clipboard provider');
  });
});

describe('api helpers', () => {
  it('throws the response text, or the status when it is empty', async () => {
    stubFetch(() => new Response('Port must be between 1 and 65535.\n', { status: 400 }));
    await expect(request('/settings')).rejects.toThrow('Port must be between 1 and 65535.');
    stubFetch(() => new Response('', { status: 502 }));
    await expect(request('/x')).rejects.toThrow('HTTP 502');
  });

  it('sends only the desktop header while streaming', async () => {
    stubFetch(() => json({ state: 'streaming', revision: 3 }));
    const confirm = vi.fn();
    expect(await interruptionHeaders(confirm)).toEqual({ 'X-Surf-Desktop': '1' });
    expect(confirm).not.toHaveBeenCalled();
  });

  it('adds the browser revision after the user agrees to close the setup browser', async () => {
    stubFetch(() => json({ state: 'setup', revision: 7 }));
    expect(await interruptionHeaders(() => Promise.resolve(true))).toEqual({
      'X-Surf-Desktop': '1', 'X-Surf-Browser-Revision': '7',
    });
    await expect(interruptionHeaders(() => Promise.resolve(false))).rejects.toThrow('Canceled; browser unchanged.');
  });

  it('refuses while standalone setup is active', async () => {
    stubFetch(() => json({ state: 'setup', revision: 1, standalone: true }));
    await expect(interruptionHeaders(() => Promise.resolve(true))).rejects.toThrow('Close standalone browser setup first.');
  });

  it('still proceeds when the backend cannot answer', async () => {
    stubFetch(() => new Response('Surf backend is not ready', { status: 502 }));
    expect(await interruptionHeaders(() => Promise.resolve(false))).toEqual({ 'X-Surf-Desktop': '1' });
  });
});
