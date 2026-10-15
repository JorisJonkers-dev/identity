# syntax=docker/dockerfile:1.10
# One image: the Go binary serves the API and embeds the web build (web/embed.go).

FROM --platform=$BUILDPLATFORM node:24-alpine AS web
RUN npm install -g pnpm@12.6.0
WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY openapi/package.json openapi/package.json
COPY web/package.json web/package.json
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile --filter @template-go-vue/web...
COPY openapi openapi
COPY web web
RUN pnpm --filter @template-go-vue/web build

# Cross-compiles on the build platform, so a multi-arch build needs no emulation for the Go stage.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web /src/web/dist web/dist
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/template-go-vue ./cmd/template-go-vue

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/template-go-vue /template-go-vue
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/template-go-vue"]
