# build - context must be the monorepo root: npm workspaces resolve
# every workspace member (ui, packages) and the build reads the root
# package.json/package-lock.json
FROM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
COPY ui/package.json ui/
COPY packages/shared/package.json packages/shared/
RUN npm ci --include-workspace-root
COPY . .
RUN npm run build --workspace ui
# runtime
FROM node:24-alpine
WORKDIR /app
COPY --from=build /app/ui/.next/standalone ./
COPY --from=build /app/ui/.next/static ./ui/.next/static
EXPOSE 3000
CMD ["node", "ui/server.js"]
