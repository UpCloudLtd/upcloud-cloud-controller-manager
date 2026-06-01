FROM alpine:3.23
RUN apk add --no-cache curl tini
COPY cloud-controller-manager /usr/local/bin/
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["cloud-controller-manager"]
