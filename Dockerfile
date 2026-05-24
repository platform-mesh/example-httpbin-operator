FROM --platform=$BUILDPLATFORM golang:1.26@sha256:2d6c80227255c3112a4d08e67ba98e58efd3846daf15d9d7d4c389565d881b1a AS builder
ARG TARGETARCH

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# Use distroless base-debian11 image to include shell access while keeping minimal footprint
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian11:debug@sha256:68e5ea65df0f5d135083d4cc1df5fc16855d61ed628254df8e1affa8ce2d3244
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
