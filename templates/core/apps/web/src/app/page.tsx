export default function Home() {
  return (
    <main className="mx-auto flex min-h-screen max-w-3xl flex-col justify-center gap-6 p-8">
      <h1 className="text-4xl font-bold">Omnistack</h1>
      <p className="text-zinc-400">
        Next.js SSR frontend, Rust (axum) API with SSE + WebSocket, Gleam realtime, Python AI service.
        Auth at the edge: Cloudflare Workers + KV or Redis on AWS.
      </p>
      <ul className="grid gap-2 text-sm text-zinc-400">
        <li><code className="text-emerald-400">apps/web</code> Next.js :3000</li>
        <li><code className="text-emerald-400">apps/api</code> Rust axum :8000 - /api/health, /api/me, /api/events (SSE), /api/ws, cache</li>
        <li><code className="text-emerald-400">apps/realtime</code> Gleam :8001 - /events + /ws fanout</li>
        <li><code className="text-emerald-400">apps/ai</code> Python FastAPI :8002 - predict/train stubs</li>
      </ul>
      <a href="/dashboard" className="w-fit rounded-lg bg-emerald-600 px-4 py-2 font-medium hover:bg-emerald-500">
        Dashboard
      </a>
    </main>
  );
}
