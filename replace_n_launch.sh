#!/bin/bash
# this should be run as root

if [[ -x /usr/local/bin/webexec ]]; then
    su $1 -c "/usr/local/bin/webexec stop"
    rm /usr/local/bin/webexec
fi
case "$(uname)" in
Darwin)
    cp webexec/webexec /usr/local/bin
    # TODO: fix launchd
    cp launchd file & load
    envsubst < "webexec/sh.webexec.daemon.tmpl" > "sh.webexec.daemon.plist"
    mv "sh.webexec.daemon.plist" /Library/LaunchDaemons

    chown root:wheel "/Library/LaunchDaemons/sh.webexec.daemon.plist"
    launchctl load "/Library/LaunchDaemons/sh.webexec.daemon.plist"
    ;;
Linux)
    if [[ -f /etc/webexec ]]; then
        echo "==X webexec is already used on this host by $(cut -d= -f2 < /etc/webexec)"
    else
        cp webexec /usr/local/bin
        ECHO_CONF="echo USER=$(whoami)"
        sh -c "$ECHO_CONF >/etc/webexec"
        cp webexecd.sh /etc/init.d/webexec
        chown root:root /etc/init.d/webexec
        update-rc.d webexec defaults
        systemctl start webexec
    fi
    ;;
esac
