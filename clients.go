package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

const FPS_FILENAME = "authorized_fingerprints"

var ClientCommands = []*cli.Command{
	{
		Name:   "add",
		Usage:  "add one or more clients",
		Action: addClients,
	}, {
		Name:   "remove",
		Usage:  "remove one or more clients",
		Action: removeClients,
	}, {
		Name:   "list",
		Usage:  "list clients",
		Action: listClients,
	},
}

func addClients(c *cli.Context) error {
	file, err := os.OpenFile(ConfPath(FPS_FILENAME), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("Failed to open authorized_fingerprints: %s", err)
	}
	defer file.Close()
	for _, fp := range c.Args().Slice() {
		if _, err := file.WriteString(fp + "\n"); err != nil {
			return fmt.Errorf("Failed to write to authorized_fingerprints: %s", err)
		}
	}
	return nil
}
func removeClients(c *cli.Context) error {
	file, err := os.OpenFile(ConfPath(FPS_FILENAME), os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("Failed to open authorized_fingerprints: %s", err)
	}
	defer file.Close()
	// read the file
	scanner := bufio.NewScanner(file)
	var fps []string
	for scanner.Scan() {
		fps = append(fps, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("Failed to read authorized_fingerprints: %s", err)
	}
	// remove the fp
	var post []string
	for _, t := range fps {
		found := false
		for _, arg := range c.Args().Slice() {
			if t == arg {
				found = true
				break
			}
		}
		if !found {
			post = append(post, t)
		}
	}
	// write the file
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("Failed to truncate authorized_fingerprints: %s", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("Failed to seek authorized_fingerprints: %s", err)
	}
	for _, t := range post {
		if _, err := file.WriteString(t + "\n"); err != nil {
			return fmt.Errorf("Failed to write to authorized_fingerprints: %s", err)
		}
	}
	return nil
}
func listClients(c *cli.Context) error {
	file, err := os.Open(ConfPath(FPS_FILENAME))
	if err != nil {
		return fmt.Errorf("Failed to open authorized_fingerprints: %s", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	fmt.Println("Authorized fingerprints:")
	for scanner.Scan() {
		line := scanner.Text()
		// skip empty lines
		if len(line) == 0 {
			continue
		}
		// skip comments
		if line[0] == '#' {
			continue
		}
		fmt.Println(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("Failed to read authorized_fingerprints: %s", err)
	}
	return nil
}
