package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// Offer encoding
// Encode encodes the input in base64
// It can optionally zip the input before encoding
func EncodeOffer(obj interface{}, b []byte) error {
	desc, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("Failed to json marshal offer: %v", obj)
	}

	base64.StdEncoding.Encode(desc, b)
	return nil
}

// Decode decodes the input from base64
// It can optionally unzip the input after decoding
func DecodeOffer(in string, obj interface{}) error {
	b, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, obj)
	if err != nil {
		panic(err)
	}
	return nil
}
