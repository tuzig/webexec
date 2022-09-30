# webexec

![Test](https://github.com/tuzig/webexec/workflows/test.yml/badge.svg)

webexec is a terminal server running over WebRTC with support for
signaling over SSH or HTTP.
Webexec listens for connection requests, executes commands over pseudo ttys
and pipes their I/O over WebRTC data channels.

webexec is currently single session - each client that authenticates has
access to the same `payload`.
In the case of [Terminal7](https://github.com/tuzig/terminal7),
our first client, it contains the layout of the panes.

webexec exposes TCP port 7777 by default, to support signalling.
There's a single endpoint `/connect`: The client exchanges tokens with the
server and then initiates a WebRTC connection.

## Install

This repo inscludes a [one-line installer](install.sh) for installing on Mac & Linux.
To use to install the latest version it run
`bash -c "$(curl -sL https://get.webexec.sh)".

### From Source

```
$ go install ./...

```

# Ports Used

webexec has a signlaing server that listen for connection request in
ports 7777. Once communication is established, WebRTC uses UDP ports with 
a default range of 60000-61000.
If you prefer another range you can set the `udp_port_min` and `udp_port_max`
in the `[net]` section of the conf file and `webexec restart`.

## Contributing

We welcome bug reports, ideas for new features and pull requests.

If you are new to open source, DON'T PANIC. Just follow these simple
steps:

1. Join the discussion at our [discord server](https://discord.gg/GneEDB7ZZQ)
2. Fork it and clone it `git clone <your-fork-url>`
3. Create your feature branch `git checkout -b my-new-feature`
4. Write tests for your new feature. We have both unit tests and integration
tests under aatp. use `go test -v` to run unit tests and `./aatp/test`
to test integration
5. Commit the failed tests `git commit -am 'Testing ... '`
6. Write the code that psses the tests and commit 
7. Push to the branch `git push --set-upstream origin my-new-feature`
8. Create a new Pull Request
