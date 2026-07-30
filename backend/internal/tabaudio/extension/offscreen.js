let socket = null;
let socketPromise = null;
let stream = null;
let context = null;
let worklet = null;
let captureGeneration = 0;

function stopCapture() {
  if (worklet) {
    worklet.port.onmessage = null;
    worklet.disconnect();
    worklet = null;
  }
  if (stream) {
    for (const track of stream.getTracks()) {
      track.stop();
    }
    stream = null;
  }
  if (context) {
    void context.close();
    context = null;
  }
}

function sendJSON(value) {
  if (socket && socket.readyState === WebSocket.OPEN) {
    socket.send(JSON.stringify(value));
  }
}

function connectSocket() {
  if (socket && socket.readyState === WebSocket.OPEN) {
    return Promise.resolve(socket);
  }
  if (socketPromise) {
    return socketPromise;
  }
  socketPromise = new Promise((resolve, reject) => {
    const next = new WebSocket(globalThis.SURF_AUDIO_CONFIG.url);
    next.binaryType = "arraybuffer";
    next.onopen = () => {
      socket = next;
      socketPromise = null;
      next.onmessage = (event) => {
        if (event.data === "stop") {
          captureGeneration++;
          stopCapture();
        }
      };
      next.onclose = () => {
        if (socket === next) {
          socket = null;
        }
      };
      resolve(next);
    };
    next.onerror = () => {
      socketPromise = null;
      reject(new Error("audio bridge connection failed"));
    };
  });
  return socketPromise;
}

async function startCapture(streamId) {
  const generation = ++captureGeneration;
  stopCapture();
  const activeSocket = await connectSocket();
  let nextStream = null;
  let nextContext = null;
  try {
    nextStream = await navigator.mediaDevices.getUserMedia({
      audio: {
        mandatory: {
          chromeMediaSource: "tab",
          chromeMediaSourceId: streamId,
        },
      },
      video: false,
    });
    nextContext = new AudioContext({
      sampleRate: 16000,
      latencyHint: "interactive",
    });
    await nextContext.audioWorklet.addModule("pcm-worklet.js");
    if (generation !== captureGeneration) {
      for (const track of nextStream.getTracks()) {
        track.stop();
      }
      void nextContext.close();
      return;
    }
    stream = nextStream;
    context = nextContext;
    const source = context.createMediaStreamSource(stream);
    worklet = new AudioWorkletNode(context, "surf-pcm");
    worklet.port.onmessage = (event) => {
      if (activeSocket.readyState === WebSocket.OPEN) {
        activeSocket.send(event.data);
      }
    };
    source.connect(worklet);
    // The worklet must be connected to an output to remain scheduled. It
    // emits silence, so this does not restore the tab's local playback.
    worklet.connect(context.destination);
    sendJSON({type: "active", sampleRate: context.sampleRate});
  } catch (error) {
    if (nextStream) {
      for (const track of nextStream.getTracks()) {
        track.stop();
      }
    }
    if (nextContext) {
      void nextContext.close();
    }
    if (generation === captureGeneration) {
      sendJSON({
        type: "error",
        error: String(error && error.message ? error.message : error),
      });
    }
  }
}

chrome.runtime.onMessage.addListener((message) => {
  if (message.type === "capture") {
    void startCapture(message.streamId);
  } else if (message.type === "capture-error") {
    void connectSocket().then(() => {
      sendJSON({type: "error", error: message.error});
    });
  }
});
