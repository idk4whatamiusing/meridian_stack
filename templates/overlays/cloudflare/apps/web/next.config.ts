import type { NextConfig } from "next";

// Cloudflare flavor: the gateway worker serves the static export (Cartis pattern).
// For SSR on Cloudflare, swap to @opennextjs/cloudflare (v2).
const nextConfig: NextConfig = {
  output: "export",
  images: { unoptimized: true },
};

export default nextConfig;