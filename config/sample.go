package config

import (
	"encoding/json"
	"errors"
	"os"
)

// CreateSample creates a sample configuration file.
func CreateSample(path string) error {
	sample := Config{
		Server: ConfigServer{
			HttpAddress:  ":7600",
			HttpsAddress: ":7601",
		},
		TLS: ConfigTLS{
			DomainNameServer: []string{},
			IP:               []string{},
			Certificates:     []*ConfigTLSPath{},
		},
		URL:           []string{"https://localhost:5000"},
		Token:         "your-token",
		CtxSizeChat:   2048,
		CtxSizeEmbed:  512,
		CtxSizeRerank: 2048,
		Database: Database{
			Sqlite:   "./vectorstore.db",
			LogLevel: LogLevelError,
		},
		LogLevel: LogLevelInfo,
	}
	raw, err := json.MarshalIndent(sample, "", "  ")
	if err != nil {
		return errors.Join(errors.New("could not marshal sample config"), err)
	}
	err = os.WriteFile(path, raw, 0600)
	if err != nil {
		return errors.Join(errors.New("could not write sample config file"), err)
	}
	return nil
}
