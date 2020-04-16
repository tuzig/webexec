webexec
=======

webexec is a user's server daemon that executs commands and pipes them over
webrtc.

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
