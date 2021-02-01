package main

import (
	"fmt"
	"github.com/pelletier/go-toml"
	"go.uber.org/zap/zapcore"
	"os"
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
ice_servers = [ "stun:stun.l.google.com:19302" ]
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
	iceServers        []string
	httpServer        string
	logFilePath       string
	logLevel          zapcore.Level
}

// LoadConf loads a configuration from a toml string and fills all Conf value.
//			If a key is missing LoadConf will load the default value
func LoadConf(s string) error {
	t, err := toml.Load(s)
	if err != nil {
		return fmt.Errorf("toml parsing failed: %s", err)
	}
	Conf.T = t
	v := t.Get("log.file")
	if v == nil {
		Conf.logFilePath = ConfPath("agent.log")
	} else {
		Conf.logFilePath = v.(string)
		if Conf.logFilePath[0] != '/' {
			Conf.logFilePath = ConfPath(Conf.logFilePath)
		}
	}
	Conf.logLevel = zapcore.ErrorLevel
	l := Conf.T.Get("log.level").(string)
	if l == "info" {
		Conf.logLevel = zapcore.InfoLevel
	} else if l == "warn" {
		Conf.logLevel = zapcore.WarnLevel
	} else if l == "debug" {
		Conf.logLevel = zapcore.DebugLevel
	}
	v = t.Get("timeouts.disconnect")
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
	// get pion's conf
	v = t.Get("log.pion_log_trace")
	if v != nil {
		err = os.Setenv("PION_LOG_TRACE", v.(string))
		if err != nil {
			return fmt.Errorf("Failed setting PION_LOG_TRACE: %s", err)
		}
	}
	v = t.Get("log.pion_log_debug")
	if v != nil {
		err = os.Setenv("PION_LOG_DEBUG", v.(string))
		if err != nil {
			return fmt.Errorf("Failed setting PION_LOG_DEBUG: %s", err)
		}
	}
	return nil
}
