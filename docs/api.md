# webexec API

The webexec server executes client commands on a remote server and pipe their
input & output over a web real time communication (aka WebRTC) connection.

webexec extends the WebRTC protocol to support signals over HTTP.
	
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

## HTTP API

The HTTP API contains a single endpoint: `/connect`.

The endpoint accepts a POST requests with the client's offer in the body as 
`plain/text`.

Upon receiving a request webexec listens for incoming connection requests from
the client and return the server's token in the HTTP response.

## WebRTC API

After receiving the server's offer using HTTP API, the client establishes
a webrtc peer connection.

Next, the client opens a bi-directional command & control data channel, labeled
`%`.

While WebRTC data channels are peer to peer, webexec protocol is a client-server
one. The client send a message with a command and the server replies 
with either an ACK message or an error.

Each message includes a `time` key, as milliseconds since epoch, to track
performance and identify disconnects.

### Authentication

Example JSON request:

```json
{
  "time": 1257894000000,
  "message_id": 123,
  "type": "auth",
  "args": {
    "token": "l;sdfjkghqop3i5utqiowrdhjfklasdjfhopqwi9rtujipw",
    "marker": 123
  }
}
```

The token should exist in `~/.webexec/autherized_tokens`. webexec will reply
with an ACK that includes the latest payload.

The `marker` field is optional and is used orderly restore.
If the client was lucky to do an orderly shutdown, he sent a `mark` command 
and got a marker in the body of the ack. Upon a data channel reconnect, t
his marker is used to collect all the output the client missed and send it over.

Example JSON reply:

```json
{
  "time": 1257894000000,
  "message_id": 123,
  "type": "ack",
  "args": {
    "ref": 12,
    "body": <payload>
  }
}
```

### Mark

When a client knows it is about to siconnect he should send a mark message
to make the restore seemles. Upon recieving this message webexec will stop
sending output to the client and mark all the connected output buffer location
with a `marker_id`. This is sent as an int in the `body` field of the 
Ack message.


### Resize

The resize message lets the client change the dimensions of a pane.

```json
{
  "time": 1257894000000,
  "message_id": 123,
  "type": "resize",
  "args": {
    "sx": 123,
    "sy": 45
  }
}
```

### Payload

To synchronize with other connected clients, webexec saves and restores client
payloads. Clients can use the payload to store information about screen layout,
window tabs, etc.

Example request:

```json
{
  "time": 1257894000000,
  "message_id": 456,
  "type": "get_payload",
  "args": {}
}
```

Example ACK reply:

```json
{
  "time": 1257894000000,
  "message_id": 678,
  "type": "ack",
  "args": {
    "ref": 456,
    "body": <payload>
  }
}
```

Client should set the payload message to change the payload.

```json
{
  "time": 1257894000000,
  "message_id": 456,
  "type": "set_payload",
  "args": {
    "payload": <client payload>
  }
}
```

### NACK

When the server encounters an error it sends a [NACK](https://webrtcglossary.com/nack/) message to the client:


```json
{
  "time": 1257894000000,
  "message_id": 124,
  "type": "nack",
  "args": {
    "ref": 12,
    "description": "Stack Overflow"
  }
}
```
