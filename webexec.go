//go:generate go run git.rootprojects.org/root/go-gitver/v2 --fail
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kardianos/osext"
	"github.com/pion/logging"
	"github.com/tuzig/webexec/pidfile"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	// Logger is our global logger
	Logger  *zap.SugaredLogger
	commit  = "0000000"
	version = "UNRELEASED"
	date    = "0000-00-00T00:00:00+0000"
	cdb     = NewClientsDB()
	// ErrAgentNotRunning is returned by commands that require a running agent
	ErrAgentNotRunning = errors.New("agent is not running")
	gotExitSignal      chan bool
	logWriter          io.Writer
	pionLoggerFactory  *logging.DefaultLoggerFactory
	done               chan os.Signal
	key                *KeyType
)

func newPionLoggerFactory() *logging.DefaultLoggerFactory {
	factory := logging.DefaultLoggerFactory{}
	factory.DefaultLogLevel = logging.LogLevelError
	factory.ScopeLevels = make(map[string]logging.LogLevel)
	factory.Writer = logWriter

	logLevels := map[string]logging.LogLevel{
		"disable": logging.LogLevelDisabled,
		"error":   logging.LogLevelError,
		"warn":    logging.LogLevelWarn,
		"info":    logging.LogLevelInfo,
		"debug":   logging.LogLevelDebug,
		"trace":   logging.LogLevelTrace,
	}

	for name, level := range logLevels {
		v := Conf.pionLevels.Get(name)
		if v == nil {
			continue
		}
		env := v.(string)
		if env == "" {
			continue
		}

		if strings.ToLower(env) == "all" {
			factory.DefaultLogLevel = level
			continue
		}

		scopes := strings.Split(strings.ToLower(env), ",")
		for _, scope := range scopes {
			factory.ScopeLevels[scope] = level
		}
	}
	return &factory
}

// InitAgentLogger intializes the global `Logger` with agent's settings
func InitAgentLogger() {
	// rotate the log file
	logWriter = &lumberjack.Logger{
		Filename:   Conf.logFilePath,
		MaxSize:    10, // megabytes
		MaxBackups: 3,
		MaxAge:     28, // days
	}
	w := zapcore.AddSync(logWriter)

	// TODO: use pion's logging
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
			MessageKey:  "webexec",
			LevelKey:    "level",
			EncodeLevel: zapcore.CapitalLevelEncoder,
			TimeKey:     "time",
			EncodeTime:  zapcore.ISO8601TimeEncoder,
		}),
		w,
		Conf.logLevel,
	)
	logger := zap.New(core)
	defer logger.Sync()
	Logger = logger.Sugar()
	// redirect stderr
	e, _ := os.OpenFile(
		Conf.errFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	Dup2(int(e.Fd()), 2)
}

// InitDevLogger starts a logger for development
func InitDevLogger() {
	zapConf := []byte(`{
		  "level": "debug",
		  "encoding": "console",
		  "outputPaths": ["stdout"],
		  "errorOutputPaths": ["stderr"],
		  "encoderConfig": {
		    "messageKey": "message",
		    "levelKey": "level",
		    "levelEncoder": "lowercase"
		  }
		}`)

	var cfg zap.Config
	if err := json.Unmarshal(zapConf, &cfg); err != nil {
		panic(err)
	}
	l, err := cfg.Build()
	Logger = l.Sugar()
	if err != nil {
		panic(err)
	}
	defer Logger.Sync()
}

// Shutdown is called when it's time to go.Sweet dreams.
func Shutdown() {
	var err error
	for _, peer := range Peers {
		if peer.PC != nil {
			err = peer.PC.Close()
			if err != nil {
				Logger.Error("Failed closing peer connection: %w", err)
			}
		}
	}
	for _, p := range Panes.All() {
		err = p.C.Process.Kill()
		if err != nil {
			Logger.Error("Failed closing a process: %w", err)
		}
	}
}

// versionCMD prints version information
func versionCMD(c *cli.Context) error {
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Git Commit Hash: %s\n", commit)
	fmt.Printf("Build Date: %s\n", date)
	return nil
}

// stop - stops the agent
func stop(c *cli.Context) error {
	LoadConf()
	pidf, err := pidfile.Open(ConfPath("agent.pid"))
	if os.IsNotExist(err) {
		return ErrAgentNotRunning
	}
	if err != nil {
		return err
	}
	if !pidf.Running() {
		return ErrAgentNotRunning
	}
	pid, err := pidf.Read()
	if err != nil {
		return fmt.Errorf("Failed to read the pidfile: %s", err)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("Failed to find the agetnt's process: %s", err)
	}
	fmt.Printf("Sending a SIGINT to agent process %d\n", pid)
	err = process.Signal(syscall.SIGINT)
	return err
}

// createPIDFile creates the pid file or returns an error if it exists
func createPIDFile() error {
	pionLoggerFactory = newPionLoggerFactory()
	pidPath := ConfPath("agent.pid")
	_, err := pidfile.New(pidPath)
	if err == pidfile.ErrProcessRunning {
		return fmt.Errorf("agent is already running, doing nothing")
	}
	if err != nil {
		return fmt.Errorf("pid file creation failed: %q", err)
	}
	return nil
}

