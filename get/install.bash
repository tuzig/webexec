#!/bin/bash
# webexec installation script
#
# This script is meant for quick & easy install via:
#   $ curl -L https://get.webexec.sh | bash
set -x
SCRIPT_COMMIT_SHA=UNKNOWN
LATEST_VERSION="0.13.2"

# The latest release is currently hard-coded.
echo "Installing " $LATEST_VERSION "version"
         
ARCH="$(uname -m)"  # -i is only linux, -m is linux and apple
if [[ "$ARCH" = x86_64* ]]; then
    if [[ "$(uname -a)" = *ARM64* ]]; then
        ARCH='arm64'
    else
        ARCH="amd64"
    fi
elif [[ "$ARCH" = i*86 ]]; then
    ARCH='386'
elif [[ "$ARCH" = arm* ]]; then
    ARCH='arm6'
elif test "$ARCH" = aARCH64; then
    ARCH='arm7'
else
    exit 1
fi


init_vars() {
	BIN="${WEBEXEC_BIN:/usr/local/bin}"

	DAEMON=webexec
	SYSTEMD=
	if systemctl --user daemon-reload >/dev/null 2>&1; then
		SYSTEMD=1
	fi
}

checks() {
	# OS verification: Linux only, point osx/win to helpful locations
	case "$(uname)" in
	Darwin)
		;;
	Linux)
		;;
	*)
		>&2 echo "webexec cannot be installed on $(uname)"; exit 1
		;;
	esac

	# HOME verification
	if [ ! -d "$HOME" ]; then
		>&2 echo "Aborting because HOME directory $HOME does not exist"; exit 1
	fi

    if [ ! -w "$HOME" ]; then
        >&2 echo "Aborting because HOME (\"$HOME\") is not writable"; exit 1
    fi

	# Validate XDG_RUNTIME_DIR
	if [ ! -w "$XDG_RUNTIME_DIR" ]; then
		if [ -n "$SYSTEMD" ]; then
			>&2 echo "Aborting because systemd was detected but XDG_RUNTIME_DIR (\"$XDG_RUNTIME_DIR\") does not exist or is not writable"
			>&2 echo "Hint: this could happen if you changed users with 'su' or 'sudo'. To work around this:"
			>&2 echo "- try again by first running with root privileges 'loginctl enable-linger <user>' where <user> is the unprivileged user and export XDG_RUNTIME_DIR to the value of RuntimePath as shown by 'loginctl show-user <user>'"
			>&2 echo "- or simply log back in as the desired unprivileged user (ssh works for remote machines)"
			exit 1
		fi
	fi

}

get_n_extract() {
	case "$(uname)" in
	Darwin)
        STATIC_RELEASE_URL="https://github.com/tuzig/webexec/releases/download/v$WEBEXEC_VERSION/webexec_${WEBEXEC_VERSION}.dmg"
        # curl -L -o webexec.dmg "$STATIC_RELEASE_URL"
        cp "/Users/daonb/src/webexec/dist/webexec_$LATEST_VERSION.dmg" .
        hdiutil attach -mountroot . -quiet -readonly -noautofsck "webexec_$LATEST_VERSION.dmg"
		;;
	Linux)
        STATIC_RELEASE_URL="https://github.com/tuzig/webexec/releases/download/v$LATEST_VERSION/webexec_${LATEST_VERSION}_$(uname -s | tr '[:upper:]' '[:lower:]')_$ARCH.tar.gz"
       curl -L -o webexec.tgz "$STATIC_RELEASE_URL"
	esac
}
# this should be run as root
replace_n_launch() {
    set -x 

    if [[ -x /usr/local/bin/webexec ]]; then
        su daonb -c "/usr/local/bin/webexec stop"
        rm /usr/local/bin/webexec
    fi
	case "$(uname)" in
	Darwin)
        cp webexec/webexec /usr/local/bin
        # TODO: fix launchd
        # cp launchd file & load
        # envsubst < "webexec/sh.webexec.daemon.tmpl" > "sh.webexec.daemon.plist"
        # sudo mv "sh.webexec.daemon.plist" /Library/LaunchDaemons

        # sudo chown root:wheel "/Library/LaunchDaemons/sh.webexec.daemon.plist"
        # sudo launchctl load "/Library/LaunchDaemons/sh.webexec.daemon.plist"
        umount webexec
        echo "Sorry but our launchd daemon is not ready yet"
        echo "Till we have it, You'll need to 'webexec start' after restart"
        su daonb -c "/usr/local/bin/webexec start"
		;;
	Linux)
        STATIC_RELEASE_URL="https://github.com/tuzig/webexec/releases/download/v$LATEST_VERSION/webexec_${LATEST_VERSION}_$(uname -s | tr '[:upper:]' '[:lower:]')_$ARCH.tar.gz"
        curl -L -o webexec.tgz "$STATIC_RELEASE_URL"
        tar zxf "webexec.tgz" --strip-components=1
        if [[ -f /etc/webexec ]]; then
            echo "==X webexec is already used on this host by $(cut -d= -f2 < /etc/webexec)"
        else
            sudo cp webexec /usr/local/bin
            ECHO_CONF="echo USER=$(whoami)"
            sudo sh -c "$ECHO_CONF >/etc/webexec"
            sudo cp webexecd.sh /etc/init.d/webexec
            sudo chown root:root /etc/init.d/webexec
            sudo update-rc.d webexec defaults
            sudo systemctl start webexec
        fi
		;;
	esac
}

do_install() {
	init_vars
	checks

	tmp=$(mktemp -d)

    cd $tmp
    get_n_extract
    export -f replace_n_launch
    echo "==> We need root access to add webexec's binary and service"
    sudo nohup bash -c replace_n_launch
}
do_install "$@"
