package main

import (
	"fmt"
	"github.com/pelletier/go-toml"
	"go.uber.org/zap/zapcore"
	"time"
)

const defaultHTTPServer = "0.0.0.0:7777"
const defaultConf = `# webexec's default configuration. in toml.
[log]
level = "error"
# for absolute path by starting with a /
file = "agent.log"
error = "agent.err"
# next can be uncommented to debug pion components
# pion_levels = { trace = "sctp" }
[net]
http_server = "0.0.0.0:7777"
ice_servers = [ "stun:stun.l.google.com:19302" ]
udp_port_min = 7000
udp_port_max = 7777
[timeouts]
disconnect = 3000
failed = 6000
keep_alive = 500
ice_gathering = 5000
[env]
COLORTERM = "truecolor"
TERM = "xterm"
`

// Conf hold the configuration variables
var Conf struct {
	T                 *toml.Tree
	disconnectTimeout time.Duration
	failedTimeout     time.Duration
	keepAliveInterval time.Duration
	gatheringTimeout  time.Duration
	iceServers        []string
	httpServer        string
	logFilePath       string
	logLevel          zapcore.Level
	errFilePath       string
	pionLevels        *toml.Tree
	env               map[string]string
	portMin           uint16
	portMax           uint16
}

// LoadConf loads a configuration from a toml string and fills all Conf value.
//			If a key is missing LoadConf will load the default value
func LoadConf(s string) error {
	t, err := toml.Load(s)
	if err != nil {
		return fmt.Errorf("toml parsing failed: %s", err)
	}
	Conf.T = t
	Conf.logFilePath = loadFilePath("log.file", "agent.log")
	Conf.errFilePath = loadFilePath("log.error", "agent.err")
	Conf.logLevel = zapcore.ErrorLevel
	l := Conf.T.Get("log.level").(string)
	if l == "info" {
		Conf.logLevel = zapcore.InfoLevel
	} else if l == "warn" {
		Conf.logLevel = zapcore.WarnLevel
	} else if l == "debug" {
		Conf.logLevel = zapcore.DebugLevel
	}
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
	v = t.Get("net.ice_servers")
	if v != nil {
		urls := v.([]interface{})
		Conf.iceServers = []string{}
		for _, u := range urls {
			Conf.iceServers = append(Conf.iceServers, u.(string))
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
	// get the udp ports
	v = t.Get("net.udp_port_min")
	if v != nil {
		Conf.portMin = uint16(v.(int64))
	}
	v = t.Get("net.udp_port_max")
	if v != nil {
		Conf.portMax = uint16(v.(int64))
	}
	// get pion's conf
	v = t.Get("log.pion_levels")
	if v != nil {
		Conf.pionLevels = v.(*toml.Tree)
	} else {
		Conf.pionLevels = &toml.Tree{}
	}
	// get env vars
	m := t.Get("env")
	if m != nil {
		Conf.env = make(map[string]string)
		for k, v := range m.(*toml.Tree).ToMap() {
			Conf.env[k] = v.(string)
		}
	}
	return nil

}

func loadFilePath(path string, def string) string {
	v := Conf.T.Get(path)
	if v == nil {
		return ConfPath(def)
	}
	ret := v.(string)
	if ret[0] != '/' {
		ret = ConfPath(def)
	}
	return ret
}
