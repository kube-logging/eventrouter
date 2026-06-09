FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine3.22@sha256:727cfc3c40be55cd1bc9a4a059406b28a059857e3be752aa9d09531e12c20c56 AS builder

RUN apk add --update --no-cache ca-certificates

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

ARG GOPROXY

COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY cmd/ /app/cmd/
COPY sinks/ /app/sinks/

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o eventrouter ./cmd

FROM gcr.io/distroless/static:nonroot@sha256:963fa6c544fe5ce420f1f54fb88b6fb01479f054c8056d0f74cc2c6000df5240

COPY --from=builder /app/eventrouter /app/eventrouter

CMD ["/app/eventrouter"]
