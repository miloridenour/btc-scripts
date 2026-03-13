FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY . .
RUN go build -o /lite-btc-oracle ./apps/lite-btc-oracle

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /lite-btc-oracle /usr/local/bin/lite-btc-oracle

WORKDIR /app

ENTRYPOINT ["lite-btc-oracle"]
CMD ["-network", "testnet4"]
