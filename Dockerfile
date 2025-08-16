FROM golang:1.25-alpine AS build
WORKDIR /src
# Copy module manifests first to leverage cache
COPY go.mod go.sum ./
RUN go mod download
COPY server ./server
# Build the binary smaller
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/app ./server

FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=build /out/app /app/app
COPY web /app/web
USER nonroot:nonroot
ENV ADDR=:8080
ENV DATABASE_URL=postgres://postgres:postgres@db:5432/trellolite?sslmode=disable
EXPOSE 8080
ENTRYPOINT ["/app/app"]
