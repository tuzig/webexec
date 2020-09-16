package main

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// MT: I use https://godoc.org/github.com/stretchr/testify/require which
// reduces a lot of boilerplate code in testing
func TestHTTPEndpoint(t *testing.T) {
	InitLogger()
	// Start the https server
	go func() {
		err := HTTPGo("0.0.0.0:7778")
		require.Nil(t, err, "HTTP Listen and Server returns an error: %q", err)
	}()

	// Prepare & POST to /connect
	offer := []byte("eyJ0eXBlIjoib2ZmZXIiLCJzZHAiOiJ2PTBcclxubz1tb3ppbGxhLi4uVEhJU19JU19TRFBBUlRBLTc1LjAgNjc3NjI2MzkwMDE0ODUwODY0OCAwIElOIElQNCAwLjAuMC4wXHJcbnM9LVxyXG50PTAgMFxyXG5hPXNlbmRyZWN2XHJcbmE9ZmluZ2VycHJpbnQ6c2hhLTI1NiAwMzpBNzoyNDo1RjpGQToyQTpFODo0NDo4MTowNzozMjpFRjoxRDoxMDo1NDpFMzoxNjo3NjpCRToyOTpGRTo4QzpGOTpFNjpFRDo1Qjo0MjpGMjpDMzpEMTpENjozNlxyXG5hPWdyb3VwOkJVTkRMRSAwXHJcbmE9aWNlLW9wdGlvbnM6dHJpY2tsZVxyXG5hPW1zaWQtc2VtYW50aWM6V01TICpcclxubT1hcHBsaWNhdGlvbiAxNTkwMiBVRFAvRFRMUy9TQ1RQIHdlYnJ0Yy1kYXRhY2hhbm5lbFxyXG5jPUlOIElQNCA1LjIyLjEzNS4yNlxyXG5hPWNhbmRpZGF0ZTowIDEgVURQIDIxMjIyNTI1NDMgZGY4NjQ1ZGQtNDRiZC00ZDRhLThhMDgtY2ExZTM0MTQ5ZTUxLmxvY2FsIDQ2NjUyIHR5cCBob3N0XHJcbmE9Y2FuZGlkYXRlOjIgMSBUQ1AgMjEwNTUyNDQ3OSBkZjg2NDVkZC00NGJkLTRkNGEtOGEwOC1jYTFlMzQxNDllNTEubG9jYWwgOSB0eXAgaG9zdCB0Y3B0eXBlIGFjdGl2ZVxyXG5hPWNhbmRpZGF0ZToxIDEgVURQIDE2ODYwNTI4NjMgNS4yMi4xMzUuMjYgMTU5MDIgdHlwIHNyZmx4IHJhZGRyIDAuMC4wLjAgcnBvcnQgMFxyXG5hPXNlbmRyZWN2XHJcbmE9ZW5kLW9mLWNhbmRpZGF0ZXNcclxuYT1pY2UtcHdkOmNiOTM2ZDdmNjNkYTYzMGE1ZWJjZDc3ZGNiOGFkMTQyXHJcbmE9aWNlLXVmcmFnOjEwN2JlOWVmXHJcbmE9bWlkOjBcclxuYT1zZXR1cDphY3RwYXNzXHJcbmE9c2N0cC1wb3J0OjUwMDBcclxuYT1tYXgtbWVzc2FnZS1zaXplOjEwNzM3NDE4MjNcclxuIn0=")
	time.Sleep(A_BIT)

	r, err := http.Post("http://127.0.0.1:7778/connect", "application/json",
		bytes.NewReader(offer))
	require.Nil(t, err, "Failed sending a post request: %q", err)
	defer r.Body.Close()
	require.Equal(t, r.StatusCode, http.StatusOK,
		"Server returned not ok status: %v", r.Status)
	// If you're using bs just to count bytes, use io.Copy with io/ioutil/Dicard
	serverOffer, err := ioutil.ReadAll(r.Body)
	require.Nil(t, err, "Failed reading resonse body: %v", err)
	require.Less(t, 1000, len(serverOffer),
		"Got a bad length response: %d", len(serverOffer))
	/*
		// There's t.Cleanup in go 1.15+
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		err := Shutdown(ctx)
		require.Nil(t, err, "Failed shutting the http server: %v", err)
	*/

}
