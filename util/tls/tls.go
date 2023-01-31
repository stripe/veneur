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
	err := unmarshal(&tlsConfig.config)
	if err != nil {
		return err
	}

	if tlsConfig.config.CaFile == "" &&
		tlsConfig.config.CertFile == "" &&
		tlsConfig.config.KeyFile == "" {
		return nil
	}

	if tlsConfig.config.CaFile == "" ||
		tlsConfig.config.CertFile == "" ||
		tlsConfig.config.KeyFile == "" {
		return errors.New("ca_file, cert_file, and key_file must all be set")
	}

	certFile, err := os.ReadFile(tlsConfig.config.CertFile)
	if err != nil {
		return err
	}
	keyFile, err := os.ReadFile(tlsConfig.config.KeyFile)
	if err != nil {
		return err
	}
	caFile, err := os.ReadFile(tlsConfig.config.CaFile)
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
