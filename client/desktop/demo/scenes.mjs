// Real scene actions through the native XTest driver. CDP only reads page state.
import fs from 'node:fs';
import path from 'node:path';

export const sceneURL = {
  browsing:'https://en.wikipedia.org/wiki/Jellyfish',
  navigation:'https://en.wikipedia.org/wiki/Jellyfish',
  map:'https://www.openstreetmap.org/#map=15/51.5050/-0.0750',
  webgl:'https://webglsamples.org/aquarium/aquarium.html',
  library:'https://en.wikipedia.org/wiki/Jellyfish',
  downloads:'https://www.gutenberg.org/ebooks/11',
};

async function control(c,pattern) {
  const s=await c.page();
  const b=[...(s.buttons||[]),...(s.links||[])].find(b=>[b.label,b.title,b.text,b.id].some(v=>pattern.test(v||'')));
  if(!b)throw new Error(`No visible page control: ${pattern}`);
  await c.click(b.x,b.y);await c.sleep(500);
}
async function navigate(c,url) {
  await c.drive('key','ctrl+l');await c.sleep(150);
  await c.drive('paste',url);await c.sleep(350);await c.drive('key','Return');
  await c.until(()=>c.state().url===url&&!c.state().loading&&!c.state().editing,'navigation');
  await c.sleep(350);
}
async function swipe(c,x1,y1,x2,y2,ms=600) {
  await c.drive('swipe',...([x1,y1,x2,y2].map(Math.round)),ms);
}
function save(c,name,data){fs.writeFileSync(path.join(c.out,name),JSON.stringify(data,null,2),{mode:0o600});}

export async function prepareScene(scene,c) {
  await c.miniGeometry(false);
  if(['browsing','navigation','library'].includes(scene)) {
    const s=await c.page();
    const close=[...s.links,...s.buttons,...s.dismissals].find(b=>/cn.*close|close.*banner/i.test(`${b.id} ${b.classes} ${b.label} ${b.title}`));
    if(close){await c.click(close.x,close.y);await c.sleep(500);}
  }
  if(scene==='library') {
    await c.drive('key','ctrl+t');await c.until(()=>c.state().tabs?.length===2,'second tab');
    await navigate(c,'https://en.wikipedia.org/wiki/Octopus');
  } else if(scene==='map') {
    if((await c.page()).buttons.some(b=>b.label==='Close'))await control(c,/^Close$/);
  } else if(scene==='downloads') {
    for(let attempt=0;attempt<5;attempt++) {
      if((await c.page()).links?.some(b=>b.href?.endsWith('/11.epub3.images')))return;
      await swipe(c,580,740,580,420);await c.sleep(600);
    }
    throw new Error('EPUB download link is not visible');
  }
}

export async function recordScene(scene,c) {
  const {sleep,mark,click,drive,until,state}=c;
  mark(scene);
  if(scene==='browsing') {
    await sleep(450);mark('scroll');
    const before=await c.page();
    await swipe(c,420,760,420,435,650);await sleep(550);
    await swipe(c,420,760,420,500,600);await sleep(550);
    await swipe(c,420,760,420,460,650);await sleep(850);
    const after=await c.page();
    if(after.scroll[1]<=before.scroll[1]+100)throw new Error('Browsing did not scroll');
    save(c,'scroll-proof.json',{before:before.scroll,after:after.scroll});
  } else if(scene==='navigation') {
    await sleep(250);mark('address');
    await navigate(c,'https://en.wikipedia.org/wiki/Octopus');
    mark('loaded');await c.move(20,20,100);await sleep(650);
  } else if(scene==='map') {
    await sleep(350);
    const before=await c.page();
    await swipe(c,580,480,370,580,800);await sleep(350);mark('zoom');
    await control(c,/^Zoom in$/i);await sleep(250);
    await swipe(c,520,560,410,430,650);await sleep(250);
    await control(c,/^Zoom in$/i);await sleep(800);
    const after=await c.page();save(c,'map-proof.json',{before:before.url,after:after.url});
    if(before.url===after.url)throw new Error('Map view did not change');
  } else if(scene==='webgl') {
    await sleep(2000);mark('change_view');
    await control(c,/^setSettingChangeView$/);await sleep(4500);
    await control(c,/^setSettingChangeView$/);await sleep(5000);
  } else if(scene==='library') {
    await sleep(250);mark('bookmark');
    await drive('key','ctrl+d');await sleep(350);
    // The visible production bar's Library control at the current portrait size.
    await click(c.logical[0]-55,c.logical[1]-21);
    await until(()=>state().panel==='Some(Library)','Library');
    await sleep(200);
    // Header and section geometry in the centered 680×560 native sheet.
    await click(c.logical[0]/2,(c.logical[1]-42)/2-280+86);
    await until(()=>state().library_section==='Bookmarks'&&state().bookmarks_count>0,'saved bookmark');
    mark('saved');await c.move(20,20,100);await sleep(700);
    await drive('key','Escape');await sleep(200);
    // Two tabs share the horizontal strip; selecting the first exercises the
    // actual widget (the trace is only an assertion, never an input channel).
    await click(370,c.logical[1]-21);
    await until(()=>state().url===sceneURL.library,'first tab selected');
    mark('tabs');await c.move(20,20,100);await sleep(500);
    await click(c.logical[0]-55,c.logical[1]-21);
    await until(()=>state().panel==='Some(Library)','reopen Library');await sleep(200);
    await click(280,(c.logical[1]-42)/2-280+170);
    await until(()=>state().url==='https://en.wikipedia.org/wiki/Octopus'&&state().panel==='None','open saved bookmark');
    mark('reopened');await c.move(20,20,100);await sleep(750);
  } else if(scene==='downloads') {
    await sleep(350);
    const b=(await c.page()).links.find(b=>b.href?.endsWith('/11.epub3.images'));
    if(!b)throw new Error('Download link moved');
    mark('download');await click(b.x,b.y);
    const dir=path.join(c.serverHome,'downloads');
    await until(()=>fs.existsSync(dir)&&fs.readdirSync(dir).some(n=>n.endsWith('.epub')),'completed EPUB download');
    await sleep(250);await click(c.logical[0]-55,c.logical[1]-21);
    await until(()=>state().panel==='Some(Library)','downloads Library');await sleep(200);
    await click(c.logical[0]/2+216,(c.logical[1]-42)/2-280+86);
    await until(()=>state().library_section==='Downloads'&&state().download_names?.some(n=>n.endsWith('.epub')),'completed file in Library');
    mark('complete');await c.move(20,20,100);await sleep(1200);
  }
}
