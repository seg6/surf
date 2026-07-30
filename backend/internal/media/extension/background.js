// Service worker entry point for Surf's tab media bridge.
const OFFSCREEN_PATH = "offscreen.html";

async function ensureOffscreenDocument() {
  const url = chrome.runtime.getURL(OFFSCREEN_PATH);
  const contexts = await chrome.runtime.getContexts({
    contextTypes: ["OFFSCREEN_DOCUMENT"],
    documentUrls: [url],
  });
  if (contexts.length !== 0) {
    return;
  }
  await chrome.offscreen.createDocument({
    url: OFFSCREEN_PATH,
    reasons: ["USER_MEDIA"],
    justification: "Stream the active tab to the Surf client",
  });
}

chrome.action.onClicked.addListener(async (tab) => {
  try {
    const streamId = await chrome.tabCapture.getMediaStreamId({
      targetTabId: tab.id,
    });
    await ensureOffscreenDocument();
    await chrome.runtime.sendMessage({type: "capture", streamId});
  } catch (error) {
    await ensureOffscreenDocument();
    await chrome.runtime.sendMessage({
      type: "capture-error",
      error: String(error && error.message ? error.message : error),
    });
  }
});
