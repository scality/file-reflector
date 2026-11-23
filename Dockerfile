# syntax=docker/dockerfile:1@sha256:572cebbc2b15e6f84c077742952674889cb63cd5e9b80abdffcbe587b6f19183

FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder

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

FROM gcr.io/distroless/static:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931

WORKDIR /
COPY --from=builder /workspace/file-reflector /file-reflector

USER 65532:65532

ENTRYPOINT ["/file-reflector"]
