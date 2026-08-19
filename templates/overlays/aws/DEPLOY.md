# AWS deploy (EC2 + Docker + Caddy + sslip.io)

Everything runs as containers on one EC2 inside your VPC. Caddy terminates TLS
automatically via Let's Encrypt for `*.YOUR-IP.sslip.io` - zero cert management.

## 1. EC2

- Launch an instance in your VPC (t3.medium is plenty for dev/staging).
- Security group: open 22, 80, 443. Keep everything else closed; databases stay
  on the internal network (compose bridge), you don't need to expose 5432/6379.
- Attach an Elastic IP if you want a stable `DOMAIN`.

## 2. On the box

    sudo dnf install -y git docker   # or your distro's equivalents
    sudo systemctl enable --now docker
    sudo usermod -aG docker "$USER"   # re-login after this
    git clone <your-repo> && cd <your-repo>

## 3. Configure + launch

    cp .env.example .env              # set DOMAIN, POSTGRES_PASSWORD, BACKEND_SECRET
    docker compose -f compose.prod.yaml up -d --build

## 4. Verify

    curl https://YOUR-PUBLIC-IP.sslip.io          # web
    curl https://api.YOUR-PUBLIC-IP.sslip.io/health

Caddy auto-redirects http -> https and renews certs itself (see the "certbot
lore" this template was born to remove).

## Scaling notes

- This runs one instance of every service (docker compose). Horizontal scale
  = multiple boxes + a load balancer; that's when realtime needs the Redis
  pub/sub broker instead of its in-memory fanout (see apps/realtime/src/broker.gleam).
- VPC subnets/peering, RDS instead of container Postgres, ECR: account-level
  choices, add them when the workloads justify it - the API talks to anything
  that speaks Postgres/Redis.
## Infrastructure (Terraform)

`infra/main.tf` provisions the EC2 host (Docker preinstalled, SSH key, SG for
22/80/443, static EIP). It does not install the app:

    cd infra
    terraform init && terraform apply   # sets SSH_PUBLIC_KEY_PATH if your key differs

Then on the host, clone the repo and follow "first deploy" below. GitHub Actions
(`.github/workflows/cd.yml`) redeploys on push to main (secrets: `SSH_HOST`,
`SSH_USER`, `SSH_KEY`).

## Google OAuth

Set `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_REDIRECT_URI`
(https://api.${DOMAIN}/api/auth/google/callback) and `APP_URL` in `.env`.
Dev login stays available for local work.