func launchAgent(address string) error {
	pidf, err := pidfile.Open(ConfPath("agent.pid"))
	if !os.IsNotExist(err) && pidf.Running() {
		fmt.Println("agent is already running, doing nothing")
		return nil
	}
	// start the agent process and exit
	execPath, err := osext.Executable()
	if err != nil {
		return fmt.Errorf("Failed to find the executable: %s", err)
	}
	cmd := exec.Command("bash", "-c",
		fmt.Sprintf("%s start --agent --address %s >> %s",
			execPath, address, Conf.logFilePath))
	cmd.Env = nil
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("agent failed to start :%q", err)
	}
	time.Sleep(100 * time.Millisecond)
	fmt.Printf("agent started as process #%d\n", cmd.Process.Pid)
	return nil
}

// start - start the user's agent
func start(c *cli.Context) error {
	err := LoadConf()
	if err != nil {
		return err
	}
	var address string
	if c.IsSet("address") {
		address = c.String("address")
	} else {
		address = Conf.httpServer
	}
	debug := c.Bool("debug")
	if debug {
		InitDevLogger()
	} else {
		if c.Bool("agent") {
			InitAgentLogger()
			err := createPIDFile()
			if err != nil {
				return err
			}
		} else {
			return launchAgent(address)
		}
	}
	// the code below runs for both --debug and --agent
	Logger.Infof("Serving http on %q", address)
	done = make(chan os.Signal, 1)
	go HTTPGo(address)
	if Conf.peerbookHost != "" {
		verified, err := verifyPeer(Conf.peerbookHost)
		if err != nil {
			return fmt.Errorf("Got an error verifying peer: %s", err)
		}
		if verified {
			Logger.Infof("** verified ** by peerbook at %s", Conf.peerbookHost)
		} else {
			Logger.Infof("** unverified ** peerbook sent you a verification email.")
			fmt.Printf("                 If you don't get it please visit %s",
				Conf.peerbookHost)
		}
		go signalingGo()
	}
	// signal handling
	if debug {
		signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	} else {
		signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	}
	<-done
	return nil
}

/* TBD:
func paste(c *cli.Context) error {
	fmt.Println("Soon, we'll be pasting data from the clipboard to STDOUT")
	return nil
}
func copyCMD(c *cli.Context) error {
	fmt.Println("Soon, we'll be copying data from STDIN to the clipboard")
	return nil
*/
// restart function restarts the agent or starts it if it is stopped
func restart(c *cli.Context) error {
	err := stop(c)
	if err != nil && err != ErrAgentNotRunning {
		return err
	}
	// wait for the process to stop
	// TODO: https://github.com/tuzig/webexec/issues/18
	time.Sleep(1 * time.Second)
	return start(c)
}

// status function prints the status of the agent
func status(c *cli.Context) error {
	LoadConf()
	pidf, err := pidfile.Open(ConfPath("agent.pid"))
	if os.IsNotExist(err) {
		fmt.Println("agent is not running")
		return nil
	}
	if err != nil {
		return err
	}
	if !pidf.Running() {
		fmt.Println("agent is not running and pid is stale")
		return nil
	}
	pid, err := pidf.Read()
	fmt.Printf("agent is running with process id %d\n", pid)
	return nil
}
func initCMD(c *cli.Context) error {
	// init the dev logger so log messages are printed on the console
	InitDevLogger()
	homePath := ConfPath("")
	_, err := os.Stat(homePath)
	if os.IsNotExist(err) {
		os.Mkdir(homePath, 0755)
		fmt.Printf("Created %q directory\n", homePath)
	} else {
		return fmt.Errorf(
			"%q already exists. To start fresh remove it and try again.",
			homePath)
	}
	confPath := ConfPath("webexec.conf")
	err = createConf(confPath)
	if err != nil {
		return err
	}
	err = LoadConf()
	if err != nil {
		return fmt.Errorf("Failed to parse default conf: %s", err)
	}
	// TODO: move this to start
	if Conf.peerbookHost != "" {
		_, err := verifyPeer(Conf.peerbookHost)
		if err != nil {
			return fmt.Errorf("Got an error verifying peer: %s", err)
		}
	}
	fmt.Printf(`Created %q config file.
peerbook sent an authorization email, please check your inbox, authorize and
run "webexec start" to start your agent.`, confPath)
	return nil

}
func main() {
	app := &cli.App{
		Name:        "webexec",
		Usage:       "execute commands and pipe their stdin&stdout over webrtc",
		HideVersion: true,
		Commands: []*cli.Command{
			/* TODO: Add clipboard commands
			{
				Name:   "copy",
				Usage:  "Copy data from STDIN to the clipboard",
				Action: copyCMD,
			}, {
				Name:   "paste",
				Usage:  "Paste data from the clipboard to STDOUT",
				Action: paste,
			},*/
			{
				Name:   "version",
				Usage:  "Print version information",
				Action: versionCMD,
			}, {
				Name:  "restart",
				Usage: "restarts the agent",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "address",
						Aliases: []string{"a"},
						Usage:   "The address to listen to",
						Value:   "0.0.0.0:7777",
					},
				},
				Action: restart,
			}, {
				Name:    "start",
				Aliases: []string{"l"},
				Usage:   "Spawns a backgroung http server & webrtc peer",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "address",
						Aliases: []string{"a"},
						Usage:   "The address to listen to",
						Value:   "",
					},
					&cli.BoolFlag{
						Name:  "debug",
						Usage: "Run in debug mode in the foreground",
					},
					&cli.BoolFlag{
						Name:  "agent",
						Usage: "Run as agent, in the background",
					},
				},
				Action: start,
			}, {
				Name:   "status",
				Usage:  "webexec agent's status",
				Action: status,
			}, {
				Name:   "stop",
				Usage:  "stop the user's agent",
				Action: stop,
			}, {
				Name:   "init",
				Usage:  "initialize the conf file",
				Action: initCMD,
			},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}
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
