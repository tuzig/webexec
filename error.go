package main

// UnauthorizedPeer is an error
type errUnauthorized struct{}

func (e *errUnauthorized) Error() string {
	return "peerbook did not recognized my fingerprint. a crow has been sent. check inbox"
}
