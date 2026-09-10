// One continuous, normal-speed take. The edit only trims the lead-in/tail and
// adds an explanatory frame; no simulated UI, retimed input or invented frames.
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..');
const take = path.resolve(process.argv[2] || '');
if (!process.argv[2]) throw new Error('Usage: node client/desktop/demo/edit.mjs TAKE_DIRECTORY');
const {markers} = JSON.parse(fs.readFileSync(path.join(take,'markers.json'),'utf8'));
const duration = markers.end - markers.reading;
const webgl = markers.webgl - markers.reading;
const tab = markers.new_tab - markers.reading;
const regular = path.join(root,'client/desktop/assets/fonts/Inter-Regular.ttf');
const medium = path.join(root,'client/desktop/assets/fonts/Inter-Medium.ttf');
const semibold = path.join(root,'client/desktop/assets/fonts/Inter-SemiBold.ttf');
const escape = s => s.replaceAll('\\','\\\\').replaceAll(':','\\:').replaceAll("'","’");
const text = (value, font, size, x, y, color, extra='') =>
  `drawtext=fontfile='${escape(font)}':text='${escape(value)}':fontsize=${size}:x=${x}:y=${y}:fontcolor=${color}${extra}`;
const captions = [
  ['Chromium runs on your computer. Surf brings it to your device.', 0, tab],
  ['Your tabs. Your navigation. A real browser session.', tab, webgl],
  ['Modern websites and interactive graphics, streamed to the client.', webgl, duration],
];
const filters = [
  `[0:v]trim=start=${markers.reading}:end=${markers.end},setpts=PTS-STARTPTS,pad=1920:1080:192:118:color=0x121416`,
  'drawbox=x=191:y=117:w=1538:h=866:color=0x43474a:t=1',
  text('Surf',semibold,36,270,26,'0xf2f3f5'),
  text('The modern web on older iPhones and iPads.',regular,21,271,72,'0xb6bac0'),
  text('LINUX CLIENT · REAL SESSION',medium,16,'1728-tw',63,'0xb6bac0'),
  ...captions.map(([value,start,end]) => text(value,medium,25,192,1014,'0xf2f3f5',`:enable='gte(t,${start})*lt(t,${end})'`)),
  text('github.com/seg6/surf',regular,18,'1728-tw',1020,'0xa2a8ae'),
].join(',') + '[framed];[1:v]scale=62:62[logo];[framed][logo]overlay=192:29:shortest=1,format=yuv420p[out]';
const output = path.join(take,'surf-demo-proof.mp4');
function run(args) {
  const result = spawnSync('ffmpeg',args,{stdio:'inherit'});
  if (result.status !== 0) throw new Error('FFmpeg edit failed');
}
run(['-hide_banner','-y','-i',path.join(take,'capture.mkv'),'-loop','1','-i',path.join(root,'backend/cmd/surf/surf-icon.png'),
  '-filter_complex',filters,'-map','[out]','-an','-t',String(duration),'-r','60',
  '-c:v','libx264','-preset','slow','-crf','19','-threads','6','-movflags','+faststart',output]);
run(['-hide_banner','-y','-ss',String(Math.min(webgl+2,duration-0.5)),'-i',output,'-frames:v','1','-update','1',path.join(take,'poster.png')]);
console.log(`Proof: ${output}`);
