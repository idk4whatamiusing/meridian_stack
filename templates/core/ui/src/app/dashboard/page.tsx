"use client";

// Dashboard: auth card, live events subscription, chat + support box.
import { useEffect, useRef, useState } from "react";
import { createClient, ME, type User } from "../../lib/gqlClient";

const api = createClient(process.env.NEXT_PUBLIC_API_URL ?? "");

export default function Dashboard() {
  const [me, setMe] = useState<User | null>(null);
  const [events, setEvents] = useState<string[]>([]);
  const [email, setEmail] = useState("");
  const [messages, setMessages] = useState<{ role: string; content: string }[]>([]);
  const [input, setInput] = useState("");
  const [streaming, setStreaming] = useState(false);
  const stopRef = useRef<(() => void) | null>(null);

  useEffect(() => {
    api
      .graphql<{ me: User | null }>(ME)
      .then((d) => setMe(d.me))
      .catch(() => setMe(null));
    // events via graphql-transport-ws (raw client)
    return api.subscribe<{ events: string }>(
      "subscription { events }",
      undefined,
      (d) => setEvents((prev) => [d.events, ...prev].slice(0, 50)),
    );
  }, []);

  const login = async () => {
    await api.graphql(`mutation ($email: String!) { login(email: $email) { id email } }`, {
      email: email || "dev@example.com",
    });
    location.reload();
  };

  const logout = async () => {
    await api.graphql("mutation { logout }");
    location.reload();
  };

  const send = async () => {
    if (!input.trim() || streaming) return;
    const message = input.trim();
    setMessages((m) => [...m, { role: "user", content: message }]);
    setInput("");
    setStreaming(true);
    let assistant = "";
    setMessages((m) => [...m, { role: "assistant", content: "" }]);
    stopRef.current = api.subscribe<{ chatStream: { delta: string; done: boolean } }>(
      "subscription ($message: String!) { chatStream(message: $message) { delta done } }",
      { message },
      (d) => {
        assistant += d.chatStream.delta;
        setMessages((m) => {
          const copy = [...m];
          copy[copy.length - 1] = { role: "assistant", content: assistant };
          return copy;
        });
        if (d.chatStream.done) setStreaming(false);
      },
      () => setStreaming(false),
    );
  };

  const askSupport = async () => {
    if (!input.trim() || streaming) return;
    const message = input.trim();
    setMessages((m) => [...m, { role: "user", content: `[support] ${message}` }, { role: "assistant", content: "" }]);
    setInput("");
    setStreaming(true);
    try {
      const d = await api.graphql<{ supportQuery: { reply: string } }>(
        `mutation ($message: String!) { supportQuery(message: $message) { reply sources { text score } } }`,
        { message },
      );
      setMessages((m) => {
        const copy = [...m];
        copy[copy.length - 1] = { role: "assistant", content: d.supportQuery.reply };
        return copy;
      });
    } catch (e) {
      setMessages((m) => {
        const copy = [...m];
        copy[copy.length - 1] = { role: "assistant", content: `support error: ${(e as Error).message}` };
        return copy;
      });
    }
    setStreaming(false);
  };

  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-6 p-8">
      <h1 className="text-2xl font-bold">Dashboard</h1>

      <section className="rounded-xl border border-zinc-800 p-4">
        <h2 className="mb-3 text-sm font-medium text-zinc-400">Auth</h2>
        {me ? (
          <div className="flex items-center justify-between">
            <p>
              logged in as <span className="font-medium">{me.email}</span>
            </p>
            <button onClick={logout} className="rounded-lg border border-zinc-700 px-3 py-1 text-sm hover:bg-zinc-900">
              logout
            </button>
          </div>
        ) : (
          <div className="flex gap-2">
            <input
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="dev@example.com"
              className="flex-1 rounded-lg border border-zinc-700 bg-zinc-900 px-3 py-1 text-sm outline-none focus:border-emerald-500"
            />
            <button onClick={login} className="rounded-lg bg-emerald-600 px-3 py-1 text-sm hover:bg-emerald-500">
              login
            </button>
            <a
              href={`${process.env.NEXT_PUBLIC_API_URL ?? ""}/api/auth/google`}
              className="rounded-lg border border-zinc-700 bg-zinc-900 px-3 py-1 text-center text-sm hover:bg-zinc-800"
            >
              google
            </a>
          </div>
        )}
      </section>

      <section className="rounded-xl border border-zinc-800 p-4">
        <h2 className="mb-3 text-sm font-medium text-zinc-400">Chat</h2>
        <div className="mb-3 space-y-2">
          {messages.length === 0 && <p className="text-sm text-zinc-600">Say something…</p>}
          {messages.map((m, i) => (
            <p key={i} className={m.role === "user" ? "text-right text-sm text-emerald-300" : "text-sm"}>
              {m.content || "…"}
            </p>
          ))}
        </div>
        <div className="flex gap-2">
          <input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && send()}
            placeholder="Message"
            className="flex-1 rounded-lg border border-zinc-700 bg-zinc-900 px-3 py-1.5 text-sm outline-none focus:border-emerald-500"
          />
          <button onClick={send} disabled={streaming} className="rounded-lg bg-emerald-600 px-3 py-1.5 text-sm hover:bg-emerald-500 disabled:opacity-50">
            send
          </button>
          <button onClick={askSupport} disabled={streaming} className="rounded-lg border border-zinc-700 px-3 py-1.5 text-sm hover:bg-zinc-900 disabled:opacity-50">
            support
          </button>
        </div>
      </section>

      <section className="rounded-xl border border-zinc-800 p-4">
        <h2 className="mb-2 text-sm font-medium text-zinc-400">Live events (subscription)</h2>
        {events.length === 0 ? (
          <p className="text-xs text-zinc-600">waiting for broadcasts…</p>
        ) : (
          <ul className="space-y-1 font-mono text-xs">
            {events.map((e, i) => (
              <li key={i} className="text-emerald-400">{e}</li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
