// Converts captured tab audio to the native client's PCM format: interleaved
// signed 16-bit samples in chunks of exactly 20 ms. The sample rate is the
// AudioContext's; the channel count comes from processorOptions.
class SurfPCMProcessor extends AudioWorkletProcessor {
  constructor(options) {
    super();
    const config = (options && options.processorOptions) || {};
    this.channels = config.channels === 2 ? 2 : 1;
    const rate = config.sampleRate > 0 ? config.sampleRate : sampleRate;
    this.chunkSamples = Math.round(rate / 50) * this.channels;
    this.chunk = new Int16Array(this.chunkSamples);
    this.used = 0;
  }

  push(sample) {
    sample = Math.max(-1, Math.min(1, sample));
    this.chunk[this.used++] = sample < 0 ? sample * 32768 : sample * 32767;
  }

  process(inputs, outputs) {
    const channels = inputs[0];
    if (channels && channels.length !== 0) {
      for (let i = 0; i < channels[0].length; i++) {
        if (this.channels === 2) {
          // Mono input is duplicated; with more than two channels only the
          // front pair is kept.
          const left = channels[0][i];
          this.push(left);
          this.push(channels.length > 1 ? channels[1][i] : left);
        } else {
          let sample = 0;
          for (const channel of channels) {
            sample += channel[i];
          }
          this.push(sample / channels.length);
        }
        if (this.used === this.chunk.length) {
          const ready = this.chunk;
          this.port.postMessage(ready.buffer, [ready.buffer]);
          this.chunk = new Int16Array(this.chunkSamples);
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
