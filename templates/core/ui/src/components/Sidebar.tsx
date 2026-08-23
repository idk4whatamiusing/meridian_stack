"use client";

// ChatGPT-style sidebar: new chat, search, grouped sessions, hover actions.
import { useEffect, useMemo, useState } from "react";
import { createClient, CHAT_SESSIONS, type ChatSession } from "../lib/gqlClient";

const api = createClient(process.env.NEXT_PUBLIC_API_URL ?? "");

function groupOf(updatedAt: string): string {
  const d = new Date(updatedAt);
  const now = Date.now();
  const day = 86400000;
  if (now - d.getTime() < day) return "Today";
  if (now - d.getTime() < 2 * day) return "Yesterday";
  if (now - d.getTime() < 7 * day) return "Previous 7 days";
  return "Older";
}

export default function Sidebar({
  activeId,
  onNavigate,
}: {
  activeId?: string;
  onNavigate?: (id: string) => void;
}) {
  const [sessions, setSessions] = useState<ChatSession[]>([]);
  const [search, setSearch] = useState("");
  const [collapsed, setCollapsed] = useState(false);

  useEffect(() => {
    api
      .graphql<{ chatSessions: ChatSession[] }>(CHAT_SESSIONS)
      .then((d) => setSessions(d.chatSessions ?? []))
      .catch(() => setSessions([]));
  }, [activeId]);

  const groups = useMemo(() => {
    const filtered = sessions.filter((s) =>
      s.title.toLowerCase().includes(search.toLowerCase()),
    );
    const out: Record<string, ChatSession[]> = {};
    for (const s of filtered) {
      const g = groupOf(s.updatedAt);
      (out[g] ??= []).push(s);
    }
    return out;
  }, [sessions, search]);

  const newChat = async () => {
    try {
      const d = await api.graphql<{
        createChatSession: ChatSession;
      }>(`mutation ($title: String) { createChatSession(title: $title) { id title updatedAt } }`, {
        title: null,
      });
      setSessions((prev) => [d.createChatSession, ...prev]);
      onNavigate?.(d.createChatSession.id);
    } catch {
      /* not logged in */
    }
  };

  const rename = async (s: ChatSession) => {
    const title = prompt("Rename chat", s.title);
    if (!title) return;
    await api.graphql(`mutation ($id: ID!, $title: String!) { renameSession(id: $id, title: $title) }`, {
      id: s.id,
      title,
    });
    setSessions((prev) => prev.map((x) => (x.id === s.id ? { ...x, title } : x)));
  };

  const remove = async (s: ChatSession) => {
    await api.graphql(`mutation ($id: ID!) { deleteSession(id: $id) }`, { id: s.id });
    setSessions((prev) => prev.filter((x) => x.id !== s.id));
  };

  if (collapsed) {
    return (
      <div className="flex w-12 flex-col items-center gap-3 border-r border-zinc-800 bg-zinc-950 p-2">
        <button onClick={() => setCollapsed(false)} aria-label="expand" className="text-zinc-400 hover:text-zinc-100">»</button>
        <button onClick={newChat} className="rounded-md bg-emerald-600 px-2 py-1 text-xs text-white">+</button>
      </div>
    );
  }

  return (
    <aside className="flex w-64 shrink-0 flex-col border-r border-zinc-800 bg-zinc-950">
      <div className="flex items-center justify-between p-3">
        <span className="text-sm font-semibold text-zinc-200">Meridian</span>
        <button onClick={() => setCollapsed(true)} aria-label="collapse" className="text-zinc-500 hover:text-zinc-200">«</button>
      </div>

      <div className="px-3 pb-2">
        <button
          onClick={newChat}
          className="w-full rounded-lg border border-zinc-700 px-3 py-1.5 text-left text-sm text-zinc-200 hover:bg-zinc-900"
        >
          + New chat
        </button>
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search chats"
          className="mt-2 w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-1.5 text-sm outline-none focus:border-emerald-600"
        />
      </div>

      <div className="flex-1 overflow-y-auto px-2">
        {Object.entries(groups).map(([group, items]) => (
          <div key={group} className="mb-3">
            <p className="px-2 py-1 text-[11px] uppercase tracking-wide text-zinc-500">{group}</p>
            {items.map((s) => (
              <div
                key={s.id}
                className={`group flex cursor-pointer items-center justify-between rounded-lg px-2 py-1.5 text-sm ${
                  s.id === activeId ? "bg-zinc-800 text-zinc-100" : "text-zinc-300 hover:bg-zinc-900"
                }`}
                onClick={() => onNavigate?.(s.id)}
              >
                <span className="truncate">{s.title}</span>
                <span className="hidden gap-1 text-zinc-500 group-hover:flex">
                  <button
                    onClick={(e) => { e.stopPropagation(); rename(s); }}
                    className="hover:text-zinc-200"
                    aria-label="rename"
                  >✎</button>
                  <button
                    onClick={(e) => { e.stopPropagation(); remove(s); }}
                    className="hover:text-red-400"
                    aria-label="delete"
                  >🗑</button>
                </span>
              </div>
            ))}
          </div>
        ))}
        {sessions.length === 0 && (
          <p className="px-2 py-4 text-xs text-zinc-600">No chats yet — log in and start one.</p>
        )}
      </div>

      <div className="border-t border-zinc-800 p-3 text-xs text-zinc-400">
        <a href="/pricing" className="block py-1 hover:text-zinc-100">Pricing</a>
        <a href="/" className="block py-1 hover:text-zinc-100">Home</a>
      </div>
    </aside>
  );
}
