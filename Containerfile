# Self-contained, multi-arch image for pascha. Built and pushed to
# ghcr.io/saidtezel/pascha by .github/workflows/image.yml.
#
# The builder stage runs on the host's native platform and
# cross-compiles for TARGETOS/TARGETARCH (set by buildx), so no
# emulation is needed to produce the arm64 image.
#
# Build locally:
#   docker build -f Containerfile -t pascha .
#
# Run:
#   docker run --rm \
#     -e FAMLY_EMAIL -e FAMLY_PASSWORD \
#     -e IMMICH_BASE_URL -e IMMICH_API_KEY \
#     ghcr.io/saidtezel/pascha:latest fetch --max-pages 1

FROM --platform=$BUILDPLATFORM golang:1.26 AS build

ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /out/pascha ./cmd/pascha

# distroless/static carries CA certificates, tzdata and a writable
# /tmp, which is all a static Go binary talking HTTPS needs.
FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev

COPY --from=build /out/pascha /pascha

# Stateless: no volume. Media is staged in /tmp per asset and goes
# straight to Immich; Immich is the record of what is uploaded.
ENV PASCHA_LOG_FORMAT=json

ENTRYPOINT ["/pascha"]
CMD ["fetch"]

LABEL org.opencontainers.image.source="https://github.com/saidtezel/pascha" \
      org.opencontainers.image.description="pascha -- Famly-to-Immich photo archiver for households (Go CLI)" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"
