//! Bounded desktop media pipeline.
//!
//! The network thread only replaces an encoded-frame slot. A dedicated worker
//! owns FFmpeg, and the UI only takes the newest decoded YUV image. No lane can
//! form an unbounded queue behind another lane.

use std::collections::VecDeque;
use std::sync::atomic::{AtomicBool, AtomicI64, AtomicU32, AtomicU64, Ordering};
use std::sync::mpsc::{self, Receiver, SyncSender};
use std::sync::{Arc, Condvar, Mutex, OnceLock, Weak};
use std::thread;
use std::time::{Duration, Instant};

use bytes::Bytes;
use cpal::SizedSample;
use cpal::traits::{DeviceTrait as _, HostTrait as _, StreamTrait as _};
use ffmpeg_the_third as ffmpeg;
use surf_core::{Frame, FrameKind, MediaAction, MediaAdmissionPolicy, monotonic_ns};
use surf_session::FrameSink;
use thiserror::Error;

const AUDIO_QUEUE_CAPACITY: usize = 8;
const AUDIO_BUFFER_MS: usize = 120;
const AUDIO_PRIME_MS: usize = 60;
const BUFFER_POOL_CAPACITY: usize = 3;

static FFMPEG_INIT: OnceLock<std::result::Result<(), String>> = OnceLock::new();

pub type Result<T> = std::result::Result<T, MediaError>;

