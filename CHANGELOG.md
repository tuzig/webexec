# ChangeLog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleashed 
## [0.2.0] - 2020-06-30

### Changes
- Authentication is based on secret which can be either a password or a hash

### Fixes
- commands now run under the authenticated user
- starting shell only once

### Adds
- adding a `body` field to the Ack message and using it return a token on auth

## [0.1.1] - 2020-06-21
### Added

- Authentication: A control message lets linux clients authenticate.
