import type {
  BrowserMode, ClipboardResult, ClipboardState, Candidate, Config, Device,
  LogSourcesPayload, PairingSession, Stats, UpdateStatus,
} from './types';

export const ADMIN = '/api/backend/api/v1/admin';

/** Marks a request as coming from the dashboard; the host rejects mutations without it. */
export const DESKTOP_HEADER = { 'X-Surf-Desktop': '1' } as const;
const JSON_HEADERS = { 'Content-Type': 'application/json', ...DESKTOP_HEADER };

export type Confirm = (message: string) => Promise<boolean>;

/** Fetches a path and throws the response text (or the status) when it is not ok. */
export async function request(path: string, init?: RequestInit): Promise<Response> {
  const response = await fetch(path, init);
  if (!response.ok) {
    throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
  }
  return response;
}

export async function getJSON<T>(path: string): Promise<T> {
  return (await request(path)).json() as Promise<T>;
}

function sendJSON<T>(path: string, method: string, body?: unknown): Promise<T> {
  return request(path, {
    method,
    headers: JSON_HEADERS,
    body: body === undefined ? undefined : JSON.stringify(body),
  }).then((response) => response.json() as Promise<T>);
}

/**
 * Headers for actions that stop the engine. When Surf's browser is open on the
 * desktop, the user must agree to close it, and the host verifies the revision
 * before stopping anything. A stopped backend cannot answer, so that case
 * proceeds with the plain header.
 */
export async function interruptionHeaders(confirm: Confirm): Promise<Record<string, string>> {
  const headers: Record<string, string> = { ...DESKTOP_HEADER };
  let mode: BrowserMode;
  try {
    mode = await getJSON<BrowserMode>(`${ADMIN}/browser`);
  } catch {
    return headers;
  }
  if (mode.state !== 'streaming') {
    if (mode.standalone) throw new Error('Close standalone browser setup first.');
    const ok = await confirm(
      'This will close the setup browser. Unsaved work and active transfers may be lost. Continue?',
    );
    if (!ok) throw new Error('Canceled; browser unchanged.');
    headers['X-Surf-Browser-Revision'] = String(mode.revision);
  }
  return headers;
}

export const api = {
  config: () => getJSON<Config>('/api/config'),
  status: () => getJSON<Stats>('/api/status'),
  updateStatus: () => getJSON<UpdateStatus>('/api/update/status'),
  updateAction: (action: 'check' | 'apply', headers: Record<string, string>) =>
    request(`/api/update/${action}`, { method: 'POST', headers }),
  restart: (headers: Record<string, string>) => request('/api/restart', { method: 'POST', headers }),
  saveSettings: (body: FormData, headers: Record<string, string>) =>
    request('/settings', { method: 'POST', headers, body }),

  browser: () => getJSON<BrowserMode>(`${ADMIN}/browser`),
  browserAction: (action: 'open' | 'resume', revision: number, force: boolean) =>
    request(`${ADMIN}/browser`, {
      method: 'POST',
      headers: JSON_HEADERS,
      body: JSON.stringify({ action, revision, force }),
    }),

  clipboard: () => getJSON<ClipboardState>(`${ADMIN}/clipboard`),
  setClipboardSync: (enabled: boolean) =>
    sendJSON<ClipboardState>(`${ADMIN}/clipboard/sync`, 'PUT', { enabled }),
  sendClipboard: (text: string) => sendJSON<ClipboardResult>(`${ADMIN}/clipboard`, 'POST', { text }),

  devices: () => getJSON<{ devices?: Device[] }>(`${ADMIN}/devices`),
  revokeDevice: (id: string) =>
    request(`${ADMIN}/devices/revoke/${encodeURIComponent(id)}`, { method: 'POST', headers: DESKTOP_HEADER }),

  pairingSession: () => getJSON<PairingSession>(`${ADMIN}/pairing/session`),
  startPairing: (publicAddress: string) =>
    sendJSON<PairingSession>(`${ADMIN}/pairing/session`, 'POST', { publicAddress }),
  stopPairing: () => request(`${ADMIN}/pairing/session`, { method: 'DELETE', headers: DESKTOP_HEADER }),
  candidates: () => getJSON<{ candidates?: Candidate[] }>(`${ADMIN}/pairing/candidates`),
  rejectCandidate: (id: string) =>
    request(`${ADMIN}/pairing/reject/${encodeURIComponent(id)}`, { method: 'POST', headers: DESKTOP_HEADER }),

  logSources: () => getJSON<LogSourcesPayload>(`${ADMIN}/logs/sources`),
  clearLogs: (params: URLSearchParams) =>
    request(`${ADMIN}/logs?${params}`, { method: 'DELETE', headers: DESKTOP_HEADER }).then(
      (response) => response.json() as Promise<{ pending?: boolean }>,
    ),
  logStreamURL: (params: URLSearchParams) => `${ADMIN}/logs/stream?${params}`,
};
