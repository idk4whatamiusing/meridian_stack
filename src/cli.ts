#!/usr/bin/env node
import { cpSync, existsSync, mkdirSync, readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { execSync } from "node:child_process";
import { dirname, isAbsolute, join } from "node:path";
import { fileURLToPath } from "node:url";
import readline from "node:readline/promises";
import { stdin as input, stdout as output } from "node:process";

const here = dirname(fileURLToPath(import.meta.url));
const templates = join(here, "..", "templates");

const skipDirs = new Set(["node_modules", "target", ".git", ".next", "out", ".venv", "__pycache__", "build", ".gleam"]);

// cpSync merges dirs without overwriting existing files - copy per-file instead
function copyTree(src: string, dest: string) {
  mkdirSync(dest, { recursive: true });
  for (const entry of readdirSync(src, { withFileTypes: true })) {
    if (skipDirs.has(entry.name)) continue;
    const s = join(src, entry.name);
    const d = join(dest, entry.name);
    if (entry.isDirectory()) copyTree(s, d);
    else writeFileSync(d, readFileSync(s));
  }
}

const args = process.argv.slice(2);
if (args.includes("--help") || args.includes("-h")) {
  console.log(`usage: create-meridian-stack [name] [variant] [options]

variants: cloudflare, aws, both (default: both)

options:
  --name <name>            project directory name
  --variant <variant>      cloudflare | aws | both
  -y, --yes               skip prompts (uses defaults: my-app / both)
  -h, --help              show this help`);
  process.exit(0);
}

const flags = new Map<string, string>();
const positional: string[] = [];
for (let i = 0; i < args.length; i++) {
  const a = args[i];
  if (a === "--name") flags.set("name", args[++i] ?? "");
  else if (a === "--variant") flags.set("variant", args[++i] ?? "");
  else if (a === "-y" || a === "--yes") flags.set("yes", "1");
  else positional.push(a);
}

const rl = readline.createInterface({ input, output });
const name =
  (flags.get("name") ?? positional[0] ?? (flags.has("yes") ? "my-app" : (await rl.question("Project name: ")).trim())) || "my-app";
let variant =
  (flags.get("variant") ?? positional[1] ?? (flags.has("yes") ? "both" : (await rl.question("Variant [cloudflare/aws/both] (default: both): ")).trim()))
    .trim()
    .toLowerCase();
rl.close();
if (!variant) variant = "both";

if (!["cloudflare", "aws", "both"].includes(variant)) {
  console.error("variant must be one of: cloudflare, aws, both");
  process.exit(1);
}

const dest = isAbsolute(name) ? name : join(process.cwd(), name);
if (existsSync(dest)) {
  console.error(`${dest} already exists`);
  process.exit(1);
}

console.log(`scaffolding ${name} (${variant}) into ${dest}...`);
copyTree(join(templates, "core"), dest);
if (variant === "both") {
  // layering: cloudflare + aws, then the both overlay resolves the few conflicts
  copyTree(join(templates, "overlays", "cloudflare"), dest);
  copyTree(join(templates, "overlays", "aws"), dest);
  copyTree(join(templates, "overlays", "both"), dest);
} else {
  copyTree(join(templates, "overlays", variant), dest);
}

const walk = (dir: string, fn: (f: string) => void) => {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      if (!skipDirs.has(entry)) walk(full, fn);
    } else {
      fn(full);
    }
  }
}

walk(dest, (f) => {
  const src = readFileSync(f, "utf8");
  if (src.includes("{{name}}")) writeFileSync(f, src.replaceAll("{{name}}", name));
});

execSync("git init -q", { cwd: dest, stdio: "ignore" });

try {
  const bunHome = join(process.env.HOME ?? "", ".bun", "bin");
  execSync("bun install", { cwd: dest, stdio: "inherit", env: { ...process.env, PATH: `${bunHome}:${process.env.PATH ?? ""}` } });
} catch {
  console.log("bun install failed or bun is missing - run `bun install` inside the project");
}

console.log(`
done! next steps:
  cd ${name}
  docker compose up -d            # postgres + redis
  bun run dev:web                 # Next.js on :3000 (terminal 1)
  bun run dev:api                 # Rust API on :8000 (terminal 2)
  bun run dev:realtime            # Gleam on :8001 (terminal 3)
  cd apps/ai && uv run --with-requirements requirements.txt uvicorn main:app --port 8002
  open http://localhost:3000/dashboard
deploy: see DEPLOY.md (${variant === "both" ? "cloudflare + aws" : variant} flavor) in the project root`);
