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

### Panes

webexec holds a table of panes and the client can CRUD them as he wishes.

### Update Panes

The update panes message let's a client resize panes

```json
{
  "time": 1257894000000,
  "message_id": 123,
  "type": "update_panes",
  "args": [
    {"id": 123,
     "sx": 80,
     "sy": 24,
     "payload": {} },
    {"id": 124,
     "sx": 120,
     "sy": 48,
     "payload": {}}
  ]
}
```

### Create Pane

When a client wants to create a new pane it sends this message with the 
command to run, the dimensions and an optinal parent pane. If provided,
the new pane will inherit it's working directory from the parent.
After receiving this message webexec will create the pseudo tty and start 
the commend.  If it succeeds it will reply with an ack where the body contains
the pane's id.  To start communicating with the pane the clients needs to open
a data channel 

```json
{
  "time": 1257894000000,
  "message_id": 123,
  "type": "create_pane",
  "args": {
    "sx": 80,
    "sy": 24,
    "command": "zsh",
    "payload": {},
    "based_on": 123
  }

}
```
### Delete Panes

Some panes die when their command exits some die because of this message.

```json
{
  "time": 1257894000000,
  "message_id": 123,
  "type": "delete_panes",
  "args": [ 123, 124]
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
