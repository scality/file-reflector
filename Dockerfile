# syntax=docker/dockerfile:1

FROM golang:1.26 AS builder

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

FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/file-reflector /file-reflector

USER 65532:65532

ENTRYPOINT ["/file-reflector"]
