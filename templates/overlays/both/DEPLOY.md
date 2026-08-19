# Deploy - both flavors

This project deploys the same repo to **Cloudflare and AWS**. Local dev uses
`standalone` SSR (default build); Cloudflare CI sets `BUILD_TARGET=export`.

CI/CD: `.github/workflows/cd.yml` deploys both on push to main -
`deploy-cloudflare` (secrets: `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`)
and `deploy-aws` (secrets: `SSH_HOST`, `SSH_USER`, `SSH_KEY`).

---

# Cloudflare

Prereqs: `bun`, a Cloudflare account, and a reachable backend for `API_ORIGIN`
(your Rust API + Postgres + Redis, e.g. on the AWS host behind
`https://api.YOUR-IP.sslip.io`, or `localhost` during `wrangler dev`).

## 1. Create the KV namespace

    cd apps/gateway
    npx wrangler kv namespace create SESSIONS

Paste the returned `id` and `preview_id` into `apps/gateway/wrangler.toml`.

## 2. Build the web static export

    BUILD_TARGET=export npm run build --workspace web   # outputs apps/web/out

## 3. Run locally

    cd apps/gateway
    cp .dev.vars.example .dev.vars    # set API_ORIGIN
    npx wrangler dev

Open the printed URL, hit /dashboard, click "dev login".

## 4. Deploy

    npx wrangler deploy

Prod secrets live in the Cloudflare dashboard (Workers > gateway > Settings > Variables):
`API_ORIGIN`, `BACKEND_SECRET`, `SESSION_TTL` - plus `GOOGLE_CLIENT_ID` and
`GOOGLE_CLIENT_SECRET` for Google OAuth, with the callback URL
`https://<worker>.<subdomain>.workers.dev/api/auth/google/callback` registered in
Google Cloud. KV binding comes from wrangler.toml.

## Notes

- Web is a static export on Cloudflare (auth + proxy live in the worker). SSR stays in dev / AWS.
- SSE flows: browser -> worker `/api/events` -> `API_ORIGIN`/api/events. WebSocket in production: connect clients directly
  to the backend, or add an SSE-only contract (workers can't open outbound WS from a fetch handler).
- Cache: set `CACHE_BACKEND=kv` on the API with `CF_ACCOUNT_ID` + `CF_KV_NAMESPACE` + `CF_API_TOKEN`
  (create a second namespace for API cache) when you don't want Redis for it.

---

# AWS (EC2 + Docker + Caddy + sslip.io)

Everything runs as containers on one EC2 inside your VPC. Caddy terminates TLS
automatically via Let's Encrypt for `*.YOUR-IP.sslip.io` - zero cert management.

## 1. EC2

- Terraform: `cd infra && terraform init && terraform apply` (EC2, SG for
  22/80/443, EIP, Docker preinstalled). Or launch manually (t3.medium is
  plenty for dev/staging): security group open 22/80/443, Elastic IP for a
  stable `DOMAIN`.

## 2. On the box

    git clone <your-repo> && cd <your-repo>

## 3. Configure + launch

    cp .env.example .env              # set DOMAIN, POSTGRES_PASSWORD, BACKEND_SECRET, GOOGLE_*
    docker compose -f compose.prod.yaml up -d --build

## 4. Verify

    curl https://YOUR-PUBLIC-IP.sslip.io          # web
    curl https://api.YOUR-PUBLIC-IP.sslip.io/health

Caddy auto-redirects http -> https and renews certs itself.

## Scaling notes

- This runs one instance of every service (docker compose). Horizontal scale
  = multiple boxes + a load balancer; that's when realtime needs the Redis
  pub/sub broker instead of its in-memory fanout (see apps/realtime/src/broker.gleam).
- VPC subnets/peering, RDS instead of container Postgres, ECR: account-level
  choices, add them when the workloads justify it - the API talks to anything
  that speaks Postgres/Redis.

## Google OAuth (AWS)

Set `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_REDIRECT_URI`
(https://api.${DOMAIN}/api/auth/google/callback) and `APP_URL` in `.env`.
Dev login stays available for local work. On Cloudflare the same keys live in
the worker's secrets instead.