// Offscreen tab media capture and encoding.
let socket = null;
let socketPromise = null;
let stream = null;
let context = null;
let worklet = null;
let captureGeneration = 0;

let videoSocket = null;
let videoSocketPromise = null;
let videoConfig = null;
let videoReader = null;
let videoEncoder = null;
let videoGeneration = 0;
let videoNeedsKeyframe = true;
let videoFrameNumber = 0;
let videoOutputReported = false;

function stopVideoEncoder() {
  videoGeneration++;
  if (videoReader) {
    void videoReader.cancel();
    videoReader = null;
  }
  if (videoEncoder) {
    if (videoEncoder.state !== "closed") {
      videoEncoder.close();
    }
    videoEncoder = null;
  }
}

function stopCapture() {
  stopVideoEncoder();
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

function sendVideoJSON(value) {
  if (videoSocket && videoSocket.readyState === WebSocket.OPEN) {
    videoSocket.send(JSON.stringify(value));
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
    const next = new WebSocket(globalThis.SURF_CAPTURE_CONFIG.audioUrl);
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

function connectVideoSocket() {
  if (videoSocket && videoSocket.readyState === WebSocket.OPEN) {
    return Promise.resolve(videoSocket);
  }
  if (videoSocketPromise) {
    return videoSocketPromise;
  }
  videoSocketPromise = new Promise((resolve, reject) => {
    const next = new WebSocket(globalThis.SURF_CAPTURE_CONFIG.videoUrl);
    next.binaryType = "arraybuffer";
    next.onmessage = (event) => {
      if (typeof event.data !== "string") {
        return;
      }
      let message;
      try {
        message = JSON.parse(event.data);
      } catch (_) {
        return;
      }
      if (message.type === "configure") {
        videoConfig = message;
        videoSocketPromise = null;
        resolve(next);
        if (stream && stream.getVideoTracks().length !== 0) {
          void startVideoEncoder(stream.getVideoTracks()[0]).catch((error) => {
            sendVideoJSON({type: "video-error", error: String(error)});
          });
        }
      } else if (message.type === "keyframe") {
        videoNeedsKeyframe = true;
      } else if (message.type === "stop-video") {
        stopVideoEncoder();
      }
    };
    next.onopen = () => {
      videoSocket = next;
      next.onclose = () => {
        if (videoSocket === next) {
          videoSocket = null;
          stopVideoEncoder();
        }
      };
    };
    next.onerror = () => {
      videoSocketPromise = null;
      reject(new Error("video bridge connection failed"));
    };
  });
  return videoSocketPromise;
}

function encodedVideoMessage(chunk, config) {
  const payload = new Uint8Array(chunk.byteLength);
  chunk.copyTo(payload);
  const message = new ArrayBuffer(16 + payload.byteLength);
  const bytes = new Uint8Array(message);
  bytes.set([0x53, 0x56, 0x49, 0x31], 0); // SVI1
  const view = new DataView(message);
  view.setUint8(4, chunk.type === "key" ? 1 : 0);
  view.setUint16(6, 16, false);
  view.setUint16(8, config.width, false);
  view.setUint16(10, config.height, false);
  view.setUint32(12, payload.byteLength, false);
  bytes.set(payload, 16);
  return message;
}

function captureVideoSize() {
  // Chrome tabCapture's constraint axes are transposed relative to the raw
  // VideoFrame display axes. This is a dimension transform, not a device
  // resolution: it applies to every client-provided width and height.
  return {width: videoConfig.height, height: videoConfig.width};
}

async function startVideoEncoder(track) {
  stopVideoEncoder();
  const generation = videoGeneration;
  const activeSocket = await connectVideoSocket();
  if (!activeSocket || !videoConfig || generation !== videoGeneration) {
    return;
  }
  // tabCapture follows Chromium's compositor, while these constraints keep
  // the track and encoder synchronized across orientation/viewport changes.
  // "detail" asks Chromium to preserve browser text and edges rather than
  // optimize primarily for camera-like motion.
  track.contentHint = "detail";
  const captureSize = captureVideoSize();
  await track.applyConstraints({
    width: {exact: captureSize.width},
    height: {exact: captureSize.height},
    frameRate: {ideal: videoConfig.fps, max: videoConfig.fps},
  });
  const config = {
    codec: videoConfig.codec,
    width: videoConfig.width,
    height: videoConfig.height,
    bitrate: videoConfig.bitrateK * 1000,
    framerate: videoConfig.fps,
    // Variable mode spends the same target bitrate where browser text and
    // scrolling are complex instead of padding static frames.
    bitrateMode: "variable",
    latencyMode: "realtime",
    hardwareAcceleration: "no-preference",
    contentHint: "detail",
    avc: {format: "annexb"},
  };
  const support = await VideoEncoder.isConfigSupported(config);
  if (!support.supported || generation !== videoGeneration) {
    throw new Error("requested H.264 encoder configuration is unsupported");
  }
  const encoder = new VideoEncoder({
    output: (chunk, metadata) => {
      if (generation === videoGeneration &&
          activeSocket.readyState === WebSocket.OPEN) {
        if (!videoOutputReported) {
          videoOutputReported = true;
          const decoder = metadata && metadata.decoderConfig ?
            metadata.decoderConfig : {};
          sendVideoJSON({
            type: "video-output",
            codedWidth: decoder.codedWidth || 0,
            codedHeight: decoder.codedHeight || 0,
            displayWidth: decoder.displayAspectWidth || 0,
            displayHeight: decoder.displayAspectHeight || 0,
          });
        }
        activeSocket.send(encodedVideoMessage(chunk, videoConfig));
      }
    },
    error: (error) => {
      if (generation === videoGeneration) {
        sendVideoJSON({type: "video-error", error: String(error)});
      }
    },
  });
  encoder.configure(support.config);
  videoEncoder = encoder;
  const processor = new MediaStreamTrackProcessor({
    track,
    maxBufferSize: 1,
  });
  const reader = processor.readable.getReader();
  videoReader = reader;
  videoNeedsKeyframe = true;
  videoFrameNumber = 0;
  videoOutputReported = false;
  const settings = track.getSettings();
  sendVideoJSON({
    type: "video-active",
    sourceWidth: settings.width || 0,
    sourceHeight: settings.height || 0,
    sourceFPS: settings.frameRate || 0,
  });

  try {
    while (generation === videoGeneration) {
      const {value: frame, done} = await reader.read();
      if (done) {
        break;
      }
      if (encoder.encodeQueueSize > 1) {
        frame.close();
        continue;
      }
      if (videoFrameNumber === 0) {
        sendVideoJSON({
          type: "video-frame",
          codedWidth: frame.codedWidth,
          codedHeight: frame.codedHeight,
          displayWidth: frame.displayWidth,
          displayHeight: frame.displayHeight,
        });
      }
      const keyInterval = Math.max(1, videoConfig.fps * 2);
      const keyFrame = videoNeedsKeyframe ||
        videoFrameNumber % keyInterval === 0;
      videoNeedsKeyframe = false;
      videoFrameNumber++;
      encoder.encode(frame, {keyFrame});
      frame.close();
    }
  } catch (error) {
    if (generation === videoGeneration) {
      sendVideoJSON({type: "video-error", error: String(error)});
    }
  }
}

async function startCapture(streamId) {
  const generation = ++captureGeneration;
  stopCapture();
  const activeSocket = await connectSocket();
  await connectVideoSocket();
  let nextStream = null;
  let nextContext = null;
  try {
    const maxVideoDimension = videoConfig ?
      Math.max(videoConfig.width, videoConfig.height) : 0;
    nextStream = await navigator.mediaDevices.getUserMedia({
      audio: {
        mandatory: {
          chromeMediaSource: "tab",
          chromeMediaSourceId: streamId,
        },
      },
      // Leave enough room for either source orientation. The video track is
      // constrained to the client-derived dimensions before encoding.
      video: videoConfig ? {
        mandatory: {
          chromeMediaSource: "tab",
          chromeMediaSourceId: streamId,
          maxWidth: maxVideoDimension,
          maxHeight: maxVideoDimension,
          maxFrameRate: videoConfig.fps,
        },
      } : false,
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
    if (stream.getVideoTracks().length !== 0 && videoConfig) {
      void startVideoEncoder(stream.getVideoTracks()[0]).catch((error) => {
        sendVideoJSON({type: "video-error", error: String(error)});
      });
    }
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
