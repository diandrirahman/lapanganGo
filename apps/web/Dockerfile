FROM node:22.14-alpine AS builder

WORKDIR /app

# Copy root level package.json if it exists, or just apps/web package.json
# Assuming standalone web app here, if monorepo with root package.json, might need adjustment.
COPY apps/web/package*.json ./apps/web/

WORKDIR /app/apps/web
RUN npm install

WORKDIR /app
COPY apps/web/ ./apps/web/

WORKDIR /app/apps/web
# Set VITE_API_BASE_URL to /api for nginx proxy
ENV VITE_API_BASE_URL=/api

ARG VITE_PLATFORM_FINANCE_ADMIN_ENABLED=false
ENV VITE_PLATFORM_FINANCE_ADMIN_ENABLED=${VITE_PLATFORM_FINANCE_ADMIN_ENABLED}

RUN npm run build

# Production image
FROM nginx:1.27-alpine
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/apps/web/dist /usr/share/nginx/html

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
