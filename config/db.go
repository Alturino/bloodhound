package config

import (
	"fmt"
	"net/url"
)

type DB struct {
	MaxConnections int    `mapstructure:"max_connections" json:"max_connections"`
	MinConnections int    `mapstructure:"min_connections" json:"min_connections"`
	Port           int    `mapstructure:"port"            json:"port"`
	Host           string `mapstructure:"host"            json:"host"`
	MigrationPath  string `mapstructure:"migration_path"  json:"migration_path"`
	User           string `mapstructure:"user"            json:"user"`
	Password       string `mapstructure:"password"        json:"password"`
	DBName         string `mapstructure:"dbname"          json:"db_name"`
	SSLMode        string `mapstructure:"sslmode"         json:"ssl_mode"`
}

func (d DB) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(d.User),
		url.QueryEscape(d.Password),
		d.Host,
		d.Port,
		d.DBName,
		d.SSLMode,
	)
}
