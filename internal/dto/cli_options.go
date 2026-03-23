package dto

import "time"

type CLIOptions struct {
	RunAddress            string        `doc:"[Host:Port] or [:Port] to listen on"  short:"p" default:":8080"`
	DatabaseURI           string        `doc:"PostgreSQL connect string"            short:"d" default:""`
	AccrualSystemAddress  string        `doc:"Accrual connect string"               short:"r" default:""`
	JwtSecretKey          string        `doc:"JWT secret key"                       short:"k" default:""`
	JwtTTL                time.Duration `doc:"JWT TTL in duration (10h20m30s)"      short:"j" default:"15m"`
	SessionTTL            time.Duration `doc:"Session TTL in duration (10h20m30s)"  short:"t" default:"100000h"`
	HttpReadHeaderTimeout time.Duration `doc:"Maximum duration for reading request headers (Slowloris protection)"            env:"HTTP_READ_HEADER_TIMEOUT" envDefault:"5s"`
	HttpIdleTimeout       time.Duration `doc:"Maximum amount of time to wait for the next request when keep-alive is enabled" env:"HTTP_IDLE_TIMEOUT"        envDefault:"30s"`
}
