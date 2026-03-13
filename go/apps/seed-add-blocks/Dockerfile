FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY --from=hivego . /hivego/

COPY . .
RUN go mod edit -replace github.com/vsc-eco/hivego=/hivego && go mod download

RUN go build -o /seed-add-blocks ./apps/seed-add-blocks

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /seed-add-blocks /usr/local/bin/seed-add-blocks

WORKDIR /app

ENTRYPOINT ["seed-add-blocks"]
CMD ["-network", "testnet4"]
