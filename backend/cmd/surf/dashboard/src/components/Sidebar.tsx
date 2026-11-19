import { useRef } from 'preact/hooks';
import { useDashboard } from '../state';
import type { Page } from '../state';
import { APPEARANCES } from '../lib/appearance';
import type { Appearance } from '../lib/appearance';

type SectionId = Exclude<Page, 'pair'>;

interface Section {
  id: SectionId;
  label: string;
  /** Glossy icon background, light to dark. */
  tint: [string, string];
  glyph: string;
}

export const SECTIONS: Section[] = [
  { id: 'overview', label: 'Overview', tint: ['#7BDA6E', '#2EA437'], glyph: 'M4 13h4v7H4zM10 8h4v12h-4zM16 4h4v16h-4z' },
  { id: 'devices', label: 'Devices', tint: ['#6DB9F8', '#1C74DB'], glyph: 'M3 5h12v10H3zM7 19h4M17 9h4v10h-4z' },
  {
    id: 'browser', label: 'Browser', tint: ['#FFBF5E', '#F07B0C'],
    glyph: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18',
  },
  {
    id: 'connection', label: 'Connection', tint: ['#B4B4B4', '#6B6B6B'],
    glyph: 'M5 12.5a10 10 0 0 1 14 0M8.5 16a5 5 0 0 1 7 0M12 19.5h.01M2 9a14 14 0 0 1 20 0',
  },
  {
    id: 'clipboard', label: 'Clipboard', tint: ['#C29BF5', '#7A46D6'],
    glyph: 'M8 3h8v4H8zM16 5h2a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2h2',
  },
  { id: 'logs', label: 'Logs', tint: ['#7C848E', '#3B424A'], glyph: 'M4 6h16M4 12h16M4 18h10' },
];

interface SidebarProps {
  appearance: Appearance;
  onAppearance: (value: Appearance) => void;
}

export function Sidebar({ appearance, onAppearance }: SidebarProps) {
  const { page, go, devices } = useDashboard();
  const list = useRef<HTMLUListElement>(null);
  const current: SectionId = page === 'pair' ? 'devices' : page;

  // Up and Down move between sections; Tab still leaves the list.
  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return;
    const buttons = [...(list.current?.querySelectorAll<HTMLButtonElement>('button') ?? [])];
    const index = buttons.indexOf(document.activeElement as HTMLButtonElement);
    if (index < 0) return;
    event.preventDefault();
    const step = event.key === 'ArrowDown' ? 1 : -1;
    buttons[(index + step + buttons.length) % buttons.length]?.focus();
  };

  return (
    <aside class="sidebar">
      <nav aria-label="Sections">
        <ul ref={list} class="nav-list" onKeyDown={onKeyDown}>
          {SECTIONS.map((section) => {
            const badge = section.id === 'devices' && devices ? String(devices.length) : '';
            return (
              <li key={section.id}>
                <button
                  type="button"
                  class="nav-item"
                  aria-current={section.id === current ? 'page' : undefined}
                  onClick={() => go(section.id)}
                >
                  <span
                    class="nav-icon"
                    aria-hidden="true"
                    style={{ background: `linear-gradient(${section.tint[0]}, ${section.tint[1]})` }}
                  >
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                      stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                      <path d={section.glyph} />
                    </svg>
                  </span>
                  <span class="nav-label">{section.label}</span>
                  {badge && <span class="nav-badge">{badge}</span>}
                </button>
              </li>
            );
          })}
        </ul>
      </nav>
      <div class="appearance" role="group" aria-labelledby="appearance-label">
        <p id="appearance-label" class="appearance-label">Appearance</p>
        <div class="segmented">
          {APPEARANCES.map((option) => (
            <button
              key={option.value}
              type="button"
              aria-pressed={appearance === option.value}
              onClick={() => onAppearance(option.value)}
            >
              {option.label}
            </button>
          ))}
        </div>
      </div>
    </aside>
  );
}
