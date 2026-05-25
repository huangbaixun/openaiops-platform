# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/gateway  ./cmd/gateway
RUN CGO_ENABLED=0 go build -o /out/query    ./cmd/query
RUN CGO_ENABLED=0 go build -o /out/ingester ./cmd/ingester

FROM alpine:3.19
RUN apk add --no-cache ca-certificates wget
COPY --from=build /out/gateway  /usr/local/bin/gateway
COPY --from=build /out/query    /usr/local/bin/query
COPY --from=build /out/ingester /usr/local/bin/ingester
EXPOSE 8080 8081 4317 4318 8082
# Default entrypoint kept as gateway for backward compat; compose services override.
ENTRYPOINT ["/usr/local/bin/gateway"]
