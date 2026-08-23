import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Omnistack",
  description: "Next.js + Rust + Gleam + Python, Cloudflare or AWS",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className="min-h-screen bg-zinc-950 text-zinc-100 antialiased">{children}</body>
    </html>
  );
}
