# ChangeLog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleashed 

### Adds 
- Ability to run as an agent
- Added sub command - start, stop, status, restart, init, help
- Added sub command placeholders - copy, paste, auth
- Zap logger
- API documentation
- Support for client payload
- Tests

### Changes
- Replaced "/etc/passwd" based auth with a single token: "THEoneANDonlyTOKEN"
- Source tree is now flat but for a pidfile package we copied
- Control message schema has changed. It now has the "type" and "args" keys
- Improved HTTP server error handling
- Removing panics


## [0.2.1] - 2020-08-02

### Fixes

- resize message
- Improved reconnect support

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
