# webexec


webexec is a server daemon that executs commands and pipes their input and 
output over webrtc.

## flow

Clients start by ssh-ing into their development machine and starting webexec 
with their offer key (encoded SDP) as their arg:

```console

    $ ssh user$example.com webexec <offer>
    <server key>

```

webexec than will look for the local webexec daemon and create it if it's not
there. Next, the cli will send the daemon a request to listen for a connection
request from offer. The server gets the listen (and other) requests over a unix
socket and then listens for the offer and return the server's key.
Upon getting the server key the client esatblishs a WebRTC peer connection
and opens multiple data channels.

## Data Channels

webexec clients use WebRTC's channel  name to pass the command to exec.
It can be used for simple commands like `echo hello world`. Or, if you 
start with a dimension, e.g. `24x80 zsh` it will open a pseudo tty and exec
shells and editors over it.
When a data channel is opened, the first message webexec sends is the channel
id. One use for this id is to reconnect to an existing channel,
e.g. `24x80 >12` to reconnect to channel 12. 
When the peer is disocnnected, webexec buffers command output and use it to 
refresh the peer upon reconnection.


## Control Channel

There's also a special channel named `%` for control commands & notifications.
The control channel is used by clients for commands such as terminal resize and 
clipboard update.

## HTTP Server

The HTTP Server is a tool to help developers test the code locally.
It's unsecured and should nopt be used in production.

The http server bypasses the need for an ssh connection, making it all play
in the browser. To connect the client POSTs to `/connect`
with a `{"offer": <offfer>}` body. webexec than creates an offer and sends it 
to the client. On getting and answer, the srever is waitin for a data channel. 

Installation
------------

```console
    $ go get github.com/afittestide/webexec
    $ webexec -h

```

Development
-----------

We welcome issues and PRs. Please run `go test ./server` before
commiting.
