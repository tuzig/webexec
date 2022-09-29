FROM ubuntu:latest
RUN apt update
RUN apt install -y git bash openssh-server
COPY ./aatp/infra/webexec/config/ssh_config /etc/ssh/
COPY ./aatp/infra/webexec/docker_entry.sh /usr/local/bin
RUN /etc/init.d/ssh start
RUN useradd -s /bin/bash -d /home/runner -M -p VDueK9QTyseeA runner
CMD bash /usr/local/bin/docker_entry.sh
