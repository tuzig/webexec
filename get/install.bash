#!/bin/bash
# webexec installation script
#
# This script is meant for quick & easy install via:
#   $ curl -L https://get.webexec.sh | bash
SCRIPT_COMMIT_SHA=UNKNOWN
LATEST_VERSION="0.14.0"

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
	if [  ! -d "$HOME" ]; then
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
        if command -v go &> /dev/null; then
            go install github.com/tuzig/webexec@v$LATEST_VERSION
            webexec init
            webexec start
        else
            echo "Sorry but our MacOS binary is still waiting notarization."
            echo "For now, you will need to compile webexec yourself."
            echo "Please install the latest go from: https://go.dev/doc/install"
            echo "and re-run this installer."
            exit -1
        fi
        # TODO: noptarize the binaries and then:
        # STATIC_RELEASE_URL="https://github.com/tuzig/webexec/releases/download/v$LATEST_VERSION/webexec_${LATEST_VERSION}.dmg"
        # curl -sL -o webexec.dmg "$STATIC_RELEASE_URL"
        # For debug:
        # cp "/Users/daonb/src/webexec/dist/webexec_$LATEST_VERSION.dmg" .
        # hdiutil attach -mountroot . -quiet -readonly -noautofsck "webexec.dmg"
        # cp webexec/* .
        # umount webexec
        
		;;
	Linux)
        STATIC_RELEASE_URL="https://github.com/tuzig/webexec/releases/download/v$LATEST_VERSION/webexec_${LATEST_VERSION}_$(uname -s | tr '[:upper:]' '[:lower:]')_$ARCH.tar.gz"
       curl -sL "$STATIC_RELEASE_URL" | tar zx --strip-components=1
       ./webexec init
	esac
}
do_install() {
	init_vars
	checks

	tmp=$(mktemp -d)

    echo "Created temp dir: $tmp"
    cd $tmp
    get_n_extract
    # TODO: fixed launchd
    if [[ "$(uname)" = Linux ]]; then
        echo "==> We need root access to add webexec's binary and service"
        sudo nohup bash webexec/replace_n_launch.sh $USER
    fi
}
do_install "$@"
