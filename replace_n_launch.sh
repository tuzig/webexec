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
    if [ -x /etc/init.d/webexec ]; then
        /etc/init.d/webexec stop
    fi
    cp webexec /usr/local/bin
    ECHO_CONF="echo USER=$(whoami)"
    sh -c "$ECHO_CONF >/etc/webexec"
    cp webexecd.sh /etc/init.d/webexec
    chown root:root /etc/init.d/webexec
    update-rc.d webexec defaults
    /etc/init.d/webexec start 
    ;;
esac
