FROM golang:1.26.4 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build \
    -mod=readonly \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/trip-service \
    ./cmd/trip-service

FROM gcr.io/distroless/static-debian13:nonroot

WORKDIR /app

COPY --from=builder /out/trip-service /app/trip-service

USER nonroot:nonroot

ENTRYPOINT ["/app/trip-service"]