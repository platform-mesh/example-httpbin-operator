FROM --platform=$BUILDPLATFORM golang:1.25@sha256:1c91b4f4391774a73d6489576878ad3ff3161ebc8c78466ec26e83474855bfcf AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 \
    GOCACHE=/root/.cache/go-build \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    make build

# Use distroless base-debian11 image to include shell access while keeping minimal footprint
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian11:debug@sha256:68e5ea65df0f5d135083d4cc1df5fc16855d61ed628254df8e1affa8ce2d3244
WORKDIR /
COPY --from=builder /workspace/bin/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
