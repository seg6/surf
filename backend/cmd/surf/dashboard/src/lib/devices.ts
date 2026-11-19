import type { Candidate, Device } from './types';

/** Go encodes an unset time as year 1; treat that, and bad input, as missing. */
export function validDate(value: string | undefined): Date | null {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime()) || date.getUTCFullYear() <= 1) return null;
  return date;
}

export function formatDateTime(value: string | undefined): string {
  const date = validDate(value);
  return date ? date.toLocaleString() : '';
}

export function deviceName(device: Device | Candidate): string {
  return device.name || ('deviceName' in device && device.deviceName) || 'Unnamed device';
}

/** What the right-hand side of a device row says. */
export function deviceStatus(device: Device, online: boolean): { online: boolean; text: string } {
  if (online) return { online: true, text: 'Connected' };
  const seen = formatDateTime(device.lastSeen);
  return { online: false, text: seen ? `Last seen ${seen}` : 'Not seen yet' };
}

export function phraseWords(phrase: string | undefined): string[] {
  return (phrase || '').split(/\s+/).filter(Boolean);
}
