// Development-only stand-in for the Go host's management server, used by
// `vite dev` and `vite preview`. It is imported by vite.config.ts only, so it
// never reaches the production bundle.
import { readFileSync } from 'node:fs';
import type { IncomingMessage, ServerResponse } from 'node:http';
import { fileURLToPath } from 'node:url';
import type { Plugin } from 'vite';

type Json = Record<string, unknown>;

const iconPath = fileURLToPath(new URL('../../surf-icon.png', import.meta.url));
const ADMIN = '/api/backend/api/v1/admin';

const world = {
  config: {
    serverName: 'living-room-pc',
    publicAddress: '',
    baseURL: 'https://127.0.0.1:18080',
    lanURL: 'https://192.168.1.20:18080',
    port: 18080,
    running: true,
    startAtLogin: true,
    startupChoiceMade: false,
    searchURL: 'https://duckduckgo.com/?q={searchTerms}',
    userAgent: '',
    viewportScale: '1.5',
  } as Json,
  update: { state: 'idle', version: '', message: '', current: '0.19.0' } as Json,
  browser: { state: 'streaming', revision: 4, host: 'living-room-pc', message: '', standalone: false, canForce: false } as Json,
  clipboard: { enabled: false, available: true, system: 'wl-clipboard' } as Json,
  devices: [
    { id: 'd1', name: 'iPad mini', pairedAt: '2026-10-02T18:20:00Z', lastSeen: '2026-11-19T20:41:00Z' },
    { id: 'd2', name: 'Linux desktop', pairedAt: '2026-09-11T09:02:00Z', lastSeen: '2026-11-18T22:10:00Z' },
    { id: 'd3', name: 'Never connected', pairedAt: '2026-11-19T08:00:00Z', lastSeen: '0001-01-01T00:00:00Z' },
  ] as Json[],
  online: { d1: true } as Record<string, boolean>,
  session: { active: false } as Json,
  candidates: [] as Json[],
  frames: 0,
  logs: [] as Json[],
};

const levels = ['info', 'info', 'info', 'warn', 'debug', 'error'];
const components = ['server', 'video', 'audio', 'capture', 'auth'];
function seedLogs() {
  const base = Date.parse('2026-11-19T21:42:06Z');
  for (let i = 0; i < 40; i++) {
    world.logs.push({
      ts: new Date(base + i * 7300).toISOString(),
      level: levels[i % levels.length],
      component: components[i % components.length],
      message: ['surf server listening with TLS on 0.0.0.0:18080', 'client subscribed', 'capture surface resized to 768x1024', 'client paused video', 'client resumed video'][i % 5],
      fields: i % 3 === 0 ? { client: 'd1', width: 768, height: 1024 } : {},
    });
  }
}
seedLogs();

function send(res: ServerResponse, body: unknown, status = 200) {
  res.statusCode = status;
  res.setHeader('Content-Type', 'application/json');
  res.end(JSON.stringify(body));
}

async function readBody(req: IncomingMessage): Promise<string> {
  const chunks: Buffer[] = [];
  for await (const chunk of req) chunks.push(chunk as Buffer);
  return Buffer.concat(chunks).toString('utf8');
}

// A small valid QR-like PNG is not needed; a 1x1 transparent PNG keeps the image slot honest.
const pixel = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=';

