# syntax=docker/dockerfile:1

# 1. Build the web UI.
FROM node:22-alpine AS ui
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. Build the Go binary with the UI embedded.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/findDevClean ./cmd/findDevClean

# 3. Minimal runtime: no shell, no package manager.
FROM gcr.io/distroless/static-debian12:nonroot
ENV HOME=/home/nonroot
COPY --from=build /out/findDevClean /findDevClean
# Listen on all interfaces inside the container; the host publishes only on
# 127.0.0.1 (see docker-compose.yml). The token is still required.
# To publish a different host port, override CMD with both flags, e.g.:
#   docker run -p 127.0.0.1:7788:7777 ... finddevclean --addr 0.0.0.0:7777 --public-port 7788
# (ENTRYPOINT already runs "serve", so pass only the flags.)
EXPOSE 7777
ENTRYPOINT ["/findDevClean", "serve"]
CMD ["--addr", "0.0.0.0:7777"]
