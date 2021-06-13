#!/usr/bin/env bash

#
# DO NOT EDIT THIS FILE
#
# It is automatically copied from https://github.com/pion/.goassets repository.
#
# If you want to update the shared CI config, send a PR to
# https://github.com/pion/.goassets instead of this repository.
#

set -e

SCRIPT_PATH=$( cd "$(dirname "${BASH_SOURCE[0]}")" ; pwd -P )
AUTHORS_PATH="$GITHUB_WORKSPACE/AUTHORS"

if [ -f ${SCRIPT_PATH}/.ci.conf ]
then
  . ${SCRIPT_PATH}/.ci.conf
fi

#
# DO NOT EDIT THIS
#
EXCLUDED_CONTRIBUTORS+=('John R. Bradley' 'renovate[bot]' 'Renovate Bot' 'Pion Bot')
# If you want to exclude a name from all repositories, send a PR to
# https://github.com/pion/.goassets instead of this repository.
# If you want to exclude a name only from this repository,
# add EXCLUDED_CONTRIBUTORS=('name') to .github/.ci.conf

CONTRIBUTORS=()

shouldBeIncluded () {
	for i in "${EXCLUDED_CONTRIBUTORS[@]}"
	do
		if [ "$i" == "$1" ] ; then
			return 1
		fi
	done
	return 0
}


IFS=$'\n' #Only split on newline
for contributor in $(git log --format='%aN <%aE>' | LC_ALL=C.UTF-8 sort -uf)
do
	if shouldBeIncluded $contributor; then
		CONTRIBUTORS+=("$contributor")
	fi
done
unset IFS

if [ ${#CONTRIBUTORS[@]} -ne 0 ]; then
	cat >$AUTHORS_PATH <<-'EOH'
	# This file lists all individuals having contributed content to the repository.
	# For how it is generated, see `pion/.goassets/ci/.github/generate-authors.sh
	EOH
    for i in "${CONTRIBUTORS[@]}"
    do
	    echo "$i" >> $AUTHORS_PATH
    done
    exit 0
fi
