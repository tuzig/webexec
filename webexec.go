package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

var Logger *zap.SugaredLogger

func attachKillHandler() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\r- Ctrl+C pressed in Terminal")
		os.Exit(0)
	}()
}

/*
** InitLogger intializes the global `Logger`
 */
func InitLogger() {
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

	Logger.Info("logger construction succeeded")
}

// Shutdown is called when it's time to go.Sweet dreams.
func Shutdown() {
	for _, peer := range Peers {
		if peer.pc != nil {
			peer.pc.Close()
		}
	}
	for _, p := range Panes {
		p.C.Process.Kill()
	}
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

/*
 * listen - listens for incoming connections
 */
func listen(c *cli.Context) error {
	port := c.String("port")
	// daemon := c.Bool("d")
	addr := strings.Join([]string{"0.0.0.0:", port}, "")
	log.Printf("Starting http server on %q", addr)
	go HTTPGo(addr)
	// TODO: make it return nil
	select {}
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
	attachKillHandler()
	InitLogger()
	app := &cli.App{
		Name:  "webexec",
		Usage: "execute commands and pipe their stdin&stdout over webrtc",
		Commands: []*cli.Command{
			{
				Name:    "listen",
				Aliases: []string{"l"},
				Usage: `listen for incoming WebRTC connections,
execute commands for authorized clients and pipe STDIN & STDOUT`,
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:    "port",
						Aliases: []string{"p"},
						Usage:   "TCP port to use for http server",
						Value:   7777,
					},
					&cli.BoolFlag{
						Name:    "daemon",
						Aliases: []string{"d"},
						Usage:   "Run as daemon, saving PID in ~/.webexec/webexec.run",
					},
					&cli.BoolFlag{
						Name:  "debug",
						Usage: "Run in debug mode",
					},
				},
				Action: listen,
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