#[derive(Debug, Error)]
pub enum MediaError {
    #[error("FFmpeg initialization failed: {0}")]
    Initialization(String),
    #[error("H.264 decoder is unavailable")]
    DecoderUnavailable,
    #[error("H.264 decoder failed: {0}")]
    Decoder(String),
    #[error("unsupported decoder pixel format: {0}")]
    PixelFormat(String),
    #[error("media worker could not start: {0}")]
    Worker(#[from] std::io::Error),
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum MediaEvent {
    RequestKeyframe,
    DecoderError(String),
    AudioReady { sample_rate: u32, channels: u16 },
    AudioUnavailable(String),
    AudioError(String),
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct Diagnostics {
    pub ingress_frames: u64,
    pub ingress_replaced: u64,
    pub ingress_contended: u64,
    pub audio_dropped: u64,
    pub audio_packets: u64,
    pub audio_queue_dropped_samples: u64,
    pub audio_underruns: u64,
    pub audio_device_errors: u64,
    pub video_packets: u64,
    pub decoded_frames: u64,
    pub output_replaced: u64,
    pub decode_errors: u64,
    pub generation_resets: u64,
    pub gaps: u64,
    pub waiting_for_idr: u64,
    pub keyframe_requests: u64,
    pub latest_decode_us: u64,
    pub latest_backend_capture_to_encode_us: u64,
    pub latest_backend_encode_to_write_us: u64,
    pub latest_network_us: u64,
    pub encoded_video_depth: u32,
    pub decoded_video_depth: u32,
    pub audio_depth: u32,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum ColorRange {
    Limited,
    Full,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum ColorMatrix {
    Bt601,
    Bt709,
}

#[derive(Clone, Copy, Debug)]
struct PacketMeta {
    sequence: u32,
    source_sequence: u32,
    generation: u32,
    interaction_id: u64,
    width: u16,
    height: u16,
    source_client_ns: Option<u64>,
    ingress_receive_ns: u64,
}

#[derive(Default)]
struct FrameBuffers {
    y: Vec<u8>,
    u: Vec<u8>,
    v: Vec<u8>,
}

#[derive(Default)]
struct BufferPool {
    frames: Mutex<Vec<FrameBuffers>>,
}

impl BufferPool {
    fn take(&self) -> FrameBuffers {
        self.frames
            .lock()
            .ok()
            .and_then(|mut frames| frames.pop())
            .unwrap_or_default()
    }

    fn give(&self, buffers: FrameBuffers) {
        if let Ok(mut frames) = self.frames.lock()
            && frames.len() < BUFFER_POOL_CAPACITY
        {
            frames.push(buffers);
        }
    }
}

/// One compact planar YUV420 image. Its storage automatically returns to the
/// bounded decoder pool when the renderer releases the frame.
pub struct DecodedFrame {
    buffers: Option<FrameBuffers>,
    pool: Weak<BufferPool>,
    pub width: u32,
    pub height: u32,
    pub sequence: u32,
    pub source_sequence: u32,
    pub generation: u32,
    pub interaction_id: u64,
    pub color_range: ColorRange,
    pub color_matrix: ColorMatrix,
    pub decode_time: Duration,
    pub source_client_ns: Option<u64>,
    pub ingress_receive_ns: u64,
}

impl std::fmt::Debug for DecodedFrame {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("DecodedFrame")
            .field("width", &self.width)
            .field("height", &self.height)
            .field("sequence", &self.sequence)
            .field("generation", &self.generation)
            .field("decode_time", &self.decode_time)
            .finish_non_exhaustive()
    }
}

impl DecodedFrame {
    pub fn y(&self) -> &[u8] {
        &self.buffers.as_ref().expect("live frame has buffers").y
    }

    pub fn u(&self) -> &[u8] {
        &self.buffers.as_ref().expect("live frame has buffers").u
    }

    pub fn v(&self) -> &[u8] {
        &self.buffers.as_ref().expect("live frame has buffers").v
    }
}

impl Drop for DecodedFrame {
    fn drop(&mut self) {
        if let (Some(pool), Some(buffers)) = (self.pool.upgrade(), self.buffers.take()) {
            pool.give(buffers);
        }
    }
}

#[derive(Default)]
struct Counters {
    ingress_frames: AtomicU64,
    ingress_replaced: AtomicU64,
    ingress_contended: AtomicU64,
    audio_dropped: AtomicU64,
    audio_packets: AtomicU64,
    audio_queue_dropped_samples: AtomicU64,
    audio_underruns: AtomicU64,
    audio_device_errors: AtomicU64,
    video_packets: AtomicU64,
    decoded_frames: AtomicU64,
    output_replaced: AtomicU64,
    decode_errors: AtomicU64,
    generation_resets: AtomicU64,
    gaps: AtomicU64,
    waiting_for_idr: AtomicU64,
    keyframe_requests: AtomicU64,
    latest_decode_us: AtomicU64,
    latest_backend_capture_to_encode_us: AtomicU64,
    latest_backend_encode_to_write_us: AtomicU64,
    latest_network_us: AtomicU64,
}

impl Counters {
    fn snapshot(&self) -> Diagnostics {
        Diagnostics {
            ingress_frames: self.ingress_frames.load(Ordering::Relaxed),
            ingress_replaced: self.ingress_replaced.load(Ordering::Relaxed),
            ingress_contended: self.ingress_contended.load(Ordering::Relaxed),
            audio_dropped: self.audio_dropped.load(Ordering::Relaxed),
            audio_packets: self.audio_packets.load(Ordering::Relaxed),
            audio_queue_dropped_samples: self.audio_queue_dropped_samples.load(Ordering::Relaxed),
            audio_underruns: self.audio_underruns.load(Ordering::Relaxed),
            audio_device_errors: self.audio_device_errors.load(Ordering::Relaxed),
            video_packets: self.video_packets.load(Ordering::Relaxed),
            decoded_frames: self.decoded_frames.load(Ordering::Relaxed),
            output_replaced: self.output_replaced.load(Ordering::Relaxed),
            decode_errors: self.decode_errors.load(Ordering::Relaxed),
            generation_resets: self.generation_resets.load(Ordering::Relaxed),
            gaps: self.gaps.load(Ordering::Relaxed),
            waiting_for_idr: self.waiting_for_idr.load(Ordering::Relaxed),
            keyframe_requests: self.keyframe_requests.load(Ordering::Relaxed),
            latest_decode_us: self.latest_decode_us.load(Ordering::Relaxed),
            latest_backend_capture_to_encode_us: self
                .latest_backend_capture_to_encode_us
                .load(Ordering::Relaxed),
            latest_backend_encode_to_write_us: self
                .latest_backend_encode_to_write_us
                .load(Ordering::Relaxed),
            latest_network_us: self.latest_network_us.load(Ordering::Relaxed),
            encoded_video_depth: 0,
            decoded_video_depth: 0,
            audio_depth: 0,
        }
    }
}

struct EncodedFrame {
    epoch: u64,
    received_ns: u64,
    bytes: Bytes,
}

#[derive(Default)]
struct IngressState {
    video: Option<EncodedFrame>,
    audio: VecDeque<EncodedFrame>,
}

#[derive(Default)]
struct AudioQueueState {
    samples: VecDeque<f32>,
    last_sequence: Option<u32>,
}

struct AudioPlayback {
    state: Mutex<AudioQueueState>,
    input_rate: AtomicU32,
    input_channels: AtomicU32,
    primed: AtomicBool,
}

impl AudioPlayback {
    fn new() -> Self {
        Self {
            state: Mutex::new(AudioQueueState::default()),
            input_rate: AtomicU32::new(0),
            input_channels: AtomicU32::new(0),
            primed: AtomicBool::new(false),
        }
    }

    fn clear(&self) {
        if let Ok(mut state) = self.state.lock() {
            state.samples.clear();
            state.last_sequence = None;
        }
        self.primed.store(false, Ordering::Release);
    }

    fn push(&self, frame: &Frame<'_>, counters: &Counters) -> std::result::Result<(), String> {
        let sample_rate = u32::from(frame.width);
        let channels = usize::from(frame.height);
        if !(8_000..=192_000).contains(&sample_rate) || !(1..=8).contains(&channels) {
            return Err(format!(
                "invalid PCM configuration {sample_rate} Hz, {channels} channels"
            ));
        }
        let bytes_per_frame = channels
            .checked_mul(2)
            .ok_or_else(|| "PCM frame width overflow".to_owned())?;
        if frame.payload.is_empty() || !frame.payload.len().is_multiple_of(bytes_per_frame) {
            return Err("PCM payload is not complete signed 16-bit frames".to_owned());
        }
        let previous_rate = self.input_rate.swap(sample_rate, Ordering::AcqRel);
        let previous_channels = self.input_channels.swap(
            u32::try_from(channels).unwrap_or(u32::MAX),
            Ordering::AcqRel,
        );
        let mut state = self
            .state
            .lock()
            .map_err(|_| "PCM queue lock was poisoned".to_owned())?;
        let discontinuity = state
            .last_sequence
            .is_some_and(|sequence| sequence.wrapping_add(1) != frame.sequence);
        if previous_rate != sample_rate
            || previous_channels != u32::try_from(channels).unwrap_or(u32::MAX)
            || discontinuity
        {
            state.samples.clear();
            self.primed.store(false, Ordering::Release);
        }
        state.last_sequence = Some(frame.sequence);

        let capacity = usize::try_from(sample_rate)
            .unwrap_or(usize::MAX)
            .saturating_mul(AUDIO_BUFFER_MS)
            / 1_000;
        let incoming = frame.payload.len() / bytes_per_frame;
        let overflow = state
            .samples
            .len()
            .saturating_add(incoming)
            .saturating_sub(capacity);
        let drop_existing = overflow.min(state.samples.len());
        for _ in 0..drop_existing {
            state.samples.pop_front();
        }
        counters.audio_queue_dropped_samples.fetch_add(
            u64::try_from(overflow).unwrap_or(u64::MAX),
            Ordering::Relaxed,
        );

        let skip_incoming = overflow.saturating_sub(drop_existing);
        for pcm_frame in frame
            .payload
            .chunks_exact(bytes_per_frame)
            .skip(skip_incoming)
        {
            let mut mixed = 0.0_f32;
            for channel in 0..channels {
                let offset = channel * 2;
                let sample = i16::from_le_bytes([pcm_frame[offset], pcm_frame[offset + 1]]);
                mixed += f32::from(sample) / 32_768.0;
            }
            state.samples.push_back(mixed / channels as f32);
        }
        Ok(())
    }
}

#[derive(Default)]
struct AudioRenderState {
    input_rate: u32,
    previous: f32,
    next: f32,
    phase: f64,
    initialized: bool,
}

struct Shared {
    ingress: Mutex<IngressState>,
    video_wake: Condvar,
    audio_wake: Condvar,
    audio: AudioPlayback,
    output: Mutex<Option<DecodedFrame>>,
    epoch: AtomicU64,
    clock_synchronized: AtomicBool,
    server_minus_client_ns: AtomicI64,
    shutdown: AtomicBool,
    counters: Counters,
}

impl Shared {
    fn new() -> Self {
        Self {
            ingress: Mutex::new(IngressState::default()),
            video_wake: Condvar::new(),
            audio_wake: Condvar::new(),
            audio: AudioPlayback::new(),
            output: Mutex::new(None),
            epoch: AtomicU64::new(1),
            clock_synchronized: AtomicBool::new(false),
            server_minus_client_ns: AtomicI64::new(0),
            shutdown: AtomicBool::new(false),
            counters: Counters::default(),
        }
    }
}

impl FrameSink for Shared {
    fn submit(&self, frame: Bytes) {
        self.counters.ingress_frames.fetch_add(1, Ordering::Relaxed);
        let epoch = self.epoch.load(Ordering::Acquire);
        let Some(kind) = frame.get(4).copied() else {
            return;
        };
        let Ok(mut ingress) = self.ingress.try_lock() else {
            self.counters
                .ingress_contended
                .fetch_add(1, Ordering::Relaxed);
            return;
        };
        let encoded = EncodedFrame {
            epoch,
            received_ns: monotonic_ns(),
            bytes: frame,
        };
        match kind {
            3 => {
                if ingress.video.replace(encoded).is_some() {
                    self.counters
                        .ingress_replaced
                        .fetch_add(1, Ordering::Relaxed);
                }
                drop(ingress);
                self.video_wake.notify_one();
            }
            4 => {
                if ingress.audio.len() == AUDIO_QUEUE_CAPACITY {
                    ingress.audio.pop_front();
                    self.counters.audio_dropped.fetch_add(1, Ordering::Relaxed);
                }
                ingress.audio.push_back(encoded);
                drop(ingress);
                self.audio_wake.notify_one();
            }
            _ => (),
        }
    }

    fn clear(&self) {
        self.epoch.fetch_add(1, Ordering::AcqRel);
        if let Ok(mut ingress) = self.ingress.lock() {
            ingress.video = None;
            ingress.audio.clear();
        }
        if let Ok(mut output) = self.output.lock() {
            *output = None;
        }
        self.audio.clear();
        self.video_wake.notify_one();
        self.audio_wake.notify_one();
    }
}

pub struct MediaPipeline {
    shared: Arc<Shared>,
    events: Receiver<MediaEvent>,
    workers: Vec<thread::JoinHandle<()>>,
}

impl MediaPipeline {
    pub fn spawn() -> Result<Self> {
        initialize_ffmpeg()?;
        let shared = Arc::new(Shared::new());
        let (event_tx, events) = mpsc::sync_channel(16);
        let video_shared = Arc::clone(&shared);
        let video_events = event_tx.clone();
        let video_worker = thread::Builder::new()
            .name("surf-video-decode".to_owned())
            .spawn(move || decoder_worker(video_shared, video_events))?;
        let audio_shared = Arc::clone(&shared);
        let audio_worker = match thread::Builder::new()
            .name("surf-audio-output".to_owned())
            .spawn(move || audio_worker(audio_shared, event_tx))
        {
            Ok(worker) => worker,
            Err(error) => {
                shared.shutdown.store(true, Ordering::Release);
                shared.video_wake.notify_one();
                let _ = video_worker.join();
                return Err(error.into());
            }
        };
        Ok(Self {
            shared,
            events,
            workers: vec![video_worker, audio_worker],
        })
    }

    pub fn frame_sink(&self) -> Arc<dyn FrameSink> {
        self.shared.clone()
    }

    pub fn take_latest_frame(&self) -> Option<DecodedFrame> {
        self.shared.output.lock().ok()?.take()
    }

    pub fn try_recv_event(&self) -> Option<MediaEvent> {
        self.events.try_recv().ok()
    }

    pub fn diagnostics(&self) -> Diagnostics {
        let mut diagnostics = self.shared.counters.snapshot();
        if let Ok(ingress) = self.shared.ingress.try_lock() {
            diagnostics.encoded_video_depth = u32::from(ingress.video.is_some());
            diagnostics.audio_depth = u32::try_from(ingress.audio.len()).unwrap_or(u32::MAX);
        }
        if let Ok(output) = self.shared.output.try_lock() {
            diagnostics.decoded_video_depth = u32::from(output.is_some());
        }
        diagnostics
    }

    pub fn set_clock_offset(&self, server_minus_client_ns: Option<i64>) {
        if let Some(offset) = server_minus_client_ns {
            self.shared
                .clock_synchronized
                .store(false, Ordering::Release);
            self.shared
                .server_minus_client_ns
                .store(offset, Ordering::Release);
            self.shared
                .clock_synchronized
                .store(true, Ordering::Release);
        } else {
            self.shared
                .clock_synchronized
                .store(false, Ordering::Release);
        }
    }

    pub fn clear(&self) {
        self.shared.clear();
    }
}

impl Drop for MediaPipeline {
    fn drop(&mut self) {
        self.shared.shutdown.store(true, Ordering::Release);
        self.shared.video_wake.notify_one();
        self.shared.audio_wake.notify_one();
        for worker in self.workers.drain(..) {
            let _ = worker.join();
        }
    }
}

fn initialize_ffmpeg() -> Result<()> {
    let result = FFMPEG_INIT.get_or_init(|| ffmpeg::init().map_err(|error| error.to_string()));
    result
        .as_ref()
        .map_err(|message| MediaError::Initialization(message.clone()))
        .copied()
}

fn open_decoder() -> Result<ffmpeg::decoder::Video> {
    let codec = ffmpeg::codec::decoder::find(ffmpeg::codec::Id::H264)
        .ok_or(MediaError::DecoderUnavailable)?;
    ffmpeg::codec::Context::new_with_codec(codec)
        .decoder()
        .open_as(codec)
        .and_then(ffmpeg::decoder::Opened::video)
        .map_err(|error| MediaError::Decoder(error.to_string()))
}

fn decoder_worker(shared: Arc<Shared>, events: SyncSender<MediaEvent>) {
    let pool = Arc::new(BufferPool::default());
    let mut decoder = match open_decoder() {
        Ok(decoder) => decoder,
        Err(error) => {
            let _ = events.try_send(MediaEvent::DecoderError(error.to_string()));
            return;
        }
    };
    let mut decoder_epoch = 0_u64;
    let mut admission_policy = MediaAdmissionPolicy::new();
    let mut pending_meta = VecDeque::new();
    let mut last_keyframe_request = None;

    while !shared.shutdown.load(Ordering::Acquire) {
        let encoded = {
            let mut ingress = match shared.ingress.lock() {
                Ok(ingress) => ingress,
                Err(_) => return,
            };
            while ingress.video.is_none() && !shared.shutdown.load(Ordering::Acquire) {
                let waited = shared
                    .video_wake
                    .wait_timeout(ingress, Duration::from_millis(250));
                let Ok((next, _)) = waited else { return };
                ingress = next;
            }
            ingress.video.take()
        };
        let Some(encoded) = encoded else { continue };
        if encoded.epoch != shared.epoch.load(Ordering::Acquire) {
            continue;
        }
        if encoded.epoch != decoder_epoch {
            decoder_epoch = encoded.epoch;
            admission_policy.reset();
            pending_meta.clear();
            decoder.flush();
        }

        let frame = match Frame::parse(&encoded.bytes) {
            Ok(frame) => frame,
            Err(error) => {
                record_decoder_error(&shared, &events, error.to_string());
                continue;
            }
        };
        if frame.kind != FrameKind::Video {
            continue;
        }
        shared
            .counters
            .video_packets
            .fetch_add(1, Ordering::Relaxed);

        let admission =
            match admission_policy.admit(frame.encoder_generation, frame.sequence, frame.idr) {
                Ok(admission) => admission,
                Err(error) => {
                    record_decoder_error(&shared, &events, error.to_string());
                    continue;
                }
            };
        if admission.generation_changed {
            shared
                .counters
                .generation_resets
                .fetch_add(1, Ordering::Relaxed);
        }
        if admission.sequence_gap {
            shared.counters.gaps.fetch_add(1, Ordering::Relaxed);
        }
        match admission.action {
            MediaAction::DropRequestKeyframe => {
                shared
                    .counters
                    .waiting_for_idr
                    .fetch_add(1, Ordering::Relaxed);
                request_keyframe(&shared, &events, &mut last_keyframe_request);
                continue;
            }
            MediaAction::ResetAndDecode => {
                decoder.flush();
                pending_meta.clear();
            }
            MediaAction::Decode => {}
        }

        let metadata = PacketMeta {
            sequence: frame.sequence,
            source_sequence: frame.source_sequence,
            generation: frame.encoder_generation,
            interaction_id: frame.interaction_id,
            width: frame.width,
            height: frame.height,
            source_client_ns: translate_server_time(&shared, frame.source_receive_ns),
            ingress_receive_ns: encoded.received_ns,
        };
        shared.counters.latest_backend_capture_to_encode_us.store(
            frame
                .encode_complete_ns
                .saturating_sub(frame.source_receive_ns)
                / 1_000,
            Ordering::Relaxed,
        );
        shared.counters.latest_backend_encode_to_write_us.store(
            frame
                .socket_write_ns
                .saturating_sub(frame.encode_complete_ns)
                / 1_000,
            Ordering::Relaxed,
        );
        if let Some(socket_write_client_ns) = translate_server_time(&shared, frame.socket_write_ns)
        {
            shared.counters.latest_network_us.store(
                encoded.received_ns.saturating_sub(socket_write_client_ns) / 1_000,
                Ordering::Relaxed,
            );
        }
        let started = Instant::now();
        let packet = ffmpeg::Packet::borrow(frame.payload);
        match decoder.send_packet(&packet) {
            Ok(()) => pending_meta.push_back(metadata),
            Err(error) => {
                record_decoder_error(&shared, &events, error.to_string());
                admission_policy.reset();
                decoder.flush();
                pending_meta.clear();
                request_keyframe(&shared, &events, &mut last_keyframe_request);
                continue;
            }
        }

        loop {
            let mut video = ffmpeg::util::frame::video::Video::empty();
            match decoder.receive_frame(&mut video) {
                Ok(()) => {
                    let output_meta = pending_meta.pop_front().unwrap_or(metadata);
                    match copy_video_frame(&video, output_meta, started.elapsed(), &pool) {
                        Ok(decoded) => publish(&shared, decoded),
                        Err(error) => {
                            record_decoder_error(&shared, &events, error.to_string());
                            admission_policy.reset();
                            decoder.flush();
                            pending_meta.clear();
                            request_keyframe(&shared, &events, &mut last_keyframe_request);
                            break;
                        }
                    }
                }
                Err(ffmpeg::Error::Other { errno })
                    if std::io::Error::from_raw_os_error(errno).kind()
                        == std::io::ErrorKind::WouldBlock =>
                {
                    break;
                }
                Err(ffmpeg::Error::Eof) => break,
                Err(error) => {
                    record_decoder_error(&shared, &events, error.to_string());
                    admission_policy.reset();
                    decoder.flush();
                    pending_meta.clear();
                    request_keyframe(&shared, &events, &mut last_keyframe_request);
                    break;
                }
            }
        }
    }
}

fn audio_worker(shared: Arc<Shared>, events: SyncSender<MediaEvent>) {
    let _stream = match start_audio_output(Arc::clone(&shared), events.clone()) {
        Ok(stream) => Some(stream),
        Err(error) => {
            let _ = events.try_send(MediaEvent::AudioUnavailable(error));
            None
        }
    };
    while !shared.shutdown.load(Ordering::Acquire) {
        let encoded = {
            let mut ingress = match shared.ingress.lock() {
                Ok(ingress) => ingress,
                Err(_) => return,
            };
            while ingress.audio.is_empty() && !shared.shutdown.load(Ordering::Acquire) {
                let waited = shared
                    .audio_wake
                    .wait_timeout(ingress, Duration::from_millis(250));
                let Ok((next, _)) = waited else { return };
                ingress = next;
            }
            ingress.audio.pop_front()
        };
        let Some(encoded) = encoded else { continue };
        if encoded.epoch != shared.epoch.load(Ordering::Acquire) {
            continue;
        }
        let frame = match Frame::parse(&encoded.bytes) {
            Ok(frame) if frame.kind == FrameKind::Audio => frame,
            Ok(_) => continue,
            Err(error) => {
                let _ = events.try_send(MediaEvent::AudioError(error.to_string()));
                continue;
            }
        };
        shared
            .counters
            .audio_packets
            .fetch_add(1, Ordering::Relaxed);
        if _stream.is_some()
            && let Err(error) = shared.audio.push(&frame, &shared.counters)
        {
            let _ = events.try_send(MediaEvent::AudioError(error));
        }
    }
}

fn start_audio_output(
    shared: Arc<Shared>,
    events: SyncSender<MediaEvent>,
) -> std::result::Result<cpal::Stream, String> {
    let host = cpal::default_host();
    let device = host
        .default_output_device()
        .ok_or_else(|| "no default audio output device".to_owned())?;
    let supported = device
        .default_output_config()
        .map_err(|error| error.to_string())?;
    let sample_format = supported.sample_format();
    let config: cpal::StreamConfig = supported.into();
    let stream = match sample_format {
        cpal::SampleFormat::I8 => build_audio_stream::<i8>(&device, &config, &shared, &events)?,
        cpal::SampleFormat::I16 => build_audio_stream::<i16>(&device, &config, &shared, &events)?,
        cpal::SampleFormat::I24 => {
            build_audio_stream::<cpal::I24>(&device, &config, &shared, &events)?
        }
        cpal::SampleFormat::I32 => build_audio_stream::<i32>(&device, &config, &shared, &events)?,
        cpal::SampleFormat::I64 => build_audio_stream::<i64>(&device, &config, &shared, &events)?,
        cpal::SampleFormat::U8 => build_audio_stream::<u8>(&device, &config, &shared, &events)?,
        cpal::SampleFormat::U16 => build_audio_stream::<u16>(&device, &config, &shared, &events)?,
        cpal::SampleFormat::U24 => {
            build_audio_stream::<cpal::U24>(&device, &config, &shared, &events)?
        }
        cpal::SampleFormat::U32 => build_audio_stream::<u32>(&device, &config, &shared, &events)?,
        cpal::SampleFormat::U64 => build_audio_stream::<u64>(&device, &config, &shared, &events)?,
        cpal::SampleFormat::F32 => build_audio_stream::<f32>(&device, &config, &shared, &events)?,
        cpal::SampleFormat::F64 => build_audio_stream::<f64>(&device, &config, &shared, &events)?,
        format => return Err(format!("unsupported output sample format {format}")),
    };
    stream.play().map_err(|error| error.to_string())?;
    let _ = events.try_send(MediaEvent::AudioReady {
        sample_rate: config.sample_rate,
        channels: config.channels,
    });
    Ok(stream)
}

fn build_audio_stream<T>(
    device: &cpal::Device,
    config: &cpal::StreamConfig,
    shared: &Arc<Shared>,
    events: &SyncSender<MediaEvent>,
) -> std::result::Result<cpal::Stream, String>
where
    T: SizedSample + cpal::FromSample<f32>,
{
    let channels = usize::from(config.channels);
    let output_rate = config.sample_rate;
    let mut render_state = AudioRenderState::default();
    let render_shared = Arc::clone(shared);
    let error_shared = Arc::clone(shared);
    let error_events = events.clone();
    let stream = device
        .build_output_stream(
            *config,
            move |output: &mut [T], _| {
                render_audio(
                    output,
                    channels,
                    output_rate,
                    &render_shared,
                    &mut render_state,
                );
            },
            move |error| {
                error_shared
                    .counters
                    .audio_device_errors
                    .fetch_add(1, Ordering::Relaxed);
                let _ = error_events.try_send(MediaEvent::AudioError(error.to_string()));
            },
            None,
        )
        .map_err(|error| error.to_string())?;
    Ok(stream)
}

fn render_audio<T>(
    output: &mut [T],
    output_channels: usize,
    output_rate: u32,
    shared: &Shared,
    render: &mut AudioRenderState,
) where
    T: SizedSample + cpal::FromSample<f32>,
{
    let write_silence = |output: &mut [T]| {
        for sample in output {
            *sample = T::from_sample(0.0);
        }
    };
    if output_channels == 0 || output_rate == 0 {
        write_silence(output);
        return;
    }
    let input_rate = shared.audio.input_rate.load(Ordering::Acquire);
    if input_rate == 0 {
        write_silence(output);
        return;
    }
    let Ok(mut queue) = shared.audio.state.try_lock() else {
        write_silence(output);
        return;
    };
    if !shared.audio.primed.load(Ordering::Acquire) {
        let prime_samples = usize::try_from(input_rate)
            .unwrap_or(usize::MAX)
            .saturating_mul(AUDIO_PRIME_MS)
            / 1_000;
        if queue.samples.len() < prime_samples.max(2) {
            write_silence(output);
            return;
        }
        shared.audio.primed.store(true, Ordering::Release);
        render.initialized = false;
    }
    if render.input_rate != input_rate {
        render.input_rate = input_rate;
        render.phase = 0.0;
        render.initialized = false;
    }
    if !render.initialized {
        let Some(previous) = queue.samples.pop_front() else {
            write_silence(output);
            return;
        };
        render.previous = previous;
        render.next = queue.samples.pop_front().unwrap_or(previous);
        render.initialized = true;
    }

    let step = f64::from(input_rate) / f64::from(output_rate);
    let mut underflow = false;
    for output_frame in output.chunks_mut(output_channels) {
        while render.phase >= 1.0 {
            render.previous = render.next;
            match queue.samples.pop_front() {
                Some(next) => render.next = next,
                None => {
                    render.next = 0.0;
                    underflow = true;
                }
            }
            render.phase -= 1.0;
        }
        let value =
            render.previous + (render.next - render.previous) * render.phase.clamp(0.0, 1.0) as f32;
        let value = T::from_sample(value);
        for sample in output_frame {
            *sample = value;
        }
        render.phase += step;
    }
    if underflow {
        shared.audio.primed.store(false, Ordering::Release);
        render.initialized = false;
        shared
            .counters
            .audio_underruns
            .fetch_add(1, Ordering::Relaxed);
    }
}

fn request_keyframe(
    shared: &Shared,
    events: &SyncSender<MediaEvent>,
    last_request: &mut Option<Instant>,
) {
    let now = Instant::now();
    if last_request.is_some_and(|last| now.duration_since(last) < Duration::from_millis(500)) {
        return;
    }
    *last_request = Some(now);
    shared
        .counters
        .keyframe_requests
        .fetch_add(1, Ordering::Relaxed);
    let _ = events.try_send(MediaEvent::RequestKeyframe);
}

fn record_decoder_error(shared: &Shared, events: &SyncSender<MediaEvent>, message: String) {
    shared
        .counters
        .decode_errors
        .fetch_add(1, Ordering::Relaxed);
    let _ = events.try_send(MediaEvent::DecoderError(message));
}

fn publish(shared: &Shared, frame: DecodedFrame) {
    shared
        .counters
        .decoded_frames
        .fetch_add(1, Ordering::Relaxed);
    shared.counters.latest_decode_us.store(
        u64::try_from(frame.decode_time.as_micros()).unwrap_or(u64::MAX),
        Ordering::Relaxed,
    );
    if let Ok(mut output) = shared.output.lock()
        && output.replace(frame).is_some()
    {
        shared
            .counters
            .output_replaced
            .fetch_add(1, Ordering::Relaxed);
    }
}

fn copy_video_frame(
    frame: &ffmpeg::util::frame::video::Video,
    metadata: PacketMeta,
    decode_time: Duration,
    pool: &Arc<BufferPool>,
) -> Result<DecodedFrame> {
    let format = frame.format();
    if !matches!(
        format,
        ffmpeg::format::Pixel::YUV420P | ffmpeg::format::Pixel::YUVJ420P
    ) {
        return Err(MediaError::PixelFormat(format!("{format:?}")));
    }
    let width = frame.width();
    let height = frame.height();
    if width == 0 || height == 0 {
        return Err(MediaError::Decoder(
            "decoder returned an empty image".to_owned(),
        ));
    }
    if (metadata.width != 0 && u32::from(metadata.width) != width)
        || (metadata.height != 0 && u32::from(metadata.height) != height)
    {
        return Err(MediaError::Decoder(format!(
            "wire size {}x{} disagrees with decoded size {width}x{height}",
            metadata.width, metadata.height
        )));
    }

    let chroma_width = width.div_ceil(2);
    let chroma_height = height.div_ceil(2);
    let mut buffers = pool.take();
    copy_plane(frame, 0, width, height, &mut buffers.y)?;
    copy_plane(frame, 1, chroma_width, chroma_height, &mut buffers.u)?;
    copy_plane(frame, 2, chroma_width, chroma_height, &mut buffers.v)?;
    let color_range = if format == ffmpeg::format::Pixel::YUVJ420P
        || frame.color_range() == ffmpeg::color::Range::JPEG
    {
        ColorRange::Full
    } else {
        ColorRange::Limited
    };
    let color_matrix = match frame.color_space() {
        ffmpeg::color::Space::BT709 => ColorMatrix::Bt709,
        _ if width >= 1280 || height > 576 => ColorMatrix::Bt709,
        _ => ColorMatrix::Bt601,
    };
    Ok(DecodedFrame {
        buffers: Some(buffers),
        pool: Arc::downgrade(pool),
        width,
        height,
        sequence: metadata.sequence,
        source_sequence: metadata.source_sequence,
        generation: metadata.generation,
        interaction_id: metadata.interaction_id,
        color_range,
        color_matrix,
        decode_time,
        source_client_ns: metadata.source_client_ns,
        ingress_receive_ns: metadata.ingress_receive_ns,
    })
}

fn translate_server_time(shared: &Shared, server_ns: u64) -> Option<u64> {
    if !shared.clock_synchronized.load(Ordering::Acquire) || server_ns == 0 {
        return None;
    }
    let offset = shared.server_minus_client_ns.load(Ordering::Acquire);
    if offset >= 0 {
        server_ns.checked_sub(offset as u64)
    } else {
        server_ns.checked_add(offset.unsigned_abs())
    }
}

fn copy_plane(
    frame: &ffmpeg::util::frame::video::Video,
    plane: usize,
    width: u32,
    height: u32,
    destination: &mut Vec<u8>,
) -> Result<()> {
    let width = usize::try_from(width).map_err(|error| MediaError::Decoder(error.to_string()))?;
    let height = usize::try_from(height).map_err(|error| MediaError::Decoder(error.to_string()))?;
    let length = width
        .checked_mul(height)
        .ok_or_else(|| MediaError::Decoder("decoded plane size overflow".to_owned()))?;
    destination.resize(length, 0);
    let source = frame.data(plane);
    let stride = frame.stride(plane);
    if stride < width || source.len() < stride.saturating_mul(height) {
        return Err(MediaError::Decoder(
            "decoder returned a truncated plane".to_owned(),
        ));
    }
    for row in 0..height {
        let source_start = row * stride;
        let destination_start = row * width;
        destination[destination_start..destination_start + width]
            .copy_from_slice(&source[source_start..source_start + width]);
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use std::sync::atomic::Ordering;

    use surf_core::{Frame, FrameKind};

    use super::{
        AudioPlayback, AudioRenderState, BufferPool, Counters, FrameBuffers, Shared, render_audio,
    };

    fn audio_frame(payload: &[u8], sequence: u32) -> Frame<'_> {
        Frame {
            kind: FrameKind::Audio,
            idr: false,
            sequence,
            source_sequence: 0,
            width: 16_000,
            height: 1,
            interaction_id: 0,
            source_receive_ns: 0,
            encode_complete_ns: 0,
            socket_write_ns: 0,
            encoder_generation: 0,
            input_receive_ns: 0,
            cdp_accepted_ns: 0,
            profile: 0,
            payload,
        }
    }

    #[test]
    fn decoder_buffers_are_bounded_and_reused() {
        let pool = BufferPool::default();
        for size in 1..=8 {
            pool.give(FrameBuffers {
                y: vec![0; size],
                u: Vec::new(),
                v: Vec::new(),
            });
        }
        let frames = pool.frames.lock().expect("pool lock");
        assert_eq!(frames.len(), super::BUFFER_POOL_CAPACITY);
    }

    #[test]
    fn pcm_queue_keeps_only_its_bounded_newest_window() {
        let audio = AudioPlayback::new();
        let counters = Counters::default();
        let payload = vec![0_u8; 4_000 * 2];
        audio
            .push(&audio_frame(&payload, 1), &counters)
            .expect("valid PCM");
        let state = audio.state.lock().expect("audio lock");
        assert_eq!(state.samples.len(), 1_920);
        assert_eq!(
            counters.audio_queue_dropped_samples.load(Ordering::Relaxed),
            2_080
        );

        drop(state);
        audio.clear();
        let initial = vec![0_u8; 500 * 2];
        let oversized = vec![0_u8; 2_500 * 2];
        audio
            .push(&audio_frame(&initial, 2), &counters)
            .expect("valid initial PCM");
        audio
            .push(&audio_frame(&oversized, 3), &counters)
            .expect("valid oversized PCM");
        assert_eq!(audio.state.lock().expect("audio lock").samples.len(), 1_920);
        assert_eq!(
            counters.audio_queue_dropped_samples.load(Ordering::Relaxed),
            3_160
        );
    }

    #[test]
    fn audio_callback_primes_and_resamples_without_channel_skew() {
        let shared = Shared::new();
        let sample = 16_384_i16.to_le_bytes();
        let mut payload = Vec::with_capacity(1_200 * 2);
        for _ in 0..1_200 {
            payload.extend_from_slice(&sample);
        }
        shared
            .audio
            .push(&audio_frame(&payload, 1), &shared.counters)
            .expect("valid PCM");
        let mut output = [0.0_f32; 960];
        let mut render = AudioRenderState::default();
        render_audio(&mut output, 2, 48_000, &shared, &mut render);
        assert!(output.iter().all(|sample| *sample > 0.49 && *sample < 0.51));
        assert!(output.chunks_exact(2).all(|frame| frame[0] == frame[1]));
        assert_eq!(shared.counters.audio_underruns.load(Ordering::Relaxed), 0);
    }
}
