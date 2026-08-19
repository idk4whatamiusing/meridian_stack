# create-meridian-stack

Scaffold a [Meridian Stack](https://github.com/idk4whatamiusing/meridian_stack) monorepo:
Next.js 16 + Rust (axum) API + Gleam realtime + Python AI service, deployable on
Cloudflare, AWS (Caddy + Docker + Terraform), or both.

## Usage

    npm create meridian-stack@latest -- --name my-app --variant both
    npm create meridian-stack@latest cloudflare      # positional args also work
    npm create meridian-stack@latest -- --help

Flags: `--name <dir>`, `--variant <cloudflare|aws|both>`, `-y/--yes` (skip prompts), `-h/--help`.
Prompts appear when flags are omitted.

## Development

    node src/cli.ts --help        # CLI is plain Node >= 23 (type stripping), no build step
    npm pack                      # test the published tarball

## CI

`.github/workflows/ci.yml` scaffolds a full project with `--yes` and builds the
api, realtime, and web workspaces of the generated output.

## Release

    npm version patch && npm publish