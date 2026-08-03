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
let videoLastKeyTimestamp = null;
let videoOutputReported = false;
let videoLatestFrame = null;
let videoLatestSourceSequence = 0;
let videoPendingFrames = [];
let videoPumpWake = null;

// Oversample Chromium's compositor at 60 Hz, then admit the newest fresh image
// to a source-driven 30 FPS pump. Asking the source for only 30 Hz made normal
// compositor jitter visible verbatim on the iPad. The latest-only handoff
// absorbs that jitter without ever building a queue of old pictures.
// Keep source capture at the presentation ceiling. GPU-backed tab frames must
// cross the compositor/codec boundary; oversampling that boundary at 60 Hz
// doubles work without increasing the native client's 30 FPS output.
const captureFrameRate = 30;
const outputFrameRate = 30;
function stopVideoEncoder() {
  videoGeneration++;
  videoPumpWake = null;
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
  if (videoLatestFrame) {
    videoLatestFrame.close();
    videoLatestFrame = null;
  }
  videoPendingFrames = [];
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
        } else if (event.data === "restart") {
          captureGeneration++;
          stopCapture();
          sendJSON({type: "inactive"});
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
        if (videoPumpWake) {
          videoPumpWake();
        }
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

function encodedVideoMessage(chunk, config, source) {
  const payload = new Uint8Array(chunk.byteLength);
  chunk.copyTo(payload);
  const message = new ArrayBuffer(20 + payload.byteLength);
  const bytes = new Uint8Array(message);
  bytes.set([0x53, 0x56, 0x49, 0x32], 0); // SVI2
  const view = new DataView(message);
  view.setUint8(4, (chunk.type === "key" ? 1 : 0) |
    (source.fresh ? 2 : 0));
  view.setUint16(6, 20, false);
  view.setUint16(8, config.width, false);
  view.setUint16(10, config.height, false);
  view.setUint32(12, payload.byteLength, false);
  view.setUint32(16, source.sequence, false);
  bytes.set(payload, 20);
  return message;
}

function captureVideoSize() {
  // Chromium's tab-capture source transposes the constrained track dimensions
  // when it builds the VideoFrame visible rectangle (with rotation=0). Ask for
  // the inverse of the desired encoded surface so both portrait and landscape
  // arrive with the exact client aspect instead of a letterboxed transpose.
  return {
    width: videoConfig.height,
    height: videoConfig.width,
  };
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
  const constraints = {
    width: {exact: captureSize.width},
    height: {exact: captureSize.height},
  };
  // Capture faster than the output clock so a compositor frame is normally
  // waiting when the next 30 Hz encode tick arrives.
  const capabilities = typeof track.getCapabilities === "function" ?
    track.getCapabilities() : {};
  const availableRate = capabilities && capabilities.frameRate ?
    capabilities.frameRate.max : 0;
  constraints.frameRate = {
    ideal: captureFrameRate,
    max: captureFrameRate,
  };
  try {
    await track.applyConstraints(constraints);
  } catch (error) {
    // Some Chromium builds advertise a rate for tabCapture, then reject the
    // combined size/rate constraint after an orientation change. Keep the
    // exact compositor dimensions and let the track choose its best cadence
    // instead of taking the whole video lane down.
    sendVideoJSON({
      type: "video-warning",
      error: String(error),
      constraint: error && error.constraint ? String(error.constraint) : "",
    });
    await track.applyConstraints({
      width: constraints.width,
      height: constraints.height,
    });
  }
  const settings = track.getSettings();
  const baseConfig = {
    codec: videoConfig.codec,
    width: videoConfig.width,
    height: videoConfig.height,
    framerate: outputFrameRate,
    latencyMode: "realtime",
    // Linux VirGL accelerates Chromium compositing but does not expose a
    // hardware video codec. Prefer the direct software AVC implementation so
    // WebCodecs does not route encode work back through the VirGL bridge.
    hardwareAcceleration: "prefer-software",
    contentHint: "detail",
    avc: {format: "annexb"},
  };
  // Constant-quantizer AVC gives text, icons and fine page edges a fixed
  // quality floor instead of allowing a bitrate controller to blur them.
  // QP 12 is effectively transparent for browser chrome at native size while
  // remaining practical on old Wi-Fi. Fall back to a deliberately generous
  // VBR target on encoders that do not implement WebCodecs quantizer mode.
  const requestedQuantizer = Number(videoConfig.quantizer);
  const quantizer = Number.isInteger(requestedQuantizer) ?
    Math.max(0, Math.min(51, requestedQuantizer)) : 12;
  let rateControl = "quantizer";
  let support = await VideoEncoder.isConfigSupported({
    ...baseConfig,
    bitrateMode: "quantizer",
  });
  if (!support.supported) {
    rateControl = "variable";
    support = await VideoEncoder.isConfigSupported({
      ...baseConfig,
      bitrate: videoConfig.bitrateK * 1000,
      bitrateMode: "variable",
    });
  }
  if (!support.supported || generation !== videoGeneration) {
    throw new Error("requested H.264 encoder configuration is unsupported");
  }
  const encoder = new VideoEncoder({
    output: (chunk, metadata) => {
      if (generation === videoGeneration &&
          activeSocket.readyState === WebSocket.OPEN) {
        const source = videoPendingFrames.shift() ||
          {sequence: 0, fresh: false};
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
        activeSocket.send(encodedVideoMessage(
          chunk, videoConfig, source));
        // A newer source image may have replaced the mailbox while this AU
        // was encoding. Give it an immediate chance to enter the paced pump
        // now that the one-in-flight slot is available.
        if (videoPumpWake) {
          videoPumpWake();
        }
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
  videoLastKeyTimestamp = null;
  videoOutputReported = false;
  videoLatestSourceSequence = 0;
  videoPendingFrames = [];
  sendVideoJSON({
    type: "video-active",
    sourceWidth: settings.width || 0,
    sourceHeight: settings.height || 0,
    sourceFPS: settings.frameRate || 0,
    sourceCapabilityFPS: availableRate || 0,
    rateControl,
    quantizer: rateControl === "quantizer" ? quantizer : -1,
  });

  // Keep exactly one latest raw image and at most one H.264 encode in flight.
  // The source is deliberately allowed to run faster than the 30 FPS output
  // ceiling, but a late tick is never made up with a burst. Static pages do
  // not consume encoder CPU: an unchanged image is encoded only when an
  // explicit keyframe request needs to bootstrap or repair a decoder.
  const intervalMS = 1000 / outputFrameRate;
  let lastSubmitAt = -intervalMS;
  let lastEncodedSourceSequence = 0;
  let encodeTimer = null;
  const scheduleVideoEncode = () => {
    if (generation !== videoGeneration || encoder.state !== "configured" ||
        !videoLatestFrame) {
      return;
    }
    const fresh = videoLatestSourceSequence !== lastEncodedSourceSequence;
    if (!fresh && !videoNeedsKeyframe) {
      return;
    }
    // encodeQueueSize only counts requests waiting to enter the codec; it can
    // return to zero before the matching output callback fires. The metadata
    // FIFO is therefore also the authoritative in-flight guard, keeping the
    // source-to-AU mapping one-frame-in/one-AU-out and bounded to one entry.
    if (videoPendingFrames.length > 0 || encoder.encodeQueueSize > 0) {
      return;
    }
    const now = performance.now();
    const waitMS = intervalMS - (now - lastSubmitAt);
    if (waitMS > 0.5) {
      if (encodeTimer === null) {
        encodeTimer = setTimeout(() => {
          encodeTimer = null;
          scheduleVideoEncode();
        }, waitMS);
      }
      return;
    }
    let frame;
    try {
      frame = videoLatestFrame.clone();
    } catch (_) {
      return;
    }
    const sourceSequence = videoLatestSourceSequence;
    const frameTimestamp = now * 1000;
    const keyFrame = videoNeedsKeyframe ||
      videoLastKeyTimestamp === null ||
      frameTimestamp < videoLastKeyTimestamp ||
      frameTimestamp - videoLastKeyTimestamp >= 2000000;
    if (videoFrameNumber === 0) {
      sendVideoJSON({
        type: "video-frame",
        codedWidth: frame.codedWidth,
        codedHeight: frame.codedHeight,
        displayWidth: frame.displayWidth,
        displayHeight: frame.displayHeight,
        visibleWidth: frame.visibleRect ? frame.visibleRect.width : 0,
        visibleHeight: frame.visibleRect ? frame.visibleRect.height : 0,
        rotation: Number.isFinite(frame.rotation) ? frame.rotation : 0,
      });
    }
    videoPendingFrames.push({
      sequence: sourceSequence,
      fresh,
    });
    try {
      const encodeOptions = {keyFrame};
      if (rateControl === "quantizer") {
        encodeOptions.avc = {quantizer};
      }
      encoder.encode(frame, encodeOptions);
    } catch (error) {
      videoPendingFrames.pop();
      frame.close();
      if (generation === videoGeneration) {
        sendVideoJSON({type: "video-error", error: String(error)});
      }
      return;
    }
    frame.close();
    lastSubmitAt = now;
    if (fresh) {
      lastEncodedSourceSequence = sourceSequence;
    }
    videoNeedsKeyframe = false;
    if (keyFrame) {
      videoLastKeyTimestamp = frameTimestamp;
    }
    videoFrameNumber++;
  };
  videoPumpWake = scheduleVideoEncode;

  const readFrames = async () => {
    try {
      while (generation === videoGeneration) {
        const {value: frame, done} = await reader.read();
        if (done) {
          break;
        }
        if (generation !== videoGeneration) {
          frame.close();
          break;
        }
        videoLatestSourceSequence++;
        const previous = videoLatestFrame;
        videoLatestFrame = frame;
        if (previous) {
          previous.close();
        }
        scheduleVideoEncode();
      }
    } catch (error) {
      if (generation === videoGeneration) {
        sendVideoJSON({type: "video-error", error: String(error)});
      }
    }
  };
  try {
    await readFrames();
  } catch (error) {
    if (generation === videoGeneration) {
      sendVideoJSON({type: "video-error", error: String(error)});
    }
  } finally {
    if (encodeTimer !== null) {
      clearTimeout(encodeTimer);
    }
    if (videoPumpWake === scheduleVideoEncode) {
      videoPumpWake = null;
    }
  }
}

async function startCapture(streamId) {
  const generation = ++captureGeneration;
  stopCapture();
  const activeSocket = await connectSocket();
  // Host-audio isolation starts before there is a video subscriber, so the
  // audio capture must not wait for the video bridge's first configuration.
  // StartVideo will reacquire this tab with a video track once configured.
  void connectVideoSocket().catch((error) => {
    sendJSON({type: "video-bridge-error", error: String(error)});
  });
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
      // Leave enough room for either source orientation at the exact current
      // client size. A viewport change reacquires the track with a new limit.
      video: videoConfig ? {
        mandatory: {
          chromeMediaSource: "tab",
          chromeMediaSourceId: streamId,
          maxWidth: Math.max(videoConfig.width, videoConfig.height),
          maxHeight: Math.max(videoConfig.width, videoConfig.height),
          maxFrameRate: captureFrameRate,
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
