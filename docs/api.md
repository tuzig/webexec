# webexec APIv1

The webexec server lets a client exec commands on a remote server and pipe their
input & output over a web real time communication (aka webrtc) peer connection. 
As signaling is not part of webrtc, this server also uses http for signaling.

	
As for piping of command's input & output, webexec uses 
http://github.com/creack/pty to start commands over a pseduo terminal. 
Upon success, webexec creates a data channel with the channel id as label.
This channel is used to receive key presses and send back stdout updates.
When a command exits, webexec closes the data channel, when the user
asks to close the pane, the client closes the data channel and the server
kills the process.

## HTTP API

In this version the http API contains a single endpooint: `/connect`.
The endpoint accepts a POST requests with the client's offer in the body as 
`plain/text`.
Upong receiving a request the server lists for incoming 
connection request from the client and return the server's token in
then http response.

## WebRTC API

After receiving the server's offer using HTTP API, the client establishs
a webrtc peer connection.
After connecting, the client opens a bi-directional
command & control data channel, labeled `%`, authenticates over it and
starts sending layout updates that can result in pseudo terminal resize
, new command execution or killing of running commands.

While webrtc data channels are peer to peer, our protocol is a client-server
one. The client send a message with a command and the server replies 
with either an ack message or an error.

To better track performance and identify disconnection each message has a
"time" key with milliescond since epoch.

### Client Messages

#### Authenticate

```json
{
    "time" : 1257894000000, 
    "message_id": 123,
    "auth": {
        "username" : "yossi",
        "secret" : "hamelech"
    }
}
```

#### Layout

On startup and whenever the client layout is changed, the client sends this
message to the server. The server stores keeps the last layout and sends it
back to the client on re-connect.

```json
{
    "time" : 1257894000000, 
    "message_id": 456,
    "layout": {"tab 1": "80x24,0,0,1",
               "tab 2": "238x54,0,0{119x54,0,0,14,118x54,120,0, 15}"
              }
}
```
`layout` is an object where every tab name is a key whose value is the 
window's layout format. The format is copied from the `window_layout` 
formating variable in the great tmux.
For example a classic ` |-` three pane layout on a 238x54 terminal is: 

`238x54,0,0{119x54,0,0,14,118x54,120,0[118x27,120,0,17,118x26,120,28,18]}`

The string is made up from cells of two type: layouts and panes. Layouts are
non-visible containers used to hold panes of the same alignment 
while panes are the terminal themselves.
Both cells begin with a dimension as in: `80x24`, followed by a comma sperated 
X offest and Y offset. For panes, the offset are followed by the channel's label 
or `TBD` for a newly cerated channel.

If the cell is a layout the Y offset is followed by either 
`{<cell 1>, <cell 2>, ..}` or `[<cell 1>, <cell 2>, ...]`.
The first is used for a vertical layout and the second for a horizontal one.

Each json string amy can contain one pane with `TBD` as it's label. If it does,
the server will start a new shell and open a new data channel to connect the 
shell's pseduo terminal with the client.
set to the new pane id. The client should close this channel when the user's 
closes the pane. The server 

### Server Messages

#### Authenticate Ack

When the server receives an authentication requests and succeedes in 
authenticating, it replies with an Ack containing a reference to the original
message - `ref`, the user's token and
the active layout, of there is one:

```json
{
    "time" : 1257894000000, 
    "message_id": 123,
    "ack": {
        "ref": 12,
        "token": "$vlwvK2LNW0LnfR9GO6p464FkGpOikd1q...",
        "layout": { "tab 1": "238x54,0,0{119x54,0,0,14,118x54,120,0[118x27,120,0,17,118x26,120,28,18]}",
                    "root": "238x54,0,0,TBD"
                  }
    }
}
```

#### Error

When the server encounters an error it sends an error message to the client:

```json
{
    "time" : 1257894000000,
    "message_id": 124,
    "error": {
        "ref": 12,
        "description": "Stack Overflow"
    }
}
```
