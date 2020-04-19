webexec
=======

webexec is a user's server daemon that executs commands and pipes them over
webrtc.

webexec is both an http and a webrtc server. the http server is used to start a
session. Upon a POST request, webexec creates an offer and sends it to the
client. On getting and answear, the srever is waitin for a data channel. 

Next, the client opens a bi-directional data channel with the command to exec
as the label. The protocol support 65536 chars so the client can pass scipts.

TODO: If the label starts with a `>` use a pseudo terminal

Installation
------------

```console
    $ go get github.com/afittestide/webexec
    $ webexec -h

```

Development
-----------

The code started with pion/webrtc example for using a bi directional channel.
When a channel is opened the server executes the command encoded in the channel
name. The output of the command is sent back to the web client over webrtc and
messages received from the client are sent downstream to the command's input.

We welcome new features and bug fixes. Please run `go test` before commiting.
