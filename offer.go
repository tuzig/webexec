package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// EncodeOffer encodes the input in base64
func EncodeOffer(dst []byte, obj interface{}) (int, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return 0, fmt.Errorf("Failed to encode offer: %q", err)
	}
	base64.StdEncoding.Encode(dst, b)
	return base64.StdEncoding.EncodedLen(len(b)), nil
}

// DecodeOffer decodes the input from base64
func DecodeOffer(dst interface{}, src []byte) error {
	b := make([]byte, base64.StdEncoding.DecodedLen(len(src)))
	_, err := base64.StdEncoding.Decode(b, src)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, dst)
	if err != nil {
		return err
	}
	return nil
}
