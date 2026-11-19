import type { Stats } from './types';

export interface Sample {
  accessUnits: number;
  at: number;
}

export function sampleOf(stats: Stats, at: number): Sample {
  return { accessUnits: Number(stats.video_access_units || 0), at };
}

/** Frames per second between two samples, or null when it cannot be computed. */
export function computeFps(previous: Sample | null, current: Sample): number | null {
  if (!previous) return null;
  const elapsed = current.at - previous.at;
  if (current.accessUnits < previous.accessUnits || elapsed <= 0) return null;
  return ((current.accessUnits - previous.accessUnits) * 1000) / elapsed;
}

export function totalDrops(stats: Stats): number {
  return Number(stats.transport_video_drops || 0) + Number(stats.transport_audio_drops || 0);
}
