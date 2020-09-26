package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
)

var ErrAgentNotRunning = errors.New("agent is not running")

// global logger
var Logger *zap.SugaredLogger

var gotExitSignal chan bool

// short period of time, used to let the current run it's course
const A_BIT = 1 * time.Millisecond

// InitAgentLogger intializes the global `Logger` with agent's settings
func InitAgentLogger() {
	cfg := zap.Config{
		Level:       zap.NewAtomicLevelAt(zap.DebugLevel),
		Encoding:    "console",
		OutputPaths: []string{ConfPath("agent.log")},
		EncoderConfig: zapcore.EncoderConfig{
			MessageKey:  "message",
			LevelKey:    "level",
			EncodeLevel: zapcore.CapitalLevelEncoder,
			TimeKey:     "time",
			EncodeTime:  zapcore.ISO8601TimeEncoder,
		},
	}
	l, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	Logger = l.Sugar()
	defer Logger.Sync()
}

// InitDevLogger intializes the global `Logger` for development
func InitDevLogger() {
	zapConf := []byte(`{
		  "level": "debug",
		  "encoding": "console",
		  "outputPaths": ["stdout", "/tmp/logs"],
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
	for _, peer := range Peers {
		if peer.PC != nil {
			peer.PC.Close()
		}
	}
	for _, p := range Panes {
		p.C.Process.Kill()
	}
}

// ConfPath returns the full path of a configuration file
func ConfPath(suffix string) string {
	usr, _ := user.Current()
	// TODO: make it configurable
	return filepath.Join(usr.HomeDir, ".webexec", suffix)
}

// init - initialize the user's .webexec directory
func initCMD(c *cli.Context) error {
	var contact string

	usr, _ := user.Current()
	home := filepath.Join(usr.HomeDir, ".webexec")
	_, err := os.Stat(home)
	if os.IsNotExist(err) {
		os.Mkdir(home, 0755)
		fmt.Println("Please enter an email or phone number (starting with + and country code):")
		fmt.Scanln(&contact)
		config := map[string]string{
			"username": usr.Username,
			"userid":   usr.Uid,
			"contact":  contact,
		}
		confFile, err := os.Create(filepath.Join(home, "config.json"))
		if err != nil {
			return fmt.Errorf("Failed to create config file: %q", err)
		}
		d, err := json.Marshal(config)
		if err != nil {
			return fmt.Errorf("Failed to erialize configuration %q", err)
		}
		confFile.Write(d)
		confFile.Close()
	} else {
		return fmt.Errorf("Can not initialize webexec as directory ~/.webexec already exists")
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
	fmt.Printf("Sending an INT signal to agent process # %d\n", pid)
	err = process.Signal(syscall.SIGINT)
	return err
}

// start - start the user's agent
func start(c *cli.Context) error {
	address := c.String("address")
	// TODO: daemon := c.Bool("d") and all it entails
	debug := c.Bool("debug")
	if debug {
		InitDevLogger()
	} else {
		if c.Bool("agent") {
			InitAgentLogger()
			pidPath := ConfPath("agent.pid")
			pid, err := pidfile.New(pidPath)
			if err == pidfile.ErrProcessRunning {
				Logger.Info("agent is already running, doing nothing")
				return fmt.Errorf("agent is already running, doing nothing")
			}
			if err != nil {
				return fmt.Errorf("pid file creation failed: %q", err)
			}
			defer pid.Remove()
		} else {
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
			cmd := exec.Command(execPath, "start", "--agent")
			logfile, err := os.Open(ConfPath("agent.err"))
			if errors.Is(err, os.ErrNotExist) {
				// TODO: make it configurable
				logfile, err = os.Create(ConfPath("agent.err"))
				if errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf(`
	Seems like ~/.webexec is missing.\n
	Have you ran "%s init"?`, execPath)
				}
				if err != nil {
					return fmt.Errorf("failed to create agent.err:%q", err)
				}
			}
			if err != nil {
				return fmt.Errorf("failed to open agent.err :%s", err)
			}
			cmd.Stderr = logfile
			err = cmd.Start()
			if err != nil {
				return fmt.Errorf("agent failed to start :%q", err)
			}
			time.Sleep(100 * time.Millisecond)
			fmt.Printf("agent started as process #%d\n", cmd.Process.Pid)
			return nil
		}
	}
	Logger.Infof("Serving http on %q", address)
	go HTTPGo(address)
	// signal handling
	gotExit := make(chan os.Signal)
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
func paste(c *cli.Context) error {
	fmt.Println("Soon, we'll be pasting data from the clipboard to STDOUT")
	return nil
}
func copyCMD(c *cli.Context) error {
	fmt.Println("Soon, we'll be copying data from STDIN to the clipboard")
	return nil
}

// restart function restarts the agent or starts it if it is stopped
func restart(c *cli.Context) error {
	err := stop(c)
	if err != nil && err != ErrAgentNotRunning {
		return err
	}
	// wait for the process to stop
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
		Name:  "webexec",
		Usage: "execute commands and pipe their stdin&stdout over webrtc",
		Commands: []*cli.Command{
			{
				Name:   "copy",
				Usage:  "Copy data from STDIN to the clipboard",
				Action: copyCMD,
			}, {
				Name:   "paste",
				Usage:  "Paste data from the clipboard to STDOUT",
				Action: paste,
			}, {
				Name:   "restart",
				Usage:  "restarts the agent",
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
						Value:   "0.0.0.0:7777",
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
				Usage:  "initialize user settings",
				Action: initCMD,
			}, {
				Name: "tokens",
				Subcommands: []*cli.Command{
					{
						Name:   "add",
						Usage:  "add <token>",
						Action: AddToken,
					}, {
						Name:   "rm",
						Usage:  "delete <token>",
						Action: DeleteToken,
					}, {
						Name:   "ls",
						Usage:  "list the tokens",
						Action: DeleteToken,
					},
				},
				Usage: "Manage user tokens",
			},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
