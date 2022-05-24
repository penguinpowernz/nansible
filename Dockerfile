FROM ubuntu:18.04

COPY bin/nansibled /usr/bin

RUN mkdir -p /data
WORKDIR /data
VOLUME /data

CMD [ "nansibled" ]