// Join two genuine normal-speed takes. Only trim, frame, caption, and fade audio
// at the cut; never replace playback sound with a separate source soundtrack.
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../../..');
const clientOnly=process.argv.includes('--client-only');
const args=process.argv.slice(2).filter(p=>p!=='--client-only');
if(args.length!==2) throw new Error('Usage: edit-mini.mjs [--client-only] READING_TAKE VIDEO_TAKE');
const takes=args.map(p=>path.resolve(p));
const data=takes.map(p=>JSON.parse(fs.readFileSync(path.join(p,'markers.json'),'utf8')));
const geometry=data[0].logical?.join('x');
if(!['1024x768','768x1024'].includes(geometry) || data.some(d=>!d.mini||d.logical?.join('x')!==geometry)) throw new Error('Both takes must use the same mini orientation');
const portrait=geometry==='768x1024';
const clientWidth=portrait?720:1280;
const clientX=portrait?48:64;
const captionX=clientX+clientWidth+(portrait?64:56);
const captionWidth=400;
const canvasWidth=captionX+captionWidth+(portrait?48:120);
const durations=data.map(d=>d.markers.end-d.markers.reading);
const transition=durations[0];
const fullscreen=transition+data[1].markers.fullscreen-data[1].markers.reading;
const duration=durations[0]+durations[1];
const fonts=path.join(root,'client/desktop/assets/fonts');
const esc=s=>s.replaceAll('\\','\\\\').replaceAll(':','\\:').replaceAll("'","’");
const text=(value,size,x,y,color='0x202529',weight='Regular',enable='')=>
  `drawtext=fontfile='${esc(path.join(fonts,`Inter-${weight}.ttf`))}':text='${esc(value)}':fontsize=${size}:x=${x}:y=${y}:fontcolor=${color}${enable?`:enable='${enable}'`:''}`;
const filters=[];
data.forEach((d,i)=>{
  const {reading,end}=d.markers;
  filters.push(`[${i}:v]trim=start=${reading}:end=${end},setpts=PTS-STARTPTS,setsar=1,fps=60[v${i}]`);
  filters.push(`[${i}:a]atrim=start=${reading}:end=${end},asetpts=PTS-STARTPTS,afade=t=in:d=0.06,afade=t=out:st=${durations[i]-.1}:d=0.1[a${i}]`);
});
filters.push('[v0][a0][v1][a1]concat=n=2:v=1:a=1[session][audio]');
const frame=[
  `[session]scale=${clientWidth}:960:flags=lanczos,pad=${canvasWidth}:1080:${clientX}:60:color=0xf5f4f0`,
  `drawbox=x=${clientX-1}:y=59:w=${clientWidth+2}:h=962:color=0xd9d9d3:t=1`,
  text('Surf',52,captionX+76,87,'0x202529','SemiBold'),
  text('For legacy Apple devices',30,captionX,265,'0x202529','Medium'),
  text('iPhone, iPad and iPod touch.',22,captionX,312,'0x60696f'),
  `drawbox=x=${captionX}:y=430:w=${captionWidth}:h=1:color=0xd9d9d3:t=fill`,
  text('Web browsing',32,captionX,490,'0x202529','Medium',`lt(t,${transition})`),
  text('A swipe through Surf.',22,captionX,550,'0x60696f','Regular',`lt(t,${transition})`),
  text('Chromium handles the scroll.',22,captionX,584,'0x60696f','Regular',`lt(t,${transition})`),
  text('YouTube',32,captionX,490,'0x202529','Medium',`gte(t,${transition})*lt(t,${fullscreen})`),
  text('YouTube, with sound.',22,captionX,550,'0x60696f','Regular',`gte(t,${transition})*lt(t,${fullscreen})`),
  text('Played through the client.',22,captionX,584,'0x60696f','Regular',`gte(t,${transition})*lt(t,${fullscreen})`),
  text('Fullscreen video',32,captionX,490,'0x202529','Medium',`gte(t,${fullscreen})`),
  text('Real playback.',22,captionX,550,'0x60696f','Regular',`gte(t,${fullscreen})`),
  text('Captured sound.',22,captionX,584,'0x60696f','Regular',`gte(t,${fullscreen})`),
  text('github.com/seg6/surf',20,captionX,997,'0x1473b8'),
  text('Film: CHARGE · © 2022 Blender Foundation · studio.blender.org',13,clientX,1044,'0x60696f','Regular',`gte(t,${transition})`),
].join(',');
if(clientOnly) filters.push('[session]format=yuv420p[out]');
else {
  filters.push(`${frame}[frame]`);
  filters.push(`[2:v]scale=64:64[logo];[frame][logo]overlay=${captionX}:84:shortest=1,format=yuv420p[out]`);
}
const output=path.join(takes[1],clientOnly?'surf-mini-client-only.mp4':'surf-demo-preview.mp4');
function run(args){const r=spawnSync('ffmpeg',['-hide_banner','-loglevel','warning','-y',...args],{stdio:'inherit'});if(r.status!==0)throw new Error('FFmpeg export failed');}
run(['-i',path.join(takes[0],'capture.mkv'),'-i',path.join(takes[1],'capture.mkv'),...(!clientOnly?['-loop','1','-i',path.join(root,'backend/cmd/surf/surf-icon.png')]:[]),
  '-filter_complex',filters.join(';'),'-map','[out]','-map','[audio]','-t',String(duration),'-c:v','libx264','-preset','slow','-crf','19','-threads','6',
  '-c:a','aac','-b:a','192k','-movflags','+faststart',output]);
// Portrait browsing communicates the device framing better than a heavily
// letterboxed cinema frame in the poster; the movie remains unmodified.
run(['-ss',String(portrait?1:fullscreen+3),'-i',output,'-frames:v','1','-update','1',path.join(takes[1],clientOnly?'poster-client-only.png':'poster-demo.png')]);
console.log(output);
