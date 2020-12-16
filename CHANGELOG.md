# ChangeLog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.5.0] - 2020-12-16

### Added 

- Pane buffer to store output
- Orderly shutdown and marker based restore

## [0.4.3] - 2020-11-24

### Fixed

- improving simple trminal reentrancy locks

## [0.4.2] - 2020-11-24

### Added

- rotating logs
- a Makefile!

### Fixed

- When a peer connection fails, close it and foggatabouit
- Solving the multi-output bug #33 by refactoring the panes and dcs management
- Improved log messages

## [0.4.1] - 2020-11-08

- Immidiatly closing a reconnect to a non-running pane
- Fixing github actions
- Removing silly linter

## [0.4.0] - 2020-10-14

### Fixed

- Linter based code beutification

### Added

- Screen buffer & cursor position restore. Monchrome & plain for now

## [0.3.0] - 2020-10-04

### Added 
- An agent that runs in the backgroung and managed by sub commands
- Sub commands - help, auth, start, stop, status, restart, init
- Added sub command placeholders - copy, paste
- Zap logger
- API documentation
- Support for client payload
- Tests

### Changed
- Replaced "/etc/passwd" based auth with a single token: "THEoneANDonlyTOKEN"
- Source tree is now flat but for a pidfile package we copied
- Control message schema has changed. It now has the "type" and "args" keys
- Improved HTTP server error handling
- Removing panics

## [0.2.1] - 2020-08-02

### Fixed

- resize message
- Improved reconnect support

## [0.2.0] - 2020-06-30

### Changed
- Authentication is based on secret which can be either a password or a hash

### Fixed
- commands now run under the authenticated user
- starting shell only once

### Added
- adding a `body` field to the Ack message and using it return a token on auth

## [0.1.1] - 2020-06-21
### Added

- Authentication: A control message lets linux clients authenticate.
