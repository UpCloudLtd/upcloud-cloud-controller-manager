FROM alpine:3.22
RUN apk add --no-cache curl tini
COPY cloud-controller-manager /usr/local/bin/
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["cloud-controller-manager"]