package main

import (
	"bufio"
	"fmt"
	"os"
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
		tokens = append(tokens, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Failed to read authorized_fingerprints: %s", err)
	}
	return tokens, nil
}

// AuthorizeTokens adds the given tokens to the tokens file
func (a *FileAuth) AuthorizeToken(token string) error {
	file, err := os.OpenFile(a.TokensFilePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("Failed to open authorized_fingerprints: %s", err)
	}
	defer file.Close()
	if _, err := file.WriteString(token + "\n"); err != nil {
		return fmt.Errorf("Failed to write to authorized_fingerprints: %s", err)
	}
	return nil
}

// IsAuthorized checks whether a client token is authorized
func (a *FileAuth) IsAuthorized(clientTokens ...string) bool {
	Logger.Infof("Checking if client is authorized: %v %v", a, clientTokens)
	tokens, err := a.ReadAuthorizedTokens()
	if err != nil {
		return false
	}
	if len(tokens) == 0 {
		// The file is empty, clientTokens should be added to the file and authorized
		a.AuthorizeToken(clientTokens[0])
		return true
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
