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

The easiest way to install is to download the 
[latest release](https://github.com/tuzig/webexec/releases) tar ball for your
system and extract it to get the `wbeexec` binary. 
We recommended moving webexec to a system-wide tools folder such as 
`/usr/local/bin`.

Before first run you need to run `webexec init` to create `~/.webexec` 
and there `webexec.conf`. After init you can run `webexec start` to launch the agent.
For other webexec commands run `webexec`.

webexec communicates over UDP ports, so if the server is behind a firewall
you'll have to allow ingress UDP traffic.
The default ports are the range 7000-7777.
If you prefer another range you can set the `udp_port_min` and `udp_port_max`
in the `[net]` section of the conf file and `webexec restart`.

For direct connections you'll also need to open TCP port 7777 you'd like clients
to using a static address.

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
