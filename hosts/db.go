package hosts

import (
	"github.com/z46-dev/go-logger"
	"github.com/z46-dev/gomysql"
)

var Hosts *gomysql.RegisteredStruct[Host]

func init() {
	var (
		err   error
		dbLog *logger.Logger = logger.NewLogger().SetPrefix("[DB]", logger.BoldGreen)
	)

	if err = gomysql.Begin(":memory:"); err != nil {
		dbLog.Errorf("Failed to initialize database: %v\n", err)
		panic(err)
	}

	if Hosts, err = gomysql.Register(Host{}); err != nil {
		dbLog.Errorf("Failed to register Host struct: %v\n", err)
		panic(err)
	}

	dbLog.Success("Database initialized!")
}
