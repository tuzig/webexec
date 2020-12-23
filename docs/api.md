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

### Authentication

Example JSON request:

```json
{
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
every pane has a `display` field that is reservered for client use.

#### Create Pane

When a client wants to create a new pane it sends this message with the 
command to run, the dimensions and an optional parent pane.
After receiving this message webexec will create the pseudo tty and start 
the commend.

```json
{
  "message_id": 123,
  "type": "create_pane",
  "args": {
    "cols": 80,
    "rows": 24,
    "command": "zsh",
    "display": {"xoff": 200, "yoff": 1300, "font_size": 16},
    "parent": 123
  }

}
```

If webexec succeeds it will reply with an ack where the body contains
the new pane's info: 

```json
{
  "message_id": 123,
  "type": "ack",
  "args": {
    "ref": 456,
    "body": {
     "id": 12, "rows": 24,"cols": 80, 
     "ctime": 84679246926, "display": {..} , "process_status": {..}
    }
  }
}
```
To start communicating with the pane the client opens
a data channel with a label `<id>`

If provided, the new pane will inherit it's working directory from its parent.


#### Update Panes

The update panes message let's a client resize panes's pty and display
information

```json
{
  "message_id": 123,
  "type": "update_panes",
  "args": [
    {"id": 123,
     "cols": 80,
     "rows": 24,
     "display": {....} },
    {"id": 124,
     "cols": 120,
     "rows": 48,
     "display": {..}}
  ]
}
```

#### Delete Panes

Some panes die when their command exits and some die because of this message.

```json
{
  "message_id": 123,
  "type": "delete_panes",
  "args": [ 123, 124]
}
```

#### Read Panes

When the client needs a fresh copy of the pane table he sends this command:

```json
{
  "message_id": 123,
  "type": "read_panes",
  "args": null
}
```

The reply being:

```json
{
  "message_id": 123,
  "type": "ack",
  "args": {
    "ref": 456,
    "body": [
     {"id": 12, "rows": 24,"cols": 80, 
      "ctime": 84679246926, "display": {..} , "process_status": {..}},
     {"id": 12, "rows": 24,"cols": 80, 
      "ctime": 84679246926, "display": {..} , "process_status": {..}}
    ]
  }
}
```


### NACK

When the server failes to execute a client's command it sends a 
[NACK](https://webrtcglossary.com/nack/) message to the client:


```json
{
  "message_id": 124,
  "type": "nack",
  "args": {
    "ref": 12,
    "description": "Stack Overflow"
  }
}
```
