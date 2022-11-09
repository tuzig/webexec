#!/bin/bash
# webexec installation script
#
# This script is meant for quick & easy install via:
#
#   $ curl -sL https://get.webexec.sh -o get-webexec.sh
#   $ ./get-webexec.sh
#
# The latest release is currently hard-coded.
LATEST_VERSION="0.17.11"
         
ARCH="$(uname -m | tr [:upper:] [:lower:])" 
if [[ "$ARCH" = arm64 ]]; then
    ARCH='arm64'
elif [[ "$ARCH" = x86_64* ]]; then
    # and now for some M1 fun
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
    >&2 echo "Sorry, unsupported architecture $ARCH"
    >&2 echo "Try installing from source: go install github.com/tuzig/webexec@latest"
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

get_dist() {
	# Every system that we officially support has /etc/os-release
	if [ -r /etc/os-release ]; then
		dist=$(. /etc/os-release && echo $ID |  tr '[:upper:]' '[:lower:]')
	fi
	# Returning an empty string here should be alright since the
	# case statements don't act unless you provide an actual value
}

checks() {
	# OS verification: Linux only, point osx/win to helpful locations
	case "$(uname)" in
	Darwin)
		;;
	Linux)
		;;
	*)
		>&2 echo "FAILED: webexec cannot be installed on $(uname)"
        >&2 echo "Try installing fro source: `go install github.com/tuzig/webexec@latest`"
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
    BASE_NAME="webexec_${LATEST_VERSION}_$(uname -s | tr [:upper:] [:lower:])_$ARCH"
    cd $1
	case "$(uname)" in
	Darwin)
        BALL_NAME="$BASE_NAME.zip"
        STATIC_RELEASE_URL="https://github.com/tuzig/webexec/releases/download/v$LATEST_VERSION/$BALL_NAME"
        curl -sL "$STATIC_RELEASE_URL" -o $BALL_NAME
        unzip $BALL_NAME
        ;;
	Linux)
        BALL_NAME="$BASE_NAME.tar.gz"
        STATIC_RELEASE_URL="https://github.com/tuzig/webexec/releases/download/v$LATEST_VERSION/$BALL_NAME"
        curl -sL "$STATIC_RELEASE_URL" | tar zx 
        ;;
	esac
    cd $BASE_NAME
    ./webexec init
}

do_install() {
	checks
    user="$(whoami)"
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
			exit 4
		fi
	fi
	get_dist
    dname="webexec.$user"
    echo "    root power required to:"
    echo "    - create /var/log/$dname & /var/run/$dname"
    echo "    - make you their owner"
    echo "    - make you their owner"

	case "$(uname)" in
	Darwin)
        echo "    - add you to the wheel & daemon groups"
        if ! command_exists curl; then
            brew install curl
        fi
        if [ $user != "root" ]; then
            $sh_c "dseditgroup -o edit -a $user -t user wheel"
            $sh_c "dseditgroup -o edit -a $user -t user daemon"
        fi
        ;;
    Linux)
        echo "    - install curl if missing"
        echo "    - add a /usr/liotmpfiles/var/log/$dname"
        if ! command_exists curl; then
            $sh_c 'apt-get update -qq >/dev/null'
            $sh_c "DEBIAN_FRONTEND=noninteractive apt-get install -y -qq curl >/dev/null"
        fi
        tmpfile="d /var/run/$dname 0755 $user ${GROUP:-root}\nd /var/log/$dname 0755 $user ${GROUP:-root}"
        $sh_c "echo $tmpfile > /usr/lib/tmpfiles.d/$dname"
        ;;

    esac
    $sh_c "mkdir -p /var/log/$dname && mkdir /var/run/$dname"
    $sh_c "chown $UID:$(id -g) /var/log/$dname /var/run/$dname"
	# Run setup for each distro accordingly
    tmp=$(mktemp -d)
    echo ">>> created temp dir at $tmp"
	if ! debug; then
        cd $tmp
	fi
    get_n_extract $tmp
    $sh_c "nohup bash ./replace_n_launch.sh $user ${HOME:-/root}"
}
do_install "$@"
