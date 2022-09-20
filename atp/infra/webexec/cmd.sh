#!/usr/bin/env bash
/etc/init.d/ssh start
su webexec <<EOS
go generate .
go build .
./webexec init 
./webexec start --debug
EOS
