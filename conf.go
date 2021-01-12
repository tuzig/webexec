package main

import (
	"fmt"
	"github.com/pelletier/go-toml"
	"time"
)

const defaultHTTPServer = "0.0.0.0:7777"
const defaultConf = `# webexec's toml configuration file
[log]
level = "error"
# for absolute path by starting with a /
file = "agent.log"
[net]
http_server = "0.0.0.0:7777"
# stun_urls = [ "stun:stun.l.google.com:19302" ]
[timeouts]
disconnect = 3000
failed = 6000
keep_alive = 1000
ice_gathering = 5000
`

// Conf hold the configuration variables
var Conf struct {
	T                 *toml.Tree
	disconnectTimeout time.Duration
	failedTimeout     time.Duration
	keepAliveInterval time.Duration
	gatheringTimeout  time.Duration
	stunURLs          []string
	httpServer        string
}

// LoadConf loads a configuration from a toml string and fills all Conf value.
//			If a key is missing LoadConf will load the default value
func LoadConf(s string) error {
	t, err := toml.Load(s)
	if err != nil {
		return fmt.Errorf("toml parsing failed: %s", err)
	}
	Conf.T = t
	v := t.Get("timeouts.disconnect")
	if v != nil {
		Conf.disconnectTimeout = time.Duration(v.(int64)) * time.Millisecond
	} else {
		Conf.disconnectTimeout = 3 * time.Second
	}
	v = t.Get("timeouts.failed")
	if v != nil {
		Conf.failedTimeout = time.Duration(v.(int64)) * time.Millisecond
	} else {
		Conf.failedTimeout = 6 * time.Second
	}
	v = t.Get("timeouts.keep_alive")
	if v != nil {
		Conf.keepAliveInterval = time.Duration(v.(int64)) * time.Millisecond
	} else {
		Conf.keepAliveInterval = 1 * time.Second
	}
	v = t.Get("timeouts.ice_gathering")
	if v != nil {
		Conf.gatheringTimeout = time.Duration(v.(int64)) * time.Millisecond
	} else {
		Conf.gatheringTimeout = 3 * time.Second
	}
	v = t.Get("net.stun_urls")
	if v != nil {
		urls := v.([]interface{})
		Conf.stunURLs = []string{}
		for _, u := range urls {
			Conf.stunURLs = append(Conf.stunURLs, u.(string))
		}
	}
	// no address is set, let's see if the conf file has it
	v = t.Get("net.http_server")
	if v != nil {
		Conf.httpServer = v.(string)
	} else {
		// when no address is given, this is the default address
		Conf.httpServer = defaultHTTPServer
	}
	return nil
}
