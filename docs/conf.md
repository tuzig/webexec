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
- ice_servers: a List of ice servers' urls
- udp_port_min: the minimum UDP port to use
- udp_port_max: the maximum UDP port to use

### timeouts

All values are in Milliseconds

- disconnect: the disconnect timeout, default: 3000
- failed: the failed timeout, default: 6000
- keep_alive: how long to wait between keep alive messages, default 500
- ice_gathering: gathering timeout, default 5000

### env 

This section include environment variables and their values. These vars will be
set for each new command launched. Default:

``` toml
...
[env]
COLORTERM = "truecolor"
TERM = "xterm"
```

### peerbook

The peerbook section is used to setup peerbook params. peerbook is a server
that keeps a live address book of users' peers. peerbook uses websocket
to forward offers and answers between clients and webexec.

- `email`: the user's email 
- `host`: the host address. default is `pb.terminal7.dev`
- `name`: the host's name default is the system's hostname




