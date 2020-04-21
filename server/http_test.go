package server

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"
)

func TestHTTPAPI(t *testing.T) {
	go HTTPGo(":8888")
	c := ConnectAPI{"eyJ0eXBlIjoib2ZmZXIiLCJzZHAiOiJ2PTBcclxubz1tb3ppbGxhLi4uVEhJU19JU19TRFBBUlRBLTc1LjAgNjc3NjI2MzkwMDE0ODUwODY0OCAwIElOIElQNCAwLjAuMC4wXHJcbnM9LVxyXG50PTAgMFxyXG5hPXNlbmRyZWN2XHJcbmE9ZmluZ2VycHJpbnQ6c2hhLTI1NiAwMzpBNzoyNDo1RjpGQToyQTpFODo0NDo4MTowNzozMjpFRjoxRDoxMDo1NDpFMzoxNjo3NjpCRToyOTpGRTo4QzpGOTpFNjpFRDo1Qjo0MjpGMjpDMzpEMTpENjozNlxyXG5hPWdyb3VwOkJVTkRMRSAwXHJcbmE9aWNlLW9wdGlvbnM6dHJpY2tsZVxyXG5hPW1zaWQtc2VtYW50aWM6V01TICpcclxubT1hcHBsaWNhdGlvbiAxNTkwMiBVRFAvRFRMUy9TQ1RQIHdlYnJ0Yy1kYXRhY2hhbm5lbFxyXG5jPUlOIElQNCA1LjIyLjEzNS4yNlxyXG5hPWNhbmRpZGF0ZTowIDEgVURQIDIxMjIyNTI1NDMgZGY4NjQ1ZGQtNDRiZC00ZDRhLThhMDgtY2ExZTM0MTQ5ZTUxLmxvY2FsIDQ2NjUyIHR5cCBob3N0XHJcbmE9Y2FuZGlkYXRlOjIgMSBUQ1AgMjEwNTUyNDQ3OSBkZjg2NDVkZC00NGJkLTRkNGEtOGEwOC1jYTFlMzQxNDllNTEubG9jYWwgOSB0eXAgaG9zdCB0Y3B0eXBlIGFjdGl2ZVxyXG5hPWNhbmRpZGF0ZToxIDEgVURQIDE2ODYwNTI4NjMgNS4yMi4xMzUuMjYgMTU5MDIgdHlwIHNyZmx4IHJhZGRyIDAuMC4wLjAgcnBvcnQgMFxyXG5hPXNlbmRyZWN2XHJcbmE9ZW5kLW9mLWNhbmRpZGF0ZXNcclxuYT1pY2UtcHdkOmNiOTM2ZDdmNjNkYTYzMGE1ZWJjZDc3ZGNiOGFkMTQyXHJcbmE9aWNlLXVmcmFnOjEwN2JlOWVmXHJcbmE9bWlkOjBcclxuYT1zZXR1cDphY3RwYXNzXHJcbmE9c2N0cC1wb3J0OjUwMDBcclxuYT1tYXgtbWVzc2FnZS1zaXplOjEwNzM3NDE4MjNcclxuIn0="}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Failed encoding the ConnectAPI as json: %v", err)
	}
	r, err := http.Post(
		"http://localhost:8888/connect", "application/json", bytes.NewReader(b))
	defer r.Body.Close()
	if err != nil {
		t.Fatalf("Failed sending a post request: %v", err)
	}
	if r.StatusCode != http.StatusOK {
		t.Fatalf("Server returned not ok status: %v", r.Status)
	}
	bs, err := ioutil.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("Failed reading resonse body: %v", err)
	}
	if len(bs) != 1600 {
		t.Fatalf("Got a bad length response: %d", len(bs))
	}
}
