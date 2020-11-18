# webexec

![Test](https://github.com/tuzig/webexec/workflows/Test/badge.svg)

webexec acts as a WebRTC peer and http server, listen for connection requests,
executes commands and pipes their I/O over WebRTC data channels.

webexec exposes TCP port 7777 by default, to support signalling.
There's a single endpoint `/connect`: The client exchanges tokens with the
server and then initiates a WebRTC connection.

## Running

To create the binary run `make build`. To install webexec on system wide
`/usr/local/bin` run `make install`

## Flow

1. Client -> Token -> Server (HTTP)
2. Server -> Token -> Client (HTTP)
3. Client -> Connect -> Server (WebRTC, using tokens)

Once the control data channel is open, it can be used to:


- Authenticate the user
- Start a command, optionally over a pseudo terminal
- Resize panes
- Pass clipboard content
- ...

The control channel is labeled `%`. Initially the client should send an AUTH
message with, and the server will verify that the client token is found in
`~/.webexec/authorized_tokens`. Up successful authentication, the server will
send an ACK message with the client latest payload in the `body` of the
response.

Once authenticated, there's no limit on the number the client data channels.

## Panes

A pane is a process running client command, a pane also holds the current
active data channels communicating with it.

Clients use WebRTC's channel name to specify the command and tty, if any.

webexec parses the data channel label by splitting it with commas.

In the simple case, data is passed to exec and input and output are piped over
WebRTC i.e. `echo,Hello World`

In the more advanced case, the first value is a tty dimension as in "24x80".
In this case, webexec uses [the pty package](https://github.com/creack/pty) to
exec the command over a pseudo terminal  i.e. `24x80,zsh`.

After starting a process webexec sends a message with the pane's id (a number),
a comma and the dimension.

To reconnect to pane 12 client opens a data channel with a ">12" as a label.

When the peer disconnects, webexec buffers command output.

Development
-----------

We welcome bug reports as well as ideas for new features.
If you are ready to code yourslef, follow these steps:

1. Fork it
2. Create your feature branch (git checkout -b my-new-feature)
3. Commit your changes (git commit -am 'Add some feature')
4. Push to the branch (git push origin my-new-feature)
5. Create new Pull Request

Please run `go test -v ./...` before pushing.
