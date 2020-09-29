package main

import (
	"bufio"
	"fmt"
	"github.com/kardianos/osext"
	"github.com/urfave/cli/v2"
	"os"
)

/* MT: I'm not sure I see the value of this code. Users can edit their own tokens file, document it.

Also: Add support for empty lines and comments when reading
*/

// The
var TokensFilePath = ConfPath("authorized_tokens")

func ReadAuthorizedTokens() ([]string, error) {

	var tokens []string
	file, err := os.Open(TokensFilePath)
	if err != nil {
		// MT: Use %w with to wrap errors
		// (see https://blog.golang.org/go1.13-errors)
		return nil, fmt.Errorf("Failed to open authorized_tokens: %s", err)
	}
	// MT: defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		tokens = append(tokens, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Failed to read authorized_tokens: %s", err)
	}
	file.Close() // MT: Should be in defer right after file open
	return tokens, nil
}

// AddToken is used to add a new client token. The function will send the owner
// a url he can click to approve & complete the addition
func AddToken(c *cli.Context) error {
	token := c.Args().First()
	file, err := os.OpenFile(
		TokensFilePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if os.IsNotExist(err) {
		// MT: I don't understand how the executable is related
		// to the error
		execPath, err := osext.Executable()
		if err != nil {
			return fmt.Errorf("Failed to find the executable: %s", err)
		}
		return fmt.Errorf(ErrHomePathMissing, execPath)
	}
	if err != nil {
		return fmt.Errorf("Failed to open authorzied_tokens for write: %s", err)
	}
	// MT: Use fmt.Fprintln or your own writeToken
	l, err := file.WriteString(token)
	if err != nil {
		return fmt.Errorf("Failed to add a token to authorzied_tokens: %s",
			err)
	}
	if l != len(token) {
		return fmt.Errorf("Failed to add a token to authorzied_tokens: wrote %d instead of %d byte",
			l, len(token))
	}
	file.WriteString("\n")
	file.Close()
	return nil
}

// DeleteToken is a command that deletes a client token
func DeleteToken(c *cli.Context) error {
	tbd := c.Args().First()
	tokens, err := ReadAuthorizedTokens()
	if err != nil {
		return err
	}
	file, err := os.Create(TokensFilePath)
	if err != nil {
		return err
	}
	for _, t := range tokens {
		if t == tbd {
			continue
		}
		err = writeToken(file, t)
		if err != nil {
			return err
		}

	}
	file.Close()

	return nil
}

// ListTokens is a command that list the authorized token
func ListTokens(c *cli.Context) error {
	tokens, err := ReadAuthorizedTokens()
	if err != nil {
		return err
	}
	for _, t := range tokens {
		fmt.Println(t)
	}
	return nil
}

// MT: Work with io.Reader and not *os.File
// I don't see the value of this function, use fmt.Fprintln
func writeToken(file *os.File, token string) error {
	l, err := file.WriteString(token)
	if err != nil {
		return fmt.Errorf("Failed to write to authorzied_tokens: %s",
			err)
	}
	if l != len(token) {
		return fmt.Errorf("Failed to write to authorzied_tokens: wrote %d instead of %d byte",
			l, len(token))
	}
	file.WriteString("\n")
	return nil
}
