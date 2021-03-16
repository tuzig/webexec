package main

import (
	"bufio"
	"fmt"
	"github.com/pelletier/go-toml"
	"go.uber.org/zap/zapcore"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"time"
)

const defaultHTTPServer = "0.0.0.0:7777"
const defaultConf = `# webexec's configuration. in toml.
# to learn more: https://github.com/tuzig/webexec/blob/master/docs/conf.md
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
		fmt.Printf("Read configuration from %q\n", confPath)
	}
	return nil
}
func createConf(confPath string) error {
	confFile, err := os.Create(confPath)
	defer confFile.Close()
	if err != nil {
		return fmt.Errorf("Failed to create config file: %q", err)
	}
	conf := defaultConf
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Would you like to use a signaling server?")
	fmt.Println("It enables behind the NAT connection and")
	fmt.Println("makes it easier for clients to find this server")
	fmt.Print("and authenticate (Y/n)")
	text, _ := reader.ReadString('\n')
	if text == "\n" || text == "y\n" || text == "yes\n" {
		fmt.Print("Please enter your email: ")
		email, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("Failed to read input: %s", err)
		}
		usr, _ := user.Current()
		// TODO: make it configurable
		tokenPath := filepath.Join(usr.HomeDir, ".ssh", "id_rsa.pub")
		conf = fmt.Sprintf(`%s[signalling]
address = "pab.tuzig.com:777"
users = [ "%s" ]
token_file = "%s"`, conf, email[:len(email)-1], tokenPath)
	}
	fmt.Printf("\n%s\n", conf)

	// creating the token file
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
