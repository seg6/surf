//! A single logical geometry contract for chrome, video, hit testing and viewport requests.
use crate::page_input::PageRect;

pub const RAIL_HEIGHT: f32 = 42.0;
pub const CONTROL: f32 = 30.0;
pub const FIND_HEIGHT: f32 = 42.0;

#[derive(Clone, Copy, Debug, PartialEq)]
pub enum Density {
    Minimal,
    Compact,
    Wide,
}

#[derive(Clone, Copy)]
pub struct BrowserLayout {
    pub rail: [f32; 4],
    pub page: PageRect,
    pub find: [f32; 4],
    pub density: Density,
}

impl BrowserLayout {
    pub fn new(size: [f32; 2], bottom: bool, find: bool) -> Self {
        let [w, h] = size;
        let extra = if find { FIND_HEIGHT } else { 0.0 };
        let rail_y = if bottom {
            (h - RAIL_HEIGHT).max(0.0)
        } else {
            0.0
        };
        let find_y = if bottom { rail_y - extra } else { RAIL_HEIGHT };
        Self {
            rail: [0.0, rail_y, w, RAIL_HEIGHT],
            page: PageRect {
                x: 0.0,
                y: if bottom {
                    0.0
                } else {
                    f64::from(RAIL_HEIGHT + extra)
                },
                width: f64::from(w.max(1.0)),
                height: f64::from((h - RAIL_HEIGHT - extra).max(1.0)),
            },
            find: [0.0, find_y, w, extra],
            density: if w >= 720.0 {
                Density::Wide
            } else if w >= 480.0 {
                Density::Compact
            } else {
                Density::Minimal
            },
        }
    }
}

/// Bounded floating surfaces flip above bottom chrome, never spill off-screen.
pub fn anchored(display: [f32; 2], anchor: [f32; 4], desired: [f32; 2]) -> ([f32; 2], [f32; 2]) {
    let size = [
        desired[0].min((display[0] - 16.0).max(1.0)),
        desired[1].min((display[1] - 16.0).max(1.0)),
    ];
    let x = anchor[0].clamp(8.0, (display[0] - size[0] - 8.0).max(8.0));
    let below = anchor[1] + anchor[3] + 6.0;
    let y = if below + size[1] <= display[1] - 8.0 {
        below
    } else {
        anchor[1] - size[1] - 6.0
    };
    (
        [x, y.clamp(8.0, (display[1] - size[1] - 8.0).max(8.0))],
        size,
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn rail_never_changes_height_across_devices() {
        for width in [320.0, 375.0, 480.0, 768.0, 1180.0] {
            for bottom in [true, false] {
                let l = BrowserLayout::new([width, 760.0], bottom, false);
                assert_eq!(l.page.height, 718.0);
                assert_eq!(l.rail[3], 42.0);
            }
        }
    }
    #[test]
    fn find_is_the_only_overlay_that_takes_page_space() {
        let l = BrowserLayout::new([375.0, 667.0], true, true);
        assert_eq!(l.page.height, 583.0);
        assert_eq!(l.find[1], 583.0);
    }
    #[test]
    fn popup_flips_and_clamps() {
        let (p, s) = anchored([320.0, 480.0], [300.0, 438.0, 30.0, 30.0], [360.0, 900.0]);
        assert_eq!(p, [8.0, 8.0]);
        assert_eq!(s, [304.0, 464.0]);
    }
}
