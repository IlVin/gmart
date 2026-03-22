package dto

import "time"

type CLIOptions struct {
	RunAddress           string        `doc:"[Host:Port] or [:Port] to listen on"  short:"p" default:":8080"`
	DatabaseURI          string        `doc:"PostgreSQL connect string"            short:"d" default:""`
	AccrualSystemAddress string        `doc:"Accrual connect string"               short:"r" default:""`
	JwtSecretKey         string        `doc:"JWT secret key"                       short:"k" default:""`
	JwtTTL               time.Duration `doc:"JWT TTL in duration (10h20m30s)"      short:"j" default:"15m"`
	SessionTTL           time.Duration `doc:"Session TTL in duration (10h20m30s)"  short:"t" default:"100000h"`
}
