# syntax=docker/dockerfile:1
#
# Built from the repository root, so the image can serve the same
# deploy/nginx.conf that production uses rather than a copy that drifts.

# ---------------------------------------------------------------------------
# Build the SPA
# ---------------------------------------------------------------------------
FROM node:22-alpine AS build

WORKDIR /src

# Lockfile first: editing application code should not reinstall dependencies.
COPY apps/web/package.json apps/web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci

COPY apps/web/ ./

# `npm run build` typechecks before bundling, so a type error fails the image
# rather than shipping.
#
# Playwright's browser download is skipped: the e2e suite does not run here.
ENV PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1
RUN npm run build

# ---------------------------------------------------------------------------
# Serve
# ---------------------------------------------------------------------------
FROM nginx:1.27-alpine AS runtime

# conf.d is included from the http block, which is where the upstream and map
# directives in this file need to live.
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/dist /usr/share/nginx/html

EXPOSE 80

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=5 \
    CMD wget --quiet --tries=1 --output-document=/dev/null http://127.0.0.1/ || exit 1
