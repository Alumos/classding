# syntax=docker/dockerfile:1
FROM node:24-alpine AS web
WORKDIR /src
RUN npm install -g pnpm@11.25.0
COPY . .
RUN pnpm install --frozen-lockfile && pnpm check:toolchain
RUN pnpm --filter @classding/teacher build && pnpm --filter @classding/admin build
RUN node scripts/prepare-static.mjs

FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS build
ARG TARGETOS=linux
ARG TARGETARCH=amd64
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
COPY --from=web /src/server/static/ ./static/
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/classding ./cmd/classding
RUN mkdir -p /data/audio && chmod 0700 /data

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/classding /classding
COPY --from=build --chown=65532:65532 /data /data
ENV DATA_DIR=/data LISTEN_ADDR=:8080 TZ=Asia/Shanghai
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/classding"]
