import type { BrowserMode, ClipboardState, UpdateStatus } from './types';

const SWITCHING = ['starting', 'opening', 'resuming'];

export function isSwitching(mode: BrowserMode | null): boolean {
  return Boolean(mode && SWITCHING.includes(mode.state));
}

export interface EngineStatus {
  tone: 'ok' | 'warn' | 'bad' | 'idle';
  text: string;
}

/** The one-line engine status shown in the toolbar. */
export function engineStatus(online: boolean | null, mode: BrowserMode | null): EngineStatus {
  if (online === null) return { tone: 'idle', text: 'Connecting…' };
  if (!online) return { tone: 'bad', text: 'Backend unavailable' };
  if (mode && mode.state !== 'streaming') {
    if (mode.standalone) return { tone: 'warn', text: 'Standalone browser setup' };
    if (isSwitching(mode)) return { tone: 'warn', text: 'Switching browser…' };
    return { tone: 'warn', text: 'Browser setup, devices paused' };
  }
  return { tone: 'ok', text: 'Online and ready' };
}

export const DEFAULT_BROWSER_COPY =
  'Use Surf’s browser on this computer to sign in to sites or change browser settings. ' +
  'Connected devices show a paused screen until you close it.';

export function browserModeCopy(mode: BrowserMode | null): string {
  if (!mode) return DEFAULT_BROWSER_COPY;
  if (mode.standalone) {
    return 'Standalone browser setup is active. Close all setup windows to finish; no streaming server is running.';
  }
  if (mode.state === 'streaming') return mode.message || DEFAULT_BROWSER_COPY;
  return `Browser on ${mode.host}. ${mode.message}`;
}

export interface BrowserControls {
  openDisabled: boolean;
  openLabel: string;
  showResume: boolean;
  resumeDisabled: boolean;
  showForce: boolean;
}

export function browserControls(mode: BrowserMode | null): BrowserControls {
  if (!mode) {
    return { openDisabled: true, openLabel: 'Open Browser Here', showResume: false, resumeDisabled: true, showForce: false };
  }
  const setup = mode.state === 'setup';
  const switching = isSwitching(mode);
  return {
    openDisabled: switching || mode.state === 'failed' || Boolean(mode.canForce),
    openLabel: setup ? 'Show Setup Browser' : 'Open Browser Here',
    showResume: !mode.standalone && (setup || mode.state === 'failed'),
    resumeDisabled: switching,
    showForce: !mode.standalone && Boolean(mode.canForce),
  };
}

export interface UpdateView {
  text: string;
  label: string;
  disabled: boolean;
  action: 'check' | 'apply';
}

export function updateView(status: UpdateStatus | null): UpdateView {
  const installed = status?.current ? `Surf ${status.current} is installed.` : 'Surf is installed.';
  const state = status?.state ?? 'idle';
  const busy = ['checking', 'downloading', 'applying'].includes(state);
  switch (state) {
    case 'available':
      return {
        text: status?.version ? `Surf ${status.version} is available.` : 'An update is available.',
        label: `Install ${status?.version ?? 'Update'}`,
        disabled: false,
        action: 'apply',
      };
    case 'checking':
      return { text: 'Checking for updates…', label: 'Checking…', disabled: true, action: 'check' };
    case 'downloading':
      return { text: 'Downloading the update…', label: 'Downloading…', disabled: true, action: 'check' };
    case 'applying':
      return { text: 'Installing the update…', label: 'Installing…', disabled: true, action: 'check' };
    case 'current':
      return { text: 'Surf is up to date.', label: 'Check Again', disabled: busy, action: 'check' };
    case 'error':
      return {
        text: `Could not check for updates.${status?.message ? ` ${status.message}` : ''}`,
        label: 'Check Again',
        disabled: false,
        action: 'check',
      };
    default:
      return { text: installed, label: 'Check for Updates', disabled: false, action: 'check' };
  }
}

export function clipboardMessage(state: ClipboardState | null): string {
  if (!state) return 'Checking clipboard support…';
  if (!state.enabled) return 'Two-way sync is off. Use Send once for temporary text.';
  if (state.error) return `Client sync is live; ${state.system} could not access the host clipboard (${state.error}).`;
  if (state.available) return `Two-way sync is live through ${state.system}.`;
  return 'Sync is live between Surf clients; this host has no system clipboard provider.';
}
