#!/bin/bash
# this should be run as root
set -x

case "$(uname)" in
Darwin)
    # TODO: Darwin is not running today - fix launchd
    cp webexec/webexec /usr/local/bin
    cp launchd file & load
    envsubst < "webexec/sh.webexec.daemon.tmpl" > "sh.webexec.daemon.plist"
    mv "sh.webexec.daemon.plist" /Library/LaunchDaemons

    chown root:wheel "/Library/LaunchDaemons/sh.webexec.daemon.plist"
    launchctl load "/Library/LaunchDaemons/sh.webexec.daemon.plist"
    ;;
Linux)
    systemctl stop webexec.service
    cp webexec /usr/local/bin
    USER="$1" HOME="$2" envsubst < webexec.service.tmpl > /etc/systemd/system/webexec.service
    chown root:root /etc/systemd/system/webexec.service
    systemctl daemon-reload
    systemctl enable webexec.service
    systemctl start webexec.service
    ;;
esac
