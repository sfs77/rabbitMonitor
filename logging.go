package main

import (
	log "github.com/sirupsen/logrus"
	"os"
	"strings"
)

func initLogger() {
	log.SetLevel(getLogLevel())
	//if strings.ToUpper(config.OutputFormat) == "JSON" {
	//	log.SetFormatter(&log.JSONFormatter{})
	//} else {
	//	// The TextFormatter is default, you don't actually have to do this.
	//	log.SetFormatter(&log.TextFormatter{})
	//}
	log.SetFormatter(&log.TextFormatter{})
}

func getLogLevel() log.Level {
	lvl := strings.ToLower(os.Getenv("LOG_LEVEL"))
	level, err := log.ParseLevel(lvl)
	if err != nil {
		level = DEFAULT_LOGLEVEL
	}
	return level
}
