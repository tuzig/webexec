package main

import (
	"bufio"
	"fmt"
	"github.com/pion/webrtc/v3"
	"os"
	"regexp"
)

// TokensFilePath holds the path to a file where each authorized token has a line
var TokensFilePath = ConfPath("authorized_tokens")

// ReadAuthorizedTokens reads the tokens file and returns all the tokens in it
func ReadAuthorizedTokens() ([]string, error) {
	var tokens []string
	file, err := os.Open(TokensFilePath)
	if err != nil {
		return nil, fmt.Errorf("Failed to open authorized_tokens: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		tokens = append(tokens, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Failed to read authorized_tokens: %s", err)
	}
	return tokens, nil
}

// IsAuthorized checks whether a client token is authorized
func IsAuthorized(token string) bool {
	tokens, err := ReadAuthorizedTokens()
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

// Authenticate gets a client's offer and ensure its fingerprint is on file
func Authenticate(offer *webrtc.SessionDescription) bool {
	r, _ := regexp.Compile("(?:a=fingerprint:)[a-z0-9]+ ([0-9][A-Z]{2}(?::))+")
	fp := r.FindString(offer.SDP)
	Logger.Infof("fingerprint=%s", fp)
	return IsAuthorized(fp[14:])
}
