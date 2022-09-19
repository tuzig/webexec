FROM golang:alpine
RUN apk update
RUN apk add git bash
RUN addgroup -S webexec && adduser -S webexec -G webexec
RUN mkdir -p /run/webexec.webexec /var/log/webexec.webexec && \
    chown webexec:webexec /run/webexec.webexec /var/log/webexec.webexec
USER webexec
WORKDIR /home/webexec/src
CMD ./atp/infra/webexec/cmd.sh
