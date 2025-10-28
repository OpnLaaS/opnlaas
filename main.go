package main

import (
	"github.com/opnlaas/laas/app"
	"github.com/opnlaas/laas/config"
	"github.com/opnlaas/laas/hosts"
	"github.com/z46-dev/go-logger"
)

var log *logger.Logger

func init() {
	log = logger.NewLogger().SetPrefix("[MAIN]", logger.BoldPurple)

	var err error
	if err = config.InitEnv(".env"); err != nil {
		log.Errorf("Failed to initialize environment: %v\n", err)
		panic(err)
	}
}

func main() {
	var err error

	if err = hosts.InitDB(); err != nil {
		log.Errorf("Failed to initialize database: %v\n", err)
		panic(err)
	}

	if err = app.StartApp(); err != nil {
		log.Errorf("Failed to run web server: %v\n", err)
		panic(err)
	}
}
