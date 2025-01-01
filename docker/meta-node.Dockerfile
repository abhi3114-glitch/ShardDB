FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod ./
# COPY go.sum ./
# RUN go mod download

COPY . .
RUN go build -o /meta-node ./cmd/meta-node

FROM alpine:latest
WORKDIR /
COPY --from=builder /meta-node /meta-node
EXPOSE 9000
ENTRYPOINT ["/meta-node"]
