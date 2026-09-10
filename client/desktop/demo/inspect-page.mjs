// Read-only CDP diagnostics for an isolated take's browser. No input, emulation,
// capture ownership, browser preferences, or public debug listener is changed.
import fs from 'node:fs';
import path from 'node:path';

export async function inspectPage(take, match, profile) {
  profile ||= fs.existsSync(path.join(take,'runtime.json'))
    ? JSON.parse(fs.readFileSync(path.join(take,'runtime.json'),'utf8')).browserProfile : undefined;
  const [port] = fs.readFileSync(path.join(profile || path.join(take,'server/profile'),'DevToolsActivePort'),'utf8').split('\n');
  if (!/^\d+$/.test(port)) throw new Error('Invalid private browser endpoint');
  const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
  const target = targets.find(t => t.type === 'page' && t.url.includes(match));
  if (!target) return {targets:targets.filter(t=>t.type==='page').map(t=>({url:t.url,title:t.title}))};
  const endpoint = new URL(target.webSocketDebuggerUrl);
  if (!['localhost','127.0.0.1'].includes(endpoint.hostname)) throw new Error('Non-local CDP endpoint');
  const socket = new WebSocket(endpoint);
  const deadline = setTimeout(()=>socket.close(),5000);
  try {
    await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject;});
    const result = new Promise((resolve,reject)=>{
      socket.onmessage = event => {
        const msg=JSON.parse(event.data);
        if(msg.id===1) msg.error ? reject(new Error(msg.error.message)) : resolve(msg.result);
      };
      socket.onclose=()=>reject(new Error('Diagnostic CDP connection closed'));
    });
    socket.send(JSON.stringify({id:1,method:'Runtime.evaluate',params:{returnByValue:true,expression:`JSON.stringify({
      title:document.title,url:location.href,viewport:[innerWidth,innerHeight],scroll:[scrollX,scrollY],
      screen:[screen.width,screen.height],dpr:devicePixelRatio,
      fullscreen:document.fullscreenElement?.tagName||null,
      text:document.body.innerText.slice(0,6000),
      dismissals:Array.from(document.querySelectorAll('[class*=close],[id*=close]')).map(b=>{const r=b.getBoundingClientRect();return {id:b.id,classes:String(b.className),label:b.getAttribute('aria-label'),title:b.title,x:r.x+r.width/2,y:r.y+r.height/2,w:r.width,h:r.height}}).filter(b=>b.w>0&&b.h>0&&b.x>=0&&b.x<innerWidth&&b.y>=0&&b.y<innerHeight),
      links:Array.from(document.querySelectorAll('a[href],.clickable')).map(b=>{const r=b.getBoundingClientRect();return {text:b.innerText?.slice(0,100),label:b.getAttribute('aria-label'),title:b.title,id:b.id,href:b.href,x:r.x+r.width/2,y:r.y+r.height/2,w:r.width,h:r.height}}).filter(b=>b.w>0&&b.h>0&&b.x>=0&&b.x<innerWidth&&b.y>=0&&b.y<innerHeight),
      videos:Array.from(document.querySelectorAll('video')).map(v=>({time:v.currentTime,paused:v.paused,ready:v.readyState,muted:v.muted,volume:v.volume,width:v.videoWidth,height:v.videoHeight,encrypted:!!v.mediaKeys,rect:v.getBoundingClientRect().toJSON(),objectFit:getComputedStyle(v).objectFit,error:v.error?.message})),
      buttons:Array.from(document.querySelectorAll('button,[role=button],[role=menuitem],[role=menuitemradio]')).map(b=>{const r=b.getBoundingClientRect();return {text:b.innerText?.slice(0,100),label:b.getAttribute('aria-label'),title:b.title,x:r.x+r.width/2,y:r.y+r.height/2,w:r.width,h:r.height}}).filter(b=>b.w>0&&b.h>0&&b.x>=0&&b.x<innerWidth&&b.y>=0&&b.y<innerHeight),
    })`}}));
    const response=await result;
    if(response.exceptionDetails) throw new Error(response.exceptionDetails.text);
    return JSON.parse(response.result.value);
  } finally { clearTimeout(deadline);socket.close(); }
}
if (process.argv[1]?.endsWith('/inspect-page.mjs')) {
  if(!process.argv[2]||!process.argv[3]) throw new Error('Usage: inspect-page.mjs TAKE URL_FRAGMENT');
  console.log(JSON.stringify(await inspectPage(path.resolve(process.argv[2]),process.argv[3]),null,2));
}
