package config

import (
	"encoding/json"
	"errors"

	"github.com/expki/go-vectorsearch/config"
)

// ParseConfig parses the raw JSON configuration.
func ParseConfig(raw []byte) (config Config, err error) {
	err = json.Unmarshal(raw, &config)
	if err != nil {
		return config, errors.Join(errors.New("unmarshal config"), err)
	}
	return config, nil
}

type Config struct {
	Server   config.ConfigServer `json:"server"`
	TLS      config.ConfigTLS    `json:"tls"`
	Database config.Database     `json:"database"`
	AI       config.AI           `json:"ai"`
	LogLevel config.LogLevel     `json:"log_level"`
}
