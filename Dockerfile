FROM golang:1.23 AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/gateway ./cmd/gateway
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/fake-vllm ./cmd/fake-vllm

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/gateway /gateway
COPY --from=builder /out/fake-vllm /fake-vllm
EXPOSE 8080
ENTRYPOINT ["/gateway"]
