use std::time::Instant;

use imgui_glow_renderer::glow::{self, HasContext as _};
use surf_client_app::RenderDiagnostics;
use surf_core::monotonic_ns;
use surf_media::{ColorMatrix, ColorRange, DecodedFrame};

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct PresentedFrame {
    pub generation: u32,
    pub source_sequence: u32,
}

pub struct VideoSurface {
    resources: GlResources,
    pending: Option<DecodedFrame>,
    visible: bool,
    diagnostics: RenderDiagnostics,
    last_presentation_ns: u64,
    surface_generation: u32,
    error: Option<String>,
    submitted: Option<(PresentedFrame, u64)>,
    drawn_viewport: [i32; 4],
}

impl VideoSurface {
    pub fn new(gl: &glow::Context) -> Result<Self, String> {
        Ok(Self {
            // SAFETY: creation happens on the event-loop thread while its GL
            // context is current. All later access stays on that thread.
            resources: unsafe { GlResources::create(gl)? },
            pending: None,
            visible: false,
            diagnostics: RenderDiagnostics::default(),
            last_presentation_ns: 0,
            surface_generation: 0,
            error: None,
            submitted: None,
            drawn_viewport: [0; 4],
        })
    }

    pub fn submit(&mut self, frame: DecodedFrame) {
        if self.pending.replace(frame).is_some() {
            self.diagnostics.replaced = self.diagnostics.replaced.saturating_add(1);
        }
        self.visible = true;
    }

    pub fn clear(&mut self) {
        self.submitted = None;
        self.pending = None;
        self.visible = false;
        self.last_presentation_ns = 0;
        self.surface_generation = 0;
        self.diagnostics.latest_frame_age_us = 0;
        self.diagnostics.latest_presentation_gap_us = 0;
    }

    pub fn render(&mut self, gl: &glow::Context, viewport: [i32; 4]) -> Option<PresentedFrame> {
        let viewport = self
            .pending
            .as_ref()
            .map(|frame| (frame.width, frame.height))
            .or(self.resources.dimensions)
            .map_or(viewport, |source| aspect_fit_viewport(viewport, source));
        self.drawn_viewport = viewport;
        // SAFETY: the event loop owns the current framebuffer and these
        // resources belong to the same glutin context.
        unsafe {
            gl.viewport(
                viewport[0],
                viewport[1],
                viewport[2].max(1),
                viewport[3].max(1),
            );
        }
        if !self.visible {
            return None;
        }
        let mut presented = None;
        if let Some(frame) = self.pending.take() {
            let started = Instant::now();
            // SAFETY: decoded planes are compact, validated, and uploaded only
            // while the host GL context is current.
            if let Err(error) = unsafe { self.resources.upload(gl, &frame) } {
                self.visible = false;
                self.error = Some(error);
                return None;
            }
            self.diagnostics.latest_upload_us =
                u64::try_from(started.elapsed().as_micros()).unwrap_or(u64::MAX);
            let origin = frame.source_client_ns.unwrap_or(frame.ingress_receive_ns);
            self.surface_generation = frame.generation;
            presented = Some(PresentedFrame {
                generation: frame.generation,
                source_sequence: frame.source_sequence,
            });
            self.submitted = presented.map(|frame| (frame, origin));
        }
        // SAFETY: resources belong to the current host GL context.
        unsafe { self.resources.draw(gl) };
        presented
    }

    pub fn drawn_viewport(&self) -> [i32; 4] {
        self.drawn_viewport
    }

    /// A successful swap is evidence of presentation submission, not physical scanout.
    /// Redrawing an unchanged texture must not acknowledge or count the frame again.
    pub fn after_swap(&mut self, success: bool) -> Option<PresentedFrame> {
        let (frame, origin) = self.submitted.take()?;
        if !success {
            return None;
        }
        let now = monotonic_ns();
        if self.last_presentation_ns != 0 {
            self.diagnostics.latest_presentation_gap_us = self
                .diagnostics
                .latest_presentation_gap_us
                .max(now.saturating_sub(self.last_presentation_ns) / 1000);
        }
        self.last_presentation_ns = now;
        self.diagnostics.latest_frame_age_us = now.saturating_sub(origin) / 1000;
        self.diagnostics.presented = self.diagnostics.presented.saturating_add(1);
        Some(frame)
    }

    pub fn surface_generation(&self) -> u32 {
        self.surface_generation
    }

    pub fn diagnostics(&mut self) -> RenderDiagnostics {
        let diagnostics = self.diagnostics;
        self.diagnostics.latest_presentation_gap_us = 0;
        diagnostics
    }

    pub fn take_error(&mut self) -> Option<String> {
        self.error.take()
    }
}

fn aspect_fit_viewport(target: [i32; 4], source: (u32, u32)) -> [i32; 4] {
    let target_width = u64::try_from(target[2].max(1)).unwrap_or(1);
    let target_height = u64::try_from(target[3].max(1)).unwrap_or(1);
    let source_width = u64::from(source.0.max(1));
    let source_height = u64::from(source.1.max(1));
    let (width, height) = if target_width.saturating_mul(source_height)
        > target_height.saturating_mul(source_width)
    {
        (
            target_height.saturating_mul(source_width) / source_height,
            target_height,
        )
    } else {
        (
            target_width,
            target_width.saturating_mul(source_height) / source_width,
        )
    };
    let width = i32::try_from(width.max(1)).unwrap_or(i32::MAX);
    let height = i32::try_from(height.max(1)).unwrap_or(i32::MAX);
    [
        target[0] + (target[2] - width) / 2,
        target[1] + (target[3] - height) / 2,
        width,
        height,
    ]
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
        // SAFETY: the caller guarantees a current GL context.
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
        // SAFETY: textures and decoded frame planes are valid for this context.
        unsafe {
            let allocate = self.dimensions != Some((frame.width, frame.height));
            let chroma_width = frame.width.div_ceil(2);
            let chroma_height = frame.height.div_ceil(2);
            gl.pixel_store_i32(glow::UNPACK_ALIGNMENT, 1);
            upload_plane(
                gl,
                self.textures[0],
                frame.width,
                frame.height,
                frame.y(),
                allocate,
            )?;
            upload_plane(
                gl,
                self.textures[1],
                chroma_width,
                chroma_height,
                frame.u(),
                allocate,
            )?;
            upload_plane(
                gl,
                self.textures[2],
                chroma_width,
                chroma_height,
                frame.v(),
                allocate,
            )?;
            self.dimensions = Some((frame.width, frame.height));
            self.range = frame.color_range;
            self.matrix = frame.color_matrix;
            Ok(())
        }
    }

    unsafe fn draw(&self, gl: &glow::Context) {
        if self.dimensions.is_none() {
            return;
        }
        // SAFETY: every handle belongs to the current context.
        unsafe {
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
    // SAFETY: the compact byte plane exactly matches width and height.
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
                Some(data),
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
                glow::PixelUnpackData::Slice(data),
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
    // SAFETY: all intermediate GL handles stay inside the current context.
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

#[cfg(test)]
mod tests {
    use super::aspect_fit_viewport;

    #[test]
    fn stale_frames_keep_their_shape_during_resize() {
        assert_eq!(
            aspect_fit_viewport([0, 32, 768, 992], (1180, 728)),
            [0, 291, 768, 473]
        );
        assert_eq!(
            aspect_fit_viewport([10, 20, 1200, 700], (768, 1024)),
            [347, 20, 525, 700]
        );
        assert_eq!(
            aspect_fit_viewport([0, 0, 768, 992], (768, 992)),
            [0, 0, 768, 992]
        );
    }
}
