package main

import (
	"bufio"
	"fmt"
	"os"
)

/* MT: I'm not sure I see the value of this code. Users can edit their own tokens file, document it.
   BD: Remove it. I guess I'm getting closer to the MVP and
       "You know the nearer your destination, the more you slip slidingaway"


Also: Add support for empty lines and comments when reading

BD: opened an issue for that https://github.com/tuzig/webexec/issues/15
*/

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
