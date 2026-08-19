FROM rust:1-slim AS build
WORKDIR /app
COPY Cargo.toml Cargo.lock ./
RUN mkdir src && echo "fn main(){}" > src/main.rs && cargo build --release || true
COPY src src
COPY migrations migrations
RUN cargo build --release

FROM debian:bookworm-slim
COPY --from=build /app/target/release/api /usr/local/bin/api
EXPOSE 8000
CMD ["api"]