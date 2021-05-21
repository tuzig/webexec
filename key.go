package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"github.com/pion/webrtc/v3"
	"io/ioutil"
	"math/big"
	"os"
	"time"
)

// KeyType is the type used to hold our key and cache the webrtc certificates
type KeyType struct {
	Name  string
	certs []webrtc.Certificate
}

func (k *KeyType) generate() (*webrtc.Certificate, error) {
	secretKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("Failed to generate key: %w", err)
	}
	origin := make([]byte, 16)
	/* #nosec */
	if _, err := rand.Read(origin); err != nil {
		return nil, err
	}

	// Max random value, a 130-bits integer, i.e 2^130 - 1
	maxBigInt := new(big.Int)
	/* #nosec */
	maxBigInt.Exp(big.NewInt(2), big.NewInt(130), nil).Sub(maxBigInt, big.NewInt(1))
	/* #nosec */
	serialNumber, err := rand.Int(rand.Reader, maxBigInt)
	if err != nil {
		return nil, err
	}

	return webrtc.NewCertificate(secretKey, x509.Certificate{
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
		NotBefore:             time.Now(),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		NotAfter:              time.Now().AddDate(10, 0, 0),
		SerialNumber:          serialNumber,
		Version:               2,
		Subject:               pkix.Name{CommonName: hex.EncodeToString(origin)},
		IsCA:                  true,
	})
}
func GetCerts() ([]webrtc.Certificate, error) {
	if key == nil {
		key = &KeyType{Name: ConfPath("certnkey.pem")}
	}
	var cert *webrtc.Certificate
	if key.certs == nil {
		pb, err := ioutil.ReadFile(key.Name)
		if err != nil {
			Logger.Infof("No certificate found, generating a fresh one at %q",
				key.Name)
			cert, err = key.generate()
			if err != nil {
				return nil, err
			}
			key.save(cert)
		} else {
			cert, err = webrtc.CertificateFromPEM(string(pb))
			if err != nil {
				Logger.Infof("Failed to decode certificate, generating a fresh one: %w", err)
				cert, err = key.generate()
				if err != nil {
					return nil, err
				}
				key.save(cert)
			}
		}
		key.certs = []webrtc.Certificate{*cert}
	}
	return key.certs, nil
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
