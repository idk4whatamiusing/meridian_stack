export default function Pricing() {
  const tiers = [
    {
      name: "Free",
      price: "$0",
      features: ["1 project", "Community support", "All core services"],
      cta: "Start free",
    },
    {
      name: "Pro",
      price: "$20/mo",
      features: ["Unlimited projects", "Priority support", "RAG + AI chat", "Custom domains"],
      cta: "Go Pro",
      highlight: true,
    },
    {
      name: "Team",
      price: "Contact us",
      features: ["Everything in Pro", "SSO / Google OAuth", "Dedicated support LLM", "SLA"],
      cta: "Talk to us",
    },
  ];

  return (
    <main className="mx-auto max-w-4xl p-10">
      <h1 className="mb-2 text-3xl font-bold">Pricing</h1>
      <p className="mb-8 text-zinc-400">Simple plans. Cancel anytime.</p>
      <div className="grid gap-6 md:grid-cols-3">
        {tiers.map((t) => (
          <div
            key={t.name}
            className={`rounded-2xl border p-6 ${
              t.highlight ? "border-emerald-600 bg-emerald-950/30" : "border-zinc-800"
            }`}
          >
            <h2 className="text-lg font-semibold">{t.name}</h2>
            <p className="my-3 text-3xl font-bold">{t.price}</p>
            <ul className="mb-6 space-y-2 text-sm text-zinc-400">
              {t.features.map((f) => (
                <li key={f}>✓ {f}</li>
              ))}
            </ul>
            <a
              href="/dashboard"
              className={`block rounded-lg px-4 py-2 text-center text-sm ${
                t.highlight ? "bg-emerald-600 hover:bg-emerald-500" : "border border-zinc-700 hover:bg-zinc-900"
              }`}
            >
              {t.cta}
            </a>
          </div>
        ))}
      </div>
    </main>
  );
}
