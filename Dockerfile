# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY server ./server
COPY web ./web
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /out/app ./server

FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=build /out/app /app/app
COPY web /app/web
USER nonroot:nonroot
ENV ADDR=:8080
ENV DATABASE_URL=postgres://postgres:postgres@db:5432/trellolite?sslmode=disable
EXPOSE 8080
ENTRYPOINT ["/app/app"]
