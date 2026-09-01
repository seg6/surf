mod page_input;
mod ui;
mod video_surface;

use std::error::Error;
use std::num::NonZeroU32;
use std::time::Instant;

use glutin::config::{ConfigTemplateBuilder, GlConfig as _};
use glutin::context::{ContextApi, ContextAttributesBuilder, NotCurrentGlContext as _, Version};
use glutin::display::{GetGlDisplay as _, GlDisplay as _};
use glutin::prelude::GlSurface as _;
use glutin::surface::{Surface, SurfaceAttributesBuilder, SwapInterval, WindowSurface};
use imgui::{ConfigFlags, Context, FontConfig, FontSource};
use imgui_glow_renderer::glow::HasContext as _;
use imgui_glow_renderer::{AutoRenderer, glow};
use imgui_winit_support::{HiDpiMode, WinitPlatform, winit};
use raw_window_handle::HasWindowHandle as _;
use winit::dpi::{LogicalSize, PhysicalSize};
use winit::event::{Event, WindowEvent};
use winit::event_loop::{ControlFlow, EventLoop};
use winit::window::{Icon, WindowAttributes};

use ui::DesktopApp;

const APP_ICON: &[u8] = include_bytes!("../../../../../backend/cmd/surf/surf-icon.png");

fn main() {
    if let Err(error) = run() {
        eprintln!("Surf desktop failed: {error}");
        std::process::exit(1);
    }
}

