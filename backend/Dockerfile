# Multi-stage build: build the SPA, build the Go binaries, ship a small image.
FROM node:20-alpine AS web
WORKDIR /src
RUN corepack enable
COPY frontend/package.json frontend/pnpm-lock.yaml* frontend/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile || pnpm install
COPY frontend/ ./
RUN VITE_OUT_DIR=dist pnpm build

FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /out/core           ./cmd/core \
 && CGO_ENABLED=0 go build -o /out/mqtt-gateway   ./cmd/mqtt-gateway \
 && CGO_ENABLED=0 go build -o /out/coap-gateway   ./cmd/coap-gateway \
 && CGO_ENABLED=0 go build -o /out/custom-gateway ./cmd/custom-gateway

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/ /app/bin/
COPY --from=web /src/dist /app/web/dist
ENV WEB_DIR=/app/web/dist HTTP_ADDR=0.0.0.0:8080
EXPOSE 8080
ENTRYPOINT ["/app/bin/core"]
