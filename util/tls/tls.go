package tls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
)

type config struct {
	CaFile   string `yaml:"ca_file"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type Tls struct {
	config
	tlsConfig *tls.Config
}

func (tlsConfig *Tls) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var config config
	err := unmarshal(&tlsConfig.config)
	if err != nil {
		return err
	}

	if tlsConfig.config.CaFile == "" ||
		tlsConfig.config.CertFile == "" ||
		tlsConfig.config.KeyFile == "" {
		return nil
	}

	certFile, err := os.ReadFile(config.CertFile)
	if err != nil {
		return err
	}
	keyFile, err := os.ReadFile(config.KeyFile)
	if err != nil {
		return err
	}
	caFile, err := os.ReadFile(config.CaFile)
	if err != nil {
		return err
	}

	certificate, err := tls.X509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM(caFile)
	if !ok {
		return errors.New("failed to append cert")
	}

	tlsConfig.tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{certificate},
		RootCAs:      caCertPool,
	}

	return nil
}

func (tlsConfig *Tls) GetTlsConfig() *tls.Config {
	return tlsConfig.tlsConfig
}
