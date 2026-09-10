// Measure the recorded sync fixture, including Surf's real playback path.
import {spawnSync} from 'node:child_process';
import path from 'node:path';
if(!process.argv[2])throw new Error('Usage: check-sync.mjs SYNC_TAKE');
const file=path.join(path.resolve(process.argv[2]),'capture.mkv');
function decode(args){
  const r=spawnSync('ffmpeg',['-v','error','-i',file,...args,'pipe:1'],{maxBuffer:32*1024*1024});
  if(r.status!==0)throw new Error(r.stderr.toString());return r.stdout;
}
const video=decode(['-an','-vf','fps=60,crop=2:2:64:64,scale=1:1,format=gray','-f','rawvideo']);
const pcm=decode(['-vn','-ac','1','-ar','48000','-f','f32le']);
const flashes=[],beeps=[];
let wasWhite=false,wasLoud=false;
for(let i=0;i<video.length;i++){
  const white=video[i]>200;
  if(white&&!wasWhite)flashes.push(i/60);wasWhite=white;
}
for(let start=0;start+480*4<=pcm.length;start+=480*4){
  let sum=0;for(let i=0;i<480;i++){const v=pcm.readFloatLE(start+i*4);sum+=v*v;}
  const loud=Math.sqrt(sum/480)>.04;
  if(loud&&!wasLoud)beeps.push(start/4/48000);wasLoud=loud;
}
if(flashes.length!==5||beeps.length!==5)throw new Error(`Expected five sync pulses; video=${JSON.stringify(flashes)} audio=${JSON.stringify(beeps)}`);
const offsets=flashes.map((time,i)=>Math.round((beeps[i]-time)*1000));
const report={videoOnsets:flashes,audioOnsets:beeps,audioMinusVideoMs:offsets,driftMs:offsets.at(-1)-offsets[0]};
console.log(JSON.stringify(report,null,2));
if(offsets.some(v=>Math.abs(v)>120)||Math.abs(report.driftMs)>50){
  throw new Error('Inspect audio/video sync before publishing; do not conceal it with an edit offset');
}
