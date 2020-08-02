# webexec


webexec is a server daemon that acts as a webrtc peer to executs commands
and pipes their i/o over webrtc's data channels.

IMPORTANT: Webexec is still not ready for darwin. It runs on the platform but
authenticates everyone. 

## flow

Clients start by POSting a request to `/connect` with their webrtc connection
offer. webexec will then listen to incoming connection requests from that peer.
Once a peer connection is esatblished, the client opens a control data
channel. This channel is used to:

- authenticate the user
- start a command, optionally over a pseudo terminal
- resize terminals
- pass the clipboard
- ...

The control channel is labeled `%` and once it's open, the client sends in 
an Auth message with a username and a secret. the secret can either be a plain
text passowd or a hasshed version equal to the user's hash in /etc/shadow.
Upon authentication, the server sends an Ack message with the hashed password
in the response. If the client uses permanent storage he should 
store this hash and not the plain text password.

Once authenticated, the client can open as many data channels as he sees fit.

## Data Channels

webexec clients use WebRTC's channel name to pass the command to exec.
It can be used for simple commands like `echo hello world` or an interactive
shell like `zsh`. 

If the channel name starts with a digit, it means there's first a
dimension, e.g. `24x80 zsh`, it will open a pseudo tty and exec
shells and editors over it.
After recieving the request and starting the process as the authenicated user,
webexec send a message with the channel `Num`. This number is a unique id 
used to reconnect to a channel - soon the client will be able to reconnect 
to a channel with a message like `24x80 >12` to reconnect to channel 12. 
When the peer is disocnnected, webexec buffers command output.

Development
-----------

We welcome issues and PRs. Please run `go test ./server` before
commiting.
