FROM golang:1.25-alpine AS builder

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download
COPY ./ ./
ARG GOPROXY
ARG OS
ARG ARCH
ARG ARM
ARG LDFLAGS
RUN CGO_ENABLED=0 GOOS=${OS} GOARCH=${ARCH} GOARM=${ARM} GOPROXY=${GOPROXY} \
  go build \
  -ldflags="-extldflags '-static' ${LDFLAGS}" \
  -o=cloud-controller-manager \
  github.com/UpCloudLtd/upcloud-cloud-controller-manager/cmd/upcloud-cloud-controller-manager

FROM alpine:3.23
RUN apk add --no-cache curl tini
WORKDIR /
COPY --from=builder /workspace/cloud-controller-manager /usr/local/bin/
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["cloud-controller-manager"]