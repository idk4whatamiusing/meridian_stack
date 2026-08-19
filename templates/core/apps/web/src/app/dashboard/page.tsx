"use client";

import { useEffect, useState } from "react";
import { createClient } from "@meridian-stack/shared";

const api = createClient(process.env.NEXT_PUBLIC_API_URL ?? "");

interface Me {
  id: string;
  email?: string;
}

export default function Dashboard() {
  const [me, setMe] = useState<Me | null>(null);
  const [events, setEvents] = useState<string[]>([]);
  const [users, setUsers] = useState<{ id: string; email: string }[]>([]);
  const [email, setEmail] = useState("");

  useEffect(() => {
    api
      .me()
      .then((d) => setMe(d.user))
      .catch(() => setMe(null));
    const es = api.events();
    es.onmessage = (e) => setEvents((prev) => [e.data, ...prev].slice(0, 50));
    return () => es.close();
  }, []);

  useEffect(() => {
    api
      .graphql<{ users: { id: string; email: string }[] }>("{ users(limit: 5) { id email } }")
      .then((d) => setUsers(d.data?.users ?? []))
      .catch(() => setUsers([]));
  }, []);

  const login = async () => {
    await api.login(email || "dev@example.com");
    location.reload();
  };

  const logout = async () => {
    await api.logout();
    location.reload();
  };

  return (
    <main className="mx-auto flex min-h-screen max-w-3xl flex-col gap-6 p-8">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Dashboard</h1>
        <a href="/" className="text-sm text-zinc-400 hover:underline">home</a>
      </div>

      <section className="rounded-xl border border-zinc-800 p-4">
        <h2 className="mb-3 text-sm font-medium text-zinc-400">Auth (session via gateway / KV or Redis)</h2>
        {me ? (
          <div className="flex items-center justify-between">
            <p>logged in as <span className="font-medium">{me.email ?? me.id}</span></p>
            <button onClick={logout} className="rounded-lg border border-zinc-700 px-3 py-1 text-sm hover:bg-zinc-900">
              logout
            </button>
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            <div className="flex gap-2">
              <input
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="dev@example.com"
                className="flex-1 rounded-lg border border-zinc-700 bg-zinc-900 px-3 py-1 text-sm outline-none focus:border-emerald-500"
              />
              <button onClick={login} className="rounded-lg bg-emerald-600 px-3 py-1 text-sm hover:bg-emerald-500">
                dev login
              </button>
            </div>
            <a
              href={`${process.env.NEXT_PUBLIC_API_URL ?? ""}/api/auth/google`}
              className="rounded-lg border border-zinc-700 bg-zinc-900 px-3 py-1 text-center text-sm hover:bg-zinc-800"
            >
              continue with google
            </a>
          </div>
        )}
      </section>

      <section className="rounded-xl border border-zinc-800 p-4">
        <h2 className="mb-3 text-sm font-medium text-zinc-400">Live events (SSE)</h2>
        {events.length === 0 ? (
          <p className="text-sm text-zinc-500">waiting - try <code>curl -X POST {process.env.NEXT_PUBLIC_API_URL ?? ""}/api/broadcast -H &quot;x-backend-secret: change-me&quot; -H &quot;Content-Type: application/json&quot; -d &#123;&quot;message&quot;:&quot;hi&quot;&#125;</code></p>
        ) : (
          <ul className="space-y-1 font-mono text-xs">
            {events.map((e, i) => (
              <li key={i} className="text-emerald-400">{e}</li>
            ))}
          </ul>
        )}
      </section>

      <section className="rounded-xl border border-zinc-800 p-4">
        <h2 className="mb-3 text-sm font-medium text-zinc-400">
          GraphQL demo{" "}
          <a href={`${process.env.NEXT_PUBLIC_API_URL ?? ""}/api/graphql`} className="text-emerald-500 hover:underline">(open GraphiQL)</a>
        </h2>
        <pre className="text-xs text-zinc-400">{`query { users(limit: 5) { id email } }`}</pre>
        <ul className="mt-2 space-y-1 font-mono text-xs">
          {users.length === 0 ? (
            <li className="text-zinc-500">no users yet - login to create one</li>
          ) : (
            users.map((u) => (
              <li key={u.id} className="text-zinc-300">{u.email}</li>
            ))
          )}
        </ul>
      </section>
    </main>
  );
}
