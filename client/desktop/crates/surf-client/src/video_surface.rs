use std::sync::atomic::{AtomicBool, AtomicU32, AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Instant;

use eframe::egui;
use egui_glow::glow::{self, HasContext as _};
use surf_core::monotonic_ns;
use surf_media::{ColorMatrix, ColorRange, DecodedFrame};

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct SurfaceDiagnostics {
    pub presented: u64,
    pub replaced: u64,
    pub latest_upload_us: u64,
    pub latest_frame_age_us: u64,
    pub latest_presentation_gap_us: u64,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct PresentedFrame {
    pub count: u64,
    pub generation: u32,
    pub source_sequence: u32,
}

pub struct VideoSurface {
    pending: Mutex<Option<DecodedFrame>>,
    resources: Mutex<Option<GlResources>>,
    error: Mutex<Option<String>>,
    visible: AtomicBool,
    presented: AtomicU64,
    replaced: AtomicU64,
    latest_upload_us: AtomicU64,
    latest_frame_age_us: AtomicU64,
    last_presentation_ns: AtomicU64,
    latest_presentation_gap_us: AtomicU64,
    surface_generation: AtomicU32,
    source_sequence: AtomicU32,
}

impl VideoSurface {
    pub fn new() -> Arc<Self> {
        Arc::new(Self {
            pending: Mutex::new(None),
            resources: Mutex::new(None),
            error: Mutex::new(None),
            visible: AtomicBool::new(false),
            presented: AtomicU64::new(0),
            replaced: AtomicU64::new(0),
            latest_upload_us: AtomicU64::new(0),
            latest_frame_age_us: AtomicU64::new(0),
            last_presentation_ns: AtomicU64::new(0),
            latest_presentation_gap_us: AtomicU64::new(0),
            surface_generation: AtomicU32::new(0),
            source_sequence: AtomicU32::new(0),
        })
    }

    pub fn submit(&self, frame: DecodedFrame) {
        if let Ok(mut pending) = self.pending.lock()
            && pending.replace(frame).is_some()
        {
            self.replaced.fetch_add(1, Ordering::Relaxed);
        }
        self.visible.store(true, Ordering::Release);
    }

    pub fn clear(&self) {
        self.visible.store(false, Ordering::Release);
        self.last_presentation_ns.store(0, Ordering::Release);
        self.latest_presentation_gap_us.store(0, Ordering::Release);
        self.latest_frame_age_us.store(0, Ordering::Release);
        self.surface_generation.store(0, Ordering::Release);
        self.source_sequence.store(0, Ordering::Release);
        if let Ok(mut pending) = self.pending.lock() {
            *pending = None;
        }
    }

    pub fn callback(self: &Arc<Self>, rect: egui::Rect) -> egui::PaintCallback {
        let surface = Arc::clone(self);
        egui::PaintCallback {
            rect,
            callback: Arc::new(egui_glow::CallbackFn::new(move |_info, painter| {
                surface.paint(painter.gl());
            })),
        }
    }

    pub fn presented(&self) -> u64 {
        self.presented.load(Ordering::Relaxed)
    }

    pub fn latest_upload_us(&self) -> u64 {
        self.latest_upload_us.load(Ordering::Relaxed)
    }

    pub fn surface_generation(&self) -> u32 {
        self.surface_generation.load(Ordering::Acquire)
    }

    pub fn presented_frame(&self) -> PresentedFrame {
        PresentedFrame {
            count: self.presented.load(Ordering::Acquire),
            generation: self.surface_generation.load(Ordering::Acquire),
            source_sequence: self.source_sequence.load(Ordering::Acquire),
        }
    }

    pub fn diagnostics(&self) -> SurfaceDiagnostics {
        SurfaceDiagnostics {
            presented: self.presented.load(Ordering::Relaxed),
            replaced: self.replaced.load(Ordering::Relaxed),
            latest_upload_us: self.latest_upload_us.load(Ordering::Relaxed),
            latest_frame_age_us: self.latest_frame_age_us.load(Ordering::Relaxed),
            latest_presentation_gap_us: self.latest_presentation_gap_us.swap(0, Ordering::AcqRel),
        }
    }

    pub fn take_error(&self) -> Option<String> {
        self.error.lock().ok()?.take()
    }

    fn paint(&self, gl: &Arc<glow::Context>) {
        if !self.visible.load(Ordering::Acquire) {
            return;
        }
        let mut resources = match self.resources.lock() {
            Ok(resources) => resources,
            Err(_) => return,
        };
        if resources.is_none() {
            match unsafe { GlResources::create(gl) } {
                Ok(created) => *resources = Some(created),
                Err(error) => {
                    self.visible.store(false, Ordering::Release);
                    if let Ok(mut slot) = self.error.lock() {
                        *slot = Some(error);
                    }
                    return;
                }
            }
        }
        let resources = resources.as_mut().expect("resources initialized");
        let mut uploaded = false;
        if let Some(frame) = self.pending.lock().ok().and_then(|mut frame| frame.take()) {
            let started = Instant::now();
            let result = unsafe { resources.upload(gl, &frame) };
            self.latest_upload_us.store(
                u64::try_from(started.elapsed().as_micros()).unwrap_or(u64::MAX),
                Ordering::Relaxed,
            );
            if let Err(error) = result {
                self.visible.store(false, Ordering::Release);
                if let Ok(mut slot) = self.error.lock() {
                    *slot = Some(error);
                }
                return;
            }
            let presented_ns = monotonic_ns();
            let previous_ns = self
                .last_presentation_ns
                .swap(presented_ns, Ordering::AcqRel);
            if previous_ns > 0 {
                self.latest_presentation_gap_us.fetch_max(
                    presented_ns.saturating_sub(previous_ns) / 1_000,
                    Ordering::Relaxed,
                );
            }
            let origin_ns = frame.source_client_ns.unwrap_or(frame.ingress_receive_ns);
            self.latest_frame_age_us.store(
                presented_ns.saturating_sub(origin_ns) / 1_000,
                Ordering::Relaxed,
            );
            self.surface_generation
                .store(frame.generation, Ordering::Release);
            self.source_sequence
                .store(frame.source_sequence, Ordering::Release);
            uploaded = true;
        }
        unsafe { resources.draw(gl) };
        if uploaded {
            self.presented.fetch_add(1, Ordering::Release);
        }
    }
}

struct GlResources {
    program: glow::Program,
    vertex_array: glow::VertexArray,
    textures: [glow::Texture; 3],
    y_texture: Option<glow::UniformLocation>,
    u_texture: Option<glow::UniformLocation>,
    v_texture: Option<glow::UniformLocation>,
    full_range: Option<glow::UniformLocation>,
    bt709: Option<glow::UniformLocation>,
    dimensions: Option<(u32, u32)>,
    range: ColorRange,
    matrix: ColorMatrix,
}

impl GlResources {
    unsafe fn create(gl: &glow::Context) -> Result<Self, String> {
        // SAFETY: the glow context is current for the egui paint callback and
        // every object created here remains owned by this same renderer.
        unsafe {
            let version = gl.get_parameter_string(glow::VERSION);
            let embedded = version.contains("OpenGL ES") || version.contains("WebGL");
            let vertex_source = if embedded {
                "#version 300 es\n\
             precision highp float;\n\
             out vec2 surf_uv;\n\
             void main() {\n\
               vec2 p = vec2((gl_VertexID == 1) ? 3.0 : -1.0,\n\
                             (gl_VertexID == 2) ? 3.0 : -1.0);\n\
               surf_uv = vec2((p.x + 1.0) * 0.5, (1.0 - p.y) * 0.5);\n\
               gl_Position = vec4(p, 0.0, 1.0);\n\
             }"
            } else {
                "#version 150\n\
             out vec2 surf_uv;\n\
             void main() {\n\
               vec2 p = vec2((gl_VertexID == 1) ? 3.0 : -1.0,\n\
                             (gl_VertexID == 2) ? 3.0 : -1.0);\n\
               surf_uv = vec2((p.x + 1.0) * 0.5, (1.0 - p.y) * 0.5);\n\
               gl_Position = vec4(p, 0.0, 1.0);\n\
             }"
            };
            let fragment_source = if embedded {
                "#version 300 es\n\
             precision highp float;\n\
             in vec2 surf_uv; out vec4 surf_color;\n\
             uniform sampler2D surf_y; uniform sampler2D surf_u; uniform sampler2D surf_v;\n\
             uniform int surf_full; uniform int surf_bt709;\n\
             void main() {\n\
               float y = texture(surf_y, surf_uv).r;\n\
               float u = texture(surf_u, surf_uv).r - 0.5;\n\
               float v = texture(surf_v, surf_uv).r - 0.5;\n\
               vec3 rgb;\n\
               if (surf_full != 0) {\n\
                 rgb = surf_bt709 != 0 ? vec3(y + 1.5748*v, y - 0.1873*u - 0.4681*v, y + 1.8556*u)\n\
                                          : vec3(y + 1.4020*v, y - 0.3441*u - 0.7141*v, y + 1.7720*u);\n\
               } else {\n\
                 y = 1.164383 * (y - 0.062745);\n\
                 rgb = surf_bt709 != 0 ? vec3(y + 1.792741*v, y - 0.213249*u - 0.532909*v, y + 2.112402*u)\n\
                                          : vec3(y + 1.596027*v, y - 0.391762*u - 0.812968*v, y + 2.017232*u);\n\
               }\n\
               surf_color = vec4(clamp(rgb, 0.0, 1.0), 1.0);\n\
             }"
            } else {
                "#version 150\n\
             in vec2 surf_uv; out vec4 surf_color;\n\
             uniform sampler2D surf_y; uniform sampler2D surf_u; uniform sampler2D surf_v;\n\
             uniform int surf_full; uniform int surf_bt709;\n\
             void main() {\n\
               float y = texture(surf_y, surf_uv).r;\n\
               float u = texture(surf_u, surf_uv).r - 0.5;\n\
               float v = texture(surf_v, surf_uv).r - 0.5;\n\
               vec3 rgb;\n\
               if (surf_full != 0) {\n\
                 rgb = surf_bt709 != 0 ? vec3(y + 1.5748*v, y - 0.1873*u - 0.4681*v, y + 1.8556*u)\n\
                                          : vec3(y + 1.4020*v, y - 0.3441*u - 0.7141*v, y + 1.7720*u);\n\
               } else {\n\
                 y = 1.164383 * (y - 0.062745);\n\
                 rgb = surf_bt709 != 0 ? vec3(y + 1.792741*v, y - 0.213249*u - 0.532909*v, y + 2.112402*u)\n\
                                          : vec3(y + 1.596027*v, y - 0.391762*u - 0.812968*v, y + 2.017232*u);\n\
               }\n\
               surf_color = vec4(clamp(rgb, 0.0, 1.0), 1.0);\n\
             }"
            };
            let program = compile_program(gl, vertex_source, fragment_source)?;
            let vertex_array = gl.create_vertex_array()?;
            let textures = [
                gl.create_texture()?,
                gl.create_texture()?,
                gl.create_texture()?,
            ];
            for texture in textures {
                gl.bind_texture(glow::TEXTURE_2D, Some(texture));
                gl.tex_parameter_i32(
                    glow::TEXTURE_2D,
                    glow::TEXTURE_MIN_FILTER,
                    glow::LINEAR as i32,
                );
                gl.tex_parameter_i32(
                    glow::TEXTURE_2D,
                    glow::TEXTURE_MAG_FILTER,
                    glow::LINEAR as i32,
                );
                gl.tex_parameter_i32(
                    glow::TEXTURE_2D,
                    glow::TEXTURE_WRAP_S,
                    glow::CLAMP_TO_EDGE as i32,
                );
                gl.tex_parameter_i32(
                    glow::TEXTURE_2D,
                    glow::TEXTURE_WRAP_T,
                    glow::CLAMP_TO_EDGE as i32,
                );
            }
            gl.bind_texture(glow::TEXTURE_2D, None);
            Ok(Self {
                y_texture: gl.get_uniform_location(program, "surf_y"),
                u_texture: gl.get_uniform_location(program, "surf_u"),
                v_texture: gl.get_uniform_location(program, "surf_v"),
                full_range: gl.get_uniform_location(program, "surf_full"),
                bt709: gl.get_uniform_location(program, "surf_bt709"),
                program,
                vertex_array,
                textures,
                dimensions: None,
                range: ColorRange::Limited,
                matrix: ColorMatrix::Bt709,
            })
        }
    }

    unsafe fn upload(&mut self, gl: &glow::Context, frame: &DecodedFrame) -> Result<(), String> {
        // SAFETY: textures belong to this current context, plane extents were
        // validated by the decoder, and uploads use their exact byte slices.
        unsafe {
            let dimensions_changed = self.dimensions != Some((frame.width, frame.height));
            let chroma_width = frame.width.div_ceil(2);
            let chroma_height = frame.height.div_ceil(2);
            gl.pixel_store_i32(glow::UNPACK_ALIGNMENT, 1);
            upload_plane(
                gl,
                self.textures[0],
                frame.width,
                frame.height,
                frame.y(),
                dimensions_changed,
            )?;
            upload_plane(
                gl,
                self.textures[1],
                chroma_width,
                chroma_height,
                frame.u(),
                dimensions_changed,
            )?;
            upload_plane(
                gl,
                self.textures[2],
                chroma_width,
                chroma_height,
                frame.v(),
                dimensions_changed,
            )?;
            self.dimensions = Some((frame.width, frame.height));
            self.range = frame.color_range;
            self.matrix = frame.color_matrix;
            Ok(())
        }
    }

    unsafe fn draw(&self, gl: &glow::Context) {
        // SAFETY: all handles were created by this context and egui restores
        // renderer state after the paint callback returns.
        unsafe {
            if self.dimensions.is_none() {
                return;
            }
            gl.disable(glow::BLEND);
            gl.use_program(Some(self.program));
            gl.bind_vertex_array(Some(self.vertex_array));
            for (unit, texture) in self.textures.iter().enumerate() {
                gl.active_texture(glow::TEXTURE0 + u32::try_from(unit).unwrap_or(0));
                gl.bind_texture(glow::TEXTURE_2D, Some(*texture));
            }
            gl.uniform_1_i32(self.y_texture.as_ref(), 0);
            gl.uniform_1_i32(self.u_texture.as_ref(), 1);
            gl.uniform_1_i32(self.v_texture.as_ref(), 2);
            gl.uniform_1_i32(
                self.full_range.as_ref(),
                i32::from(self.range == ColorRange::Full),
            );
            gl.uniform_1_i32(
                self.bt709.as_ref(),
                i32::from(self.matrix == ColorMatrix::Bt709),
            );
            gl.draw_arrays(glow::TRIANGLES, 0, 3);
        }
    }
}

unsafe fn upload_plane(
    gl: &glow::Context,
    texture: glow::Texture,
    width: u32,
    height: u32,
    data: &[u8],
    allocate: bool,
) -> Result<(), String> {
    let width = i32::try_from(width).map_err(|error| error.to_string())?;
    let height = i32::try_from(height).map_err(|error| error.to_string())?;
    // SAFETY: `texture` belongs to the current context and `data` contains the
    // compact width-by-height plane checked by `surf-media`.
    unsafe {
        gl.bind_texture(glow::TEXTURE_2D, Some(texture));
        if allocate {
            gl.tex_image_2d(
                glow::TEXTURE_2D,
                0,
                glow::R8 as i32,
                width,
                height,
                0,
                glow::RED,
                glow::UNSIGNED_BYTE,
                glow::PixelUnpackData::Slice(Some(data)),
            );
        } else {
            gl.tex_sub_image_2d(
                glow::TEXTURE_2D,
                0,
                0,
                0,
                width,
                height,
                glow::RED,
                glow::UNSIGNED_BYTE,
                glow::PixelUnpackData::Slice(Some(data)),
            );
        }
    }
    Ok(())
}

unsafe fn compile_program(
    gl: &glow::Context,
    vertex_source: &str,
    fragment_source: &str,
) -> Result<glow::Program, String> {
    // SAFETY: shader and program handles are created, used, and destroyed on
    // the supplied current context; no handles escape on an error path.
    unsafe {
        let program = gl.create_program()?;
        let vertex = gl.create_shader(glow::VERTEX_SHADER)?;
        gl.shader_source(vertex, vertex_source);
        gl.compile_shader(vertex);
        if !gl.get_shader_compile_status(vertex) {
            let error = gl.get_shader_info_log(vertex);
            gl.delete_shader(vertex);
            gl.delete_program(program);
            return Err(format!("YUV vertex shader failed: {error}"));
        }
        let fragment = gl.create_shader(glow::FRAGMENT_SHADER)?;
        gl.shader_source(fragment, fragment_source);
        gl.compile_shader(fragment);
        if !gl.get_shader_compile_status(fragment) {
            let error = gl.get_shader_info_log(fragment);
            gl.delete_shader(vertex);
            gl.delete_shader(fragment);
            gl.delete_program(program);
            return Err(format!("YUV fragment shader failed: {error}"));
        }
        gl.attach_shader(program, vertex);
        gl.attach_shader(program, fragment);
        gl.link_program(program);
        gl.detach_shader(program, vertex);
        gl.detach_shader(program, fragment);
        gl.delete_shader(vertex);
        gl.delete_shader(fragment);
        if !gl.get_program_link_status(program) {
            let error = gl.get_program_info_log(program);
            gl.delete_program(program);
            return Err(format!("YUV shader link failed: {error}"));
        }
        Ok(program)
    }
}
