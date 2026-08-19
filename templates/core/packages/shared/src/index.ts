export interface SessionUser {
  id: string;
  email: string;
}

export interface HealthResponse {
  status: string;
  db: string;
  cache: string;
}

export interface BroadcastMessage {
  message: string;
  at: string;
}

export interface GraphqlResult<T> {
  data: T;
  errors?: { message: string }[];
}

const json = (r: Response) => (r.ok ? r.json() : Promise.reject(new Error(`${r.status} ${r.statusText}`)));

export function createClient(baseURL: string) {
  const base = baseURL.replace(/\/$/, "");
  const post = <T,>(path: string, body?: unknown, headers: Record<string, string> = {}) =>
    fetch(`${base}${path}`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", ...headers },
      body: body === undefined ? undefined : JSON.stringify(body),
    }).then(json) as Promise<T>;

  return {
    me(): Promise<{ user: SessionUser | null }> {
      return fetch(`${base}/api/me`, { credentials: "include" }).then(json);
    },
    health: () => fetch(`${base}/api/health`).then(json) as Promise<HealthResponse>,
    login: (email: string) => post("/api/auth/dev-login", { email }) as Promise<{ ok: true; user: SessionUser }>,
    logout: () => post("/api/auth/logout") as Promise<{ ok: true }>,
    broadcast: (message: string, secret: string) =>
      post("/api/broadcast", { message }, { "x-backend-secret": secret }) as Promise<{ ok: true }>,
    graphql: <T,>(query: string) => post("/api/graphql", { query }) as Promise<GraphqlResult<T>>,
    events: () => new EventSource(`${base}/api/events`, { withCredentials: true }),
  };
}

export type ApiClient = ReturnType<typeof createClient>;