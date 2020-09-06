package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"syscall"

	"github.com/urfave/cli/v2"

	"github.com/pion/logging"
)

var Logger logging.LeveledLogger

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
	var client string
	if c.NArg() == 1 {
		client = c.Args().First()
	} else {
		client = ""
	}
	log.Printf("Starting http server on port 8888 %v", client)
	go HTTPGo("0.0.0.0:8888")
	// TODO: make it return nil
	select {}
}

func main() {
	attachKillHandler()
	app := &cli.App{
		Name:  "webexec",
		Usage: "execute commands and pipe their stdin&stdout over webrtc",
		Commands: []*cli.Command{
			{
				Name:    "listen",
				Aliases: []string{"l"},
				Usage:   "listen for incoming connections",
				Action:  listen,
			}, {
				Name:   "init",
				Usage:  "initialize user settings",
				Action: initUser,
			}, {
				Name: "token",
				Subcommands: []*cli.Command{
					{
						Name:   "add",
						Usage:  "add <token>",
						Action: AddToken,
					}, {
						Name:   "delete",
						Usage:  "delete <token>",
						Action: DeleteToken,
					}, {
						Name:   "list",
						Usage:  "list the tokens",
						Action: DeleteToken,
					},
				},
				Usage:  "initialize user settings",
				Action: initUser,
			},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
