# Security

Tight security is one of the design goals of webexec and its client, Terminal 7.
To acheieve it we rely on IETF's DTLS for encryption.
To learn more about WebRTC's security you are invited to read
[A study of WebRTC Security](https://webrtc-security.github.io).

WebRTC does not include signalling so webexec includes it's own HTTP based
signaling. 
Signaling covers the flow before establishing the WebRTC connection and is
focused on the client sending an "offer" and the webexec returning an "answer".
webexec supports two types of signalling:

## POST-based signaling

This method only works for servers with static IPs.
webexec listens for requests coming on a dedicated port - 7777 by default.
The `/connect` API endpoint accepts post requests with offers.
webexec extracts the fingerprint from the offer and test whether the
fingerprint is in `/.webexec/authorized_tokens`.
If it is, the request is accepetd, webexec replys with his answer
and waits for a webrtc connection from that client. 

## WebSocket based signaling

webexec can also use an HTTPS signaling server -
[peerbook](https://tuzig.com/tuzig/peerbook)
- to make the setup time quicker, simplify first connection and connect to
behind-the-nat server.  If the user configured the peerbook section in
`webexec.conf` webexec will use peerbook to verify itself and accept offers.

Upon start webexec will first POST to `/verify` with the peer details in the
body. If verification failed, peerbook sends the user an email with a 
short-lived-link he can use to see his list of peers and chose which to verify
or unverify. 

Once verified, webexec will keep the websocket connection open and use it
to accept offers from clients and send back candidates. Before forwarding
peerbook will check to ensure both the source and target were verified
by their user. To prevent masquerading, upon receiving an offer webexec 
extracts the peer's fingerprint and ensures it equals to the target is the one
that was signaling him.

Verified clients use the websocket connection to recieve an updated list peers
with their names, kind & fingerprints. peerbook keeps track of which peers
are online so whenever a peer's status change, all connected peer get the 
updated list.

For detailed API, please refer to the [api doc](api.md).

If you have any security concerns please conntact us at
[security@tuzig.com](mailto:security@tuzig.com).
