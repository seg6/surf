use std::path::PathBuf;

fn main() {
    let manifest =
        PathBuf::from(std::env::var_os("CARGO_MANIFEST_DIR").expect("CARGO_MANIFEST_DIR is set"));
    let core = manifest.join("../../../core");
    let sources = [
        "src/core.c",
        "src/diagnostics.c",
        "src/frame.c",
        "src/h264.c",
        "src/input.c",
        "src/media.c",
        "src/protocol.c",
        "src/session.c",
    ];

    let mut build = cc::Build::new();
    build
        .include(core.join("include"))
        .std("c99")
        .warnings(true)
        .extra_warnings(true)
        .flag_if_supported("-Wpedantic")
        .flag_if_supported("-Wconversion")
        .flag_if_supported("-Wshadow");
    for source in sources {
        let path = core.join(source);
        println!("cargo:rerun-if-changed={}", path.display());
        build.file(path);
    }
    println!(
        "cargo:rerun-if-changed={}",
        core.join("include/surf/diagnostics.h").display()
    );
    println!(
        "cargo:rerun-if-changed={}",
        core.join("include/surf/core.h").display()
    );
    println!(
        "cargo:rerun-if-changed={}",
        core.join("include/surf/frame.h").display()
    );
    println!(
        "cargo:rerun-if-changed={}",
        core.join("include/surf/h264.h").display()
    );
    println!(
        "cargo:rerun-if-changed={}",
        core.join("include/surf/input.h").display()
    );
    println!(
        "cargo:rerun-if-changed={}",
        core.join("include/surf/media.h").display()
    );
    println!(
        "cargo:rerun-if-changed={}",
        core.join("include/surf/protocol.h").display()
    );
    println!(
        "cargo:rerun-if-changed={}",
        core.join("include/surf/session.h").display()
    );
    build.compile("surf_client_core");
}
