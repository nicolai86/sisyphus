FROM alpine:3.4

RUN apk update && \
  apk add git && \
  apk --no-cache add ca-certificates && \
  update-ca-certificates && \
  adduser -h /home/sisyphus sisyphus -s /bin/false -D

WORKDIR /home/sisyphus

COPY bin/greenkeepr-javascript /home/sisyphus
