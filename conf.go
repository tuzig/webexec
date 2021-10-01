package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"time"

	"github.com/pelletier/go-toml"
	"go.uber.org/zap/zapcore"
)

const defaultHTTPServer = "0.0.0.0:7777"
const defaultConf = `# webexec's configuration. in toml.
# to learn more: https://github.com/tuzig/webexec/blob/master/docs/conf.md
[log]
level = "info"
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
peerbook = 3000
[env]
COLORTERM = "truecolor"
TERM = "xterm"
`
const abConfTemplate = `%s[peerbook]
email = "%s"`
const defaultPeerbookHost = "pb.terminal7.dev"

// Conf hold the configuration variables
var Conf struct {
	T                 *toml.Tree
	disconnectTimeout time.Duration
	failedTimeout     time.Duration
	keepAliveInterval time.Duration
	gatheringTimeout  time.Duration
	peerbookTimeout   time.Duration
	iceServers        []string
	httpServer        string
	logFilePath       string
	logLevel          zapcore.Level
	errFilePath       string
	pionLevels        *toml.Tree
	env               map[string]string
	portMin           uint16
	portMax           uint16
	peerbookHost      string
	email             string
	name              string
	insecure          bool
}

var emailRegex = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+\\/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

// parseConf loads a configuration from a toml string and fills all Conf value.
//			If a key is missing LoadConf will load the default value
func parseConf(s string) error {
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
	v = t.Get("timeouts.peerbook")
	if v != nil {
		Conf.peerbookTimeout = time.Duration(v.(int64)) * time.Millisecond
	} else {
		Conf.peerbookTimeout = 3 * time.Second
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
	// unsecured cotrol which shema to use
	v = t.Get("peerbook.insecure")
	if v != nil {
		Conf.insecure = v.(bool)
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
	v = t.Get("peerbook.email")
	if v != nil {
		Conf.email = v.(string)
		url := t.Get("peerbook.host")
		if url != nil {
			Conf.peerbookHost = url.(string)
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

// loadConf load the conf file
func LoadConf() error {
	confPath := ConfPath("webexec.conf")
	_, err := os.Stat(confPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("No configuration file found. Please run `webexec init`")
	} else {
		b, err := ioutil.ReadFile(confPath)
		if err != nil {
			return fmt.Errorf("Failed to read conf file %q: %s", confPath,
				err)
		}
		err = parseConf(string(b))
		if err != nil {
			return fmt.Errorf("Failed to parse conf file %q: %s", confPath,
				err)
		}
	}
	return nil
}

func isValidEmail(email string) bool {
	if len(email) < 3 && len(email) > 254 {
		return false
	}
	return emailRegex.MatchString(email)
}

// createConf creates the configuration files based on user input
func createConf(confPath string) error {
	conf := defaultConf
	stdin := bufio.NewReader(os.Stdin)
	fmt.Println("To make it easier for your clients to find this server")
	fmt.Println("and verify its fingerprints you are invited")
	fmt.Println("to publish it to your private addres book.")
please:
	fmt.Print("Please enter your email (blank to skip): ")
	email, err := stdin.ReadString('\n')
	if err != nil {
		return fmt.Errorf("Failed to read input: %s", err)
	}
	email = email[:len(email)-1]
	if email != "" {
		if !isValidEmail(email) {
			fmt.Println("Sorry, not a valid email. Please try again.")
			goto please
		}
		conf = fmt.Sprintf(abConfTemplate, conf, email)
	}
	confFile, err := os.Create(confPath)
	defer confFile.Close()
	if err != nil {
		return fmt.Errorf("Failed to create config file: %q", err)
	}
	_, err = confFile.WriteString(conf)
	if err != nil {
		return fmt.Errorf("Failed to write to configuration file: %s", err)
	}
	// creating the token file
	// TODO: Rinse - add TokensPath()
	if TokensFilePath == "" {
		TokensFilePath = ConfPath("authorized_tokens")
	}
	_, err = os.Stat(TokensFilePath)
	if os.IsNotExist(err) {
		tokensFile, err := os.Create(TokensFilePath)
		defer tokensFile.Close()
		if err != nil {
			return fmt.Errorf("Failed to create the tokens file at %q: %w",
				TokensFilePath, err)
		}
		fmt.Printf("Created %q tokens file\n", confPath)
	}
	return nil
}

// ConfPath returns the full path of a configuration file
func ConfPath(suffix string) string {
	usr, _ := user.Current()
	// TODO: make it configurable
	return filepath.Join(usr.HomeDir, ".webexec", suffix)
}
