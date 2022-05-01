package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/pion/webrtc/v3"
)

// TokensFilePath holds the path to a file where each authorized token has a line
var TokensFilePath string

func compressFP(fp string) string {
	hex := strings.Split(fp, " ")[1]
	s := strings.Replace(hex, ":", "", -1)
	return strings.ToUpper(s)
}

// ReadAuthorizedTokens reads the tokens file and returns all the tokens in it
func ReadAuthorizedTokens() ([]string, error) {
	var tokens []string
	if TokensFilePath == "" {
		TokensFilePath = ConfPath("authorized_fingerprints")
	}
	file, err := os.Open(TokensFilePath)
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

// GetFingerprint extract the fingerprints from a client's offer and returns
// a compressed fingerprint
func GetFingerprint(offer *webrtc.SessionDescription) (string, error) {
	s, err := offer.Unmarshal()
	if err != nil {
		return "", fmt.Errorf("Failed to unmarshal sdp: %w", err)
	}
	var f string
	if fingerprint, haveFingerprint := s.Attribute("fingerprint"); haveFingerprint {
		f = fingerprint
	} else {
		for _, m := range s.MediaDescriptions {
			if fingerprint, found := m.Attribute("fingerprint"); found {
				f = fingerprint
				break
			}
		}
	}
	if f == "" {
		return "", fmt.Errorf("Offer has no fingerprint: %v", offer)
	}
	return compressFP(f), nil
}
