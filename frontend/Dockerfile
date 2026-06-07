# syntax=docker/dockerfile:1.7

ARG NODE_VERSION=22-alpine
ARG NPM_CONFIG_REGISTRY=https://registry.npmjs.org/

FROM node:${NODE_VERSION} AS build

ARG NPM_CONFIG_REGISTRY
ENV npm_config_registry=${NPM_CONFIG_REGISTRY}

WORKDIR /app/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend ./
RUN npm run build

FROM nginx:1.27-alpine

COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/frontend/dist /usr/share/nginx/html

EXPOSE 80
