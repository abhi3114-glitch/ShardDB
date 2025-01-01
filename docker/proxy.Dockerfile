FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod ./
# COPY go.sum ./
# RUN go mod download

COPY . .
RUN go build -o /proxy ./cmd/proxy

FROM alpine:latest
WORKDIR /
COPY --from=builder /proxy /proxy
EXPOSE 8080
ENTRYPOINT ["/proxy"]
