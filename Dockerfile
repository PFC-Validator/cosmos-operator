FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

RUN apk add --update --no-cache\
    gcc\
    libc-dev\
    git\
    make\
    bash\
    g++\
    linux-headers\
    perl\
    snappy-dev\
    zlib-dev\
    bzip2-dev\
    lz4-dev\
    zstd-dev

ARG TARGETARCH
ARG BUILDARCH

RUN if [ "${TARGETARCH}" = "arm64" ] && [ "${BUILDARCH}" != "arm64" ]; then \
        wget -c https://storage.googleapis.com/strangelove-public/musl/aarch64-linux-musl-cross.tgz -O - | tar -xzvv --strip-components 1 -C /usr; \
    elif [ "${TARGETARCH}" = "amd64" ] && [ "${BUILDARCH}" != "amd64" ]; then \
        wget -c https://storage.googleapis.com/strangelove-public/musl/x86_64-linux-musl-cross.tgz -O - | tar -xzvv --strip-components 1 -C /usr; \
    fi

RUN set -eux;\
    if [ "${TARGETARCH}" = "arm64" ] && [ "${BUILDARCH}" != "arm64" ]; then \
        echo aarch64 > /etc/apk/arch;\
    elif [ "${TARGETARCH}" = "amd64" ] && [ "${BUILDARCH}" != "amd64" ]; then \
        echo x86_64 > /etc/apk/arch;\
    fi;\
    apk add --update --no-cache\
    snappy-static\
    zlib-static\
    bzip2-static\
    lz4-static\
    zstd-static\
    --allow-untrusted

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY *.go .
COPY api/ api/
COPY cmd/ cmd/
COPY controllers/ controllers/
COPY internal/ internal/

ARG VERSION

RUN set -eux;\
    if [ "${TARGETARCH}" = "arm64" ] && [ "${BUILDARCH}" != "arm64" ]; then\
        export CC=aarch64-linux-musl-gcc CXX=aarch64-linux-musl-g++;\
    elif [ "${TARGETARCH}" = "amd64" ] && [ "${BUILDARCH}" != "amd64" ]; then\
        export CC=x86_64-linux-musl-gcc CXX=x86_64-linux-musl-g++;\
    fi;\
    export  GOOS=linux \
            GOARCH=$TARGETARCH \
            CGO_ENABLED=1 \
            LDFLAGS='-linkmode external -extldflags "-static"';\
    go build -tags 'pebbledb' -ldflags "-X github.com/bryanlabs/cosmos-operator/internal/version.version=$VERSION $LDFLAGS" -a -o manager .

# Build final image from scratch
FROM scratch

LABEL org.opencontainers.image.source=https://github.com/PFC-Validator/cosmos-operator

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
