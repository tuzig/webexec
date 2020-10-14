package main

import (
	"encoding/base64"
	"encoding/json"
)

// EncodeOffer encodes the input in base64
func EncodeOffer(obj interface{}) string {
	b, err := json.Marshal(obj)
	if err != nil {
		Logger.Errorf("Failed to encode offer: %q", err)
		return ""
	}

	return base64.StdEncoding.EncodeToString(b)
}

// DecodeOffer decodes the input from base64
func DecodeOffer(in string, obj interface{}) error {
	b, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, obj)
	if err != nil {
		return err
	}
	return nil
}
