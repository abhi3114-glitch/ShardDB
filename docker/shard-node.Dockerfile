FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod ./
# COPY go.sum ./
# RUN go mod download

COPY . .
RUN go build -o /shard-node ./cmd/shard-node

FROM alpine:latest
WORKDIR /
COPY --from=builder /shard-node /shard-node
EXPOSE 9000
ENTRYPOINT ["/shard-node"]