fn run() -> Result<(), Box<dyn Error>> {
    let event_loop = EventLoop::new()?;
    let attributes = WindowAttributes::default()
        .with_title("Surf")
        .with_inner_size(LogicalSize::new(1180.0, 760.0))
        .with_min_inner_size(LogicalSize::new(320.0, 320.0))
        .with_window_icon(load_window_icon());
    let (window, config) = glutin_winit::DisplayBuilder::new()
        .with_window_attributes(Some(attributes))
        .build(&event_loop, ConfigTemplateBuilder::new(), |configs| {
            configs
                .max_by_key(|config| (config.srgb_capable(), config.num_samples()))
                .expect("glutin supplied no framebuffer configuration")
        })?;
    let window = window.ok_or("glutin did not create a window")?;
    let raw_window = window.window_handle()?.as_raw();
    let context_attributes = ContextAttributesBuilder::new().build(Some(raw_window));
    let fallback_attributes = ContextAttributesBuilder::new()
        .with_context_api(ContextApi::Gles(Some(Version::new(3, 0))))
        .build(Some(raw_window));
    // SAFETY: both attribute sets reference the live winit window.
    let not_current = unsafe {
        config
            .display()
            .create_context(&config, &context_attributes)
            .or_else(|_| {
                config
                    .display()
                    .create_context(&config, &fallback_attributes)
            })?
    };
    let mut surface_size = window.inner_size();
    let initial = nonzero_size(surface_size);
    let surface_attributes = SurfaceAttributesBuilder::<WindowSurface>::new()
        .with_srgb(Some(true))
        .build(raw_window, initial.0, initial.1);
    // SAFETY: the surface targets the same live window and display as config.
    let surface = unsafe {
        config
            .display()
            .create_window_surface(&config, &surface_attributes)?
    };
    let gl_context = not_current.make_current(&surface)?;
    let _ = surface.set_swap_interval(
        &gl_context,
        SwapInterval::Wait(NonZeroU32::new(1).expect("one is non-zero")),
    );
    // SAFETY: the glutin context is current on this event-loop thread.
    let gl = unsafe {
        glow::Context::from_loader_function_cstr(|name| {
            config.display().get_proc_address(name).cast()
        })
    };

    let mut imgui = Context::create();
    imgui.set_ini_filename(None);
    imgui.io_mut().config_flags |= ConfigFlags::NAV_ENABLE_KEYBOARD;
    install_style(&mut imgui);
    if let Ok(clipboard) = Clipboard::new() {
        imgui.set_clipboard_backend(clipboard);
    }
    let mut platform = WinitPlatform::new(&mut imgui);
    platform.attach_window(imgui.io_mut(), &window, HiDpiMode::Rounded);
    let hidpi_factor = platform.hidpi_factor() as f32;
    install_fonts(&mut imgui, hidpi_factor);
    imgui.io_mut().font_global_scale = 1.0 / hidpi_factor;

    let mut renderer = AutoRenderer::new(gl, &mut imgui)?;
    let shared_gl = renderer.gl_context().clone();
    let mut app = DesktopApp::new(&shared_gl)?;
    let mut last_frame = Instant::now();

    #[allow(deprecated)]
    event_loop.run(move |event, target| {
        target.set_control_flow(ControlFlow::Poll);
        platform.handle_event(imgui.io_mut(), &window, &event);
        match event {
            Event::NewEvents(_) => {
                let now = Instant::now();
                imgui
                    .io_mut()
                    .update_delta_time(now.duration_since(last_frame));
                last_frame = now;
            }
            Event::WindowEvent { event, .. } => match event {
                WindowEvent::CloseRequested => target.exit(),
                WindowEvent::Resized(size) => {
                    resize_surface(&surface, &gl_context, size);
                    surface_size = size;
                }
                WindowEvent::ScaleFactorChanged { .. } => {
                    surface_size = window.inner_size();
                    resize_surface(&surface, &gl_context, surface_size);
                }
                WindowEvent::RedrawRequested => {
                    // Some window systems apply request_inner_size synchronously and
                    // intentionally emit no Resized event. Keep both ImGui and the GL
                    // swapchain tied to Winit's actual inner size on every frame.
                    let actual_size = window.inner_size();
                    if actual_size != surface_size {
                        resize_surface(&surface, &gl_context, actual_size);
                        surface_size = actual_size;
                    }
                    sync_imgui_display_size(imgui.io_mut(), surface_size, window.scale_factor());
                    if let Err(error) = platform.prepare_frame(imgui.io_mut(), &window) {
                        app.report_host_error(format!("window input: {error}"));
                    }
                    app.tick();
                    if let Some(dark) = app.take_theme_request() {
                        apply_palette(imgui.style_mut(), dark);
                    }
                    let frame = imgui.frame();
                    app.draw(frame, &window);
                    platform.prepare_render(frame, &window);
                    let draw_data = imgui.render();

                    // A device preset can synchronously resize the window from
                    // inside app.draw. Rendering draw data built for the old size
                    // would make the compositor stretch the complete UI. Resize and
                    // present a clean frame; the next redraw rebuilds ImGui exactly.
                    let size_after_draw = window.inner_size();
                    if size_after_draw != surface_size {
                        resize_surface(&surface, &gl_context, size_after_draw);
                        surface_size = size_after_draw;
                        // SAFETY: this thread owns the current context and framebuffer.
                        unsafe {
                            shared_gl.viewport(
                                0,
                                0,
                                i32::try_from(surface_size.width).unwrap_or(i32::MAX),
                                i32::try_from(surface_size.height).unwrap_or(i32::MAX),
                            );
                            shared_gl.clear_color(0.063, 0.071, 0.078, 1.0);
                            shared_gl.clear(glow::COLOR_BUFFER_BIT);
                        }
                        if let Err(error) = surface.swap_buffers(&gl_context) {
                            app.report_host_error(format!("swap buffers: {error}"));
                        }
                        window.request_redraw();
                        return;
                    }

                    // SAFETY: this thread owns the current context and default framebuffer.
                    unsafe {
                        shared_gl.viewport(
                            0,
                            0,
                            i32::try_from(window.inner_size().width).unwrap_or(i32::MAX),
                            i32::try_from(window.inner_size().height).unwrap_or(i32::MAX),
                        );
                        shared_gl.clear_color(0.063, 0.071, 0.078, 1.0);
                        shared_gl.clear(glow::COLOR_BUFFER_BIT);
                    }
                    app.render_video(
                        &shared_gl,
                        window.scale_factor(),
                        window.inner_size().height,
                    );
                    // imgui-rs represents an empty frame with a null draw-list
                    // pointer and count zero. Avoid constructing a Rust slice
                    // from that sentinel (debug UB checks reject even a
                    // zero-length slice with a null pointer).
                    if draw_data.draw_lists_count() != 0
                        && let Err(error) = renderer.render(draw_data)
                    {
                        app.report_host_error(format!("ImGui renderer: {error}"));
                    }
                    if let Err(error) = surface.swap_buffers(&gl_context) {
                        app.report_host_error(format!("swap buffers: {error}"));
                    }
                    if app.after_present(&window) {
                        target.exit();
                    }
                }
                other => app.handle_window_event(&window, &other),
            },
            Event::AboutToWait => window.request_redraw(),
            _ => {}
        }
    })?;
    Ok(())
}

