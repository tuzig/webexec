#!/usr/bin/env bash

set -x
EXE="/webexec/webexec"
CONF="/config/webexec"

/etc/init.d/ssh start
cp $EXE /usr/local/bin
mkdir -p /home/runner/.config/webexec
cp -r "$CONF" /home/runner/.config/
chown -R runner /home/runner
su -c "$EXE stop && $EXE start --debug" runner
