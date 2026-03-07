package iso

import "github.com/z46-dev/go-logger"

var isoLog *logger.Logger = logger.NewLogger().SetPrefix("[ISO]", logger.BoldBlue).IncludeTimestamp()
