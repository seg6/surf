//! Upstream SVGs, rasterized once per display scale, never through a text font.
use imgui_glow_renderer::glow::{self, HasContext};
use resvg::{tiny_skia, usvg};
use std::rc::Rc;

pub const SIZE: f32 = 18.0;

#[derive(Clone, Copy)]
pub struct Icon(usize);

macro_rules! icons {
    ($($name:ident = $index:literal => $file:literal),+ $(,)?) => {
        $(pub const $name: Icon = Icon($index);)+
        const SOURCES: &[&[u8]] = &[
            $(include_bytes!(concat!("../../../assets/icons/", $file, ".svg"))),+
        ];
    };
}
icons! {
    BACK = 0 => "chevron-left", FORWARD = 1 => "chevron-right",
    RELOAD = 2 => "rotate-cw", STOP = 3 => "square", CLOSE = 4 => "x",
    PLUS = 5 => "plus", MORE = 6 => "ellipsis", STAR = 7 => "star",
    BOOK = 8 => "book-open", TABS = 9 => "panels-top-left",
    SEARCH = 10 => "search", GEAR = 11 => "settings", GAUGE = 12 => "gauge",
    SHARE = 13 => "share", EXPAND = 14 => "maximize", READER = 15 => "newspaper",
    MEDIA = 16 => "circle-play", SERVER = 17 => "monitor",
}

pub struct IconAtlas {
    gl: Rc<glow::Context>,
    texture: glow::Texture,
    pixels: u32,
    trees: Vec<usvg::Tree>,
}

fn raster(tree: &usvg::Tree, pixels: u32) -> tiny_skia::Pixmap {
    let mut pixmap = tiny_skia::Pixmap::new(pixels, pixels).expect("bounded icon dimensions");
    let scale = pixels as f32 / 24.0;
    resvg::render(
        tree,
        tiny_skia::Transform::from_scale(scale, scale),
        &mut pixmap.as_mut(),
    );
    // Only coverage matters. White straight-alpha pixels let ImGui tint the
    // icon without multiplying premultiplied antialiasing a second time.
    for pixel in pixmap.data_mut().as_chunks_mut::<4>().0 {
        pixel[..3].fill(255);
    }
    pixmap
}

impl IconAtlas {
    pub fn new(gl: &Rc<glow::Context>) -> Result<Self, String> {
        let trees = SOURCES
            .iter()
            .map(|svg| {
                usvg::Tree::from_data(svg, &usvg::Options::default()).map_err(|e| e.to_string())
            })
            .collect::<Result<Vec<_>, _>>()?;
        // SAFETY: created and destroyed on the owning GL thread.
        let texture = unsafe { gl.create_texture()? };
        let mut atlas = Self {
            gl: gl.clone(),
            texture,
            pixels: 0,
            trees,
        };
        atlas.prepare(1.0);
        Ok(atlas)
    }

    pub fn prepare(&mut self, dpi: f32) {
        let pixels = (SIZE * dpi).round().clamp(1.0, 144.0) as u32;
        if self.pixels == pixels {
            return;
        }
        self.pixels = pixels;
        // Transparent gutters prevent neighboring atlas cells bleeding under filtering.
        let cell = pixels + 2;
        let width = cell * self.trees.len() as u32;
        let mut rgba = vec![0u8; (width * cell * 4) as usize];
        for (index, tree) in self.trees.iter().enumerate() {
            let image = raster(tree, pixels);
            for y in 0..pixels as usize {
                let start = ((y + 1) * width as usize + index * cell as usize + 1) * 4;
                let source = y * pixels as usize * 4;
                rgba[start..start + pixels as usize * 4]
                    .copy_from_slice(&image.data()[source..source + pixels as usize * 4]);
            }
        }
        // SAFETY: active GL host context; owned, bounded RGBA upload.
        unsafe {
            self.gl.bind_texture(glow::TEXTURE_2D, Some(self.texture));
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
                glow::RGBA8 as i32,
                width as i32,
                cell as i32,
                0,
                glow::RGBA,
                glow::UNSIGNED_BYTE,
                Some(&rgba),
            );
            self.gl.bind_texture(glow::TEXTURE_2D, None);
        }
    }

    pub fn draw(&self, ui: &imgui::Ui, icon: Icon, min: [f32; 2], max: [f32; 2]) {
        let a = [
            (min[0] + max[0] - SIZE) * 0.5,
            (min[1] + max[1] - SIZE) * 0.5,
        ];
        let b = [a[0] + SIZE, a[1] + SIZE];
        let cell = (self.pixels + 2) as f32;
        let width = cell * SOURCES.len() as f32;
        // SAFETY: public API during an active UI frame; respects fade/disabled alpha.
        let color = unsafe { imgui::sys::igGetColorU32_Col(imgui::StyleColor::Text as i32, 1.0) };
        ui.get_window_draw_list()
            .add_image(imgui::TextureId::new(self.texture.0.get() as usize), a, b)
            .uv_min([(icon.0 as f32 * cell + 1.0) / width, 1.0 / cell])
            .uv_max([
                (icon.0 as f32 * cell + 1.0 + self.pixels as f32) / width,
                (1.0 + self.pixels as f32) / cell,
            ])
            .col(color)
            .build();
    }
}

impl Drop for IconAtlas {
    fn drop(&mut self) {
        // SAFETY: Assets is dropped before the host GL context.
        unsafe {
            self.gl.delete_texture(self.texture);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn svg_canvases_and_symmetric_icon_pixels_are_centered() {
        for (index, source) in SOURCES.iter().enumerate() {
            let tree = usvg::Tree::from_data(source, &usvg::Options::default()).unwrap();
            assert_eq!(tree.size().width(), 24.0);
            assert_eq!(tree.size().height(), 24.0);
            for pixels in [18, 23, 27, 36] {
                let image = raster(&tree, pixels);
                let mut weight = 0.0;
                let mut moment = [0.0, 0.0];
                for y in 0..pixels {
                    for x in 0..pixels {
                        let alpha = image.data()[((y * pixels + x) * 4 + 3) as usize] as f64;
                        weight += alpha;
                        moment[0] += (x as f64 + 0.5) * alpha;
                        moment[1] += (y as f64 + 0.5) * alpha;
                    }
                }
                assert!(weight > 0.0, "empty icon {index}");
                if [STOP.0, CLOSE.0, PLUS.0, MORE.0, EXPAND.0].contains(&index) {
                    for m in moment {
                        assert!(
                            (m / weight - pixels as f64 / 2.0).abs() < 0.15,
                            "icon {index} is off center at {pixels}px"
                        );
                    }
                }
            }
        }
    }
}