fn resize_surface(
    surface: &Surface<WindowSurface>,
    context: &glutin::context::PossiblyCurrentContext,
    size: PhysicalSize<u32>,
) {
    let (width, height) = nonzero_size(size);
    surface.resize(context, width, height);
}

fn sync_imgui_display_size(io: &mut imgui::Io, size: PhysicalSize<u32>, scale_factor: f64) {
    let logical = size.to_logical::<f64>(scale_factor);
    io.display_size = [logical.width as f32, logical.height as f32];
}

fn nonzero_size(size: PhysicalSize<u32>) -> (NonZeroU32, NonZeroU32) {
    (
        NonZeroU32::new(size.width.max(1)).expect("clamped non-zero width"),
        NonZeroU32::new(size.height.max(1)).expect("clamped non-zero height"),
    )
}

fn load_window_icon() -> Option<Icon> {
    let image = image::load_from_memory(APP_ICON).ok()?.into_rgba8();
    let (width, height) = image.dimensions();
    Icon::from_rgba(image.into_raw(), width, height).ok()
}

fn install_fonts(imgui: &mut Context, hidpi_factor: f32) {
    let size = 13.0 * hidpi_factor;
    imgui.fonts().add_font(&[FontSource::DefaultFontData {
        config: Some(FontConfig {
            size_pixels: size,
            pixel_snap_h: true,
            ..FontConfig::default()
        }),
    }]);
}

fn install_style(imgui: &mut Context) {
    let style = imgui.style_mut();
    style.window_padding = [7.0, 6.0];
    style.frame_padding = [5.0, 3.0];
    style.item_spacing = [4.0, 3.0];
    style.item_inner_spacing = [4.0, 3.0];
    style.scrollbar_size = 9.0;
    style.grab_min_size = 8.0;
    style.window_rounding = 4.0;
    style.child_rounding = 3.0;
    style.frame_rounding = 3.0;
    style.popup_rounding = 4.0;
    style.scrollbar_rounding = 4.0;
    style.tab_rounding = 3.0;
    style.window_border_size = 1.0;
    style.child_border_size = 0.0;
    style.frame_border_size = 1.0;
    apply_palette(style, true);
}

