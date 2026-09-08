// Service worker entry point for Surf's tab media bridge.
const OFFSCREEN_PATH = "offscreen.html";
let captureSequence = Promise.resolve();

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

async function captureTab(tab) {
  try {
    await ensureOffscreenDocument();
    // Release the previous stream before requesting another ID for its tab.
    // Stopping it only inside startCapture is too late: Chrome rejects the ID
    // request while that tab still has an active capture.
    await chrome.runtime.sendMessage({type: "prepare-capture"});
    const streamId = await chrome.tabCapture.getMediaStreamId({
      targetTabId: tab.id,
    });
    await chrome.runtime.sendMessage({type: "capture", streamId});
  } catch (error) {
    await ensureOffscreenDocument();
    await chrome.runtime.sendMessage({
      type: "capture-error",
      error: String(error && error.message ? error.message : error),
    });
  }
}

chrome.action.onClicked.addListener((tab) => {
  // Startup, tab activation, and video subscription can request capture at
  // once. Finish each acquisition before allowing another to replace it.
  captureSequence = captureSequence.then(() => captureTab(tab)).catch((error) => {
    console.error("Surf tab capture:", error);
  });
});
