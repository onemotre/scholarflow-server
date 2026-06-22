FROM golang:1.26.4-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/server ./cmd/server
RUN go build -o /out/worker ./cmd/worker

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build /out/worker /app/worker
