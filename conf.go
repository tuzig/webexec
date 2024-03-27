package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"time"

	"github.com/pelletier/go-toml"
	"github.com/pion/webrtc/v3"
	"github.com/tuzig/webexec/httpserver"
	"github.com/tuzig/webexec/peers"
	"go.uber.org/zap/zapcore"
)

const defaultHTTPServer = httpserver.AddressType("127.0.0.1:7777")

const defaultConf = `# webexec's configuration. in toml.
# to learn more: https://github.com/tuzig/webexec/blob/master/docs/conf.md
[log]
level = "info"
# for absolute path by starting with a /
# relative path is /var/log/webexec.$USER
file = "webexec.log"
error = "webexec.err"
# next can be uncommented to debug pion components
# pion_levels = { trace = "sctp" }
[net]
http_server = "0.0.0.0:7777"
udp_port_min = 60000
udp_port_max = 61000
[timeouts]
ack_timeout = 3000
disconnect = 3000
failed = 6000
ice_gathering = 5000
keep_alive = 500
peerbook = 3000
[[ice_servers]]
urls = [ "stun:stun.l.google.com:19302" ]
[env]
COLORTERM = "truecolor"
TERM = "xterm"
`
const abConfTemplate = `%s[peerbook]
host = "%s"
user_id = "%s"
name = "%s"
`
const defaultPeerbookHost = "api.peerbook.io"

type ICEServer struct {
	URLs     []string `toml:"urls"`
	Username string   `toml:"username,omitempty"`
	Password string   `toml:"password,omitempty"`
}

// Conf hold the configuration variables
var Conf struct {
	logFilePath     string
	logLevel        zapcore.Level
	errFilePath     string
	peerbookTimeout time.Duration
	iceServers      []webrtc.ICEServer
	peerbookHost    string
	insecure        bool
	peerbookUID     string
	name            string
	peerConf        *peers.Conf
	T               *toml.Tree
}

var emailRegex = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+\\/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

// parseConf loads a configuration from a toml string and fills all Conf value.
//
//	If a key is missing LoadConf will load the default value
func parseConf(s string) (*peers.Conf, httpserver.AddressType, error) {
	t, err := toml.Load(s)
	if err != nil {
		return nil, "", fmt.Errorf("toml parsing failed: %s", err)
	}
	Conf.T = t
	Conf.logFilePath = logFilePath("log.file", "webexec.log")
	Conf.errFilePath = logFilePath("log.error", "webexec.err")
	Conf.logLevel = zapcore.ErrorLevel
	v := Conf.T.Get("log.level")
	if v != nil {
		l := v.(string)
		if l == "info" {
			Conf.logLevel = zapcore.InfoLevel
		} else if l == "warn" {
			Conf.logLevel = zapcore.WarnLevel
		} else if l == "debug" {
			Conf.logLevel = zapcore.DebugLevel
		}
	} else {
		Conf.logLevel = zapcore.WarnLevel
	}
	v = t.Get("timeouts.peerbook")
	if v != nil {
		Conf.peerbookTimeout = time.Duration(v.(int64)) * time.Millisecond
	} else {
		Conf.peerbookTimeout = 3 * time.Second
	}
	// start of peers configuration
	peersConf := &peers.Conf{}
	v = t.Get("timeouts.disconnect")
	if v != nil {
		peersConf.DisconnectTimeout = time.Duration(v.(int64)) * time.Millisecond
	} else {
		peersConf.DisconnectTimeout = 3 * time.Second
	}
	v = t.Get("timeouts.failed")
	if v != nil {
		peersConf.FailedTimeout = time.Duration(v.(int64)) * time.Millisecond
	} else {
		peersConf.FailedTimeout = 6 * time.Second
	}
	v = t.Get("timeouts.keep_alive")
	if v != nil {
		peersConf.KeepAliveInterval = time.Duration(v.(int64)) * time.Millisecond
	} else {
		peersConf.KeepAliveInterval = 1 * time.Second
	}
	v = t.Get("timeouts.ice_gathering")
	if v != nil {
		peersConf.GatheringTimeout = time.Duration(v.(int64)) * time.Millisecond
	} else {
		peersConf.GatheringTimeout = 3 * time.Second
	}
	v = t.Get("timeouts.ack_timeout")
	if v != nil {
		peersConf.AckTimeout = time.Duration(v.(int64)) * time.Millisecond
	} else {
		peersConf.AckTimeout = 3 * time.Second
	}
	v = t.Get("ice_servers")
	if v != nil {
		Conf.iceServers = []webrtc.ICEServer{}
		for _, u2 := range v.([]*toml.Tree) {
			var u ICEServer
			err := u2.Unmarshal(&u)
			if err != nil {
				return nil, "", fmt.Errorf("failed to parse ice server configuration: %s", err)
			}
			s := webrtc.ICEServer{
				URLs:           u.URLs,
				Username:       u.Username,
				Credential:     u.Password,
				CredentialType: webrtc.ICECredentialTypePassword,
			}
			Conf.iceServers = append(Conf.iceServers, s)
		}
	}
	// no address is set, let's see if the conf file has it
	var addr httpserver.AddressType
	v = t.Get("net.http_server")
	if v != nil {
		addr = httpserver.AddressType(v.(string))
	} else {
		// when no address is given, this is the default address
		addr = defaultHTTPServer
	}
	// get the udp ports
	v = t.Get("net.udp_port_min")
	if v != nil {
		peersConf.PortMin = uint16(v.(int64))
	} else {
		peersConf.PortMin = 60000
	}
	v = t.Get("net.udp_port_max")
	if v != nil {
		peersConf.PortMax = uint16(v.(int64))
	} else {
		peersConf.PortMax = 61000
	}
	// unsecured cotrol which shema to use
	v = t.Get("peerbook.insecure")
	if v != nil {
		Conf.insecure = v.(bool)
	}
	// get env vars
	m := t.Get("env")
	if m != nil {
		peersConf.Env = make(map[string]string)
		for k, v := range m.(*toml.Tree).ToMap() {
			peersConf.Env[k] = v.(string)
		}
	}
	v = t.Get("peerbook.user_id")
	if v != nil {
		Conf.peerbookUID = v.(string)
		host := t.Get("peerbook.host")
		if host != nil {
			Conf.peerbookHost = host.(string)
		} else {
			Conf.peerbookHost = defaultPeerbookHost
		}
		name := t.Get("peerbook.name")
		if name != nil {
			Conf.name = name.(string)
		} else {
			Conf.name, err = os.Hostname()
			if err != nil {
				Logger.Warnf("Failed to get hostname, using `anonymous`")
				name = "anonymous"
			}
		}
	}
	Conf.peerConf = peersConf
	return peersConf, addr, nil
}

