package tls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
)

type Config struct {
	CaFile     string `yaml:"ca_file"`
	CertFile   string `yaml:"cert_file"`
	KeyFile    string `yaml:"key_file"`
	ServerName string `yaml:"server_name"`
}

type Tls struct {
	Config
}

func (config *Tls) UnmarshalYAML(unmarshal func(interface{}) error) error {
	err := unmarshal(&config.Config)
	if err != nil {
		return err
	}

	if config.CaFile == "" ||
		config.CertFile == "" ||
		config.KeyFile == "" {
		return errors.New("ca_file, cert_file, and key_file must all be set")
	}

	return nil
}

func (config *Tls) GetTlsConfig() (*tls.Config, error) {
	certFile, err := os.ReadFile(config.CertFile)
	if err != nil {
		return nil, err
	}
	keyFile, err := os.ReadFile(config.KeyFile)
	if err != nil {
		return nil, err
	}
	caFile, err := os.ReadFile(config.CaFile)
	if err != nil {
		return nil, err
	}

	certificate, err := tls.X509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM(caFile)
	if !ok {
		return nil, errors.New("failed to append cert")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{certificate},
		RootCAs:      caCertPool,
		ServerName:   config.ServerName,
	}, nil
}
