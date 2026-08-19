# Meridian Stack template

Scaffold a polyglot full-stack monorepo: **Next.js (SSR)** frontend, **Rust (axum)**
API with REST + GraphQL + SSE + WebSocket + pluggable cache, **Gleam** realtime
service with Redis pub/sub, **Python (FastAPI)** AI service. Google OAuth + a dev
login. Auth lives at the edge in a **Cloudflare Worker** (KV sessions) or in the
**API (Redis sessions)** on AWS. GitHub Actions CI/CD + Terraform (AWS flavor)
included.

## Quickstart

    bun create meridian-stack    # or: npx create-meridian-stack <name> <cloudflare|aws>
    cd <name>
    docker compose up -d        # postgres + redis (dev infra only)
    bun run dev:web             # Next.js  :3000   (terminal 1)
    bun run dev:api             # Rust     :8000   (terminal 2)
    bun run dev:realtime        # Gleam    :8001   (terminal 3)
    cd apps/ai && uv run --with-requirements requirements.txt uvicorn main:app --port 8002

Open http://localhost:3000/dashboard, click **dev login** - it creates a session
and you'll see live SSE events.

## Layout

| Path | Tech | What it is |
|---|---|---|
| `apps/web` | Next.js 16 + Tailwind 4 | SSR frontend; `output: export` on the Cloudflare flavor |
| `apps/api` | Rust axum 0.8, SQLx, Redis, reqwest | REST + GraphQL (`/api/graphql` + GraphiQL) + `/events` SSE + `/ws` + `/api/broadcast`; `Cache` trait: **kv** (CF KV) or **redis**; looks up sessions from Redis |
| `apps/realtime` | Gleam + mist | SSE + WS fanout via an in-process broker, fanned out to Redis pub/sub when `REDIS_URL` is set (horizontal scale) |
| `apps/ai` | Python FastAPI | `/predict` + `/train` stubs - fill in your models |
| `packages/shared` | TypeScript | types shared across the workspaces |
| `gateway` (CF flavor only) | Hono Worker | dev-login / sessions in KV, proxies `/api/*` + `/events` to the backend with `x-user-id` + `x-backend-secret`, serves the web export |
| `compose.prod.yaml` (AWS flavor) | Caddy + Docker | one-command prod stack with auto-TLS on `*.YOUR-IP.sslip.io` |

## Endpoints

- GET  `:8000/api/health`                API + DB + cache status
- POST `:8000/api/auth/dev-login`        dev session -> Set-Cookie
- GET  `:8000/api/auth/google`           Google OAuth login (+ `/callback` verifies the id_token against Google JWKS)
- GET  `:8000/api/me`                    session user (gateway header on CF, Redis cookie on AWS)
- POST `:8000/api/graphql`               GraphQL (GraphiQL at GET) - `me`, `users`, `health`
- GET  `:8000/api/events`                SSE stream (also proxied by the CF gateway)
- POST `:8000/api/broadcast`             `x-backend-secret: <BACKEND_SECRET>` + JSON `{"message":"hi"}` -> SSE fans out
- WS   `:8000/api/ws`                    echo + broadcast
- GET  `:8001/events` / WS `:8001/ws` / POST `:8001/broadcast`  (Gleam realtime)

All client paths use the `/api/*` prefix so the same URLs work on both flavors:
on Cloudflare the gateway intercepts `/api/*` and forwards them (with the session user) to the backend.

## Auth model

Session = random id -> JSON/session in KV (Cloudflare gateway) or Redis (API on AWS).
The gateway validates the cookie, then forwards `x-user-id` + `x-backend-secret` to
the backend (backend never sees cookies). On AWS the API validates the cookie
against Redis directly. Two login paths: **Google OAuth** (id_token verified
against the Google JWKS - `GOOGLE_CLIENT_ID`/`GOOGLE_CLIENT_SECRET`/`GOOGLE_REDIRECT_URI`)
and **dev login** (any email, for local dev only).

## Config

Copy `.env.example` files and set what applies. CI/CD: `ci.yml` runs web/api/realtime/ai
checks on every PR; the Cloudflare flavor also deploys the gateway worker on push to
`main`; the AWS flavor SSH-deploys `compose.prod.yaml` (secrets: `CLOUDFLARE_API_TOKEN`,
`CLOUDFLARE_ACCOUNT_ID`, or `SSH_HOST`/`SSH_USER`/`SSH_KEY`). AWS infrastructure via
`infra/` (Terraform: EC2 + SG + EIP + docker install). Biggest knobs:
- `CACHE_BACKEND` (`redis` default | `kv`) on the API
- `NEXT_PUBLIC_API_URL` in `apps/web` (empty = same-origin; set `http://localhost:8000` in AWS dev)
- `API_ORIGIN` + `BACKEND_SECRET` in the gateway's `.dev.vars` / prod vars

## Deploy

Pick your flavor's `DEPLOY.md`: `cloudflare/` or `aws/` were copied into your
project root during scaffolding.

## Deliberately deferred (v2+)

GraphQL (async-graphql - your Cartis/FitMentor apps show the schema is product-shaped),
GitHub Actions CI/CD, Terraform, real OAuth, OpenNext SSR on Cloudflare,
realtime Redis pub/sub / Durable Objects fanout. Each is a ~half-day lift on top
of this foundation.