async function handle(req: IncomingMessage, res: ServerResponse): Promise<boolean> {
  const url = new URL(req.url ?? '/', 'http://localhost');
  const path = url.pathname;
  const method = req.method ?? 'GET';

  if (path === '/icon.png') {
    res.setHeader('Content-Type', 'image/png');
    res.end(readFileSync(iconPath));
    return true;
  }
  if (path === '/api/config') return send(res, world.config), true;
  if (path === '/api/status') {
    world.frames += 120;
    return send(res, {
      browserMode: world.browser, video_access_units: world.frames, clients: world.online.d1 ? 1 : 0,
      tabs: 4, transport_video_drops: 0, transport_audio_drops: 0, casting: true, videoSubs: 1, audioSubs: 1,
    }), true;
  }
  if (path === '/api/update/status') return send(res, world.update), true;
  if (path === '/api/update/check') {
    world.update = { ...world.update, state: 'available', version: '0.20.0' };
    return res.writeHead(204).end(), true;
  }
  if (path === '/api/update/apply' || path === '/api/restart') return res.writeHead(204).end(), true;
  if (path === '/settings') {
    await readBody(req);
    world.config.startupChoiceMade = true;
    return res.writeHead(204).end(), true;
  }

  if (path === `${ADMIN}/browser`) {
    if (method === 'POST') {
      const body = JSON.parse(await readBody(req)) as { action: string };
      world.browser = body.action === 'open'
        ? { ...world.browser, state: 'setup', host: 'living-room-pc', message: 'Close all setup windows to resume.', revision: 5 }
        : { ...world.browser, state: 'streaming', message: '', revision: 6 };
      return res.writeHead(204).end(), true;
    }
    return send(res, world.browser), true;
  }
  if (path === `${ADMIN}/clipboard/sync`) {
    world.clipboard.enabled = (JSON.parse(await readBody(req)) as { enabled: boolean }).enabled;
    return send(res, world.clipboard), true;
  }
  if (path === `${ADMIN}/clipboard`) {
    if (method === 'POST') return send(res, { ok: true, delivered: ['d1'], failed: [] }), true;
    return send(res, world.clipboard), true;
  }
  if (path === `${ADMIN}/devices`) return send(res, { devices: world.devices }), true;
  if (path.startsWith(`${ADMIN}/devices/revoke/`)) {
    const id = decodeURIComponent(path.split('/').pop() ?? '');
    world.devices = world.devices.filter((device) => device.id !== id);
    return res.writeHead(204).end(), true;
  }
  if (path === `${ADMIN}/pairing/session`) {
    if (method === 'POST') world.session = { active: true, code: '482913', publicAddress: '192.168.1.20:18080', qrPNG: pixel };
    if (method === 'DELETE') {
      world.session = { active: false };
      return res.writeHead(204).end(), true;
    }
    return send(res, world.session), true;
  }
  if (path === `${ADMIN}/pairing/candidates`) return send(res, { candidates: world.candidates }), true;
  if (path.startsWith(`${ADMIN}/pairing/reject/`)) {
    world.candidates = [];
    world.session = { active: false };
    return res.writeHead(204).end(), true;
  }
  if (path === `${ADMIN}/logs/sources`) {
    return send(res, { devices: world.devices.map(({ id, name }) => ({ id, name })), online: world.online }), true;
  }
  if (path === `${ADMIN}/logs/stream`) {
    res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-store' });
    res.write(`data: ${JSON.stringify({ reset: true, records: world.logs })}\n\n`);
    const timer = setInterval(() => {
      const record = { ts: new Date().toISOString(), level: 'info', component: 'video', message: 'client resumed video', fields: {} };
      res.write(`data: ${JSON.stringify({ records: [record] })}\n\n`);
    }, 4000);
    req.on('close', () => clearInterval(timer));
    return true;
  }
  if (path === `${ADMIN}/logs` && method === 'DELETE') return send(res, { pending: false }), true;

  // Test hooks for looking at states by hand. The real UI has no equivalent.
  if (path === '/__mock/candidate') {
    world.candidates = [{ id: 'c1', deviceID: 'c1', deviceName: 'PS Vita', phrase: 'harbor maple tide finch quartz luna', requestedAt: new Date().toISOString(), paired: false }];
    return send(res, { ok: true }), true;
  }
  if (path === '/__mock/browser') {
    world.browser = { ...world.browser, state: 'setup', message: 'Close all setup windows to resume.', canForce: url.searchParams.get('force') === '1' };
    return send(res, world.browser), true;
  }
  if (path === '/__mock/offline-update') {
    world.update = { ...world.update, state: 'error', message: 'The update server did not answer.' };
    return send(res, world.update), true;
  }
  return false;
}

export function mockApi(): Plugin {
  const middleware = (req: IncomingMessage, res: ServerResponse, next: () => void) => {
    handle(req, res).then(
      (handled) => {
        if (!handled) next();
      },
      (error: unknown) => send(res, { error: String(error) }, 500),
    );
  };
  return {
    name: 'surf-dashboard-mock-api',
    configureServer: (server) => void server.middlewares.use(middleware),
    configurePreviewServer: (server) => void server.middlewares.use(middleware),
  };
}
