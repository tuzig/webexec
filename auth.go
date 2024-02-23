package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// FileAuth is an authentication backend that checks tokens against a file of
// authorized tokens
type FileAuth struct {
	TokensFilePath string
}

func NewFileAuth(filepath string) *FileAuth {
	if filepath == "" {
		filepath = ConfPath("authorized_fingerprints")
	}
	// creating the token file
	_, err := os.Stat(filepath)
	if os.IsNotExist(err) {
		tokensFile, err := os.Create(filepath)
		defer tokensFile.Close()
		if err != nil {
			return nil
		}
	}
	return &FileAuth{TokensFilePath: filepath}
}

// ReadAuthorizedTokens reads the tokens file and returns all the tokens in it
func (a *FileAuth) ReadAuthorizedTokens() ([]string, error) {
	var tokens []string
	file, err := os.Open(a.TokensFilePath)
	if err != nil {
		return nil, fmt.Errorf("Failed to open authorized_fingerprints: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// allow comment line that start with #
		if len(line) > 0 && line[0] == '#' {
			continue
		}
		// ignore all the text after the first space
		// this is to allow comments in the file
		if i := strings.Index(line, " "); i != -1 {
			line = line[:i]
		}
		// ignore empty lines
		if len(line) == 0 {
			continue
		}
		tokens = append(tokens, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Failed to read authorized_fingerprints: %s", err)
	}
	return tokens, nil
}

// IsAuthorized checks whether a client token is authorized
func (a *FileAuth) IsAuthorized(clientTokens ...string) bool {
	Logger.Infof("Checking if client is authorized: %v %v", a, clientTokens)
	tokens, err := a.ReadAuthorizedTokens()
	if err != nil {
		return false
	}
	for _, ct := range clientTokens {
		for _, token := range tokens {
			if token == ct {
				return true
			}
		}
	}
	return false
}