func logFilePath(path string, def string) string {
	v := Conf.T.Get(path)
	if v == nil {
		return LogPath(def)
	}
	ret := v.(string)
	if ret[0] != '/' {
		ret = LogPath(def)
	}
	return ret
}

// loadConf load the conf file
func LoadConf(certs []webrtc.Certificate) (*peers.Conf, httpserver.AddressType, error) {
	confPath := ConfPath("webexec.conf")
	_, err := os.Stat(confPath)
	if os.IsNotExist(err) {
		return nil, "", fmt.Errorf("Missing conf file, run `webexec init` to create")
	}
	b, err := ioutil.ReadFile(confPath)
	if err != nil {
		return nil, "", fmt.Errorf("Failed to read conf file %q: %s", confPath,
			err)
	}
	conf, addr, err := parseConf(string(b))
	if err != nil {
		return nil, "", fmt.Errorf("Failed to parse conf file %q: %s", confPath,
			err)
	}
	conf.Certificate = &certs[0]
	conf.Logger = Logger
	conf.GetICEServers = GetICEServers
	conf.GetWelcome = GetWelcome
	conf.OnCTRLMsg = handleCTRLMsg

	return conf, addr, err
}

func isValidEmail(email string) bool {
	if len(email) < 3 && len(email) > 254 {
		return false
	}
	return emailRegex.MatchString(email)
}

// createConf creates the configuration files based on the defaults and user
// id, host and name
func createConf(peerbookUID string, pbHost string, name string) (string, error) {
	conf := defaultConf
	if pbHost == "" {
		pbHost = defaultPeerbookHost
	}
	if peerbookUID != "" {
		conf = fmt.Sprintf(abConfTemplate, conf, pbHost, peerbookUID, name)
	}
	confPath := ConfPath("webexec.conf")
	confFile, err := os.Create(confPath)
	defer confFile.Close()
	if err != nil {
		return "", fmt.Errorf("Failed to create config file: %q", err)
	}
	_, err = confFile.WriteString(conf)
	if err != nil {
		return "", fmt.Errorf("Failed to write to configuration file: %s", err)
	}
	return confPath, nil
}

// ConfPath returns the full path of a configuration file
func ConfPath(suffix string) string {
	usr, _ := user.Current()
	return filepath.Join(usr.HomeDir, ".config", "webexec", suffix)
}

// RunPath returns the full path of a run file: socket & pid
func RunPath(suffix string) string {
	usr, _ := user.Current()
	dir := filepath.Join(usr.HomeDir, ".local", "state", "webexec")
	_, err := os.Stat(dir)
	if os.IsNotExist(err) {
		os.MkdirAll(dir, 0755)
	}
	return filepath.Join(dir, suffix)
}

// LogPath returns the full path of a run file: socket & pid
func LogPath(suffix string) string {
	usr, _ := user.Current()
	dir := filepath.Join(usr.HomeDir, ".local", "state", "webexec")
	_, err := os.Stat(dir)
	if os.IsNotExist(err) {
		os.MkdirAll(dir, 0755)
	}
	return filepath.Join(dir, suffix)
}
