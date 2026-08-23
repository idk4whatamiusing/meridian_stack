export default function Home() {
  return (
    <main className="mx-auto flex min-h-screen max-w-3xl flex-col justify-center gap-6 p-8">
      <h1 className="text-4xl font-bold">Omnistack</h1>
      <p className="text-zinc-400">
        Next.js SSR frontend, Rust (axum) API with SSE + WebSocket, Gleam realtime, Python AI service.
        Auth at the edge: Cloudflare Workers + KV or Redis on AWS.
      </p>
      <ul className="grid gap-2 text-sm text-zinc-400">
        <li><code className="text-emerald-400">ui</code> Next.js :3000</li>
        <li><code className="text-emerald-400">api</code> Go :8000 - GraphQL + OAuth; <code>db</code> Rust tonic :8010 - Postgres gatekeeper</li>
        <li><code className="text-emerald-400">realtime</code> Gleam :8001 - events fanout + Redis pub/sub</li>
        <li><code className="text-emerald-400">ai</code> hybrid Go+Python :8002 - RAG, providers, local support LLM</li>
      </ul>
      <a href="/dashboard" className="w-fit rounded-lg bg-emerald-600 px-4 py-2 font-medium hover:bg-emerald-500">
        Dashboard
      </a>
    </main>
  );
}
