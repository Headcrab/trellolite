# syntax=docker/dockerfile:1@sha256:38387523653efa0039f8e1c89bb74a30504e76ee9f565e25c9a09841f9427b05
FROM docker.io/library/golang:1.25-alpine@sha256:77dd832edf2752dafd030693bef196abb24dcba3a2bc3d7a6227a7a1dae73169 AS build
WORKDIR /src
# Copy module manifests first to leverage cache
COPY go.mod go.sum ./
# Use BuildKit caches for modules and build cache
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	go mod download
COPY server ./server
# Build the binary with caches and smaller binary size
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/app ./server

FROM gcr.io/distroless/static:nonroot@sha256:cdf4daaf154e3e27cfffc799c16f343a384228f38646928a1513d925f473cb46
WORKDIR /app
COPY --from=build /out/app /app/app
COPY web /app/web
USER nonroot:nonroot
ENV ADDR=:8080
ENV DATABASE_URL=postgres://postgres:postgres@db:5432/trellolite?sslmode=disable
EXPOSE 8080
ENTRYPOINT ["/app/app"]
