import { Hono } from "hono";

export interface Env {
  API_ORIGIN: string;
  BACKEND_SECRET: string;
  SESSION_TTL: string;
  GOOGLE_CLIENT_ID: string;
  GOOGLE_CLIENT_SECRET: string;
  SESSIONS: KVNamespace;
}

const app = new Hono<{ Bindings: Env }>();

const ttl = (e: Env) => Number(e.SESSION_TTL) || 604800;

function sessionId(c: any) {
  const raw = c.req.raw.headers.get("cookie") ?? "";
  for (const part of raw.split(";")) {
    const p = part.trim();
    if (p.startsWith("session=")) return p.slice(8);
  }
  return null;
}

// dev-login: create a session in KV and set the cookie (real Google OAuth adds a callback route + id_token verify)
app.post("/api/auth/dev-login", async (c) => {
  const body = await c.req.json().catch(() => ({}));
  const email = (body as any).email || "dev@example.com";
  const id = crypto.randomUUID();
  await c.env.SESSIONS.put(id, email, { expirationTtl: ttl(c.env) });
  c.header(
    "Set-Cookie",
    `session=${id}; Path=/; HttpOnly; SameSite=Lax; Max-Age=${ttl(c.env)}`,
  );
  return c.json({ ok: true });
});

app.post("/api/auth/logout", async (c) => {
  const id = sessionId(c);
  if (id) await c.env.SESSIONS.delete(id);
  c.header("Set-Cookie", "session=; Path=/; HttpOnly; Max-Age=0");
  return c.json({ ok: true });
});

app.get("/api/me", async (c) => {
  const id = sessionId(c);
  const email = id ? await c.env.SESSIONS.get(id) : null;
  if (!email) return c.json({ error: "unauthorized" }, 401);
  return c.json({ user: { id, email } });
});

// google oauth: redirect to Google, then verify the id_token with the JWKS and create the session
app.get("/api/auth/google", (c) => {
  if (!c.env.GOOGLE_CLIENT_ID) return c.redirect("/?error=google_not_configured");
  const origin = new URL(c.req.url).origin;
  const params = new URLSearchParams({
    client_id: c.env.GOOGLE_CLIENT_ID,
    redirect_uri: `${origin}/api/auth/google/callback`,
    response_type: "code",
    scope: "openid email profile",
    prompt: "select_account",
  });
  return c.redirect(`https://accounts.google.com/o/oauth2/v2/auth?${params}`);
});

app.get("/api/auth/google/callback", async (c) => {
  const origin = new URL(c.req.url).origin;
  const code = c.req.query("code");
  if (!c.env.GOOGLE_CLIENT_ID || !c.env.GOOGLE_CLIENT_SECRET || !code) {
    return c.redirect("/?error=google_bad_callback");
  }

  const tokenRes = await fetch("https://oauth2.googleapis.com/token", {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      code,
      client_id: c.env.GOOGLE_CLIENT_ID,
      client_secret: c.env.GOOGLE_CLIENT_SECRET,
      redirect_uri: `${origin}/api/auth/google/callback`,
      grant_type: "authorization_code",
    }),
  });
  const { id_token } = (await tokenRes.json()) as { id_token?: string };
  const claims = id_token ? await verifyGoogleToken(id_token, c.env.GOOGLE_CLIENT_ID) : null;
  if (!claims?.email_verified) return c.redirect("/?error=google_invalid_token");

  const id = crypto.randomUUID();
  await c.env.SESSIONS.put(id, claims.email, { expirationTtl: ttl(c.env) });
  c.header(
    "Set-Cookie",
    `session=${id}; Path=/; HttpOnly; SameSite=Lax; Max-Age=${ttl(c.env)}`,
  );
  return c.redirect("/");
});

async function verifyGoogleToken(token: string, clientId: string): Promise<{ email: string; email_verified: boolean } | null> {
  const [h, p, sig] = token.split(".");
  if (!h || !p || !sig) return null;
  const header = JSON.parse(atob(h.replace(/-/g, "+").replace(/_/g, "/"))) as { kid?: string; alg?: string };
  if (header.alg !== "RS256" || !header.kid) return null;

  const certs = (await (await fetch("https://www.googleapis.com/oauth2/v3/certs")).json()) as {
    keys: { kid: string; n: string; e: string }[];
  };
  const key = certs.keys.find((k) => k.kid === header.kid);
  if (!key) return null;

  const cryptoKey = await crypto.subtle.importKey(
    "jwk",
    { kty: "RSA", n: key.n, e: key.e, alg: "RS256" },
    { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
    false,
    ["verify"],
  );
  const valid = await crypto.subtle.verify(
    "RSASSA-PKCS1-v1_5",
    cryptoKey,
    Uint8Array.from(atob(sig.replace(/-/g, "+").replace(/_/g, "/")), (ch) => ch.charCodeAt(0)),
    new TextEncoder().encode(`${h}.${p}`),
  );
  if (!valid) return null;

  const claims = JSON.parse(atob(p.replace(/-/g, "+").replace(/_/g, "/"))) as {
    aud?: string; exp?: number; email?: string; email_verified?: boolean;
  };
  if (claims.aud !== clientId) return null;
  if (!claims.exp || claims.exp * 1000 < Date.now()) return null;
  if (!claims.email) return null;
  return { email: claims.email, email_verified: !!claims.email_verified };
}

// everything else on /api/* proxies to the Rust backend with the session user attached
async function proxy(c: any) {
  const id = sessionId(c);
  const email = id ? await c.env.SESSIONS.get(id) : null;
  if (!email) return c.json({ error: "unauthorized" }, 401);

  const url = new URL(c.req.url);
  const headers = new Headers(c.req.raw.headers);
  headers.set("x-user-id", id);
  headers.set("x-user-email", email);
  headers.set("x-backend-secret", c.env.BACKEND_SECRET);

  const resp = await fetch(c.env.API_ORIGIN + url.pathname + url.search, {
    method: c.req.raw.method,
    headers,
    body: c.req.raw.body,
  });
  return new Response(resp.body, { status: resp.status, headers: resp.headers });
}

app.all("/api/*", proxy);

export default app;