# Thin runtime layer for pascha. Pairs with the v2.0.0 catalog's
# container-image template in `from_artifacts: true` mode: the
# go-release-binary job builds + cosign-signs the binary, this
# Containerfile COPYs it onto ci-runtime-go (UBI micro). The
# binary verifiable via `cosign verify-blob` is byte-for-byte the
# binary inside the container.
#
# No Go toolchain stage. No recompile. Provable parity.
#
# Multi-arch: TARGETARCH is set automatically by the catalog
# container-image template when multi_arch: true. Defaults to amd64
# for single-arch builds. arm64 binaries land in dist/ via
# go-release-binary's parallel:matrix.
#
# Build (via catalog, from a tag pipeline):
#   automatic — go-release-binary -> container-image (from_artifacts)
#
# Run:
#   docker run --rm \
#     -e FAMLY_EMAIL -e FAMLY_PASSWORD \
#     -e IMMICH_BASE_URL -e IMMICH_API_KEY \
#     registry.gitlab.com/dunn.dev/pascha/cli:latest fetch --max-pages 1

FROM registry.gitlab.com/dunn.dev/pipeline/ci-runtime-go:4.12.0@sha256:b4ba9539e96ca60a259c0b388646072d009eba369ceedf0c9f37b6ce2b6765fe

ARG VERSION=dev
ARG TARGETARCH=amd64

COPY dist/pascha-linux-${TARGETARCH} /pascha

# Stateless: no volume. Media is staged in /tmp per asset and goes
# straight to Immich; Immich is the record of what is uploaded.
ENV PASCHA_LOG_FORMAT=json

ENTRYPOINT ["/pascha"]
CMD ["fetch"]

LABEL org.opencontainers.image.source="https://github.com/saidtezel/pascha" \
      org.opencontainers.image.description="pascha -- Famly-to-Immich photo archiver for households (Go CLI)" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"
