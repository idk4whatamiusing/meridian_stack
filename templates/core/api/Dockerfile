# build context = repo root (needs packages/proto via api/pb? no - pb is committed Go code)
FROM golang:1-alpine AS build
WORKDIR /src
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/ .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl
COPY --from=build /out/api /usr/local/bin/api
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=10s \
  CMD curl -fsS http://localhost:8000/api/health || exit 1
CMD ["api"]
