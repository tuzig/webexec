# webexec API

The webexec server executes client commands on a remote server and pipes their
input & output over a WebRTC peer connections.
If the client provides terminal dimenstions commands are connected 
through a pseudo tty. The client can change the dimensions through a `resize`
command.

Each command can be connected to mutiple clients, multi casting output to all 
connected data channels and receiving input from them all.

To better support clients synchronization, webexec saves a payload that 
clients can get and set, This was added on request from out first frontend -
Terminal 7 - that needed a way to store the screen layout.

	
## HTTP API

The HTTP API contains a single endpoint: `/connect`.

The endpoint accepts a POST requests with the a json encoded body with
three fields: 
- `offer` a base64 client's offer to connect. 
- `fingerprint` holds the public key of the client's certificate. 
- `api_version` for the API version
 
```json
{
  "api_version": 1,
  "fingerprint": "sha-256 B5:00:66:8D:0D:53:0E:F2:8B:D6:70:AF:AA:14:63:6F:B7:F7:E9:B0:54:20:FB:5D:5C:1F:33:28:69:51:2C:CD",
  "offer":  "FGFGFGFG..."
}

```

Upon receiving a request, webexec checks if the token is in 
`~/.webexec/autherized_tokens`. If it's not, the server will reply
with a 401 error code.  If it's in the file, webexec will start listening for
a webrtc peer connection request that offer and reply with his webrtc answer
in the http body.

After the client connects, webexec ensure the same fingerprint is used by 
the peer connection.


## WebRTC API

After receiving the server's offer using HTTP API, the client establishes
a webrtc peer connection. Once connected, the client can execute commands 
by opening data channels that connect it with a pane.


## Control Channel

Smart clients can do more than just exec commands. They can resize pane,
mark & restore and access a payload to use as it pleases. To send these commands
the client opens a bi-directional command & control data channel, labeled
`%`.

Webexec replies to each command with with a `ack` or a `nack` message.

### Add Pane

A pane is the basic an object that connects a process, a pseudo tty and a set of 
data channels.  When a client wants to create a new pane it sends this message with the 
command to run, the dimensions and an optional parent pane.
After receiving this message webexec will create the pseudo tty, start 
the commend and open a data channel.

The new channel's label includes two integers separated by a `:`. 
The first integer is the message_id and the second is the server's id to be
used in future commands.

For example, a cdc message like:

```json
{
  "message_id": 123,
  "type": "add_pane",
  "args": {
    "sx": 80,
    "sy": 24,
    "command": "zsh",
    "parent": 123
  }
}
```

will get the server to start a new zsh connected through a psedu tty to a newly
created data channel with the label "123:89". 

If command is "*" webexec willl start the user's defualt shell

The message's ack will have the pane's id in the body.

### Reconnect to  Pane

To restore connection to a previously opened pane use the reconnect message:

```json
{
  "message_id": 123,
  "type": "reconnect_pane",
  "args": {
    "id": 56
  }
}
```

### Mark

When a client knows it is about to disconnect he should send a mark message
to make the restore seemles. Upon recieving this message webexec will stop
sending output to the client and mark all the connected output buffer location
with a `marker_id`. This is sent as an int in the `body` field of the 
Ack message.


### Restore

This request is used when reconnecting after an orderly disconnect.
this request requires a marker recieved in the ack to the "mark" message.
After getting this message and new channels the client reconnects to will first
be send all ithe output since the marker was received.

Example JSON request:

```json
{
  "time": 1257894000000,
  "message_id": 12,
  "type": "restore",
  "args": {
    "marker": 123
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
