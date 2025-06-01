package config

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/expki/go-vectorsearch/config"
)

// CreateSample creates a sample configuration file.
func CreateSample(path string) error {
	sample := Config{
		Server: config.ConfigServer{
			HttpAddress:  ":7600",
			HttpsAddress: ":7601",
		},
		TLS: config.ConfigTLS{
			DomainNameServer: []string{},
			IP:               []string{},
			Certificates:     []*config.ConfigTLSPath{},
		},
		AI: config.AI{
			Embed: config.Ollama{
				Url:    []string{"http://localhost:11434"},
				Model:  "nomic-embed-text",
				NumCtx: 8192,
			},
			Generate: config.Ollama{
				Url:    []string{"http://localhost:11434"},
				Model:  "llama3.2",
				NumCtx: 128_000,
			},
			Chat: config.Ollama{
				Url:    []string{"http://localhost:11434"},
				Model:  "llama3.2",
				NumCtx: 128_000,
			},
		},
		Database: config.Database{
			Sqlite:   "./vectorstore.db",
			Cache:    "./cache",
			LogLevel: config.LogLevelError,
		},
		LogLevel: config.LogLevelInfo,
	}
	raw, err := json.MarshalIndent(sample, "", "    ")
	if err != nil {
		return errors.Join(errors.New("could not marshal sample config"), err)
	}
	err = os.WriteFile(path, raw, 0600)
	if err != nil {
		return errors.Join(errors.New("could not write sample config file"), err)
	}
	return nil
}
