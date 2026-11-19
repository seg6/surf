import { createContext } from 'preact';
import type { ComponentChildren } from 'preact';
import { useCallback, useContext, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { api, interruptionHeaders } from './lib/api';
import { computeFps, sampleOf } from './lib/metrics';
import type { Sample } from './lib/metrics';
import { draftFromConfig, draftsEqual, settingsFormData } from './lib/settings';
import type { SettingsDraft } from './lib/settings';
import type {
  BrowserMode, Candidate, ClipboardState, Config, Device, PairingSession, Stats, UpdateStatus,
} from './lib/types';
import { useUI } from './components/ui';

export type Page = 'overview' | 'devices' | 'pair' | 'browser' | 'connection' | 'clipboard' | 'logs';

const PAGES: Page[] = ['overview', 'devices', 'pair', 'browser', 'connection', 'clipboard', 'logs'];
const DISMISS_KEY = 'surf-dashboard-autostart-dismissed';

const message = (error: unknown) => (error instanceof Error ? error.message : String(error));

/** Runs `task` now and again `ms` after each run finishes. Overlapping runs never happen. */
function usePoll(task: () => Promise<void> | void, ms: number): void {
  const latest = useRef(task);
  latest.current = task;
  useEffect(() => {
    let stopped = false;
    let timer: number | undefined;
    const tick = async () => {
      try {
        await latest.current();
      } catch {
        /* each task reports its own errors */
      }
      if (!stopped) timer = window.setTimeout(tick, ms);
    };
    void tick();
    return () => {
      stopped = true;
      window.clearTimeout(timer);
    };
  }, [ms]);
}

function pageFromHash(): Page | null {
  const name = location.hash.replace(/^#/, '');
  if (name === 'pair') return 'devices'; // an invitation is only started by a click
  return PAGES.includes(name as Page) ? (name as Page) : null;
}

function readDismissed(): boolean {
  try {
    return localStorage.getItem(DISMISS_KEY) === '1';
  } catch {
    return false;
  }
}

export interface Dashboard {
  page: Page;
  go: (page: Page) => void;

  config: Config | null;
  draft: SettingsDraft | null;
  dirty: boolean;
  saving: boolean;
  updateDraft: (patch: Partial<SettingsDraft>) => void;
  saveSettings: () => Promise<void>;
  showAutostartNotice: boolean;
  dismissAutostartNotice: () => void;

  online: boolean | null;
  stats: Stats | null;
  fps: number | null;
  mode: BrowserMode | null;
  browserBusy: boolean;
  browserAction: (action: 'open' | 'resume', force?: boolean) => Promise<void>;
  restart: () => Promise<void>;

  update: UpdateStatus | null;
  runUpdate: (action: 'check' | 'apply') => Promise<void>;

  devices: Device[] | null;
  devicesError: string;
  onlineDevices: Record<string, boolean>;
  revokeDevice: (device: Device) => Promise<void>;

  session: PairingSession | null;
  pairingBusy: boolean;
  openPairing: () => Promise<void>;
  cancelPairing: () => Promise<void>;
  candidates: Candidate[];
  candidatesError: string;
  rejectCandidate: (id: string) => Promise<void>;

  clipboard: ClipboardState | null;
  clipboardPending: boolean;
  setClipboardSync: (enabled: boolean) => Promise<void>;
  sendClipboard: (text: string) => Promise<void>;

  copy: (value: string, notice?: string) => Promise<void>;
}

const Context = createContext<Dashboard | null>(null);

export function useDashboard(): Dashboard {
  const value = useContext(Context);
  if (!value) throw new Error('useDashboard must be used inside DashboardProvider');
  return value;
}

export function DashboardProvider({ children }: { children: ComponentChildren }) {
  const { toast, confirm } = useUI();

  const [page, setPage] = useState<Page>(() => pageFromHash() ?? 'overview');
  const pageRef = useRef(page);
  pageRef.current = page;

  const go = useCallback((next: Page) => {
    setPage(next);
    try {
      history.replaceState(null, '', `#${next}`);
    } catch {
      /* the page still changes */
    }
  }, []);

  // Engine status
  const [online, setOnline] = useState<boolean | null>(null);
  const [stats, setStats] = useState<Stats | null>(null);
  const [fps, setFps] = useState<number | null>(null);
  const [mode, setMode] = useState<BrowserMode | null>(null);
  const previousSample = useRef<Sample | null>(null);

  const pollStatus = useCallback(async () => {
    try {
      const next = await api.status();
      const sample = sampleOf(next, performance.now());
      const rate = computeFps(previousSample.current, sample);
      previousSample.current = sample;
      setStats(next);
      setOnline(true);
      if (rate !== null) setFps(rate);
      if (next.browserMode) setMode(next.browserMode);
    } catch {
      setOnline(false);
    }
  }, []);

  const markRestarting = useCallback(() => {
    setOnline(false);
    previousSample.current = null;
    setFps(null);
  }, []);

  // Configuration and the settings draft
  const [config, setConfig] = useState<Config | null>(null);
  const [saved, setSaved] = useState<SettingsDraft | null>(null);
  const [draft, setDraft] = useState<SettingsDraft | null>(null);
  const [saving, setSaving] = useState(false);
  const [dismissed, setDismissed] = useState(readDismissed);

  const loadConfig = useCallback(async () => {
    const next = await api.config();
    const fresh = draftFromConfig(next);
    setConfig(next);
    setSaved(fresh);
    setDraft(fresh);
  }, []);

  const dirty = Boolean(draft && saved && !draftsEqual(draft, saved));
  const updateDraft = useCallback((patch: Partial<SettingsDraft>) => {
    setDraft((current) => (current ? { ...current, ...patch } : current));
  }, []);

  const confirmClose = useCallback(
    (text: string) =>
      confirm({ title: 'Close the setup browser?', message: text, confirmLabel: 'Close and Continue', danger: true }),
    [confirm],
  );

  const saveSettings = useCallback(async () => {
    if (!draft) return;
    setSaving(true);
    try {
      const headers = await interruptionHeaders(confirmClose);
      await api.saveSettings(settingsFormData(draft), headers);
      await loadConfig();
      toast('Settings saved. The engine is restarting.');
      markRestarting();
    } catch (error) {
      toast(message(error));
    } finally {
      setSaving(false);
    }
  }, [draft, confirmClose, loadConfig, toast, markRestarting]);

  const dismissAutostartNotice = useCallback(() => {
    setDismissed(true);
    try {
      localStorage.setItem(DISMISS_KEY, '1');
    } catch {
      /* dismissed for this visit only */
    }
  }, []);

  // Engine actions
  const restart = useCallback(async () => {
    try {
      await api.restart(await interruptionHeaders(confirmClose));
      toast('The engine is restarting.');
      markRestarting();
    } catch (error) {
      toast(message(error));
    }
  }, [confirmClose, toast, markRestarting]);

  const [browserBusy, setBrowserBusy] = useState(false);
  const browserAction = useCallback(
    async (action: 'open' | 'resume', force = false) => {
      setBrowserBusy(true);
      try {
        const current = await api.browser();
        setMode(current);
        const focus = action === 'open' && current.state === 'setup';
        if (!focus) {
          const ok = await confirm(
            force
              ? {
                  title: 'Force close the browser?',
                  message: 'Force close Surf’s unresponsive browser? Recent profile changes and unsaved work may be lost.',
                  confirmLabel: 'Force Close',
                  danger: true,
                }
              : action === 'open'
                ? {
                    title: 'Open the browser on this computer?',
                    message:
                      'Open your Surf tabs on this computer? Connected devices will pause. Pages reload; unsaved work and active transfers may be lost.',
                    confirmLabel: 'Open Browser',
                  }
                : {
                    title: 'Resume on devices?',
                    message:
                      'Close the browser on this computer and resume on devices? Pages reload; unsaved work and active transfers may be lost.',
                    confirmLabel: 'Close and Resume',
                  },
          );
          if (!ok) return;
        }
        await api.browserAction(action, current.revision, force);
        await pollStatus();
      } catch (error) {
        toast(message(error));
        await pollStatus();
      } finally {
        setBrowserBusy(false);
      }
    },
    [confirm, pollStatus, toast],
  );

  // Updates
  const [update, setUpdate] = useState<UpdateStatus | null>(null);
  const pollUpdate = useCallback(async () => {
    try {
      setUpdate(await api.updateStatus());
    } catch {
      /* keep the last known state */
    }
  }, []);

  const runUpdate = useCallback(
    async (action: 'check' | 'apply') => {
      try {
        const headers = action === 'apply' ? await interruptionHeaders(confirmClose) : { 'X-Surf-Desktop': '1' };
        await api.updateAction(action, headers);
        await pollUpdate();
      } catch (error) {
        toast(message(error));
      }
    },
    [confirmClose, pollUpdate, toast],
  );

  // Devices
  const [devices, setDevices] = useState<Device[] | null>(null);
  const [devicesError, setDevicesError] = useState('');
  const [onlineDevices, setOnlineDevices] = useState<Record<string, boolean>>({});

  const loadDevices = useCallback(async () => {
    try {
      const payload = await api.devices();
      setDevices(payload.devices || []);
      setDevicesError('');
    } catch (error) {
      setDevicesError(message(error));
    }
    try {
      setOnlineDevices((await api.logSources()).online || {});
    } catch {
      /* connection state is a nicety; the list still works */
    }
  }, []);

  // Pairing
  const [session, setSession] = useState<PairingSession | null>(null);
  const sessionRef = useRef(session);
  sessionRef.current = session;
  const [pairingBusy, setPairingBusy] = useState(false);
  const [candidates, setCandidates] = useState<Candidate[]>([]);
  const [candidatesError, setCandidatesError] = useState('');

  const loadCandidates = useCallback(async () => {
    try {
      if (sessionRef.current?.active) {
        const current = await api.pairingSession();
        if (current.active) {
          setSession(current);
        } else {
          setSession(null);
          if (pageRef.current === 'pair') go('devices');
        }
      }
      const payload = await api.candidates();
      setCandidates((payload.candidates || []).filter((candidate) => !candidate.paired));
      setCandidatesError('');
    } catch (error) {
      setCandidatesError(message(error));
    }
  }, [go]);

  const openPairing = useCallback(async () => {
    go('pair');
    setPairingBusy(true);
    try {
      let current: PairingSession | null = null;
      try {
        current = await api.pairingSession();
      } catch {
        current = null;
      }
      if (!current?.active) {
        const address = String(config?.publicAddress || config?.lanURL || '').trim();
        current = await api.startPairing(address);
      }
      setSession(current);
      await loadCandidates();
    } catch (error) {
      toast(message(error));
      go('devices');
    } finally {
      setPairingBusy(false);
    }
  }, [config, go, loadCandidates, toast]);

  const cancelPairing = useCallback(async () => {
    try {
      await api.stopPairing();
    } catch (error) {
      toast(message(error));
      return;
    }
    setSession(null);
    go('devices');
    await loadCandidates();
  }, [go, loadCandidates, toast]);

  const rejectCandidate = useCallback(
    async (id: string) => {
      try {
        await api.rejectCandidate(id);
        toast('Pairing cancelled');
        setSession(null);
        if (pageRef.current === 'pair') go('devices');
        await loadCandidates();
        await loadDevices();
      } catch (error) {
        toast(message(error));
      }
    },
    [go, loadCandidates, loadDevices, toast],
  );

  const revokeDevice = useCallback(
    async (device: Device) => {
      const ok = await confirm({
        title: `Revoke ${device.name || 'this device'}?`,
        message: 'A revoked device has to pair again before it can connect.',
        confirmLabel: 'Revoke',
        danger: true,
      });
      if (!ok) return;
      try {
        await api.revokeDevice(device.id);
        toast('Device revoked');
        await loadDevices();
        await loadCandidates();
      } catch (error) {
        toast(message(error));
      }
    },
    [confirm, loadCandidates, loadDevices, toast],
  );

  // Clipboard
  const [clipboard, setClipboard] = useState<ClipboardState | null>(null);
  const [clipboardPending, setClipboardPending] = useState(false);

  const setClipboardSync = useCallback(
    async (enabled: boolean) => {
      setClipboardPending(true);
      try {
        const state = await api.setClipboardSync(enabled);
        setClipboard(state);
        toast(state.enabled ? 'Two-way clipboard sync enabled' : 'Two-way clipboard sync disabled');
      } catch (error) {
        toast(message(error));
      } finally {
        setClipboardPending(false);
      }
    },
    [toast],
  );

  const sendClipboard = useCallback(
    async (text: string) => {
      try {
        if (!text) throw new Error('Enter some text first');
        const result = await api.sendClipboard(text);
        if (!result.ok) {
          throw new Error(`Sent to ${result.delivered.length}; ${result.failed.length} device(s) did not acknowledge`);
        }
        if (!result.delivered.length) throw new Error('No Surf device is connected');
        toast(`Sent once to ${result.delivered.length} connected device(s)`);
      } catch (error) {
        toast(message(error));
      }
    },
    [toast],
  );

  const copy = useCallback(
    async (value: string, notice = 'Copied to clipboard') => {
      try {
        await navigator.clipboard.writeText(value);
      } catch {
        // Plain-HTTP or unfocused contexts reject the async API.
        const area = document.createElement('textarea');
        area.value = value;
        document.body.appendChild(area);
        area.select();
        document.execCommand('copy');
        area.remove();
      }
      toast(notice);
    },
    [toast],
  );

  // Startup and the polling loops
  usePoll(pollStatus, 2000);
  usePoll(pollUpdate, 2000);
  usePoll(loadDevices, 5000);
  usePoll(loadCandidates, 1500);

  // The tray opens the dashboard with a hash to request one action. Each is run
  // once and then replaced by the plain page name, so a reload does not repeat it.
  const runHash = useCallback(async () => {
    const hash = location.hash;
    if (hash === '#browser-setup-open' || hash === '#browser-resume') {
      go('browser');
      await browserAction(hash === '#browser-resume' ? 'resume' : 'open');
    } else if (hash === '#restart-engine') {
      go('overview');
      await restart();
    } else {
      const next = pageFromHash();
      if (next) setPage(next);
    }
  }, [browserAction, go, restart]);

  useEffect(() => {
    void (async () => {
      try {
        await loadConfig();
      } catch (error) {
        toast(message(error));
      }
      try {
        const current = await api.pairingSession();
        if (current.active) setSession(current);
      } catch {
        /* no invitation is open */
      }
      try {
        setClipboard(await api.clipboard());
      } catch (error) {
        toast(message(error));
      }
      await pollStatus();
      await runHash();
    })();
    const onHash = () => void runHash();
    window.addEventListener('hashchange', onHash);
    return () => window.removeEventListener('hashchange', onHash);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const value = useMemo<Dashboard>(
    () => ({
      page, go,
      config, draft, dirty, saving, updateDraft, saveSettings,
      showAutostartNotice: Boolean(config) && !config?.startupChoiceMade && !dismissed,
      dismissAutostartNotice,
      online, stats, fps, mode, browserBusy, browserAction, restart,
      update, runUpdate,
      devices, devicesError, onlineDevices, revokeDevice,
      session, pairingBusy, openPairing, cancelPairing, candidates, candidatesError, rejectCandidate,
      clipboard, clipboardPending, setClipboardSync, sendClipboard,
      copy,
    }),
    [
      page, go, config, draft, dirty, saving, updateDraft, saveSettings, dismissed, dismissAutostartNotice,
      online, stats, fps, mode, browserBusy, browserAction, restart, update, runUpdate,
      devices, devicesError, onlineDevices, revokeDevice,
      session, pairingBusy, openPairing, cancelPairing, candidates, candidatesError, rejectCandidate,
      clipboard, clipboardPending, setClipboardSync, sendClipboard, copy,
    ],
  );

  return <Context.Provider value={value}>{children}</Context.Provider>;
}
