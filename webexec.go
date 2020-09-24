package main

import (
	"encoding/json"
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

var Logger *zap.SugaredLogger

var gotExitSignal chan bool

const A_BIT = 1 * time.Millisecond

// InitAgent intializes the global `Logger` with agent's settings
// and starts listening for signals
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

// InitDev intializes the global `Logger` and capture the system signals
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

//onfPath returns the full path of a configuration file
func ConfPath(suffix string) string {
	usr, _ := user.Current()
	// TODO: make it configurable
	return filepath.Join(usr.HomeDir, ".webexec", suffix)
}

/*
 * initUser - initialize the user's .webexec directory
 */
func initUser(c *cli.Context) error {
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
		return fmt.Errorf("agent is not runnin")
	}
	if err != nil {
		return err
	}
	if !pidf.Running() {
		return fmt.Errorf("agent is not runnin")
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
			// start the agent and exit
			execPath, err := osext.Executable()
			if err != nil {
				return fmt.Errorf("Failed to find the executable: %s", err)
			}
			cmd := exec.Command(execPath, "start", "--agent")
			err = cmd.Start()
			if err != nil {
				return fmt.Errorf("agent failed to start :%q", err)
			}
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
		Logger.Info("Exiting on SIGINT")
	}
	<-gotExit

	return nil
}
func pasteCB(c *cli.Context) error {
	fmt.Println("Soon, we'll be pasting data from the clipboard to STDOUT")
	return nil
}
func copyCB(c *cli.Context) error {
	fmt.Println("Soon, we'll be copying data from STDIN to the clipboard")
	return nil
}

func main() {
	app := &cli.App{
		Name:  "webexec",
		Usage: "execute commands and pipe their stdin&stdout over webrtc",
		Commands: []*cli.Command{
			{
				Name:    "start",
				Aliases: []string{"l"},

				Usage: "Spawns a backgroung http server & webrtc peer",
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
				Name:   "stop",
				Usage:  "stop the user's agent",
				Action: stop,
			}, {
				Name:   "init",
				Usage:  "initialize user settings",
				Action: initUser,
			}, {
				Name:   "copy",
				Usage:  "Copy data from STDIN to the clipboard",
				Action: copyCB,
			}, {
				Name:   "paste",
				Usage:  "Paste data from the clipboard to STDOUT",
				Action: pasteCB,
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
				Usage:  "Manage user tokens",
				Action: initUser,
			},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
