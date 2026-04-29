FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /build/hombox ./cmd/server

FROM alpine:3.21

RUN apk add --no-cache curl ca-certificates
COPY --from=builder /build/hombox /usr/local/bin/hombox

EXPOSE 8080
ENTRYPOINT ["hombox"]
