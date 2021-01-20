# Security

Tight security is one of the design goals of webexec and its client, Terminal 7.
To acheieve it we rely on webrtc's builtin encryption and certificates.

webrtc does not include signalling so webexec implments it over http. 
webexec listens for requests coming on a dedicated port - 7777 by default.
The only API endpoint is `/connect`, accepting post requests.
These request include the finger print of the client's certificate.
If the fingerprint is in `/.webexec/authorized_tokens` the request is accepetd
and webexec starts listening for webrtc connection. 
Upon connection, webexec getsa session description with the fingerprint.
If it's different then the one used in the initial request, the connection is 
closed.

After the connection is approved, all encryption is done by DTLS which is the 
datagram version of the TLS protcol and is intended to provide similar security 
guarantees.

For detailed api, please refer to the [api doc](api.md).

If you have any security concerns please conntact us at
[security@tuzig.com](mailto:security@tuzig.com).
