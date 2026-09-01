//! Bounded desktop media pipeline.
//!
//! The network thread only replaces an encoded-frame slot. A dedicated worker
//! owns FFmpeg, and the UI only takes the newest decoded YUV image. No lane can
//! form an unbounded queue behind another lane.

use std::collections::VecDeque;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::mpsc::{self, Receiver, SyncSender};
use std::sync::{Arc, Condvar, Mutex, OnceLock, Weak};
use std::thread;
use std::time::{Duration, Instant};

use bytes::Bytes;
use ffmpeg_the_third as ffmpeg;
use surf_core::{Frame, FrameKind, MediaAction, MediaAdmissionPolicy};
use surf_session::FrameSink;
use thiserror::Error;

const AUDIO_QUEUE_CAPACITY: usize = 8;
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
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct Diagnostics {
    pub ingress_frames: u64,
    pub ingress_replaced: u64,
    pub ingress_contended: u64,
    pub audio_dropped: u64,
    pub video_packets: u64,
    pub decoded_frames: u64,
    pub output_replaced: u64,
    pub decode_errors: u64,
    pub generation_resets: u64,
    pub gaps: u64,
    pub waiting_for_idr: u64,
    pub keyframe_requests: u64,
    pub latest_decode_us: u64,
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
    video_packets: AtomicU64,
    decoded_frames: AtomicU64,
    output_replaced: AtomicU64,
    decode_errors: AtomicU64,
    generation_resets: AtomicU64,
    gaps: AtomicU64,
    waiting_for_idr: AtomicU64,
    keyframe_requests: AtomicU64,
    latest_decode_us: AtomicU64,
}

impl Counters {
    fn snapshot(&self) -> Diagnostics {
        Diagnostics {
            ingress_frames: self.ingress_frames.load(Ordering::Relaxed),
            ingress_replaced: self.ingress_replaced.load(Ordering::Relaxed),
            ingress_contended: self.ingress_contended.load(Ordering::Relaxed),
            audio_dropped: self.audio_dropped.load(Ordering::Relaxed),
            video_packets: self.video_packets.load(Ordering::Relaxed),
            decoded_frames: self.decoded_frames.load(Ordering::Relaxed),
            output_replaced: self.output_replaced.load(Ordering::Relaxed),
            decode_errors: self.decode_errors.load(Ordering::Relaxed),
            generation_resets: self.generation_resets.load(Ordering::Relaxed),
            gaps: self.gaps.load(Ordering::Relaxed),
            waiting_for_idr: self.waiting_for_idr.load(Ordering::Relaxed),
            keyframe_requests: self.keyframe_requests.load(Ordering::Relaxed),
            latest_decode_us: self.latest_decode_us.load(Ordering::Relaxed),
        }
    }
}

struct EncodedFrame {
    epoch: u64,
    bytes: Bytes,
}

#[derive(Default)]
struct IngressState {
    video: Option<EncodedFrame>,
    audio: VecDeque<EncodedFrame>,
}

struct Shared {
    ingress: Mutex<IngressState>,
    wake: Condvar,
    output: Mutex<Option<DecodedFrame>>,
    epoch: AtomicU64,
    shutdown: AtomicBool,
    counters: Counters,
}

impl Shared {
    fn new() -> Self {
        Self {
            ingress: Mutex::new(IngressState::default()),
            wake: Condvar::new(),
            output: Mutex::new(None),
            epoch: AtomicU64::new(1),
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
            bytes: frame,
        };
        match kind {
            3 => {
                if ingress.video.replace(encoded).is_some() {
                    self.counters
                        .ingress_replaced
                        .fetch_add(1, Ordering::Relaxed);
                }
            }
            4 => {
                if ingress.audio.len() == AUDIO_QUEUE_CAPACITY {
                    ingress.audio.pop_front();
                    self.counters.audio_dropped.fetch_add(1, Ordering::Relaxed);
                }
                ingress.audio.push_back(encoded);
            }
            _ => return,
        }
        drop(ingress);
        self.wake.notify_one();
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
        self.wake.notify_one();
    }
}

pub struct MediaPipeline {
    shared: Arc<Shared>,
    events: Receiver<MediaEvent>,
    worker: Option<thread::JoinHandle<()>>,
}

impl MediaPipeline {
    pub fn spawn() -> Result<Self> {
        initialize_ffmpeg()?;
        let shared = Arc::new(Shared::new());
        let worker_shared = Arc::clone(&shared);
        let (event_tx, events) = mpsc::sync_channel(16);
        let worker = thread::Builder::new()
            .name("surf-video-decode".to_owned())
            .spawn(move || decoder_worker(worker_shared, event_tx))?;
        Ok(Self {
            shared,
            events,
            worker: Some(worker),
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
        self.shared.counters.snapshot()
    }

    pub fn clear(&self) {
        self.shared.clear();
    }
}

impl Drop for MediaPipeline {
    fn drop(&mut self) {
        self.shared.shutdown.store(true, Ordering::Release);
        self.shared.wake.notify_one();
        if let Some(worker) = self.worker.take() {
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
            while ingress.video.is_none()
                && ingress.audio.is_empty()
                && !shared.shutdown.load(Ordering::Acquire)
            {
                let waited = shared
                    .wake
                    .wait_timeout(ingress, Duration::from_millis(250));
                let Ok((next, _)) = waited else { return };
                ingress = next;
            }
            ingress.video.take().or_else(|| ingress.audio.pop_front())
        };
        let Some(encoded) = encoded else { continue };
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
        if frame.kind == FrameKind::Audio {
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
        };
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
    })
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
    use super::{BufferPool, FrameBuffers};

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
}
