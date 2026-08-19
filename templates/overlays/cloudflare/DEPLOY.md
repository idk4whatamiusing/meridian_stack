# Cloudflare deploy

Prereqs: `bun`, a Cloudflare account, and a reachable backend for `API_ORIGIN` (your Rust API + Postgres + Redis,
e.g. on an EC2 behind `https://api.YOUR-IP.sslip.io`, or `localhost` during `wrangler dev`).

## 1. Create the KV namespace

    cd apps/gateway
    npx wrangler kv namespace create SESSIONS

Paste the returned `id` and `preview_id` into `apps/gateway/wrangler.toml`.

## 2. Build the web static export

    bun run build:web          # outputs apps/web/out

## 3. Run locally

    cd apps/gateway
    cp .dev.vars.example .dev.vars    # set API_ORIGIN
    npx wrangler dev

Open the printed URL, hit /dashboard, click "dev login".

## 4. Deploy

    npx wrangler deploy

Prod secrets live in the Cloudflare dashboard (Workers > gateway > Settings > Variables):
`API_ORIGIN`, `BACKEND_SECRET`, `SESSION_TTL` — plus `GOOGLE_CLIENT_ID` and
`GOOGLE_CLIENT_SECRET` for Google OAuth, with the callback URL
`https://<worker>.<subdomain>.workers.dev/api/auth/google/callback` registered in
Google Cloud. KV binding comes from wrangler.toml. CI/CD: `.github/workflows/cd.yml`
builds the web export and deploys the worker on push to main.

## Notes

- Web is a static export on Cloudflare (auth + proxy live in the worker). SSR stays in dev / AWS.
- SSE flows: browser -> worker `/api/events` -> `API_ORIGIN`/api/events. WebSocket in production: connect clients directly
  to the backend, or add an SSE-only contract (workers can't open outbound WS from a fetch handler).
- Cache: set `CACHE_BACKEND=kv` on the API with `CF_ACCOUNT_ID` + `CF_KV_NAMESPACE` + `CF_API_TOKEN`
  (create a second namespace for API cache) when you don't want Redis for it.