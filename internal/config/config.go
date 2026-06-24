// Package config holds runtime configuration for the Plato server.
package config

// Config is the server runtime configuration.
type Config struct {
	Port    int
	DBPath  string
	WikiDir string
}

// Defaults returns the default configuration.
func Defaults() Config {
	return Config{
		Port:    8080,
		DBPath:  "./plato.db",
		WikiDir: "./data",
	}
}
