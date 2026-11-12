# Node version matches CI.
FROM node:26.13.1-alpine3.23 AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
RUN npm run build

# Serves the build and proxies /v1 to the API, so the browser talks to one origin.
FROM nginxinc/nginx-unprivileged:1.30.3-alpine3.23
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/dist /usr/share/nginx/html
EXPOSE 8080
