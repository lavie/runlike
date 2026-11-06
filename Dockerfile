# runlike in the official docker CLI image, so `docker container inspect`
# works against the mounted socket:
#   docker run --rm -v /var/run/docker.sock:/var/run/docker.sock IMAGE CONTAINER
# Multi-arch: docker buildx build --platform linux/amd64,linux/arm64 \
#   --build-arg VERSION=$(git describe --tags) .
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS build
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /runlike ./cmd/runlike

FROM docker:29.9.3-cli@sha256:2357639ad1ea7d0194134bf3292622fd4e562483966ed674e5d7c945b5f3b28d
COPY --from=build /runlike /usr/local/bin/runlike
ENTRYPOINT ["runlike"]
