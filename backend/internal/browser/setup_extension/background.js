// This observer has no page scripts or network endpoint. Only Surf's private
// DevTools connection reads it. Chrome supplies real window/tab order.
let snapshot = {tabs: [], active: 0, windows: 0, ready: false};
let chain = Promise.resolve();
let closingWindows = new Set();
let focusedWindow = null;
let windowOrder = [];
let tabIDs = [];

async function refresh() {
  const windows = (await chrome.windows.getAll({populate: true, windowTypes: ["normal"]}))
    .filter(w => !w.incognito);
  const live = new Set(windows.map(w => w.id));
  windowOrder = windowOrder.filter(id => live.has(id));
  for (const w of windows) if (!windowOrder.includes(w.id)) windowOrder.push(w.id);
  windows.sort((a, b) => windowOrder.indexOf(a.id) - windowOrder.indexOf(b.id));
  // Closing a window removes its tabs individually. Do not record that
  // intermediate partial list as the session to restore.
  if (windows.some(w => closingWindows.has(w.id))) return;
  if (!windows.length) { snapshot.windows = 0; return; }
  const tabs = [];
  const ids = [];
  let active = -1;
  const activeWindow = (windows.find(w => w.focused) || {}).id ?? focusedWindow;
  for (const w of windows) {
    for (const t of (w.tabs || []).sort((a, b) => a.index - b.index)) {
      if (t.active && (active < 0 || w.id === activeWindow)) active = tabs.length;
      tabs.push(t.pendingUrl || t.url || "about:blank");
      ids.push(t.id);
    }
  }
  snapshot = {tabs, active: Math.max(0, active), windows: windows.length, ready: true};
  tabIDs = ids;
}
function schedule() { chain = chain.then(refresh).catch(() => {}); }
chrome.tabs.onCreated.addListener(schedule);
chrome.tabs.onUpdated.addListener(schedule);
chrome.tabs.onMoved.addListener(schedule);
chrome.tabs.onAttached.addListener(schedule);
chrome.tabs.onDetached.addListener(schedule);
chrome.tabs.onActivated.addListener(info => { focusedWindow = info.windowId; schedule(); });
chrome.tabs.onRemoved.addListener((id, info) => {
  if (info.isWindowClosing) closingWindows.add(info.windowId);
  else {
    // Explicitly removing the last tab is different from closing its window:
    // don't resurrect that tab if this also makes the last window disappear.
    const index = tabIDs.indexOf(id);
    if (index >= 0) {
      tabIDs.splice(index, 1);
      snapshot.tabs.splice(index, 1);
      if (snapshot.active >= index) snapshot.active = Math.max(0, snapshot.active - 1);
    }
  }
  schedule();
});
chrome.windows.onCreated.addListener(schedule);
chrome.windows.onRemoved.addListener(id => { closingWindows.delete(id); schedule(); });
chrome.windows.onFocusChanged.addListener(id => {
  if (id !== chrome.windows.WINDOW_ID_NONE) focusedWindow = id;
  schedule();
});
globalThis.surfSetupSnapshot = async () => { schedule(); await chain; return snapshot; };
schedule();
