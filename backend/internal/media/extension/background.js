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

// Capturing a tab silences that tab on the host, but nothing silences the
// others: a tab that has just been left, or one playing in the background,
// would come out of the host's own speakers. The client hears the stream, so
// every tab is muted for the host's output. Muting does not affect what
// tabCapture delivers, so the captured tab keeps sending audio.
function muteTab(tab) {
  if (!tab || tab.id === undefined || (tab.mutedInfo && tab.mutedInfo.muted)) {
    return;
  }
  chrome.tabs.update(tab.id, {muted: true}).catch(() => {});
}

function muteAllTabs() {
  chrome.tabs.query({}).then((tabs) => tabs.forEach(muteTab)).catch(() => {});
}

muteAllTabs();
chrome.runtime.onStartup.addListener(muteAllTabs);
chrome.tabs.onCreated.addListener(muteTab);
chrome.tabs.onUpdated.addListener((_id, changeInfo, tab) => {
  if (changeInfo.mutedInfo || changeInfo.status === "loading") {
    muteTab(tab);
  }
});

chrome.action.onClicked.addListener((tab) => {
  // Startup, tab activation, and video subscription can request capture at
  // once. Finish each acquisition before allowing another to replace it.
  captureSequence = captureSequence.then(() => captureTab(tab)).catch((error) => {
    console.error("Surf tab capture:", error);
  });
});
