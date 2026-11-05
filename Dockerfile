# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

FROM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS builder

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
RUN apk add --no-cache libcap \
    && setcap cap_chown,cap_dac_override,cap_fowner+ep /workspace/file-reflector \
    && getcap /workspace/file-reflector

FROM gcr.io/distroless/static:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329

WORKDIR /
COPY --from=builder /workspace/file-reflector /file-reflector

USER 65532:65532

ENTRYPOINT ["/file-reflector"]
