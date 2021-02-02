//go:generate go run git.rootprojects.org/root/go-gitver/v2 --fail
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kardianos/osext"
	"github.com/tuzig/webexec/pidfile"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	// Logger is our global logger
	Logger             *zap.SugaredLogger
	commit             = "0000000"
	version            = "UNRELEASED"
	date               = "0000-00-00T00:00:00+0000"
	cdb                = NewClientsDB()
	ErrAgentNotRunning = errors.New("agent is not running")
	gotExitSignal      chan bool
)

// InitAgentLogger intializes the global `Logger` with agent's settings
func InitAgentLogger() {
	// rotate the log file
	w := zapcore.AddSync(&lumberjack.Logger{
		Filename:   Conf.logFilePath,
		MaxSize:    10, // megabytes
		MaxBackups: 3,
		MaxAge:     28, // days
	})

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
			MessageKey:  "message",
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
	syscall.Dup2(int(e.Fd()), 2)
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

// ConfPath returns the full path of a configuration file
func ConfPath(suffix string) string {
	usr, _ := user.Current()
	// TODO: make it configurable
	return filepath.Join(usr.HomeDir, ".webexec", suffix)
}

// versionCMD prints version information
func versionCMD(c *cli.Context) error {
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Git Commit Hash: %s\n", commit)
	fmt.Printf("Build Date: %s\n", date)
	return nil
}

// loadConf creates the home dir and the files
func loadConf(c *cli.Context) error {
	usr, _ := user.Current()
	home := filepath.Join(usr.HomeDir, ".webexec")
	_, err := os.Stat(home)
	if os.IsNotExist(err) {
		os.Mkdir(home, 0755)
		fmt.Printf("First run, created %q directory\n", home)
	}
	conf_path := filepath.Join(home, "webexec.conf")
	_, err = os.Stat(conf_path)
	if os.IsNotExist(err) {
		confFile, err := os.Create(conf_path)
		defer confFile.Close()
		if err != nil {
			return fmt.Errorf("Failed to create config file: %q", err)
		}
		confFile.WriteString(defaultConf)
		err = LoadConf(defaultConf)
		if err != nil {
			return fmt.Errorf("Failed to parse default conf: %s", err)
		}
		fmt.Printf("Created %q default config file\n", conf_path)
	} else {
		b, err := ioutil.ReadFile(conf_path)
		if err != nil {
			return fmt.Errorf("Failed to read conf file %q: %s", conf_path,
				err)
		}
		err = LoadConf(string(b))
		if err != nil {
			return fmt.Errorf("Failed to parse conf file %q: %s", conf_path,
				err)
		}
		fmt.Printf("Read configuration from %q\n", conf_path)
	}
	_, err = os.Stat(TokensFilePath)
	if os.IsNotExist(err) {
		tokensFile, err := os.Create(TokensFilePath)
		defer tokensFile.Close()
		if err != nil {
			return fmt.Errorf("Failed to create the tokens file at %q: %w",
				TokensFilePath, err)
		}
		fmt.Printf("Created %q tokens file\n", conf_path)
	}
	return nil
}

// stop - stops the agent
func stop(c *cli.Context) error {
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

// agentStart starts the user agent
func agentStart() error {
	InitAgentLogger()
	Logger.Infof("os.Environ - %v", os.Environ())
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
			err := agentStart()
			if err != nil {
				return fmt.Errorf("Failed to start as agent: %w", err)
			}
		} else {
			return launchAgent(address)
		}
	}
	// the code below runs for --debug and --agent
	Logger.Infof("Serving http on %q", address)
	go HTTPGo(address)
	// signal handling
	gotExit := make(chan os.Signal, 1)
	if debug {
		signal.Notify(gotExit, os.Interrupt, syscall.SIGTERM)
	} else {
		signal.Notify(gotExit, syscall.SIGINT)
	}
	<-gotExit
	if !debug {
		Logger.Info("Exiting on SIGINT")
	}
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
}
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

func main() {
	app := &cli.App{
		Name:        "webexec",
		Usage:       "execute commands and pipe their stdin&stdout over webrtc",
		HideVersion: true,
		Before:      loadConf,

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
			},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}
