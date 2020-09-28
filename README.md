# webexec


webexec is a user's agents that acts as a webrtc peer to executs commands
and pipes their i/o over webrtc's data channels. 

For signalling webexec using an http server, typiclly at port 7777. There's a
single endpoint `/connect` where the client posts to with his token and the 
server reply with his, so the client can intiate the WebRTC connection.

## flow

Clients start by POSting a request to `/connect` with their webrtc connection
offer. webexec will then listen to incoming connection requests from that peer.
Once a peer connection is esatblished, the client opens a control data
channel. This channel is used to:

- authenticate the user
- start a command, optionally over a pseudo terminal
- resize panes
- pass the clipboard
- ...

The control channel is labeled `%` and once it's open, the client sends in 
an Auth message with his token. When webexec receives such a message, it reads
`~/.webexec/authorized_tokens` and looks for the client's token. 
When found, webexec sends an Ack message with the client's last payload in the
`body` of the response. 

Once authenticated, the client can open as many data channels as he sees fit.

## Panes

webexec keeps track of Panes where ach pane has its own process running the
client's command and a slice of data channels. 
Clients use WebRTC's channel name to specify the command and the tty, if any.
webexec parses the data channel label by slicing it by commas.
In the simple case, the parsed slice is passed to exec and input and output
are piped over WebRTC i.e. `echo,Hello World`

In the more advanced case, the first value is a tty dimension as in "24x80".
In this case, webexec uses https://github.com/creack/pty to exec the command
over a pseudo terminal  i.e. `24x80,zsh`.

After recieving the request and starting the process 
webexec sends a message with the pane's id, a comma and the dimension. 
The id is a number used for resize and reconnect. 
To reconnect to pane 12 client opens a data channel with a ">12" as a label.
When the peer is disocnnected, webexec buffers command output.

Development
-----------

We welcome issues with bugs or features and of course PRs.
Please run `go test` before commiting.
