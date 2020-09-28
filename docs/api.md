# webexec API


The webexec server lets a client run commands on a remote server and pipe their
input & output over a web real time communication (aka webrtc) connection. 
As signaling is not part of webrtc, webexec uses http for signaling.

	
 HTTP API

In this version the http API contains a single endpooint: `/connect`.
The endpoint accepts a POST requests with the client's offer in the body as 
`plain/text`.
Upong receiving a request webexec listens for incoming 
connection requests from the client and return the server's token in
then http response.

## WebRTC API

After receiving the server's offer using HTTP API, the client establishs
a webrtc peer connection.
Next, the client opens a bi-directional
command & control data channel, labeled `%`.

While webrtc data channels are peer to peer, our protocol is a client-server
one. The client send a message with a command and the server replies 
with either an ack message or an error.

To better track performance and identify disconnection each message includes a
"time" key with milliescond since epoch.

### Authenticate

```json
{
    "time" : 1257894000000, 
    "message_id": 123,
    "type": "auth",
    "args": {
        "token" : "l;sdfjkghqop3i5utqiowrdhjfklasdjfhopqwi9rtujipw"
    }
}
```

If the token exists in `~/.webexec/autherized_tokens` webexec replies with 
a an ack that include the latest payload:

```json
{
    "time" : 1257894000000, 
    "message_id": 123,
    "type": "ack",
    "args": {
        "ref": 12,
        "body": <payload>
    }
}
```

### Resize

Panes that started with dimensions pipe the data through a pseudo terminal.
The resize message lets the client change the dimensions of a pane.

```json
{
    "time" : 1257894000000, 
    "message_id": 123,
    "type": "resize",
    "args": {
        "sx": 123,
        "sy": 45
    }
}
```

### Payload

Webexec saves and restore a payload for the client so it can synchoronize with
the other connected clients. Clients can use the payload to store information
about screen layout, window tabs, etc. 

```json
{
    "time" : 1257894000000, 
    "message_id": 456,
    "type": "get_payload",
    "args": {}
}
```

To which the server replies with an Ack that looks just like the Auth ack:

```json
{
    "time" : 1257894000000, 
    "message_id": 678,
    "type": "ack"
    "args": {
        "ref": 456,
        "body": <payload>
    }
}
```

To change the payload the client can send a set payload message"

```json
{
    "time" : 1257894000000, 
    "message_id": 456,
    "type": "set_payload",
    "args": {
        "payload": <client payload>
    }
}
```

## Nack

When the server encounters an error it sends a nack message to the client:

```json
    "time" : 1257894000000,
    "message_id": 124,
    "type": "nack",
    "args": {
        "ref": 12,
        "description": "Stack Overflow"
    }
}
```
