# syntax=docker/dockerfile:1@sha256:87999aa3d42bdc6bea60565083ee17e86d1f3339802f543c0d03998580f9cb89

FROM golang:1.26@sha256:792443b89f65105abba56b9bd5e97f680a80074ac62fc844a584212f8c8102c3 AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION="dev"

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
    -ldflags "-X github.com/scality/file-reflector/pkg/version.Version=${VERSION}" \
    -o file-reflector ./cmd

# Grant the capabilities the agent needs to mirror source onto target as
# a non-root user, regardless of the permissions and ownership of the
# files it touches:
#   - cap_dac_override: read any source entry and create/write/delete in
#     any target directory, bypassing r/w/x permission checks
#   - cap_fowner: chmod target entries the agent does not own (e.g. a file
#     previously chowned to another uid)
#   - cap_chown: chown synced entries to an arbitrary uid:gid (--owner)
# BuildKit preserves the security.capability xattr across the COPY into
# the final stage. getcap prints the result so it is visible in the
# build log.
RUN apt-get update \
    && apt-get install --yes --no-install-recommends libcap2-bin \
    && rm -rf /var/lib/apt/lists/* \
    && setcap cap_chown,cap_dac_override,cap_fowner+ep /workspace/file-reflector \
    && getcap /workspace/file-reflector

FROM gcr.io/distroless/static:nonroot@sha256:963fa6c544fe5ce420f1f54fb88b6fb01479f054c8056d0f74cc2c6000df5240

WORKDIR /
COPY --from=builder /workspace/file-reflector /file-reflector

USER 65532:65532

ENTRYPOINT ["/file-reflector"]
