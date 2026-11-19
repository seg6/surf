package browser

// webauthnShim disables passkey prompts inside the remote Chromium: sites
// would otherwise try to use a platform authenticator that doesn't exist in
// the backend browser and dead-end the login flow. Ported verbatim from server.js.
const webauthnShim = `
(function () {
  function notAllowed() {
    var e;
    try { e = new DOMException('Passkeys are disabled in this remote browser.', 'NotAllowedError'); }
    catch (_) { e = new Error('NotAllowedError'); e.name = 'NotAllowedError'; }
    return Promise.reject(e);
  }
  function define(target, name, value) {
    try {
      Object.defineProperty(target, name, { configurable: true, value: value });
      return true;
    } catch (_) { return false; }
  }
  function wrapCredentials(creds) {
    if (!creds || creds.__surfNoPasskeys) return;
    var origGet = creds.get;
    var origCreate = creds.create;
    var proto = null;
    try { proto = Object.getPrototypeOf(creds); } catch (_) {}
    var get = function (opts) {
      if (opts && opts.publicKey) return notAllowed();
      return origGet ? origGet.apply(this, arguments) : notAllowed();
    };
    var create = function (opts) {
      if (opts && opts.publicKey) return notAllowed();
      return origCreate ? origCreate.apply(this, arguments) : notAllowed();
    };
    if (!define(creds, 'get', get) && proto) define(proto, 'get', get);
    if (!define(creds, 'create', create) && proto) define(proto, 'create', create);
    try { Object.defineProperty(creds, '__surfNoPasskeys', { value: true }); } catch (_) {}
  }
  wrapCredentials(navigator.credentials);
  try {
    if (window.PublicKeyCredential) {
      window.PublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable = function () { return Promise.resolve(false); };
      window.PublicKeyCredential.isConditionalMediationAvailable = function () { return Promise.resolve(false); };
      Object.defineProperty(window, 'PublicKeyCredential', { configurable: true, get: function () { return undefined; } });
    }
  } catch (_) {}
})();
`

// Keep the remote page pinned at its scroll limits. Chromium owns touch
// gestures, but root-page rubber-banding still looks like the streamed texture
// itself is being dragged beyond the viewport. The value only belongs on the
// element that defines the viewport scroller: <body> when <html> is visible
// (its overflow propagates), otherwise <html>. A <body> that is its own scroll
// container (for example overflow:hidden under a scrolling <html>) with
// overscroll-behavior:none swallows touch scrolling instead of chaining it.
const overscrollShim = `
(function () {
  function pinRootScroller() {
    var root = document.documentElement;
    if (!root) return;
    root.style.overscrollBehavior = 'none';
    var style = window.getComputedStyle(root);
    if (document.body && style.overflowX === 'visible' && style.overflowY === 'visible') {
      document.body.style.overscrollBehavior = 'none';
    }
  }
  pinRootScroller();
  document.addEventListener('DOMContentLoaded', pinRootScroller, false);
  window.addEventListener('load', pinRootScroller, false);
})();
`

func (b *Controller) installCompatScripts(session string) {
	_ = b.cdp.Dispatch(session, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": webauthnShim})
	_ = b.cdp.Dispatch(session, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": overscrollShim})
}
