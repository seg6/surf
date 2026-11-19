import type { LogRecord, LogSource, LogSourcesPayload } from './types';

export interface LogParts {
  timestamp: string;
  level: string;
  component: string;
  message: string;
  fields: string;
}

export function logRecordParts(record: LogRecord | undefined): LogParts {
  const message = String(record?.message || '');
  const fields = Object.entries(record?.fields || {})
    .sort(([a], [b]) => a.localeCompare(b))
    .filter(([key]) => !message.includes(`${key}=`))
    .map(([key, value]) => `${key}=${typeof value === 'string' ? value : JSON.stringify(value)}`)
    .join(' ');
  return {
    timestamp: String(record?.ts || ''),
    level: String(record?.level || 'info').toUpperCase(),
    component: String(record?.component || 'app'),
    message,
    fields,
  };
}

/** Case-insensitive match against the whole serialized record. */
export function filterLogRecords(records: LogRecord[], query: string): LogRecord[] {
  const needle = query.trim().toLowerCase();
  if (!needle) return records;
  return records.filter((record) => JSON.stringify(record).toLowerCase().includes(needle));
}

/** Local HH:MM:SS for an ISO timestamp. Unparseable input is shown unchanged. */
export function formatLogTime(timestamp: string): string {
  const date = new Date(timestamp);
  if (!timestamp || Number.isNaN(date.getTime())) return timestamp;
  const two = (value: number) => String(value).padStart(2, '0');
  return `${two(date.getHours())}:${two(date.getMinutes())}:${two(date.getSeconds())}`;
}

export function buildLogSources(payload: LogSourcesPayload): LogSource[] {
  return [
    { id: 'server', label: 'Server', source: 'server' },
    { id: 'desktop', label: 'Desktop', source: 'desktop' },
    ...(payload.devices || []).map((device): LogSource => ({
      id: `device:${device.id}`,
      label: `${device.name || 'Device'}${payload.online?.[device.id] ? ' · online' : ' · offline'}`,
      source: 'device',
      deviceID: device.id,
    })),
  ];
}

export function logQuery(source: LogSource): URLSearchParams {
  const params = new URLSearchParams({ source: source.source || source.id });
  if (source.deviceID) params.set('deviceID', source.deviceID);
  return params;
}

/** Applies a stream message: a reset replaces the buffer, otherwise it appends. */
export function applyLogUpdate(
  current: LogRecord[],
  update: { reset?: boolean; records?: LogRecord[] },
): LogRecord[] {
  return update.reset ? update.records || [] : current.concat(update.records || []);
}
