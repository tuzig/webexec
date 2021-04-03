package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"github.com/pion/webrtc/v3"
	"io/ioutil"
	"os"
)

// KeyType is the type used to hold our key and cache the webrtc certificates
type KeyType struct {
	Name  string
	certs []webrtc.Certificate
}

func (k *KeyType) generate() (*webrtc.Certificate, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("Failed to generate key: %w", err)
	}
	cert, err := webrtc.GenerateCertificate(privKey)
	return cert, nil
}
func (k *KeyType) GetCerts() ([]webrtc.Certificate, error) {
	var cert *webrtc.Certificate
	if k.certs == nil {
		pb, err := ioutil.ReadFile(k.Name)
		if err != nil {
			Logger.Infof("No key found, generating a fresh one at %q", k.Name)
			cert, err = k.generate()
			if err != nil {
				return nil, err
			}
			k.save(cert)
		} else {
			cert, err = webrtc.CertificateFromPEM(string(pb))
			if err != nil {
				Logger.Infof("Failed to decode certificate, generating a fresh one: %w", err)
				cert, err = k.generate()
				if err != nil {
					return nil, err
				}
				k.save(cert)
			}
		}
		k.certs = []webrtc.Certificate{*cert}
	}
	return k.certs, nil
}

func (k *KeyType) save(cert *webrtc.Certificate) error {
	o, err := os.Create(k.Name)
	defer o.Close()
	if err != nil {
		return fmt.Errorf("Failed to create private key file: %w", err)
	}
	pem, err := cert.PEM()
	if err != nil {
		return err
	}
	_, err = o.Write([]byte(pem))
	return err
}
