FROM golang:bullseye
RUN apt update
RUN apt install -y git bash openssh-server
COPY ./atp/infra/webexec/config/ssh_config /etc/ssh/
RUN /etc/init.d/ssh start
RUN useradd -d /home/webexec -M -p VDueK9QTyseeA webexec
WORKDIR /home/webexec
CMD bash -x ./atp/infra/webexec/cmd.sh
