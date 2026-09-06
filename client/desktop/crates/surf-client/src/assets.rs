use imgui_glow_renderer::glow::{self, HasContext};
use std::rc::Rc;

pub struct Assets {
    gl: Rc<glow::Context>,
    mark: glow::Texture,
}

impl Assets {
    pub fn new(gl: &Rc<glow::Context>) -> Result<Self, String> {
        let image =
            image::load_from_memory(include_bytes!("../../../../ios/Resources/brand-mark.png"))
                .map_err(|e| e.to_string())?
                .into_rgba8();
        // SAFETY: assets are created, drawn and dropped on the GL host thread.
        let mark = unsafe {
            let texture = gl.create_texture()?;
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
            gl.tex_image_2d(
                glow::TEXTURE_2D,
                0,
                glow::SRGB8_ALPHA8 as i32,
                image.width() as i32,
                image.height() as i32,
                0,
                glow::RGBA,
                glow::UNSIGNED_BYTE,
                Some(image.as_raw()),
            );
            gl.bind_texture(glow::TEXTURE_2D, None);
            texture
        };
        Ok(Self {
            gl: gl.clone(),
            mark,
        })
    }
    pub fn mark(&self) -> imgui::TextureId {
        imgui::TextureId::new(self.mark.0.get() as usize)
    }
}

impl Drop for Assets {
    fn drop(&mut self) {
        // SAFETY: the host owns the current context until after DesktopApp drops.
        unsafe {
            self.gl.delete_texture(self.mark);
        }
    }
}
