FROM rust:1-slim AS build
WORKDIR /app
COPY Cargo.toml Cargo.lock ./
RUN mkdir src && echo "fn main(){}" > src/main.rs && cargo build --release || true
COPY src src
COPY migrations migrations
# touch forces cargo to see real source as newer than the dummy build
# (COPY preserves mtimes unreliably, so incremental reuse would keep the dummy)
RUN find src -type f -exec touch {} + && cargo build --release

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/*
COPY --from=build /app/target/release/api /usr/local/bin/api
EXPOSE 8000
CMD ["api"]