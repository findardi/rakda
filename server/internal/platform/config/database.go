package config

import (
	"fmt"
	"net/url"
	"time"
)

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string

	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifeTime time.Duration
	ConnMaxIdleTime time.Duration
}

func LoadDatabaseConfig() (DatabaseConfig, error) {
	port, err := GetEnvInt("DB_PORT", 5432)
	if err != nil {
		return DatabaseConfig{}, err
	}

	maxOpen, err := GetEnvInt("DB_MAX_OPEN_CONNS", 25)
	if err != nil {
		return DatabaseConfig{}, err
	}

	maxIdle, err := GetEnvInt("DB_MAX_IDLE_CONNS", 25)
	if err != nil {
		return DatabaseConfig{}, err
	}

	maxLifeTime, err := GetEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute)
	if err != nil {
		return DatabaseConfig{}, err
	}

	maxIdleTime, err := GetEnvDuration("DB_CONN_MAX_IDLE_TIME", 10*time.Minute)
	if err != nil {
		return DatabaseConfig{}, err
	}

	return DatabaseConfig{
		Host:            GetEnv("DB_HOST", "localhost"),
		Port:            port,
		User:            GetEnv("DB_USER", "root"),
		Password:        GetEnv("DB_PASSWORD", "mypassword"),
		Name:            GetEnv("DB_NAME", "dev_rakda"),
		SSLMode:         GetEnv("DB_SSLMODE", "disable"),
		MaxOpenConns:    maxOpen,
		MaxIdleConns:    maxIdle,
		ConnMaxLifeTime: maxLifeTime,
		ConnMaxIdleTime: maxIdleTime,
	}, nil
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(d.User),
		url.QueryEscape(d.Password),
		d.Host,
		d.Port,
		d.Name,
		d.SSLMode,
	)
}
