class SurfPCMProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.chunk = new Int16Array(320);
    this.used = 0;
  }

  process(inputs, outputs) {
    const channels = inputs[0];
    if (channels && channels.length !== 0) {
      for (let i = 0; i < channels[0].length; i++) {
        let sample = 0;
        for (const channel of channels) {
          sample += channel[i];
        }
        sample /= channels.length;
        sample = Math.max(-1, Math.min(1, sample));
        this.chunk[this.used++] = sample < 0 ? sample * 32768 : sample * 32767;
        if (this.used === this.chunk.length) {
          const ready = this.chunk;
          this.port.postMessage(ready.buffer, [ready.buffer]);
          this.chunk = new Int16Array(320);
          this.used = 0;
        }
      }
    }
    for (const output of outputs) {
      for (const channel of output) {
        channel.fill(0);
      }
    }
    return true;
  }
}

registerProcessor("surf-pcm", SurfPCMProcessor);
