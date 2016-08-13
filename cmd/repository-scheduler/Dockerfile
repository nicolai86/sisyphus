FROM alpine:3.4

RUN adduser -h /home/sisyphus sisyphus -s /bin/false -D

WORKDIR /home/sisyphus
USER sisyphus

COPY bin/repository-scheduler /home/sisyphus
