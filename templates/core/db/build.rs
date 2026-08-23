fn main() -> Result<(), Box<dyn std::error::Error>> {
    let manifest = env!("CARGO_MANIFEST_DIR");
    let proto_root = format!("{manifest}/../packages/proto");
    tonic_build::configure().compile_protos(
        &[format!("{proto_root}/db.proto")],
        &[proto_root],
    )?;
    Ok(())
}
