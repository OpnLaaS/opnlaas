package hosts

import (
	"github.com/opnlaas/laas/config"
	"github.com/z46-dev/go-logger"
	"github.com/z46-dev/gomysql"
)

var Hosts *gomysql.RegisteredStruct[Host]

func InitDB() (err error) {
	var dbLog *logger.Logger = logger.NewLogger().SetPrefix("[DB]", logger.BoldGreen)

	if err = gomysql.Begin(config.Config.Database.File); err != nil {
		dbLog.Errorf("Failed to initialize database: %v\n", err)
		return
	}

	if Hosts, err = gomysql.Register(Host{}); err != nil {
		dbLog.Errorf("Failed to register Host struct: %v\n", err)
		return
	}

	dbLog.Success("Database initialized!")
	return
}

func CloseDB() (err error) {
	return gomysql.Close()
}

func DatabaseFilePath() string {
	return config.Config.Database.File
}
