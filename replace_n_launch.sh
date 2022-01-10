#!/bin/sh
# this should be run as root
set -x

command_exists() {
	command -v "$@" > /dev/null 2>&1
}

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
        rm /etc/init.d/webexec
    fi
    if [ -f /etc/systemd/system/webexec.service ]; then
        systemctl stop webexec.service
        rm /etc/systemd/system/webexec.service
    fi
    cp webexec /usr/local/bin
    systemctl=0
    if command_exists systemctl; then
        systemctl >/dev/null
        if [ $? -eq 0 ]; then
            cp webexec.service.tmpl webexec.service
            sed -i "s/\$USER/$1/g; s?\$HOME?$2?g" webexec.service
            chown root:root webexec.service
            mv webexec.service /etc/systemd/system/webexec.service
            systemctl daemon-reload
            systemctl enable webexec.service
            systemctl start webexec.service
            systemctl=1
        fi
    fi
    if [ $systemctl -eq 0 ]; then
        sh -c "echo USER=$1 >/etc/webexec"
        cp webexecd.sh /etc/init.d/webexec
        chown root:root /etc/init.d/webexec
        update-rc.d webexec defaults
        /etc/init.d/webexec start 
    fi
    ;;
esac
