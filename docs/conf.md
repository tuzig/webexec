# Conf file Format

webexec stores it's setting in a toml formatted configuration file. The file is
stored in `~/.webexec/webbxec.conf` and created based on a default of first
run. 


## Sections

### log

- level: lowest log level to capture. one of: trace, info, debug, warn, error
- file: path to the log file. relatives paths start `at ~/.webexec`
- error: path to the error file. . relatives paths start `at ~/.webexec`
- pion_levels: A mapping of log level to pion components, i.e. `{ trace = "sctp" }`

### net

- http_server: The address the https server listen on. default: `0.0.0.0:7777`
- udp_port_min: the minimum UDP port to use
- udp_port_max: the maximum UDP port to use

### timeouts

All values are in Milliseconds

- ack_timeout: how long to wait for a control message ack, default: 3000
- disconnect: the disconnect timeout, default: 3000
- failed: the failed timeout, default: 6000
- keep_alive: how long to wait between keep alive messages, default 500
- ice_gathering: gathering timeout, default 5000
- peerbook: how long to wait before peerbook reconnnect, default 3000

### env 

This section include environment variables and their values. These vars will be
set for each new command launched. Default:

``` toml
...
[env]
COLORTERM = "truecolor"
TERM = "xterm"
```
### ice_server

A list of ice server and their credentials

```toml
....
[[ice_servers]]
urls = [ "turn:45.83.40.91:3478" ]
username = "terminal7"
password = "secret"
```

### peerbook

The peerbook section is used to setup peerbook params. peerbook is a server
that keeps a live address book of users' peers. peerbook uses websocket
to forward offers and answers between clients and webexec.

- `user_id`: the user's ID  
- `host`: peerbook's address. default is `api.peerbook.io`
- `name`: the host's name default is the system's hostname
