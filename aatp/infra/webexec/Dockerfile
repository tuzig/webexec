FROM golang:bullseye
RUN apt update
RUN apt install -y git bash openssh-server
COPY ./aatp/infra/webexec/config/ssh_config /etc/ssh/
RUN /etc/init.d/ssh start
RUN useradd -s /bin/bash -d /home/runner -M -p VDueK9QTyseeA runner
CMD bash /webexec/aatp/infra/webexec/docker_entry.sh
