import { useEffect, useRef, useState } from 'preact/hooks';
import { DashboardProvider, useDashboard } from './state';
import type { Page } from './state';
import { UIProvider } from './components/ui';
import { Header } from './components/Header';
import { Sidebar } from './components/Sidebar';
import { PairingDialog } from './components/PairingDialog';
import { Overview } from './pages/Overview';
import { Devices } from './pages/Devices';
import { Pair } from './pages/Pair';
import { Browser } from './pages/Browser';
import { Connection } from './pages/Connection';
import { Clipboard } from './pages/Clipboard';
import { Logs } from './pages/Logs';
import { applyAppearance, loadAppearance, saveAppearance } from './lib/appearance';
import type { Appearance } from './lib/appearance';

const TITLES: Record<Page, string> = {
  overview: 'Overview',
  devices: 'Devices',
  pair: 'Pair a New Device',
  browser: 'Browser',
  connection: 'Connection',
  clipboard: 'Clipboard',
  logs: 'Logs',
};

function PageBody({ page }: { page: Page }) {
  switch (page) {
    case 'devices': return <Devices />;
    case 'pair': return <Pair />;
    case 'browser': return <Browser />;
    case 'connection': return <Connection />;
    case 'clipboard': return <Clipboard />;
    case 'logs': return <Logs />;
    default: return <Overview />;
  }
}

function Shell() {
  const { page, go, showAutostartNotice, dismissAutostartNotice } = useDashboard();
  const [appearance, setAppearance] = useState<Appearance>(loadAppearance);
  const heading = useRef<HTMLHeadingElement>(null);
  const first = useRef(true);

  // The inline script in index.html sets the theme before first paint; this
  // keeps it in step with the setting and with the operating system.
  useEffect(() => {
    const query = window.matchMedia('(prefers-color-scheme: dark)');
    const apply = () => applyAppearance(appearance, query.matches);
    apply();
    query.addEventListener('change', apply);
    return () => query.removeEventListener('change', apply);
  }, [appearance]);

  const chooseAppearance = (value: Appearance) => {
    saveAppearance(value);
    setAppearance(value);
  };

  // Announce a section change by moving focus to its heading.
  useEffect(() => {
    document.title = `${TITLES[page]} - Surf`;
    if (first.current) {
      first.current = false;
      return;
    }
    heading.current?.focus();
  }, [page]);

  return (
    <div class="app">
      <Header />
      <div class="layout">
        <Sidebar appearance={appearance} onAppearance={chooseAppearance} />
        <main class="main">
          <div class={`content${page === 'logs' ? ' content-wide' : ''}`}>
            <div class="page-head">
              {page === 'pair' && (
                <button type="button" class="btn btn-pill" onClick={() => go('devices')}>
                  <span aria-hidden="true">‹ </span>Devices
                </button>
              )}
              <h1 ref={heading} tabIndex={-1}>{TITLES[page]}</h1>
            </div>
            {showAutostartNotice && (
              <section class="notice" aria-labelledby="autostart-title">
                <div>
                  <h2 id="autostart-title" class="notice-title">Start Surf automatically?</h2>
                  <p>Clients can connect while Surf is running. You can choose this under Connection.</p>
                </div>
                <div class="actions">
                  {page !== 'connection' && (
                    <button type="button" class="btn" onClick={() => go('connection')}>
                      Open Connection
                    </button>
                  )}
                  <button type="button" class="btn" onClick={dismissAutostartNotice}>
                    Dismiss
                  </button>
                </div>
              </section>
            )}
            <PageBody page={page} />
          </div>
        </main>
      </div>
      <PairingDialog />
    </div>
  );
}

export function App() {
  return (
    <UIProvider>
      <DashboardProvider>
        <Shell />
      </DashboardProvider>
    </UIProvider>
  );
}
