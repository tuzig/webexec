# webexec API

The webexec server executes client commands on a remote server and pipe their
input & output over a web real time communication (aka WebRTC) connection.

webexec extends the WebRTC protocol to support signals over HTTP.
	
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
    "token": "l;sdfjkghqop3i5utqiowrdhjfklasdjfhopqwi9rtujipw"
  }
}
```

The token should exist in `~/.webexec/autherized_tokens`. webexec will reply
with an ACK that includes the latest payload.

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
