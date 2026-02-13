FROM golang:1.23-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /demo-client .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /demo-client /demo-client
ENTRYPOINT ["/demo-client"]
