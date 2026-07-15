/* wrp client. Runs on iOS 6 Safari: ES5 only, touch events, canvas, Blob
   URLs. Frame contract: binary RBR1 messages (header length-prefixed; >=32
   carries the page scroll offset), one 'ready' ack for EVERY received frame
   (rendered or skipped — the server pipelines against these), {t:'poke'}
   watchdog if the pipe stalls. All input coordinates are sent as viewport
   fractions (0..1). Scrolling is locally predicted: the canvas pans under
   the finger instantly and incoming frames are drawn offset by however much
   the server still lags (rubber-banding back when it can't catch up). */
(function () {
  'use strict';

  var canvas = document.getElementById('screen');
  var ctx = canvas.getContext('2d');
  var wrap = document.getElementById('wrap');
  var urlBox = document.getElementById('url');
  var omnibox = document.getElementById('omnibox');
  var progressEl = document.getElementById('progress');
  var siteIcon = document.getElementById('siteicon');
  var hidden = document.getElementById('hidden');
  var tabsEl = document.getElementById('tabs');
  var hintEl = document.getElementById('hint');
  var kbdBtn = document.getElementById('kbd');
  var starBtn = document.getElementById('star');
  var reloadBtn = document.getElementById('reload');
  var backBtn = document.getElementById('back');
  var fwdBtn = document.getElementById('fwd');
  var suggEl = document.getElementById('sugg');
  var findBar = document.getElementById('findbar');
  var findQ = document.getElementById('findq');
  var catcher = document.getElementById('catcher');
  var popover = document.getElementById('popover');
  var shade = document.getElementById('shade');
  var panel = document.getElementById('panel');
  var debugEl = document.getElementById('debug');
  var ringEl = document.getElementById('dragring');

  var ws = null;
  var wsAttempts = 0;
  var URL_ = window.URL || window.webkitURL;
  var TOUCH = ('ontouchstart' in window);
  var SENTINEL = ' ';
  var kbdUp = false;
  var zoom = 1;
  var loading = false;

  // ---------------- hint toast ----------------
  var hintTimer = null;
  function setHint(t, sticky) {
    if (hintTimer) { clearTimeout(hintTimer); hintTimer = null; }
    if (t) {
      hintEl.innerHTML = t; hintEl.className = '';
      if (!sticky) hintTimer = setTimeout(function () { hintEl.className = 'off'; }, 1800);
    } else { hintEl.className = 'off'; }
  }
  function send(o) { if (ws && ws.readyState === 1) { try { ws.send(JSON.stringify(o)); } catch (e) {} } }

  // ---------------- debug ----------------
  var lastFrame = 0, rx = 0, txReady = 0, txPoke = 0, recent = [], dbgOn = false;
  var fps = 0, fpsFrames = 0, fpsLast = new Date().getTime();
  function stateName() { return ws ? ['CONNECTING', 'OPEN', 'CLOSING', 'CLOSED'][ws.readyState] : 'null'; }
  function renderDbg() {
    if (!dbgOn) return;
    var age = ((new Date().getTime() - lastFrame) / 1000).toFixed(1);
    debugEl.innerHTML =
      'ws: ' + stateName() + '   buffered: ' + (ws ? ws.bufferedAmount : 0) +
      '\nclient: ' + (window.__V || '') +
      '\nframes rx: ' + rx + '   fps: ' + fps.toFixed(1) + '   last: ' + age + 's ago' +
      '\ntx ready: ' + txReady + '   tx poke: ' + txPoke +
      '\ncanvas: ' + canvas.width + 'x' + canvas.height + '   zoom: ' + zoom.toFixed(2) +
      '\nkbd up: ' + kbdUp +
      '\nrecent: ' + recent.join('  ');
  }
  setInterval(renderDbg, 500);
  setInterval(function () {
    var now = new Date().getTime();
    fps = fpsFrames * 1000 / Math.max(1, now - fpsLast);
    fpsFrames = 0; fpsLast = now;
  }, 1000);
  function toggleDbg() { dbgOn = !dbgOn; debugEl.className = dbgOn ? '' : 'off'; renderDbg(); }

  // ---------------- layout ----------------
  function layout() {
    var cw = wrap.clientWidth, ch = wrap.clientHeight;
    if (!cw || !ch || !canvas.width || !canvas.height) return;
    var s = Math.min(cw / canvas.width, ch / canvas.height);
    var w = Math.round(canvas.width * s), h = Math.round(canvas.height * s);
    canvas.style.width = w + 'px';
    canvas.style.height = h + 'px';
    canvas.style.marginTop = Math.max(0, Math.round((ch - h) / 2)) + 'px';
  }
  var sizeTimer = null;
  function sendSize() {
    if (kbdUp) return; // keyboard shrinking the viewport is not a real resize
    var w = wrap.clientWidth, h = wrap.clientHeight;
    if (w > 0 && h > 0) send({ t: 'size', w: w, h: h });
  }
  function queueSize() { if (sizeTimer) clearTimeout(sizeTimer); sizeTimer = setTimeout(sendSize, 350); }
  window.addEventListener('resize', function () { layout(); queueSize(); }, false);
  window.addEventListener('orientationchange', function () {
    setTimeout(function () { layout(); sendSize(); }, 400);
  }, false);

  // ---------------- progress (fills the address field) ----------------
  var progTimer = null;
  function progressStart() {
    if (progTimer) clearInterval(progTimer);
    var w = 12;
    progressEl.style.opacity = '1';
    progressEl.style.width = w + '%';
    progTimer = setInterval(function () {
      w += (85 - w) * 0.08;
      progressEl.style.width = w.toFixed(1) + '%';
    }, 250);
  }
  function progressDone() {
    if (progTimer) { clearInterval(progTimer); progTimer = null; }
    progressEl.style.width = '100%';
    setTimeout(function () { progressEl.style.opacity = '0'; }, 250);
    setTimeout(function () { if (!loading) progressEl.style.width = '0'; }, 650);
  }
  function setLoading(on) {
    loading = on;
    reloadBtn.innerHTML = on ? '&#10005;' : '&#8635;';
    if (on) progressStart(); else progressDone();
  }

  var touching = false, pinching = false;

  // ---------------- frames ----------------
  var img = new Image();
  var rendering = false;
  var nextItem = null;   // latest received-but-not-drawn frame {src}
  var pendingUrl = null;
  var lastSeq = 0;
  var zoomPreviewClear = false;

  function revoke(src) {
    if (src && URL_ && URL_.revokeObjectURL) { try { URL_.revokeObjectURL(src); } catch (e) {} }
  }
  function ack() { txReady++; send({ t: 'ready' }); }
  function frameDone() {
    rendering = false;
    ack();
    if (nextItem) processNext();
  }
  img.onload = function () {
    var src = pendingUrl;
    pendingUrl = null;
    var fw = img.width, fh = img.height;
    if (fw && fh && (canvas.width !== fw || canvas.height !== fh)) {
      canvas.width = fw; canvas.height = fh; layout();
    }
    try { ctx.drawImage(img, 0, 0); } catch (e) {}
    revoke(src);
    if (zoomPreviewClear) {
      zoomPreviewClear = false;
      canvas.style.webkitTransform = 'translateZ(0)';
    }
    lastFrame = new Date().getTime();
    fpsFrames++;
    frameDone();
  };
  img.onerror = function () {
    revoke(pendingUrl); pendingUrl = null;
    lastFrame = new Date().getTime();
    frameDone();
  };
  function processNext() {
    if (rendering || !nextItem) return;
    var item = nextItem; nextItem = null;
    pendingUrl = item.src;
    rendering = true;
    img.src = item.src;
  }
  function copyBuffer(bytes) {
    if (bytes.byteOffset === 0 && bytes.byteLength === bytes.buffer.byteLength) return bytes.buffer;
    if (bytes.buffer.slice) return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
    var copy = new Uint8Array(bytes.byteLength);
    copy.set(bytes);
    return copy.buffer;
  }
  function renderBinaryFrame(ab) {
    try {
      var dv = new DataView(ab);
      if (ab.byteLength < 24 || dv.getUint8(0) !== 82 || dv.getUint8(1) !== 66 || dv.getUint8(2) !== 82 || dv.getUint8(3) !== 49) throw new Error('bad');
      var headerLen = dv.getUint16(6, false), seq = dv.getUint32(8, false);
      var len = dv.getUint32(20, false) || (ab.byteLength - headerLen);
      if (headerLen < 24 || headerLen + len > ab.byteLength) throw new Error('bad');
      if (!URL_ || !URL_.createObjectURL || !window.Blob) throw new Error('no blob');
      if (seq && lastSeq && seq < lastSeq) { ack(); return; }
      lastSeq = seq || lastSeq;
      var src = URL_.createObjectURL(new Blob([copyBuffer(new Uint8Array(ab, headerLen, len))], { type: 'image/jpeg' }));
      rx++;
      recent.unshift('f·' + Math.round(len / 1024) + 'k'); if (recent.length > 8) recent.pop();
      if (nextItem) {
        // Skipping a frame we'll never draw: still ack it (the server's
        // pipeline window is counted per frame) and free its blob.
        revoke(nextItem.src);
        ack();
      }
      nextItem = { src: src };
      processNext();
    } catch (e) { ack(); }
  }
  function handleBinaryMessage(data) {
    if (data && typeof data.byteLength === 'number') { renderBinaryFrame(data); return; }
    if (data && typeof data.size === 'number' && window.FileReader) {
      var r = new FileReader();
      r.onload = function () { renderBinaryFrame(r.result); };
      r.onerror = function () { ack(); };
      try { r.readAsArrayBuffer(data); } catch (e) { ack(); }
      return;
    }
    ack();
  }
  // Watchdog: if an ack was ever lost, nudge the server so it resets the
  // window and flushes the latest frame instead of freezing.
  setInterval(function () {
    if (ws && ws.readyState === 1 && (new Date().getTime() - lastFrame) > 1500) { txPoke++; send({ t: 'poke' }); }
  }, 1000);

  // ---------------- pointer input ----------------
  // toRemote returns viewport FRACTIONS (0..1); resolution-independent.
  function toRemote(cx, cy) {
    var r = canvas.getBoundingClientRect();
    var x = (cx - r.left) / Math.max(1, r.width), y = (cy - r.top) / Math.max(1, r.height);
    if (x < 0) x = 0; if (x > 1) x = 1;
    if (y < 0) y = 0; if (y > 1) y = 1;
    return { x: x, y: y, rw: r.width, rh: r.height };
  }
  function spawnFleck(cx, cy) {
    var f = document.createElement('div');
    f.className = 'fleck';
    f.style.left = (cx - 14) + 'px';
    f.style.top = (cy - 14) + 'px';
    document.body.appendChild(f);
    setTimeout(function () { if (f.parentNode) f.parentNode.removeChild(f); }, 380);
  }
  function showRing(cx, cy) {
    ringEl.style.left = (cx - 20) + 'px';
    ringEl.style.top = (cy - 20) + 'px';
    ringEl.className = 'show';
  }
  function hideRing() { ringEl.className = ''; }

  var t0 = 0, startX = 0, startY = 0, lastX = 0, lastY = 0, moved = false;
  var rectW = 1, rectH = 1;
  var lastRX = 0, lastRY = 0, velX = 0, velY = 0, lastMoveTime = 0, lastWasTouch = false, inertiaTimer = null;
  var TAP_MOVE = 12, TAP_MS = 400;
  var lpTimer = null, lpFired = false, dragMoved = false, lastLpMove = 0;
  var pinchD0 = 0, pinchCX = 0, pinchCY = 0, pinchScale = 1;

  function stopInertia() { if (inertiaTimer) { clearTimeout(inertiaTimer); inertiaTimer = null; } }
  function cancelLp() { if (lpTimer) { clearTimeout(lpTimer); lpTimer = null; } }
  function clampVelocity(v) { return v > 2.6 ? 2.6 : (v < -2.6 ? -2.6 : v); }

  // sendScroll converts screen-px deltas into a wheel message (fractions).
  function sendScroll(fx, fy, dxPx, dyPx) {
    send({ t: 'wheel', x: fx, y: fy, dx: -dxPx / rectW, dy: -dyPx / rectH });
  }
  function startInertia() {
    var vx = clampVelocity(velX), vy = clampVelocity(velY);
    var speed = Math.sqrt(vx * vx + vy * vy);
    if (!lastWasTouch || speed < 0.10) return;
    var last = new Date().getTime();
    stopInertia();
    function tick() {
      var now = new Date().getTime();
      var dt = Math.min(34, Math.max(8, now - last));
      last = now;
      sendScroll(lastRX, lastRY, vx * dt, vy * dt);
      var decay = Math.pow(0.998, dt); // UIScrollView-normal feel
      vx *= decay; vy *= decay;
      if (Math.sqrt(vx * vx + vy * vy) > 0.02) inertiaTimer = setTimeout(tick, 16);
      else inertiaTimer = null;
    }
    inertiaTimer = setTimeout(tick, 16);
  }

  function touchDist(e) {
    var a = e.touches[0], b = e.touches[1];
    var dx = a.clientX - b.clientX, dy = a.clientY - b.clientY;
    return Math.sqrt(dx * dx + dy * dy);
  }
  function pinchBegin(e) {
    cancelLp(); stopInertia();
    pinching = true;
    pinchD0 = touchDist(e) || 1;
    pinchScale = 1;
    pinchCX = (e.touches[0].clientX + e.touches[1].clientX) / 2;
    pinchCY = (e.touches[0].clientY + e.touches[1].clientY) / 2;
    var r = canvas.getBoundingClientRect();
    canvas.style.webkitTransformOrigin = (pinchCX - r.left) + 'px ' + (pinchCY - r.top) + 'px';
  }
  function pinchMove(e) {
    var s = (touchDist(e) || 1) / pinchD0;
    var target = zoom * s;
    if (target < 0.85) s = 0.85 / zoom;
    if (target > 3.4) s = 3.4 / zoom;
    pinchScale = s;
    canvas.style.webkitTransform = 'scale(' + s + ')';   // instant local preview
  }
  function pinchEnd() {
    pinching = false;
    var target = zoom * pinchScale;
    if (target < 1.05) target = 1;
    if (target > 3) target = 3;
    var r = toRemote(pinchCX, pinchCY);
    send({ t: 'zoom', scale: target, cx: r.x, cy: r.y });
    zoomPreviewClear = true;   // cleared when the next (zoomed) frame lands
    setHint(target === 1 ? 'Zoom reset' : ('Zoom ' + target.toFixed(1) + '&times;'));
  }

  function onStart(e) {
    if (e.touches && e.touches.length === 2) { pinchBegin(e); if (e.preventDefault) e.preventDefault(); return; }
    if (e.touches && e.touches.length > 2) return;
    if (pinching) return;
    stopInertia();
    touching = true;
    var p = e.touches ? e.touches[0] : e;
    var r = toRemote(p.clientX, p.clientY);
    t0 = new Date().getTime(); startX = p.clientX; startY = p.clientY;
    lastX = p.clientX; lastY = p.clientY; moved = false;
    rectW = r.rw; rectH = r.rh;
    lastRX = r.x; lastRY = r.y; velX = 0; velY = 0; lastMoveTime = t0; lastWasTouch = !!e.touches;
    lpFired = false; dragMoved = false;
    cancelLp();
    if (lastWasTouch) {
      lpTimer = setTimeout(function () {
        lpTimer = null;
        if (!moved && !pinching) {
          lpFired = true;
          var rr = toRemote(startX, startY);
          send({ t: 'lpdown', x: rr.x, y: rr.y });
          showRing(startX, startY);
        }
      }, 550);
    }
    if (e.preventDefault) e.preventDefault();
  }
  function onMove(e) {
    if (pinching && e.touches && e.touches.length === 2) { pinchMove(e); if (e.preventDefault) e.preventDefault(); return; }
    if (e.touches && e.touches.length > 1) return;
    var p = e.touches ? e.touches[0] : e;
    if (lpFired) {
      // Long-press drag: real mouse drag on the remote page.
      if (!dragMoved && (Math.abs(p.clientX - startX) > 6 || Math.abs(p.clientY - startY) > 6)) dragMoved = true;
      if (dragMoved) {
        var now2 = new Date().getTime();
        if (now2 - lastLpMove > 40) {
          lastLpMove = now2;
          var rr = toRemote(p.clientX, p.clientY);
          send({ t: 'lpmove', x: rr.x, y: rr.y });
        }
        showRing(p.clientX, p.clientY);
      }
      if (e.preventDefault) e.preventDefault();
      return;
    }
    var now = new Date().getTime();
    var dx = p.clientX - lastX, dy = p.clientY - lastY;
    if (!moved && (Math.abs(p.clientX - startX) > TAP_MOVE || Math.abs(p.clientY - startY) > TAP_MOVE)) {
      moved = true;
      cancelLp();
    }
    if (moved) {
      // Anchor all wheel events of one gesture at the touch-down point, like
      // real touch scrolling: mid-drag the finger crossing another scrollable
      // region must not switch scroll targets.
      sendScroll(lastRX, lastRY, dx, dy);
      var dt = Math.max(1, now - lastMoveTime);
      var ix = dx / dt, iy = dy / dt; // finger px/ms; sendScroll negates
      velX = velX ? (velX * 0.55 + ix * 0.45) : ix;
      velY = velY ? (velY * 0.55 + iy * 0.45) : iy;
      lastX = p.clientX; lastY = p.clientY;
      lastMoveTime = now;
    }
    if (e.preventDefault) e.preventDefault();
  }
  function onEnd(e) {
    cancelLp();
    touching = false;
    if (pinching) {
      if (!e.touches || e.touches.length < 2) pinchEnd();
      if (e.preventDefault) e.preventDefault();
      return;
    }
    if (lpFired) {
      var pt = (e.changedTouches && e.changedTouches[0]) || e;
      var rr = toRemote(pt.clientX || startX, pt.clientY || startY);
      send({ t: 'lpup', x: rr.x, y: rr.y, sel: !dragMoved });
      hideRing();
      lpFired = false; dragMoved = false;
      if (e.preventDefault) e.preventDefault();
      return;
    }
    var dt = new Date().getTime() - t0;
    if (!moved && dt < TAP_MS) {
      var r = toRemote(startX, startY);
      send({ t: 'click', x: r.x, y: r.y });
      spawnFleck(startX, startY);
    } else if (moved) {
      startInertia();
    }
    if (e.preventDefault) e.preventDefault();
  }
  canvas.addEventListener('touchstart', onStart, false);
  canvas.addEventListener('touchmove', onMove, false);
  canvas.addEventListener('touchend', onEnd, false);
  canvas.addEventListener('touchcancel', onEnd, false);
  canvas.addEventListener('mousedown', onStart, false);
  canvas.addEventListener('mousemove', function (e) { if (t0 && (e.buttons === 1 || e.which === 1)) onMove(e); }, false);
  canvas.addEventListener('mouseup', function (e) { onEnd(e); t0 = 0; }, false);
  function wheelEvt(e) {
    var r = toRemote(e.clientX, e.clientY);
    var d = e.wheelDelta ? -e.wheelDelta : (e.detail * 40);
    rectW = r.rw; rectH = r.rh;
    sendScroll(r.x, r.y, 0, -d);
    if (e.preventDefault) e.preventDefault();
  }
  canvas.addEventListener('DOMMouseScroll', wheelEvt, false);
  canvas.addEventListener('mousewheel', wheelEvt, false);

  // ---------------- keyboard ----------------
  // Hidden input carries a sentinel char so iOS reports backspace as a shrink.
  var KEYMAP = { 9: ['Tab', 9], 13: ['Enter', 13], 27: ['Escape', 27],
    37: ['ArrowLeft', 37], 38: ['ArrowUp', 38], 39: ['ArrowRight', 39], 40: ['ArrowDown', 40] };
  function pressKey(name, code) {
    send({ t: 'key', down: true, key: name, code: name, keyCode: code });
    send({ t: 'key', down: false, key: name, code: name, keyCode: code });
  }
  function kbdToggle() {
    if (kbdUp) { hidden.blur(); }
    else { hidden.value = SENTINEL; hidden.focus(); }
  }
  kbdBtn.addEventListener('click', kbdToggle, false);
  hidden.addEventListener('focus', function () { kbdUp = true; kbdBtn.className = 'btn pulse'; }, false);
  hidden.addEventListener('blur', function () { kbdUp = false; kbdBtn.className = 'btn'; }, false);
  hidden.addEventListener('keydown', function (e) {
    var k = KEYMAP[e.keyCode];
    if (k) { pressKey(k[0], k[1]); if (e.preventDefault) e.preventDefault(); return; }
    if (!TOUCH && e.keyCode === 8) { pressKey('Backspace', 8); if (e.preventDefault) e.preventDefault(); }
  }, false);
  hidden.addEventListener('input', function () {
    var v = hidden.value;
    if (v.length < SENTINEL.length) { pressKey('Backspace', 8); }
    else if (v.length - SENTINEL.length > 2) {
      // Autocomplete/burst insert: one insertText beats N key events.
      send({ t: 'paste', text: v.slice(SENTINEL.length) });
    } else {
      for (var i = SENTINEL.length; i < v.length; i++) send({ t: 'key', text: v.charAt(i) });
    }
    hidden.value = SENTINEL;
  }, false);

  // ---------------- omnibox ----------------
  function go(u) {
    var v = (u !== undefined) ? u : urlBox.value;
    if (v) send({ t: 'nav', url: v });
    urlBox.blur();
    hideSugg();
  }
  var suggTimer = null;
  function hideSugg() { suggEl.style.display = 'none'; suggEl.innerHTML = ''; }
  function showSugg(items) {
    if (!items || !items.length || document.activeElement !== urlBox) { hideSugg(); return; }
    suggEl.innerHTML = '';
    for (var i = 0; i < items.length; i++) {
      (function (it) {
        var d = document.createElement('div');
        d.className = 'item';
        var t = document.createElement('span'); t.className = 't';
        t.appendChild(document.createTextNode(it.title || it.url));
        var u = document.createElement('span'); u.className = 'u';
        u.appendChild(document.createTextNode(it.url));
        d.appendChild(t); d.appendChild(u);
        d.addEventListener('mousedown', function (e) { if (e.preventDefault) e.preventDefault(); go(it.url); }, false);
        d.addEventListener('touchend', function (e) { if (e.preventDefault) e.preventDefault(); go(it.url); }, false);
        suggEl.appendChild(d);
      })(items[i]);
    }
    suggEl.style.display = 'block';
  }
  urlBox.addEventListener('input', function () {
    if (suggTimer) clearTimeout(suggTimer);
    var v = urlBox.value;
    if (!v) { hideSugg(); return; }
    suggTimer = setTimeout(function () { send({ t: 'suggest', q: v }); }, 250);
  }, false);
  urlBox.addEventListener('keydown', function (e) { if (e.keyCode === 13) { go(); if (e.preventDefault) e.preventDefault(); } }, false);
  urlBox.addEventListener('focus', function () {
    omnibox.className = 'focus';
    setTimeout(function () { try { urlBox.setSelectionRange(0, urlBox.value.length); } catch (e) {} }, 50);
  }, false);
  urlBox.addEventListener('blur', function () {
    omnibox.className = '';
    setTimeout(hideSugg, 250);
  }, false);
  // The form makes the iOS keyboard's Return key a real submit ("Go").
  document.getElementById('urlform').addEventListener('submit', function (e) {
    if (e.preventDefault) e.preventDefault();
    go();
    return false;
  }, false);
  function insertUrlText(text) {
    var v = urlBox.value, s = v.length, e = v.length;
    try { s = urlBox.selectionStart; e = urlBox.selectionEnd; } catch (err) {}
    urlBox.value = v.slice(0, s) + text + v.slice(e);
    try { urlBox.focus(); urlBox.setSelectionRange(s + text.length, s + text.length); } catch (err2) {}
  }

  backBtn.addEventListener('click', function () { if (!backBtn.disabled) send({ t: 'back' }); }, false);
  fwdBtn.addEventListener('click', function () { if (!fwdBtn.disabled) send({ t: 'fwd' }); }, false);
  reloadBtn.addEventListener('click', function () { send({ t: loading ? 'stop' : 'reload' }); }, false);
  document.getElementById('dotcom').addEventListener('click', function () { insertUrlText('.com'); }, false);
  starBtn.addEventListener('click', function () { send({ t: 'bookmark' }); }, false);
  function setStar(on) {
    starBtn.className = on ? 'inbtn on' : 'inbtn';
    starBtn.innerHTML = on ? '&#9733;' : '&#9734;';
  }

  // ---------------- fullscreen ----------------
  var fsBtn = document.getElementById('fs');
  var fsDot = document.getElementById('fsdot');
  function setFullscreen(on) {
    document.body.className = on ? 'fs' : '';
    setTimeout(function () { layout(); sendSize(); }, 60);
  }
  fsBtn.addEventListener('click', function () { setFullscreen(true); }, false);
  fsDot.addEventListener('click', function () { setFullscreen(false); }, false);

  // ---------------- tabs ----------------
  var lastTabs = [];
  function renderTabs(list) {
    lastTabs = list;
    tabsEl.innerHTML = '';
    var activeIcon = '';
    for (var i = 0; i < list.length; i++) {
      (function (tab) {
        if (tab.active && tab.icon) activeIcon = tab.icon;
        var el = document.createElement('span');
        el.className = 'tab' + (tab.active ? ' on' : '');
        if (tab.icon) {
          var f = document.createElement('img');
          f.className = 'fav'; f.src = tab.icon;
          f.onerror = function () { if (f.parentNode) f.parentNode.removeChild(f); };
          el.appendChild(f);
        }
        var lbl = document.createElement('span');
        lbl.className = 'lbl';
        lbl.appendChild(document.createTextNode(tab.title || 'New tab'));
        el.appendChild(lbl);
        var x = document.createElement('span');
        x.className = 'x'; x.innerHTML = '&times;';
        x.onclick = function (e) { if (e.stopPropagation) e.stopPropagation(); send({ t: 'tab', action: 'close', id: tab.id }); return false; };
        el.appendChild(x);
        el.onclick = function () {
          if (tab.active) return;
          send({ t: 'tab', action: 'select', id: tab.id });
          // Optimistic: highlight now; the server's tabs broadcast confirms.
          for (var j = 0; j < lastTabs.length; j++) lastTabs[j].active = (lastTabs[j].id === tab.id);
          renderTabs(lastTabs);
        };
        tabsEl.appendChild(el);
      })(list[i]);
    }
    var plus = document.createElement('span');
    plus.id = 'newtab'; plus.innerHTML = '+';
    plus.onclick = function () { send({ t: 'tab', action: 'new' }); };
    tabsEl.appendChild(plus);
    if (activeIcon) {
      siteIcon.src = activeIcon;
      siteIcon.style.display = 'block';
      siteIcon.onerror = function () { siteIcon.style.display = 'none'; };
    } else {
      siteIcon.style.display = 'none';
    }
  }

  // ---------------- popover menu ----------------
  function closePopover() { popover.className = ''; catcher.className = ''; }
  function pi(label, fn) {
    var b = document.createElement('button');
    b.className = 'mi';
    b.appendChild(document.createTextNode(label));
    b.addEventListener('click', function () { closePopover(); fn(); }, false);
    return b;
  }
  (function buildPopover() {
    var caret = document.createElement('div');
    caret.className = 'caret';
    popover.appendChild(caret);
    popover.appendChild(pi('Find in page', function () { openFind(); }));
    popover.appendChild(pi('Paste text', function () { openPaste(); }));
    popover.appendChild(pi('Downloads', function () { send({ t: 'downloads' }); }));
    popover.appendChild(pi('History & bookmarks', function () { send({ t: 'hist' }); }));
    popover.appendChild(pi('Fullscreen', function () { setFullscreen(true); }));
    popover.appendChild(pi('Debug overlay', function () { toggleDbg(); }));
    popover.appendChild(pi('Log out', function () { location.href = '/logout'; }));
  })();
  document.getElementById('menu').addEventListener('click', function () {
    if (popover.className === 'show') { closePopover(); return; }
    popover.className = 'show';
    catcher.className = 'show';
  }, false);
  catcher.addEventListener('click', closePopover, false);
  catcher.addEventListener('touchstart', function (e) { closePopover(); if (e.preventDefault) e.preventDefault(); }, false);

  // ---------------- sheets ----------------
  function closeSheet() { shade.className = ''; panel.className = ''; panel.innerHTML = ''; }
  shade.addEventListener('click', closeSheet, false);
  function openSheet(title) {
    panel.innerHTML = '';
    var hd = document.createElement('div');
    hd.className = 'hd';
    var h = document.createElement('h2');
    h.appendChild(document.createTextNode(title));
    hd.appendChild(h);
    var done = document.createElement('button');
    done.className = 'done';
    done.appendChild(document.createTextNode('Done'));
    done.addEventListener('click', closeSheet, false);
    hd.appendChild(done);
    panel.appendChild(hd);
    shade.className = 'show';
    panel.className = 'show';
    return panel;
  }
  function mi(label, sub, fn) {
    var b = document.createElement('button');
    b.className = 'mi';
    b.appendChild(document.createTextNode(label));
    if (sub) {
      var s = document.createElement('span');
      s.className = 'sub';
      s.appendChild(document.createTextNode(sub));
      b.appendChild(s);
    }
    b.addEventListener('click', fn, false);
    return b;
  }
  function emptyRow(text) {
    var d = document.createElement('div');
    d.className = 'empty';
    d.appendChild(document.createTextNode(text));
    return d;
  }

  function openCopySheet(text) {
    var p = openSheet('Selected text');
    if (!text) {
      p.appendChild(emptyRow('Nothing was selected there. Long-press directly on a word to select it, or long-press and drag across text.'));
      return;
    }
    var ta = document.createElement('textarea');
    ta.value = text;
    p.appendChild(ta);
    var row = document.createElement('div'); row.className = 'row';
    var sel = document.createElement('button'); sel.className = 'act pri';
    sel.appendChild(document.createTextNode('Select all'));
    sel.addEventListener('click', function () {
      try { ta.focus(); ta.setSelectionRange(0, ta.value.length); } catch (e) {}
      setHint('Now tap Copy in the popup');
    }, false);
    row.appendChild(sel);
    p.appendChild(row);
  }

  function openPaste() {
    var p = openSheet('Paste text');
    var ta = document.createElement('textarea');
    p.appendChild(ta);
    var row = document.createElement('div'); row.className = 'row';
    var sendBtn = document.createElement('button'); sendBtn.className = 'act pri';
    sendBtn.appendChild(document.createTextNode('Type it into the page'));
    sendBtn.addEventListener('click', function () {
      if (ta.value) send({ t: 'paste', text: ta.value });
      closeSheet();
    }, false);
    row.appendChild(sendBtn);
    p.appendChild(row);
    setTimeout(function () { try { ta.focus(); } catch (e) {} }, 100);
  }

  function fmtSize(n) {
    if (n > 1048576) return (n / 1048576).toFixed(1) + ' MB';
    if (n > 1024) return Math.round(n / 1024) + ' KB';
    return n + ' B';
  }
  function showDownloads(items) {
    var p = openSheet('Downloads');
    if (!items || !items.length) {
      p.appendChild(emptyRow('Nothing here yet. Files you download in the remote browser land in this list.'));
      return;
    }
    items.sort(function (a, b) { return (b.ts || 0) - (a.ts || 0); });
    for (var i = 0; i < items.length; i++) {
      (function (it) {
        p.appendChild(mi(it.name, fmtSize(it.size || 0), function () {
          window.open('/downloads/' + encodeURIComponent(it.name), '_blank');
        }));
      })(items[i]);
    }
  }

  function showHist(m) {
    var p = openSheet('History & bookmarks');
    var i;
    if (m.bookmarks && m.bookmarks.length) {
      var s1 = document.createElement('div'); s1.className = 'sec';
      s1.appendChild(document.createTextNode('Bookmarks'));
      p.appendChild(s1);
      for (i = 0; i < m.bookmarks.length; i++) {
        (function (it) {
          p.appendChild(mi(it.title || it.url, it.url, function () { closeSheet(); go(it.url); }));
        })(m.bookmarks[i]);
      }
    }
    var s2 = document.createElement('div'); s2.className = 'sec';
    s2.appendChild(document.createTextNode('History'));
    p.appendChild(s2);
    if (!m.hist || !m.hist.length) {
      p.appendChild(emptyRow('No history yet. Pages you visit show up here.'));
      return;
    }
    for (i = 0; i < m.hist.length; i++) {
      (function (it) {
        p.appendChild(mi(it.title || it.url, it.url, function () { closeSheet(); go(it.url); }));
      })(m.hist[i]);
    }
  }

  // ---------------- find in page ----------------
  function openFind() {
    findBar.className = 'show';
    setTimeout(function () { try { findQ.focus(); } catch (e) {} }, 100);
  }
  function closeFind() { findBar.className = ''; findQ.blur(); }
  function findGo(dir) { if (findQ.value) send({ t: 'find', q: findQ.value, dir: dir }); }
  document.getElementById('findprev').addEventListener('click', function () { findGo(-1); }, false);
  document.getElementById('findnext').addEventListener('click', function () { findGo(1); }, false);
  document.getElementById('findclose').addEventListener('click', closeFind, false);
  findQ.addEventListener('keydown', function (e) {
    if (e.keyCode === 13) { findGo(1); if (e.preventDefault) e.preventDefault(); }
    if (e.keyCode === 27) closeFind();
  }, false);

  // ---------------- websocket ----------------
  function handleJSON(m) {
    recent.unshift(m.t); if (recent.length > 8) recent.pop();
    switch (m.t) {
      case 'hello':
        // Resync if the server's viewport disagrees with this screen (a lost
        // or stale size message otherwise leaves us letterboxed).
        if (!kbdUp && (m.vw !== wrap.clientWidth || m.vh !== wrap.clientHeight)) queueSize();
        break;
      case 'tabs': renderTabs(m.tabs); break;
      case 'url':
        if (document.activeElement !== urlBox) urlBox.value = m.url;
        setStar(!!m.starred);
        break;
      case 'histstate':
        backBtn.disabled = !m.back;
        fwdBtn.disabled = !m.fwd;
        break;
      case 'loading': setLoading(!!m.on); break;
      case 'editable':
        if (m.on) {
          if (!kbdUp) {
            hidden.value = SENTINEL;
            hidden.focus();                 // works on some builds; pulse is the fallback cue
            kbdBtn.className = 'btn pulse';
            setHint('Text field — tap &#9000; to type');
          }
        } else {
          if (kbdUp) hidden.blur();
          kbdBtn.className = 'btn';
        }
        break;
      case 'zoom': zoom = m.scale || 1; break;
      case 'copytext': openCopySheet(m.text); break;
      case 'found': if (!m.on) setHint('Not found on this page'); break;
      case 'suggest': showSugg(m.items); break;
      case 'hist': showHist(m); break;
      case 'starred': setStar(!!m.on); break;
      case 'downloads': showDownloads(m.items); break;
      case 'download': setHint('Downloaded: ' + m.name); break;
      case 'toast': setHint(m.text); break;
    }
  }
  function connect() {
    var proto = (location.protocol === 'https:') ? 'wss://' : 'ws://';
    ws = new WebSocket(proto + location.host + '/ws?k=' + encodeURIComponent(window.__T || '') + '&v=' + encodeURIComponent(window.__V || ''));
    try { ws.binaryType = 'arraybuffer'; } catch (e) {}
    ws.onopen = function () {
      wsAttempts = 0;
      setHint('Connected');
      sendSize();
    };
    ws.onmessage = function (ev) {
      if (typeof ev.data !== 'string') { handleBinaryMessage(ev.data); return; }
      var m; try { m = JSON.parse(ev.data); } catch (e) { return; }
      handleJSON(m);
    };
    ws.onclose = function () {
      wsAttempts++;
      var delay = Math.min(15000, 1500 * Math.pow(2, Math.min(6, wsAttempts - 1)));
      setHint('Connection lost — reconnecting…', true);
      setTimeout(connect, delay);
    };
    ws.onerror = function () { try { ws.close(); } catch (e) {} };
  }
  setHint('Connecting…', true);
  layout();
  connect();
})();
