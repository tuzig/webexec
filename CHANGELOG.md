# Change Log

All notable changes to this project will be documented in this file, 

webexec adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

his file's format is define in 
[Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
and the release workflow reads it to set github's release notes.


## [1.2.2] 2023-9-11

### Fixed

- after install, the agent was exiting with an error

## [1.2.1] 2023-9-10

### Fixed 

- first marker restore 

## [1.2.0] 2023-9-6

### Added

- broadcasting layout changes to all connected peers
- rename admin command to rename a peer

### Fixed

- welcome message newlines
- restore buffer now sends the buffer only to the requesting peer
- upgrading using the installer

## [1.1.1] 2023-8-15

### Fixed

- New lines in welcome message

## [1.1.0] 2023-8-14

### Added

- The upgrade subcommand
- Welcome message on connect
- Message when a new version is available
- A uri that returns the latest version in https://version.webexec.sh/latest

## [1.0.1] 2023-8-3

### Fixed 

- The install script

## [1.0.0] 2023-7-31

### Fixed

- Use the standard "Authorization" header for "Bearer <token>"

### Changed

- Supporting PeerBook 1.3

## [0.19.2] 2022-3-7

### Changed

- Ice servers list is now passed using a getter function

## [0.19.1] 2022-3-3

### Changed

- Auth backend IsAuthorized method is now using ellipses

## [0.19.0] 2022-3-1

### Added 

- split to two subpackages: httpserver & peers

## [0.18.0] 2022-1-14

### Added

- Restored peerbook support. Now in beta, peerbook service includes
behind-the-NAT connection and 2FA protected address book and a 
websocket signaling server.

## [0.17.14] 2022-1-8

### Fixed

- READY is sent only when agent is ready
- A race condition on with `webexec accept` on a LAN

## [0.17.13] 2022-11-27

### Fixed

- Restore default ICE server configuration as EC2s don't know their address

## [0.17.12] 2022-11-08

### Fixed

- Simplifyied the installer and make it work for Mac & Linux

## [0.17.11] 2022-11-08

### Added

- Notarization of Mac binaries

## [0.17.10] 2022-10-24

### Added

- accept command emits a line with "READY" when it's ready to accept offers
and candidates

### Fixed 

- accept command exits on 0 length candidate

## [0.17.9] 2022-10-06

### Fixed

- updted the install script which was stuck on 0.17.6

### [0.17.8] 2022-09-30

### Fixed

- the accept command for SSH based signaling

### Added

- a docker compose based automated testing infrastructure
- an automated acceptance test procedure for `webexec accept`

## [0.17.7] 2022-09-18

### Fixed

- pid file is at ~/.local/run/webexec.pid
- socket file is at ~/.local/run/webexec.sock
- log file is called `webexec.log`

## [0.17.6] 2022-09-12

### Fixed

- run files are stored in ~/.local/run and ~/.local/log

## [0.17.5] 2022-08-13

### Fixed

- installer messages are much improved on a mac

## [0.17.4] 2022-08-13

### Fixed

- run files such as .pid and .sock are in /var/run/webexec.<user>
- Peerbook connection is much stabler

## [0.17.2] 2022-05-06

### Fixed 

- a silly bug in installation script

## [0.17.1] 2022-05-03

### Fixed 

- a silly bug in the changelog

## [0.17.0] 2022-05-03

### Added 

- The accept command, enabling using SSH for signaling

### Fixed 

- moving `authorized_tokens` file to `authorized_fingerprints`
- Try to connect even no turn servers are available
- Handling of PeerBook disconnection
- MacOS one line installer

## [0.16.0] 2022-01-12

### Added

- Client can send "*" in the shell to use user's default shell

## [0.15.5] 2022-01-10

### Fixed

- Installer on WSL2

## [0.15.4] 2022-01-04

### Added

- Extending support of old/minimized linux distribuitions
- Restored github release changelog

## [0.15.3] 2021-12-23

### Fixed

- SystemD template and rendering

## [0.15.2] 2021-12-22

### Fixed

- Increasing PeerBook connection handshake timeout

## [0.15.1] 2021-12-14

### Fixed 

- Added missing files 

## [0.15.0] 2021-12-14

### Added

- Using TURN server from PeerBook

## [0.14.1] 2021-12-07

### Fixed

- the tests
- linux installer
- linux boot script was fixed and refactored to systemd

## [0.14.0] 2021-12-06

### Added

- the init command

## [0.13.3] 2021-12-06

### Fixed

- MacOS one line install - with service

## [0.13.2] 2021-11-30

### Fixed

- MacOS one line install - still no service

## [0.13.1] 2021-11-25

## Fixed 

- the release workflow

## [0.13.0] 2021-11-25

### Added

- one line installer
- init scripts for linux & mac
- letting the client start a pane in the directory of a "parent" pane

### Fixed

- creating config only on start and improving messaging

## [0.12.1] - 2021-11-17

### Fixed

- installation script 

## [0.12.0] - 2021-11-16

### Added 

- one-line-installer
- init.d script 

## [0.11.1] - 2021-11-15

### Fixed

- updated dependencies based on the workflow's recommendation

## [0.11.0] - 2021-11-15

### Removed

- no more init command; files are created on first run

### Added

- add_pane and reconnect_pane commands
- free pass for localhost - no need to add their key

### Fixed

- crash on on closing an already closed peerbook connection
- new pane flow was streamlined to fix [re]connection failures
- a send thread was added and data is no longer lost on high load
  (TODO: add a high load test)

## [0.10.20] - 2021-6-13

### Fixed 

- generate aythors script no returns 0 on success

## [0.10.19] - 2021-6-13

### ADDED 

- change log entries (TODO: to add a local .git/hooks to test this)

## [0.10.18] - 2021-6-13

### Fixed

- a typo that failed the release action
 
## [0.10.17] - 2021-6-13

### Added 

- github action to generate authors (based on work done for pion/.goassets)

## [0.10.16] - 2021-5-22

### Added

- disconnecting peer on authorization removal

### Fixed

- passing candidates regardless of signalling state (which is unreliable)

## [0.10.15] - 2021-5-22

### Fixed

- Fixing Dup2 conmpatability

## [0.10.14] - 2021-5-22

### Fixed 

- upgrading github action's go to 1.16

## [0.10.13] - 2021-5-21

### Fixed 

- agent.err should work better and capture panix on darwin as well
- init command now insists on starting with a fresh conf folder
- improved peerbook support
- default log level is info for now

## [0.10.12] - 2021-5-19

### Added

- A changlelog entry

## [0.10.10] - 2021-5-19

### Fixed

- (probably) binary failed to run on darwin

## [0.10.9] - 2021-5-19

### Changed

- supporting latest peerbook protocol i.e, adding "peer_upadte" hanfling

### Fixed

- Improved connection speed (~200ms quicker)
- Better logs

## [0.10.8] - 2021-5-4

### Fixed

- improved error handling of peerbook's errors
- improved websoucket buffer's and timeout

## [0.10.7] - 2021-5-2

### Fixed

- the previous version translatted "forever" as twice

## [0.10.6] - 2021-5-2

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
