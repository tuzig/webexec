#!/bin/bash
set -x
# webexec installation script (Rootless mode)
#
# This script is meant for quick & easy install via:
#   $ curl -fsSL https://get.webexec.sh | bash
SCRIPT_COMMIT_SHA=UNKNOWN
LATEST_VERSION="0.11.1"

# This script should be run with an unprivileged user and install/setup Docker under $HOME/bin/.

# The latest release is currently hard-coded.
echo "# Installing " $LATEST_VERISON "version"
         
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

STATIC_RELEASE_URL="https://github.com/tuzig/webexec/releases/download/v$LATEST_VERSION/webexec_${LATEST_VERSION}_$(uname -s | tr '[:upper:]' '[:lower:]')_$ARCH.tar.gz"

init_vars() {
	BIN="${WEBEXEC_BIN:-$HOME/bin}"

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

do_install() {
	init_vars
	checks

	tmp=$(mktemp -d)
	trap "rm -rf $tmp" EXIT INT TERM
	(
		cd "$tmp"
		curl -L -o webexec.tgz "$STATIC_RELEASE_URL"
	)
	(
        mkdir -p "$BIN"
        cd "$BIN"
		tar zxf "$tmp/webexec.tgz" --strip-components=1
        echo "==> We need root access to add the binary and the service"
        sudo cp webexec /usr/local/bin
	)
	case "$(uname)" in
	Darwin)
        echo "darwin startup script is not ready yet"
        echo "sorry, but you'll have to start manually on reboot"

		;;
	Linux)
        if [[ -f /etc/webexec ]]; then
            echo "==X webexec is already used on this host by $(cut -d= -f2 < /etc/webexec)"
        else
            ECHO_CONF="echo USER=$(whoami)"
            sudo sh -c "$ECHO_CONF >/etc/webexec"
            sudo cp webexecd.sh /etc/init.d/webexec
            sudo chown root:root /etc/init.d/webexec
            sudo update-rc.d webexec defaults
            sudo update-rc.d webexec enable
        fi
		;;
	esac
}
do_install "$@"
