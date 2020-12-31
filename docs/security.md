# Security

Tight security is one of the design goals of webexec and its client, Terminal 7.
To acheieve it we rely on webrtc's builtin encryption and client
tokens.

webrtc does not include signalling so webexec implments it over http. 
webexec listens for post requests coming on a dedicated port - 7777 by default.
The only API endpoint is `/connect` where post reaqests are accepted,
expecting a vlid client's offer in the body.
Upon recieving the request, webexec starts listening to the offer and
replies with an answear which the client uses to make the webrtc connections.

![webexec authorization flow diagram](images/auth_flow.png)

webexec marks new connections as "unauthorized". To authorize, the client opens
a data channel labeled "%" for command & control messages. Over this channel the 
client sends the "auth" message with its token. Client should generate The token 
on the first run and store them for future connections.

webexec chacks if the token is one of the lines
in `~/.webexec/authorized_tokens` and if so, authorizes the connection. 

The token, program output and keyboard  input are all is secured by webertc's
underlying DTLS, a protocol defined by RFC 6347: "The DTLS protocol is based on
the Transport Layer Security (TLS) protocol and provides equivalent security
guarantees.". 

For detailed api, please refer to the [api doc](api.md).

If you have any security concerns please conntact us at
[security@tuzig.com](mailto:security@tuzig.com).
