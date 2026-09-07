use imgui_glow_renderer::glow::{self, HasContext};
use std::rc::Rc;

type PendingIcon = (
    u64,
    std::sync::mpsc::Receiver<(String, Option<image::RgbaImage>)>,
);
pub struct Assets {
    pub ui_icons: crate::icons::IconAtlas,
    gl: Rc<glow::Context>,
    mark: glow::Texture,
    icons: std::collections::BTreeMap<String, glow::Texture>,
    pending: Vec<PendingIcon>,
    generation: u64,
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
            ui_icons: crate::icons::IconAtlas::new(gl)?,
            gl: gl.clone(),
            mark,
            icons: std::collections::BTreeMap::new(),
            pending: Vec::new(),
            generation: 0,
        })
    }
    pub fn mark(&self) -> imgui::TextureId {
        imgui::TextureId::new(self.mark.0.get() as usize)
    }

    pub fn icon(&self, path: &str) -> Option<imgui::TextureId> {
        self.icons
            .get(path)
            .map(|t| imgui::TextureId::new(t.0.get() as usize))
    }
    pub fn clear_icons(&mut self) {
        self.generation = self.generation.wrapping_add(1);
        // SAFETY: called on the owning GL thread between frames.
        for (_, texture) in std::mem::take(&mut self.icons) {
            unsafe {
                self.gl.delete_texture(texture);
            }
        }
    }
    pub fn decode_icon(&mut self, path: String, bytes: Vec<u8>) {
        if self.pending.len() >= 8 || self.icons.contains_key(&path) {
            return;
        }
        let (tx, rx) = std::sync::mpsc::sync_channel(1);
        if std::thread::Builder::new()
            .name("surf-icon".into())
            .spawn(move || {
                let image = (|| {
                    let mut reader = image::ImageReader::new(std::io::Cursor::new(bytes))
                        .with_guessed_format()
                        .ok()?;
                    let mut limits = image::Limits::default();
                    limits.max_image_width = Some(512);
                    limits.max_image_height = Some(512);
                    limits.max_alloc = Some(4 * 1024 * 1024);
                    reader.limits(limits);
                    Some(reader.decode().ok()?.into_rgba8())
                })();
                let _ = tx.send((path, image));
            })
            .is_ok()
        {
            self.pending.push((self.generation, rx));
        }
    }
    pub fn upload_icons(&mut self) {
        let mut completed = Vec::new();
        self.pending.retain(|(generation, rx)| match rx.try_recv() {
            Ok((path, image)) => {
                if *generation == self.generation {
                    completed.push((path, image));
                }
                false
            }
            Err(std::sync::mpsc::TryRecvError::Empty) => true,
            Err(_) => false,
        });
        for (path, image) in completed {
            let Some(image) = image else {
                continue;
            };
            // SAFETY: bounded decoded RGBA data; upload and eviction on the GL thread.
            unsafe {
                if self.icons.len() >= 64
                    && let Some((_, texture)) = self.icons.pop_first()
                {
                    self.gl.delete_texture(texture);
                }
                let Ok(texture) = self.gl.create_texture() else {
                    continue;
                };
                self.gl.bind_texture(glow::TEXTURE_2D, Some(texture));
                self.gl.tex_parameter_i32(
                    glow::TEXTURE_2D,
                    glow::TEXTURE_MIN_FILTER,
                    glow::LINEAR as i32,
                );
                self.gl.tex_parameter_i32(
                    glow::TEXTURE_2D,
                    glow::TEXTURE_MAG_FILTER,
                    glow::LINEAR as i32,
                );
                self.gl.tex_image_2d(
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
                self.gl.bind_texture(glow::TEXTURE_2D, None);
                self.icons.insert(path, texture);
            }
        }
    }
}

impl Drop for Assets {
    fn drop(&mut self) {
        self.clear_icons();
        // SAFETY: the host owns the current context until after DesktopApp drops.
        unsafe {
            self.gl.delete_texture(self.mark);
        }
    }
}
