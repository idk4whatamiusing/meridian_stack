# build context = repo ROOT (build.rs compiles ../packages/proto/db.proto)
FROM rust:1-slim AS build
WORKDIR /build
RUN apt-get update \
    && apt-get install -y --no-install-recommends protobuf-compiler pkg-config libssl-dev \
    && rm -rf /var/lib/apt/lists/*

COPY packages/proto packages/proto

# dependency cache: dummy main first so deps compile once
COPY db/Cargo.toml db/Cargo.lock db/
COPY db/build.rs db/build.rs
# ../packages/proto must resolve inside the context, so crate sits in a subdir
WORKDIR /build/db
RUN mkdir src && echo "fn main(){}" > src/main.rs && cargo build --release || true

COPY db/src ./src
COPY db/migrations ./migrations
RUN find src -type f -exec touch {} + && cargo build --release

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/*
COPY --from=build /build/db/target/release/db /usr/local/bin/db
EXPOSE 8010 8011
CMD ["db"]