fn apply_palette(style: &mut imgui::Style, dark: bool) {
    use imgui::StyleColor;
    if !dark {
        style.colors[StyleColor::Text as usize] = [0.08, 0.09, 0.10, 1.0];
        style.colors[StyleColor::TextDisabled as usize] = [0.42, 0.45, 0.48, 1.0];
        style.colors[StyleColor::WindowBg as usize] = [0.94, 0.95, 0.96, 0.98];
        style.colors[StyleColor::ChildBg as usize] = [0.94, 0.95, 0.96, 0.0];
        style.colors[StyleColor::PopupBg as usize] = [0.98, 0.98, 0.99, 0.99];
        style.colors[StyleColor::Border as usize] = [0.66, 0.68, 0.70, 1.0];
        style.colors[StyleColor::FrameBg as usize] = [0.86, 0.87, 0.88, 1.0];
        style.colors[StyleColor::FrameBgHovered as usize] = [0.80, 0.82, 0.84, 1.0];
        style.colors[StyleColor::FrameBgActive as usize] = [0.75, 0.78, 0.80, 1.0];
        style.colors[StyleColor::Button as usize] = [0.94, 0.95, 0.96, 0.0];
        style.colors[StyleColor::ButtonHovered as usize] = [0.82, 0.84, 0.86, 1.0];
        style.colors[StyleColor::ButtonActive as usize] = [0.75, 0.78, 0.80, 1.0];
        style.colors[StyleColor::Header as usize] = [0.86, 0.87, 0.88, 1.0];
        style.colors[StyleColor::HeaderHovered as usize] = [0.80, 0.82, 0.84, 1.0];
        style.colors[StyleColor::HeaderActive as usize] = [0.75, 0.78, 0.80, 1.0];
        style.colors[StyleColor::CheckMark as usize] = [0.04, 0.52, 0.68, 1.0];
        style.colors[StyleColor::SliderGrab as usize] = [0.04, 0.52, 0.68, 1.0];
        style.colors[StyleColor::Separator as usize] = [0.66, 0.68, 0.70, 1.0];
        style.colors[StyleColor::NavHighlight as usize] = [0.04, 0.52, 0.68, 1.0];
        return;
    }
    style.colors[StyleColor::Text as usize] = [0.91, 0.92, 0.94, 1.0];
    style.colors[StyleColor::TextDisabled as usize] = [0.49, 0.53, 0.57, 1.0];
    style.colors[StyleColor::WindowBg as usize] = [0.098, 0.11, 0.125, 0.98];
    style.colors[StyleColor::ChildBg as usize] = [0.098, 0.11, 0.125, 0.0];
    style.colors[StyleColor::PopupBg as usize] = [0.137, 0.153, 0.173, 0.99];
    style.colors[StyleColor::Border as usize] = [0.204, 0.227, 0.251, 1.0];
    style.colors[StyleColor::FrameBg as usize] = [0.137, 0.153, 0.173, 1.0];
    style.colors[StyleColor::FrameBgHovered as usize] = [0.18, 0.20, 0.22, 1.0];
    style.colors[StyleColor::FrameBgActive as usize] = [0.20, 0.23, 0.25, 1.0];
    style.colors[StyleColor::Button as usize] = [0.098, 0.11, 0.125, 0.0];
    style.colors[StyleColor::ButtonHovered as usize] = [0.16, 0.18, 0.20, 1.0];
    style.colors[StyleColor::ButtonActive as usize] = [0.20, 0.23, 0.25, 1.0];
    style.colors[StyleColor::Header as usize] = [0.137, 0.153, 0.173, 1.0];
    style.colors[StyleColor::HeaderHovered as usize] = [0.18, 0.20, 0.22, 1.0];
    style.colors[StyleColor::HeaderActive as usize] = [0.20, 0.23, 0.25, 1.0];
    style.colors[StyleColor::CheckMark as usize] = [0.325, 0.718, 0.824, 1.0];
    style.colors[StyleColor::SliderGrab as usize] = [0.325, 0.718, 0.824, 1.0];
    style.colors[StyleColor::Separator as usize] = [0.204, 0.227, 0.251, 1.0];
    style.colors[StyleColor::NavHighlight as usize] = [0.325, 0.718, 0.824, 1.0];
}

struct Clipboard(arboard::Clipboard);

impl Clipboard {
    fn new() -> Result<Self, arboard::Error> {
        arboard::Clipboard::new().map(Self)
    }
}

impl imgui::ClipboardBackend for Clipboard {
    fn get(&mut self) -> Option<String> {
        self.0.get_text().ok()
    }

    fn set(&mut self, value: &str) {
        let _ = self.0.set_text(value.to_owned());
    }
}
