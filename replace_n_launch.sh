#!/bin/sh
# this should be run as root
command_exists() {
	command -v "$@" > /dev/null 2>&1
}

if command_exists webexec; then
    webexec stop
fi
cp webexec /usr/local/bin
