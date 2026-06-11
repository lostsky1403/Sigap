use std::env;
use std::path::PathBuf;

fn main() {
    let out_dir = PathBuf::from(env::var("OUT_DIR").unwrap());
    tonic_build::configure()
        .build_server(true)
        .build_client(false)
        .out_dir(out_dir)
        .compile(
            &["protos/sigap/queue_engine.proto"],
            &["protos"],
        )
        .unwrap_or_else(|e| panic!("Failed to compile protos: {}", e));

    println!("cargo:rerun-if-changed=protos/sigap/queue_engine.proto");
}
