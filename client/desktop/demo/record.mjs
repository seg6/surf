// Real Surf session capture. No gallery fixtures, personal profiles or external
// control listener. Run with Node; artifacts stay under the ignored .local tree.
import { spawn, execFileSync } from 'node:child_process';
import fs from 'node:fs';
import net from 'node:net';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { inspectPage } from './inspect-page.mjs';
import { sceneURL, prepareScene, recordScene } from './scenes.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..');
const mini = process.argv.includes('--mini');
const portrait = process.argv.includes('--portrait');
if (portrait && !mini) throw new Error('--portrait requires --mini');
const inspect = process.argv.includes('--inspect');
const youtube = process.argv.includes('--youtube');
const sync = process.argv.includes('--sync');
const netflix = process.argv.includes('--netflix');
const scene = process.argv.find(arg=>arg.startsWith('--scene='))?.slice(8);
if (scene && (!mini || youtube || sync || netflix)) throw new Error('--scene requires --mini and cannot combine with media flags');
if (scene && !sceneURL[scene]) throw new Error(`Unknown scene: ${scene}`);
if ((youtube || sync || netflix) && !mini) throw new Error('Media scenes require --mini');
if ([youtube,sync,netflix].filter(Boolean).length > 1) throw new Error('Choose one scene at a time');
if (netflix && !inspect) throw new Error('Netflix currently requires --inspect: verify playback before making a public take');
const logical = mini ? (portrait ? [768,1024] : [1024,768]) : [1536,864];
// Current desktop viewport requests are physical pixels. Keep 1:1 here so
// the host page really is mini-sized, rather than just the ImGui controls.
const scale = 1;
const pixels = logical.map(n => n * scale);
const size = pixels.join('x');
const initialURL = process.env.SURF_DEMO_START_URL || sceneURL[scene] || (sync ? `data:text/html;base64,${fs.readFileSync(path.join(root,'client/desktop/demo/sync.html')).toString('base64')}` : netflix ? 'https://www.netflix.com/browse' : youtube ? 'https://www.youtube.com/watch?v=UXqq0ZvbOnk&t=55s' : 'https://en.wikipedia.org/wiki/Jellyfish');
const demoRoot = path.join(root, '.local/demo');
fs.mkdirSync(demoRoot, {recursive:true});
const out = fs.mkdtempSync(path.join(demoRoot, 'take-'));
const bin = path.join(root, 'client/desktop/target/debug');
const serverHome = path.join(out, 'server');
const clientHome = path.join(out, 'client');
const browserProfile = netflix ? path.join(demoRoot,'netflix/profile') : path.join(serverHome,'profile');
if (netflix) {
  if (!fs.statSync(browserProfile).isDirectory()) throw new Error('Create and sign into the dedicated Netflix demo profile first');
  // Never delete Chrome's lock or take over an interactive browser session.
  try { fs.lstatSync(path.join(browserProfile,'SingletonLock')); throw new Error('Close the Netflix demo Chrome completely before recording'); }
  catch (error) { if (error.code !== 'ENOENT') throw error; }
}
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const children = [];
let state = {};
let recording;
let audioModule;
let startTime;
const markers = {};
function start(command, args, env, name, extraPipe = false) {
  const log = fs.createWriteStream(path.join(out, `${name}.log`), {
    fd:fs.openSync(path.join(out, `${name}.log`), 'a', 0o600), autoClose:true});
  const child = spawn(command, args, {cwd:root, env, stdio:['ignore','pipe','pipe', ...(extraPipe ? ['pipe'] : [])]});
  child.stdout.pipe(log, {end:false});
  child.stderr.pipe(log, {end:false});
  child.done = new Promise((resolve, reject) => {
    child.on('error', reject);
    child.on('exit', (code, signal) => { log.end(); resolve({code, signal}); });
  });
  children.push(child);
  return child;
}
async function run(command, args, env = process.env, name = 'command') {
  const result = await start(command, args, env, name).done;
  if (result.code !== 0) throw new Error(`${name} failed; inspect ${out}/${name}.log`);
}
async function until(predicate, label, timeout = 35000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) {
    if (await predicate()) return;
    await sleep(100);
  }
  throw new Error(`Timed out: ${label}; last UI state ${JSON.stringify(state)}`);
}
async function stop(child, signal = 'SIGTERM') {
  if (!child || child.exitCode !== null || child.signalCode !== null) return;
  child.kill(signal);
  const exited = await Promise.race([child.done.then(() => true), sleep(6000).then(() => false)]);
  if (!exited) { child.kill('SIGKILL'); await child.done; }
}
for (const signal of ['SIGINT','SIGTERM']) process.once(signal, () => {
  for (const child of children) if (child.exitCode === null && child.signalCode === null) child.kill('SIGTERM');
});
try {
  fs.mkdirSync(clientHome, {recursive:true, mode:0o700});
  fs.writeFileSync(path.join(clientHome, 'desktop-preferences.json'), JSON.stringify({dark:!mini,bottom:true,mobile:false,device_preset:5,landscape:!portrait}), {mode:0o600});
  const version = fs.readFileSync(path.join(root, 'VERSION'), 'utf8').trim();
  const backend = path.join(out, 'surf');
  await run('go', ['-C', 'backend', 'build', '-ldflags', `-X surf-backend/internal/config.AppVersion=${version}`, '-o', backend, './cmd/surf'], process.env, 'build-backend');
  await run('cargo', ['build','--locked','--manifest-path','client/desktop/Cargo.toml','-p','surf-client','--example','ui-input'], process.env, 'build-input');
  await run('cargo', ['build','--locked','--manifest-path','client/desktop/Cargo.toml','-p','surf-session','--example','probe'], process.env, 'build-pairing');
  await run('cargo', ['build','--locked','--manifest-path','client/desktop/Cargo.toml','-p','surf-client'], process.env, 'build-client');
  const display = start('Xvfb', ['-displayfd','3','-screen','0',`${size}x24`,'-nolisten','tcp'], process.env, 'display', true);
  let number = '';
  display.stdio[3].on('data', data => { number += data; });
  await until(() => /^\d+\n/.test(number), 'private display');
  const port = await new Promise((resolve, reject) => {
    const socket = net.createServer();
    socket.once('error', reject);
    socket.listen(0, '127.0.0.1', () => { const port = socket.address().port; socket.close(() => resolve(port)); });
  });
  const env = {...process.env, DISPLAY:`:${number.trim()}`, WAYLAND_DISPLAY:'',
    XDG_SESSION_TYPE:'x11', WINIT_UNIX_BACKEND:'x11', WINIT_X11_SCALE_FACTOR:String(scale), LIBGL_ALWAYS_SOFTWARE:'1',
    SURF_HOME:serverHome, PROFILE:browserProfile, DOWNLOADS:path.join(serverHome,'downloads'),
    UPLOADS:path.join(serverHome,'uploads'), PORT:String(port), BIND_ADDR:'127.0.0.1',
    SURF_ADVERTISE:'0', SURF_CONTENT_BLOCKER:'0', SURF_SERVER_NAME:'Surf demo',
    START_URL:'about:blank#surf-new', SURF_CLIENT_HOME:clientHome, SURF_UI_SIZE:logical.join('x'), SURF_UI_TRACE:'1'};
  if (netflix) env.CHROME = path.join(root,'client/desktop/demo/chrome-netflix.sh');
  if (mini) {
    env.SURF_UI_TOUCH = '1';
    env.PULSE_SINK = `surf_demo_${path.basename(out).replaceAll('-','_')}`;
    env.PIPEWIRE_PROPS = `target.object=${env.PULSE_SINK}`;
    env.ALSA_CONFIG_PATH = path.join(root,'client/desktop/tests/alsa-headless.conf');
    await run('pactl',['load-module','module-null-sink',`sink_name=${env.PULSE_SINK}`,'rate=48000','channels=2'],process.env,'audio-sink');
    audioModule = fs.readFileSync(path.join(out,'audio-sink.log'),'utf8').trim();
    if (!/^\d+$/.test(audioModule)) throw new Error('No owned audio sink module ID');
  }
  delete env.SURF_UI_GALLERY;
  delete env.SURF_UI_CAPTURE;
  delete env.SURF_SMOKE_EXIT_AFTER_FRAMES;
  // Keep the headless host browser on the host's graphics environment. Only
  // the client/capture uses Xvfb; forcing its software GL onto Chromium can
  // disable WebGL and is not representative of normal Surf operation.
  const browserEnv = {...env, DISPLAY:process.env.DISPLAY || '',
    WAYLAND_DISPLAY:process.env.WAYLAND_DISPLAY || '',
    XDG_SESSION_TYPE:process.env.XDG_SESSION_TYPE || '',
    LIBGL_ALWAYS_SOFTWARE:process.env.LIBGL_ALWAYS_SOFTWARE || ''};
  const server = start(backend, ['serve'], browserEnv, 'server');
  await until(() => fs.existsSync(path.join(serverHome,'daemon.json')), 'backend');
  const pair = start(backend, ['pair'], env, 'pair');
  let code;
  await until(() => {
    code = fs.readFileSync(path.join(out,'pair.log'),'utf8').match(/Pairing code: (\d{6})/)?.[1];
    return code;
  }, 'pairing invitation');
  await run(path.join(bin,'examples/probe'), [`127.0.0.1:${port}`,code,'--confirm'], env, 'pairing-client');
  await stop(pair);
  const client = start(path.join(bin,'surf-client'), [initialURL], env, 'client');
  let lines = '';
  client.stderr.on('data', data => {
    lines += data;
    const complete = lines.split('\n');
    lines = complete.pop();
    for (const line of complete) if (line.startsWith('SURF_UI_STATE ')) {
      try { state = JSON.parse(line.slice(14)); } catch {}
    }
  });
  const drive = (...args) => run(path.join(bin,'examples/ui-input'), args.map(String), env, 'input');
  const point = n => Math.round(n * scale);
  const move = (x,y,ms=400) => drive('move',point(x),point(y),ms);
  const click = (x,y) => drive('click',point(x),point(y));
  const page = async () => {
    const current = state.url && !state.url.startsWith('about:') ? state.url : initialURL;
    const snapshot=await inspectPage(out, new URL(current).hostname, browserProfile);
    fs.writeFileSync(path.join(out,'last-page.json'),JSON.stringify(snapshot,null,2),{mode:0o600});
    return snapshot;
  };
  const pageButton = async pattern => {
    const snapshot = await page();
    const button = snapshot.buttons?.find(b => pattern.test(b.label || '') || pattern.test(b.text || ''));
    if(!button) throw new Error(`Page button not found: ${pattern}`);
    await click(button.x,button.y);
    await sleep(300);
  };
  const miniGeometry = async fullscreen => {
    if (!mini) return;
    const expected = [logical[0], logical[1] - (fullscreen ? 0 : 42)];
    await until(async () => {
      if (state.fullscreen !== fullscreen || state.chrome_visible === fullscreen
          || state.window?.join('x') !== logical.join('x')
          || state.video_dimensions?.join('x') !== expected.join('x')
          || state.viewport?.join('x') !== expected.join('x')) return false;
      return (await page()).viewport?.join('x') === expected.join('x');
    }, `real mini ${fullscreen ? 'fullscreen' : 'browsing'} geometry`);
  };
  const revealVideoControls = async () => {
    const snapshot = await page();
    const rect = snapshot.videos?.find(v => v.rect?.width > 0)?.rect;
    if (!rect) throw new Error('No visible video geometry');
    await click(rect.x + rect.width/2, rect.y + rect.height/2);
    await sleep(300);
  };
  await until(() => state.connected && state.video_ready && state.title && state.url !== 'about:blank#surf-new' && !state.loading, 'initial page loaded');
  if (mini) {
    // PipeWire's ALSA compatibility path can ignore PULSE_SINK. Resolve ownership
    // by the exact child PID, never by a generic application name or default sink.
    const list = kind => JSON.parse(execFileSync('pactl',['-f','json','list',kind],{encoding:'utf8'}));
    let owned;
    await until(() => {
      const clients = list('clients').filter(c => String(c.properties?.['application.process.id']) === String(client.pid));
      owned = list('sink-inputs').filter(s => clients.some(c => String(c.index) === String(s.client)) || String(s.properties?.['application.process.id']) === String(client.pid));
      return owned.length > 0;
    },'owned client audio output');
    for (const stream of owned) await run('pactl',['move-sink-input',String(stream.index),env.PULSE_SINK],process.env,'audio-route');
  }
  // Set this while the client holds the display open; an otherwise empty Xvfb
  // resets root resources when xsetroot disconnects.
  await run('xsetroot', ['-cursor_name','left_ptr'], env, 'cursor');
  await sleep(2000);
  const sceneContext={drive,click,move,page,until,sleep,logical,out,serverHome,state:()=>state,miniGeometry};
  if (scene) await prepareScene(scene,sceneContext);
  if (youtube) {
    // Landscape can clip the consent actions; portrait usually fits them.
    // Reveal only when needed, using the real client rather than a DOM scroll.
    if (!(await page()).buttons?.some(b => b.text === 'Reject all' || b.label === 'Reject all')) {
      await drive('swipe',point(logical[0]*.5),point(logical[1]*.75),point(logical[0]*.5),point(logical[1]*.5),400);
      await sleep(700);
    }
    await pageButton(/^Reject all$/);
    await until(async()=>{const s=await page();return s.videos?.some(v=>!v.paused&&v.ready>=3);},'YouTube playback');
    // Choose a crisp source in the real player, off camera, then return to the
    // normal page for the recorded fullscreen transition.
    await revealVideoControls();
    await pageButton(/^Full screen/);
    await until(async()=>(await page()).fullscreen,'player fullscreen');
    await miniGeometry(true);
    await pageButton(/^Settings$/);
    await pageButton(/Quality/);
    await pageButton(/^1080p/);
    let pausedSince;
    await until(async()=>{
      const snapshot = await page();
      // Quality changes can temporarily pause the media element while the
      // player still intends to play. Give it time; if the state stays stale,
      // synchronize with the real Pause/Play controls before recording.
      if (snapshot.videos?.some(v=>v.paused)) {
        pausedSince ||= Date.now();
        const play = snapshot.buttons?.find(b=>/^Play \(k\)/.test(b.label || ''));
        if (play) { await click(play.x,play.y); await sleep(800); }
        else if(Date.now()-pausedSince>1800) {
          const pause=snapshot.buttons?.find(b=>/^Pause \(k\)/.test(b.label || ''));
          if(pause){await click(pause.x,pause.y);await sleep(800);pausedSince=Date.now();}
        }
        return false;
      }
      return snapshot.videos?.some(v=>v.width>=1920&&v.ready>=3);
    },'1080p source');
    await pageButton(/^Exit full screen/);
    await until(async()=>!(await page()).fullscreen,'normal video page');
    await miniGeometry(false);
    fs.writeFileSync(path.join(out,'preflight.json'),JSON.stringify(await page(),null,2));
    await sleep(1500);
  }
  await move(mini?logical[0]*.83:1180,mini?logical[1]*.73:560,0);
  fs.writeFileSync(path.join(out,'runtime.json'),JSON.stringify({display:env.DISPLAY,scale,logical,pixels,sink:env.PULSE_SINK,browserProfile}),{mode:0o600});
  console.log(`Ready: ${out} on ${env.DISPLAY}`);
  const audio = mini ? ['-thread_queue_size','512','-f','pulse','-i',`${env.PULSE_SINK}.monitor`,'-c:a','pcm_s16le'] : ['-an'];
  recording = start('ffmpeg', ['-hide_banner','-y','-thread_queue_size','512','-f','x11grab','-draw_mouse','0','-framerate','60',
    '-video_size',size,'-i',`${env.DISPLAY}+0,0`,...audio,'-c:v','libx264','-preset','veryfast',
    '-crf','12','-threads','4','-pix_fmt','yuv420p',path.join(out,'capture.mkv')], env, 'capture');
  startTime = performance.now();
  const mark = name => { markers[name] = (performance.now() - startTime) / 1000; console.log(`Shot: ${name}`); };
  await sleep(700);
  mark('reading');
  if (inspect) {
    console.log('Inspection capture: up to 180 seconds; create inspect.done in the take directory to finish.');
    const inspectionEnd = Date.now() + 180000;
    while (Date.now() < inspectionEnd && !fs.existsSync(path.join(out,'inspect.done'))) await sleep(250);
    mark('end');
    await stop(recording,'SIGINT');
    fs.writeFileSync(path.join(out,'markers.json'), JSON.stringify({version,mini,scale,logical,pixels,markers,state},null,2));
  } else if (mini) {
    if (scene) {
      await recordScene(scene,{...sceneContext,mark});
    } else if (sync) {
      mark('sync');
      await click(logical[0]/2,(logical[1]-42)/2);
      await sleep(10000);
    } else if (youtube) {
      mark('video');
      await sleep(3500);
      await revealVideoControls();
      await pageButton(/^Full screen/);
      await until(async()=>(await page()).fullscreen,'recorded fullscreen transition');
      await miniGeometry(true);
      fs.writeFileSync(path.join(out,'fullscreen-page.json'),JSON.stringify(await page(),null,2),{mode:0o600});
      mark('fullscreen');
      await move(20,20,300);
      await sleep(8500);
    } else {
      await miniGeometry(false);
      const before = await page();
      await sleep(1200);
      await drive('swipe',point(logical[0]*.55),point(logical[1]*.75),point(logical[0]*.55),point(logical[1]*.45),480);
      await sleep(3500);
      const after = await page();
      if (after.scroll[1] <= before.scroll[1] + 20) throw new Error('Native swipe did not scroll the page');
      fs.writeFileSync(path.join(out,'swipe.json'),JSON.stringify({before:before.scroll,after:after.scroll}),{mode:0o600});
    }
    mark('end');
    await stop(recording,'SIGINT');
    fs.writeFileSync(path.join(out,'markers.json'), JSON.stringify({version,mini,scale,logical,pixels,markers,state},null,2));
  } else {
  await sleep(1700);
  await drive('scroll',5,170);
  await sleep(2200);
  mark('new_tab');
  await drive('key','ctrl+t');
  await sleep(700);
  await drive('key','ctrl+l');
  await drive('paste','https://webglsamples.org/aquarium/aquarium.html');
  await drive('key','Return');
  await until(() => /aquarium/.test(state.url || '') && !state.loading && !state.editing, 'WebGL page loaded');
  await sleep(2500);
  mark('webgl');
  await drive('move',1140,610,600);
  await sleep(5300);
  mark('end');
  await stop(recording,'SIGINT');
  fs.writeFileSync(path.join(out,'markers.json'), JSON.stringify({version,markers,state},null,2));
  }
  console.log(`Capture: ${out}`);
} finally {
  await stop(recording,'SIGINT');
  for (const child of [...children].reverse()) await stop(child);
  if (audioModule && /^\d+$/.test(audioModule)) await run('pactl',['unload-module',audioModule],process.env,'audio-cleanup');
  console.log(`Artifacts retained: ${out}`);
}
