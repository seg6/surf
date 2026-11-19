export type Appearance = 'system' | 'light' | 'dark';

export const APPEARANCE_KEY = 'surf-dashboard-appearance';
export const APPEARANCES: { value: Appearance; label: string }[] = [
  { value: 'system', label: 'System' },
  { value: 'light', label: 'Light' },
  { value: 'dark', label: 'Dark' },
];

export function parseAppearance(value: unknown): Appearance {
  return value === 'light' || value === 'dark' || value === 'system' ? value : 'system';
}

export function resolveTheme(preference: Appearance, systemDark: boolean): 'light' | 'dark' {
  return preference === 'system' ? (systemDark ? 'dark' : 'light') : preference;
}

// Storage can be unavailable in private browsing modes, so every access is guarded.
export function loadAppearance(): Appearance {
  try {
    return parseAppearance(localStorage.getItem(APPEARANCE_KEY));
  } catch {
    return 'system';
  }
}

export function saveAppearance(value: Appearance): void {
  try {
    localStorage.setItem(APPEARANCE_KEY, value);
  } catch {
    /* the choice still applies for this visit */
  }
}

/** Applies the theme to the document. This changes only the dashboard. */
export function applyAppearance(preference: Appearance, systemDark: boolean): void {
  document.documentElement.dataset.theme = resolveTheme(preference, systemDark);
}
