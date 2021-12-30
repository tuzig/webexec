#!/bin/bash
# webexec installation script
#
# This script is meant for quick & easy install via:
#
#   $ curl -L https://get.webexec.sh -o get-webexec.sh && bash get-webexec.sh
#
set -x
SCRIPT_COMMIT_SHA=UNKNOWN
LATEST_VERSION="0.15.3"

# The latest release is currently hard-coded.
echo "Installing webexec version " $LATEST_VERSION
         
ARCH="$(uname -m | tr [:upper:] [:lower:])" 
if [[ "$ARCH" = x86_64* ]]; then
    if [[ "$(uname -a)" = *ARM64* ]]; then
        ARCH='arm64'
    else
        ARCH="amd64"
    fi
elif [ "$ARCH" = i386 ]; then
    ARCH='386'
elif [[ "$ARCH" = armv6* ]]; then
    ARCH='armv6'
elif [[ "$ARCH" = armv7* ]]; then
    ARCH='armv7'
elif [[ "$ARCH" = aarch64 ]]; then
    ARCH='armv7'
else
    echo "Sorry, unsupported architecture $ARCH"
    echo "Try using: `go install github.com/tuzig/webexec` to install from source"
    exit 1
fi

DEBUG=${DEBUG:-}
while [ $# -gt 0 ]; do
	case "$1" in
		--debug)
			DEBUG=1
			;;
		--*)
			echo "Illegal option $1"
			;;
	esac
	shift $(( $# > 0 ? 1 : 0 ))
done

debug() {
	if [ -z "$DEBUG" ]; then
		return 1
	else
		return 0
	fi
}

command_exists() {
	command -v "$@" > /dev/null 2>&1
}

get_distribution() {
	lsb_dist=""
	# Every system that we officially support has /etc/os-release
	if [ -r /etc/os-release ]; then
		lsb_dist="$(. /etc/os-release && echo "$ID")"
	fi
	# Returning an empty string here should be alright since the
	# case statements don't act unless you provide an actual value
	echo "$lsb_dist"
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

}

get_n_extract() {
	case "$(uname)" in
	Darwin)
        if command_exists go; then
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
        BALL_NAME="webexec_${LATEST_VERSION}_$(uname -s | tr [:upper:] [:lower:])_$ARCH.tar.gz"
        STATIC_RELEASE_URL="https://github.com/tuzig/webexec/releases/download/v$LATEST_VERSION/$BALL_NAME"
        curl -sL "$STATIC_RELEASE_URL" -o $1/$BALL_NAME
        tar zx --strip-components=1 -C $1 < $1/$BALL_NAME 
	esac
}

do_install() {
	checks

	sh_c='sh -c'
	if [ "$user" != 'root' ]; then
		if command_exists sudo; then
			sh_c='sudo -E sh -c'
		elif command_exists su; then
			sh_c='su -c'
		else
			cat >&2 <<-'EOF'
			Error: this installer needs the ability to run commands as root.
			We are unable to find either "sudo" or "su" available to make this happen.
			EOF
			exit 1
		fi
	fi
	lsb_dist=$( get_distribution )
	lsb_dist="$(echo "$lsb_dist" | tr '[:upper:]' '[:lower:]')"
	# Run setup for each distro accordingly
	case "$lsb_dist" in
		ubuntu|debian|raspbian)
			if ! command_exists curl; then
				$sh_c 'apt-get update -qq >/dev/null'
				$sh_c "DEBIAN_FRONTEND=noninteractive apt-get install -y -qq $pre_reqs >/dev/null"
			fi
			;;
    esac

    tmp=$(mktemp -d)
    echo "Created temp dir at $tmp"
    get_n_extract $tmp
	if debug; then
		wd=.
	else
		wd=$tmp
	fi
    $wd/webexec init
    # TODO: fixed launchd
    if [ "$(uname)" = Linux ]; then
		$sh_c "nohup bash cd $wd && replace_n_launch.sh $USER $HOME"
    fi
}
do_install "$@"
