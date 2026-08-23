# build context = repo ROOT (the Ai server imports api/pb via a replace directive)
FROM golang:1-alpine AS gobuild
WORKDIR /build
COPY api/ api/
COPY ai/ ai/
# ai/go.mod has: replace .../api => ../api  -> resolves to /build/api
WORKDIR /build/ai
RUN go mod download && CGO_ENABLED=0 go build -o /out/ai ./cmd/ai

FROM python:3.12-slim
RUN apt-get update && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=gobuild /out/ai /usr/local/bin/ai
COPY ai/python/requirements.txt python/
RUN pip install --no-cache-dir -r python/requirements.txt
COPY ai/python/app.py python/

# start.sh: rag sidecar (:8003) first, then the Go gRPC server (:8002)
RUN printf '#!/bin/sh\nset -e\nuvicorn app:app --app-dir python --host 127.0.0.1 --port 8003 &\nexec ai\n' > start.sh && chmod +x start.sh

EXPOSE 8002 8081
HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=15s \
  CMD curl -fsS http://localhost:8081/healthz || exit 1
CMD ["./start.sh"]
