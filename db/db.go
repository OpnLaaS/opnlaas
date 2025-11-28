package db

import (
	"github.com/opnlaas/opnlaas/config"
	"github.com/z46-dev/go-logger"
	"github.com/z46-dev/gomysql"
)

var (
	Hosts           *gomysql.RegisteredStruct[Host]
	StoredISOImages *gomysql.RegisteredStruct[StoredISOImage]

	// You should not be calling this api directly for lock safety
	bookingPeople *gomysql.RegisteredStruct[BookingPerson]
	// You should not be calling this api directly for lock safety
	bookingContainers *gomysql.RegisteredStruct[BookingContainer]
	// You should not be calling this api directly for lock safety
	bookingVMs *gomysql.RegisteredStruct[BookingVM]
	// You should not be calling this api directly for lock safety
	bookingRequests *gomysql.RegisteredStruct[BookingRequest]
	// You should not be calling this api directly for lock safety
	bookings *gomysql.RegisteredStruct[Booking]
)

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

	if StoredISOImages, err = gomysql.Register(StoredISOImage{}); err != nil {
		dbLog.Errorf("Failed to register StoredISOImage struct: %v\n", err)
		return
	}

	if bookings, err = gomysql.Register(Booking{}); err != nil {
		dbLog.Errorf("Failed to register Booking struct: %v\n", err)
		return
	}

	if bookingRequests, err = gomysql.Register(BookingRequest{}); err != nil {
		dbLog.Errorf("Failed to register BookingRequest struct: %v\n", err)
		return
	}

	if bookingPeople, err = gomysql.Register(BookingPerson{}); err != nil {
		dbLog.Errorf("Failed to register BookingPerson struct: %v\n", err)
		return
	}

	if bookingContainers, err = gomysql.Register(BookingContainer{}); err != nil {
		dbLog.Errorf("Failed to register BookingContainer struct: %v\n", err)
		return
	}

	if bookingVMs, err = gomysql.Register(BookingVM{}); err != nil {
		dbLog.Errorf("Failed to register BookingVM struct: %v\n", err)
		return
	}

	BeginPeriodicRefreshes()

	dbLog.Success("Database initialized!")
	return
}

func CloseDB() (err error) {
	return gomysql.Close()
}

func DatabaseFilePath() string {
	return config.Config.Database.File
}
