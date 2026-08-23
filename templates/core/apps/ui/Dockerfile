# build - context must be the monorepo root: bun's frozen lockfile resolves
# every workspace member (apps/*, packages/*) and the build reads the root
# package.json/bun.lock
FROM oven/bun:1-alpine AS build
WORKDIR /app
COPY package.json bun.lock* ./
COPY . .
RUN bun install --frozen-lockfile
RUN bun --cwd apps/ui build
# runtime
FROM oven/bun:1-alpine
WORKDIR /app
COPY --from=build /app/apps/ui/.next/standalone ./
COPY --from=build /app/apps/ui/.next/static ./apps/ui/.next/static
EXPOSE 3000
CMD ["node", "apps/ui/server.js"]
