#!/usr/bin/env bash

mkdir -p sites/version
git describe --tags $(git rev-list --tags --max-count=1) | cut -c 2- > sites/version/latest
