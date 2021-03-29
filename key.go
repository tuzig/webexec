package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"fmt"
	"github.com/pion/webrtc/v3"
	"io/ioutil"
)

// KeyType is the type used to hold our key and cache the webrtc certificates
type KeyType struct {
	Path  string
	certs []webrtc.Certificate
}

func loadKey(path string) (*KeyType, error) {
	return &KeyType{Path: path, certs: nil}, nil
}
func (k *KeyType) GetCerts() ([]webrtc.Certificate, error) {
	var err error
	var privKey *ecdsa.PrivateKey
	if k.certs == nil {
		privKey, err = k.read()
		if privKey == nil {
			Logger.Infof("No key found, generating a fresh one at %q", k.Path)
			privKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				return nil, fmt.Errorf("Failed to generate key: %w", err)
			}
			k.write(privKey)
		}
		Logger.Infof("got a private key: %v", privKey)
		Logger.Infof("	  with a a public key: %s", privKey.Public())
		if err != nil {
			return nil, err
		}
		cert, err := webrtc.GenerateCertificate(privKey)
		if err != nil {
			return nil, err
		}
		k.certs = []webrtc.Certificate{*cert}
	}
	return k.certs, nil
}

func (k *KeyType) read() (*ecdsa.PrivateKey, error) {
	// TODO: make it configurable
	privBytes, err := ioutil.ReadFile(k.Path)
	if err != nil {
		return nil, fmt.Errorf("No ECDSA private key file found")
	}

	parsedKey, err := x509.ParsePKCS8PrivateKey(privBytes)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse ECDSA key, generating a temp one: %w",
			err)
	}
	privateKey, ok := parsedKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("Unable to parse ECDSA key")
	}

	return privateKey, nil
}
func (k *KeyType) write(privKey *ecdsa.PrivateKey) error {
	privBuf, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return fmt.Errorf("Failed to marshal private key: %w", err)
	}
	ioutil.WriteFile(k.Path, privBuf, 0600)
	pubKey := privKey.Public()
	pubBuf, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return fmt.Errorf("Failed to marshal public key: %w", err)
	}
	ioutil.WriteFile(ConfPath("public.key"), pubBuf, 0600)
	return nil
}
