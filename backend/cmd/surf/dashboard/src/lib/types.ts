// Shapes returned by the host's management server. These mirror the payloads
// the previous single-file dashboard consumed; nothing here is new.

export interface Config {
  serverName?: string;
  publicAddress?: string;
  baseURL: string;
  lanURL?: string;
  lanURLs?: string[];
  port: number;
  running?: boolean;
  startAtLogin?: boolean;
  startupChoiceMade?: boolean;
  searchURL?: string;
  userAgent?: string;
  viewportScale?: string | number;
  audioQuality?: string;
}

export type BrowserState =
  | 'streaming'
  | 'starting'
  | 'opening'
  | 'resuming'
  | 'setup'
  | 'failed'
  | string;

export interface BrowserMode {
  state: BrowserState;
  revision: number;
  host?: string;
  message?: string;
  standalone?: boolean;
  canForce?: boolean;
}

export interface Stats {
  browserMode?: BrowserMode;
  video_access_units?: number;
  transport_video_drops?: number;
  transport_audio_drops?: number;
  clients?: number;
  tabs?: number;
  casting?: boolean;
  videoSubs?: number;
  audioSubs?: number;
}

export type UpdateState =
  | 'idle'
  | 'checking'
  | 'downloading'
  | 'applying'
  | 'available'
  | 'current'
  | 'error'
  | string;

export interface UpdateStatus {
  state: UpdateState;
  version?: string;
  message?: string;
  current?: string;
}

export interface Device {
  id: string;
  name?: string;
  pairedAt?: string;
  lastSeen?: string;
}

export interface Candidate {
  id: string;
  deviceID?: string;
  deviceName?: string;
  name?: string;
  phrase?: string;
  requestedAt?: string;
  paired?: boolean;
}

export interface PairingSession {
  active?: boolean;
  code?: string;
  publicAddress?: string;
  qrPNG?: string;
}

export interface ClipboardState {
  enabled?: boolean;
  available?: boolean;
  system?: string;
  error?: string;
}

export interface ClipboardResult {
  ok: boolean;
  delivered: unknown[];
  failed: unknown[];
}

export interface LogRecord {
  ts?: string;
  level?: string;
  component?: string;
  message?: string;
  fields?: Record<string, unknown>;
}

export interface LogUpdate {
  reset?: boolean;
  records?: LogRecord[];
}

export interface LogSourcesPayload {
  devices?: { id: string; name?: string }[];
  online?: Record<string, boolean>;
}

export interface LogSource {
  id: string;
  label: string;
  source: 'server' | 'desktop' | 'device';
  deviceID?: string;
}
