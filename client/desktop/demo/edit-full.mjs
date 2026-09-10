// Reproducible, normal-speed repository demo. Private take paths live in an
// ignored manifest; only the explicitly selected intervals reach the export.
import fs from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..');
const manifestPath = process.argv[2];
if (!manifestPath) throw new Error('Usage: edit-full.mjs MANIFEST.json');
const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
if (!Array.isArray(manifest.scenes) || !manifest.scenes.length) throw new Error('No scenes');
const out = fs.mkdtempSync(path.join(root, '.local/demo/full-'));
const fonts = path.join(root, 'client/desktop/assets/fonts');
const escape = value => value.replaceAll('\\', '\\\\').replaceAll(':', '\\:').replaceAll("'", '’');
const drawText = (value, size, x, y, color = '0x202529', weight = 'Regular') =>
  `drawtext=fontfile='${escape(path.join(fonts, `Inter-${weight}.ttf`))}':text='${escape(value)}':fontsize=${size}:x=${x}:y=${y}:fontcolor=${color}`;
function run(program, args, capture = false) {
  const result = spawnSync(program, args, {stdio: capture ? 'pipe' : 'inherit', encoding: 'utf8'});
  if (result.error || result.status !== 0) throw new Error(`${program} failed: ${result.error || result.stderr || result.status}`);
  return result.stdout;
}
const filters = [], inputs = [], chapters = [];
let total = 0;
manifest.scenes.forEach((scene, i) => {
  const take = path.resolve(root, scene.take);
  const data = JSON.parse(fs.readFileSync(path.join(take, 'markers.json'), 'utf8'));
  if (!data.mini || data.logical?.join('x') !== '768x1024' || data.pixels?.join('x') !== '768x1024') throw new Error(`Not a native portrait take: ${scene.take}`);
  const start = scene.start ?? data.markers.reading;
  const end = scene.end ?? data.markers.end;
  let duration = end - start;
  if (!Number.isFinite(start) || start < 0 || !Number.isFinite(duration) || duration <= 0 || end > data.markers.end + 0.1) throw new Error(`Invalid trim: ${scene.take}`);
  const source = path.join(take, 'capture.mkv');
  const probe = JSON.parse(run('ffprobe', ['-v', 'error', '-show_streams', '-show_format', '-of', 'json', source], true));
  const video = probe.streams.find(s => s.codec_type === 'video');
  if (video?.width !== 768 || video?.height !== 1024 || !probe.streams.some(s => s.codec_type === 'audio') || Number(probe.format.duration) + 1 / 60 < end) throw new Error(`Invalid capture: ${scene.take}`);
  // The wall-clock stop marker may land between recorded frames. Trim to a
  // whole number of real frames, rather than inventing a final held frame.
  duration = Math.floor((Math.min(end, Number(probe.format.duration)) - start) * 60) / 60;
  inputs.push('-ss', String(start), '-t', String(duration), '-i', source);
  // Apply chapter text before concatenation, so there are no boundary overlaps.
  const copy = [drawText(scene.title, 32, 832, 490, '0x202529', 'Medium')];
  (scene.lines || []).forEach((line, n) => copy.push(drawText(line, 22, 832, 550 + n * 34, '0x60696f')));
  if (scene.credit) copy.push(drawText(scene.credit, 13, 48, 1044, '0x60696f'));
  filters.push(`[${i}:v]setpts=PTS-STARTPTS,setsar=1,fps=60,scale=720:960:flags=lanczos,pad=1280:1080:48:60:color=0xf5f4f0,${copy.join(',')}[v${i}]`);
  const gain = Number(scene.gainDB || 0);
  if (!Number.isFinite(gain) || Math.abs(gain) > 12) throw new Error('Unexpected audio gain');
  filters.push(`[${i}:a]asetpts=PTS-STARTPTS,aresample=48000,volume=${gain}dB,apad,atrim=duration=${duration},afade=t=in:d=0.06,afade=t=out:st=${Math.max(0, duration - .12)}:d=0.12[a${i}]`);
  chapters.push({start: total, end: total + duration, title: scene.title, lines: scene.lines || [], credit: scene.credit || ''});
  total += duration;
});
const n = manifest.scenes.length;
filters.push(`${manifest.scenes.map((_, i) => `[v${i}][a${i}]`).join('')}concat=n=${n}:v=1:a=1[sequence][audio]`);
filters.push(`[sequence]drawbox=x=47:y=59:w=722:h=962:color=0xd9d9d3:t=1,${[
  drawText('Surf', 52, 908, 87, '0x202529', 'SemiBold'),
  drawText('For legacy Apple devices', 30, 832, 265, '0x202529', 'Medium'),
  drawText('iPhone, iPad and iPod touch.', 22, 832, 312, '0x60696f'),
  'drawbox=x=832:y=430:w=400:h=1:color=0xd9d9d3:t=fill',
  drawText('github.com/seg6/surf', 20, 832, 997, '0x1473b8'),
].join(',')}[frame]`);
filters.push(`[${n}:v]scale=64:64[logo];[frame][logo]overlay=832:84:shortest=1,format=yuv420p[out]`);
const videoPath = path.join(out, 'surf-demo.mp4');
console.log(`Exporting ${total.toFixed(2)} seconds to ${videoPath}`);
run('ffmpeg', ['-hide_banner', '-loglevel', 'warning', '-y', ...inputs,
  '-loop', '1', '-i', path.join(root, 'backend/cmd/surf/surf-icon.png'),
  '-filter_complex_threads', '4', '-filter_complex', filters.join(';'),
  '-map', '[out]', '-map', '[audio]', '-t', String(total),
  '-c:v', 'libx264', '-preset', 'medium', '-crf', '19', '-threads', '6',
  '-c:a', 'aac', '-b:a', '192k', '-movflags', '+faststart', videoPath]);
run('ffmpeg', ['-hide_banner', '-loglevel', 'error', '-y', '-ss', '1', '-i', videoPath, '-frames:v', '1', '-update', '1', path.join(out, 'poster.png')]);
fs.writeFileSync(path.join(out, 'chapters.json'), JSON.stringify(chapters, null, 2));
fs.copyFileSync(path.join(root, 'client/desktop/demo/CREDITS.md'), path.join(out, 'CREDITS.md'));
fs.writeFileSync(path.join(out, 'edit-manifest.private.json'), JSON.stringify(manifest, null, 2), {mode: 0o600});
console.log(`Finished: ${videoPath}`);
