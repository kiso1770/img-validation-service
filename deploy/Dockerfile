FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/img-validation-service ./cmd/main.go

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /bin/img-validation-service /img-validation-service

RUN adduser -D -u 65532 svc
USER svc

EXPOSE 8080 9090

ENTRYPOINT ["/img-validation-service"]
