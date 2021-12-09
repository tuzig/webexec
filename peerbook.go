package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/pion/webrtc/v3"
)

type TURNResponse struct {
	TTL     int                 `json:"ttl"`
	Servers []map[string]string `json:"ice_servers"`
}

var PBICEServer *webrtc.ICEServer

func verifyPeer(host string) (bool, error) {
	fp := getFP()
	msg := map[string]string{"fp": fp, "email": Conf.email,
		"kind": "webexec", "name": Conf.name}
	m, err := json.Marshal(msg)
	schema := "https"
	if Conf.insecure {
		schema = "http"
	}
	url := url.URL{Scheme: schema, Host: host, Path: "/verify"}
	resp, err := http.Post(url.String(), "application/json", bytes.NewBuffer(m))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := ioutil.ReadAll(resp.Body)
		return false, fmt.Errorf(string(b))
	}
	var ret map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&ret)
	if err != nil {
		return false, err
	}
	v, found := ret["verified"]
	if found {
		return v.(bool), nil
	}
	_, found = ret["peers"]
	if found {
		return true, nil
	}
	return false, nil
}
func getICEServers(host string) ([]webrtc.ICEServer, error) {
	if host == "" {
		return Conf.iceServers, nil
	}
	if PBICEServer == nil {
		schema := "https"
		if Conf.insecure {
			schema = "http"
		}
		url := url.URL{Scheme: schema, Host: host, Path: "/turn"}
		resp, err := http.Post(url.String(), "application/json", nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			b, _ := ioutil.ReadAll(resp.Body)
			return nil, fmt.Errorf(string(b))
		}
		var d TURNResponse
		err = json.NewDecoder(resp.Body).Decode(&d)
		if err != nil {
			return nil, err
		}
		if err != nil {
			return nil, err
		}
		s := d.Servers[0]

		PBICEServer = &webrtc.ICEServer{
			URLs:       []string{s["urls"]},
			Username:   s["username"],
			Credential: s["credential"],
		}

	}
	return append(Conf.iceServers, *PBICEServer), nil
}
