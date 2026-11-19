import type { Config } from './types';
import { presetForURL } from './search';

/** Everything the settings form edits, held as the strings the inputs show. */
export interface SettingsDraft {
  serverName: string;
  port: string;
  publicAddress: string;
  searchPreset: string;
  searchURL: string;
  viewportScale: string;
  userAgent: string;
  audioQuality: string;
  startAtLogin: boolean;
}

export function draftFromConfig(config: Config): SettingsDraft {
  const searchURL = config.searchURL || '';
  return {
    serverName: config.serverName || 'Surf',
    port: String(config.port),
    publicAddress: config.publicAddress || '',
    searchPreset: presetForURL(searchURL),
    searchURL,
    viewportScale: config.viewportScale ? String(config.viewportScale) : '',
    userAgent: config.userAgent || '',
    audioQuality: config.audioQuality === 'standard' ? 'standard' : 'high',
    startAtLogin: Boolean(config.startAtLogin),
  };
}

/** The multipart body `/settings` expects. A checked box is sent as "on". */
export function settingsFormData(draft: SettingsDraft): FormData {
  const form = new FormData();
  form.set('serverName', draft.serverName);
  form.set('port', draft.port);
  form.set('publicAddress', draft.publicAddress);
  form.set('searchURL', draft.searchURL);
  form.set('viewportScale', draft.viewportScale);
  form.set('userAgent', draft.userAgent);
  form.set('audioQuality', draft.audioQuality);
  if (draft.startAtLogin) form.set('startAtLogin', 'on');
  return form;
}

export function draftsEqual(a: SettingsDraft, b: SettingsDraft): boolean {
  return (Object.keys(a) as (keyof SettingsDraft)[]).every((key) => a[key] === b[key]);
}
