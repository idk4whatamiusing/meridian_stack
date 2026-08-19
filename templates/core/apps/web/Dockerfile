# build
FROM oven/bun:1-alpine AS build
WORKDIR /app
COPY package.json bun.lock* ./
COPY apps/web apps/web
COPY packages packages
RUN bun install --frozen-lockfile
RUN bun --cwd apps/web build
# runtime
FROM oven/bun:1-alpine
WORKDIR /app
COPY --from=build /app/apps/web/.next/standalone ./
COPY --from=build /app/apps/web/.next/static ./apps/web/.next/static
EXPOSE 3000
CMD ["node", "apps/web/server.js"]
