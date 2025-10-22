package config

import (
	"encoding/json"

	"github.com/rs/zerolog"
)

type Database struct {
	Name           string `mapstructure:"name"            json:"name"`
	Host           string `mapstructure:"host"            json:"host"`
	MigrationPath  string `mapstructure:"migration_path"  json:"migration_path"`
	Password       string `mapstructure:"password"        json:"password"`
	TimeZone       string `mapstructure:"timezone"        json:"timezone"`
	Username       string `mapstructure:"username"        json:"username"`
	MaxConnections int    `mapstructure:"max_connections" json:"max_connections"`
	MinConnections int    `mapstructure:"min_connections" json:"min_connections"`
	Port           uint16 `mapstructure:"port"            json:"port"`
}

func (d Database) MarshalJSON() ([]byte, error) {
	d.Password = "***"
	type D Database
	return json.Marshal(D(d))
}

func (d Database) MarshalZerologObject(e *zerolog.Event) {
	e.Str("name", d.Name).
		Str("host", d.Host).
		Str("username", d.Username).
		Str("migration_path", d.MigrationPath).
		Str("password", "***")
}
