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
	// creating the token file
	if filepath == "" {
		filepath = ConfPath("authorized_fingerprints")
	}
	_, err := os.Stat(filepath)
	if os.IsNotExist(err) {
		tokensFile, err := os.Create(filepath)
		defer tokensFile.Close()
		if err != nil {
			Logger.Errorf("Failed to create authorized_fingerprints: %s", err)
			return nil
		}
	}
	return &FileAuth{TokensFilePath: filepath}
}
func compressFP(fp string) string {
	hex := strings.Split(fp, " ")[1]
	s := strings.Replace(hex, ":", "", -1)
	return strings.ToUpper(s)
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

// IsAuthorized checks whether a client token is authorized
func (a *FileAuth) IsAuthorized(token string) bool {
	tokens, err := a.ReadAuthorizedTokens()
	if err != nil {
		Logger.Error(err)
		return false
	}
	for _, at := range tokens {
		if token == at {
			return true
		}
	}
	return false
}
