# webexec

![Test](https://github.com/tuzig/webexec/workflows/Test/badge.svg)

webexec is a terminal server running over WebRTC with http for signalling.
Webexec listens for connection requests, executes commands over pseudo ttys
and pipes their I/O over WebRTC data channels.

webexec is currently single session - each client that authenticates has
access to the same list of "Panes" and layout geomtry. 

webexec exposes TCP port 7777 by default, to support signalling.
There's a single endpoint `/connect`: The client exchanges tokens with the
server and then initiates a WebRTC connection.

## Install

This repo inscludes a [one-line installer](install.sh) for installing on Mac & Linux.
To use to install the latest version it run `curl https://get.webexec.sh | bash`.


### from the source

```

$ go install ./...

```

# Firewal

webexec has a signlaing server that listen for connection request in
ports 7777. Once communication is established, WebRTC uses UDP ports with 
a default range of 60000-61000.
If you prefer another range you can set the `udp_port_min` and `udp_port_max`
in the `[net]` section of the conf file and `webexec restart`.

[TODO: add instructions as to how to make it run on boot]

## Contributing

We welcome bug reports, ideas for new features and pull requests.

If you are new to open source, DON'T PANIC. Just follow these simple
steps:

1. Join the discussion at our [discord server](https://discord.gg/GneEDB7ZZQ)
2. Fork it and clone it `git clone <your-fork-url>`
3. Create your feature branch `git checkout -b my-new-feature`
4. Write tests for your new feature and run them `go test -v`
5. Commit the failed tests `git commit -am 'Testing ... '`
6. Write the code that psses the tests and commit 
7. Push to the branch `git push --set-upstream origin my-new-feature`
8. Create a new Pull Request
