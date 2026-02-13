FROM golang:1.23-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /auth-service .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /auth-service /auth-service
ENTRYPOINT ["/auth-service"]
