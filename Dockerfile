FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS server
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./internal/app/web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/golive ./cmd/golive
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/golive-admin ./cmd/golive-admin

FROM gcr.io/distroless/static-debian12:nonroot AS app-runtime
COPY --from=server /out/golive /golive
EXPOSE 8080 8443
USER nonroot:nonroot
ENTRYPOINT ["/golive"]

FROM postgres:17-alpine AS admin-runtime
COPY --from=server /out/golive-admin /usr/local/bin/golive-admin
ENTRYPOINT ["/usr/local/bin/golive-admin"]
