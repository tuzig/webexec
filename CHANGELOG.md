# Change Log

All notable changes to this project will be documented in this fil, 

webexec adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

his file's format is define in 
[Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
and the release workflow reads it to set github's release notes.

Known issues:
- defunct shell processes stay around till the agent is stopped

## [0.10.6] - 2021-5-02

### Fixed

- forever trying to reconnect to peerbook
- removing a hidden loop that could cause webexec to hang


## [0.10.5] - 2021-4-25

### Fixed

- a re-entrancy crash eas fixed using a mutex

## [0.10.4] - 2020-4-21

### Fixed

- updated the changelog

## [0.10.3] - 2020-4-21

### Fixed

- improving tar ball names

## [0.10.2] - 2020-4-21

### Fixed

- updated the changelog

## [0.10.1] - 2020-4-21

### Fixed

- updated the changelog and the README

## [0.10.0] - 2020-4-21

### Added 

- `webexec init` to initialize configuration 
- support for behind-the-nat hosts through a ginaling server - peerbook
- webexec.conf support for `[env]` section for env vars to set in shells.
- webexec.conf support for `net.upd_port_min` and `net.udp_port_max`
- settings file documentation

### Changed

- Certificates are now consistent 
- Improved security docs

## [0.8.4] - 2020-2-4

### Fixed

- Release action

## [0.8.3] - 2020-2-4

### Fixed

- Building darwin

## [0.8.2] - 2020-2-4

### Fixed

- Building linux_arm64

## [0.8.1] - 2020-2-4

### Added

- redirect err to an error file
- Added pion logging configuration 

### Fixed
 
- exit when port is busy
- pion's logging goes tot he same file as webexec log

## [0.8.0] - 2020-1-20

### Fixed

- use webrtc's certificate to authenticate

## [0.7.1] - 2020-1-15

### Fixed

- renamed cong 'stun_urls' to 'ice_servers' and added it to default conf

## [0.7.0] - 2020-1-14

### Added

- "api_version" in auth message. current is version is 2

## [0.6.4] - 2020-1-13

### Fixed

- "release" workflow should be working now

## [0.6.3] - 2020-1-12

### Fixed

- "release" workflow should be working now

## [0.6.2] - 2020-1-12

### Fixed

- "release" workflow should really work now

## [0.6.1] - 2020-1-12

### Fixed

- "release" workflow should work now

## [0.6.0] - 2020-1-12

### Added

- extended the conf file to include timeouts, stun server, etc.

### Changed

- upgraded pion/webrtc to v3

### Fixed 

- improved stability by introducting the client database 

## [0.5.5] - 2020-1-6

### Fixed

- Automated release should be working now

## [0.5.4] - 2020-1-5

### Added

- Auto releasing with wide os & architecture support

### Fixed

- reentrancy on dcs db causing a crash

### Changed

- removed the `init` subcommand, home dir is created if missing

## [0.5.3] - 2020-1-4

### Added

- Security doc
- Installation instructions
- Producing binaries on release
 
### Fixed

- Replacing C terminal emulation with vt10x - a go project

## [0.5.2] - 2020-12-21

### Fixed

- crash on very active, app switching clients
- daylight hours are now increasesing


## [0.5.1] - 2020-12-16

### Fixed

- Continous integration for generating binaries

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
