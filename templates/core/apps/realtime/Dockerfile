FROM gleam-lang/gleam:latest-erlang-alpine AS build
WORKDIR /app
COPY gleam.toml manifest.toml ./
COPY src src
RUN gleam build --runtime erlang

FROM erlang:alpine
WORKDIR /app
COPY --from=build /app/build/prod/erlang /app/build/prod/erlang
EXPOSE 8001
CMD ["/bin/sh", "-c", "erl -pa /app/build/prod/erlang/*/build/**/ebin -pa /app/build/prod/erlang/*/_gleam_artefacts -s realtime main"]