export interface SearchPreset {
  label: string;
  /** The search template, or '' for Chromium's default. */
  value: string;
}

export const CUSTOM_SEARCH = 'custom';

export const SEARCH_PRESETS: SearchPreset[] = [
  { label: 'Chromium’s default', value: '' },
  { label: 'Google', value: 'https://www.google.com/search?q={searchTerms}' },
  { label: 'DuckDuckGo', value: 'https://duckduckgo.com/?q={searchTerms}' },
  { label: 'Bing', value: 'https://www.bing.com/search?q={searchTerms}' },
  { label: 'Startpage', value: 'https://www.startpage.com/do/search?q={searchTerms}' },
];

/** Picks the menu entry for a saved search address; anything unknown is custom. */
export function presetForURL(url: string | undefined): string {
  const value = url ?? '';
  return SEARCH_PRESETS.some((preset) => preset.value === value) ? value : CUSTOM_SEARCH;
}
