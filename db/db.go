package db

import (
	"os"
	"slices"

	"github.com/opnlaas/opnlaas/config"
	"github.com/z46-dev/go-logger"
	"github.com/z46-dev/gomysql"
)

var (
	Hosts           *gomysql.RegisteredStruct[Host]
	StoredISOImages *gomysql.RegisteredStruct[StoredISOImage]
	HostPXEProfiles *gomysql.RegisteredStruct[HostPXEProfile]

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
	// You should not be calling this api directly for lock safety
	bookingProvisioningStatuses *gomysql.RegisteredStruct[BookingProvisioningStatus]
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

	if HostPXEProfiles, err = gomysql.Register(HostPXEProfile{}); err != nil {
		dbLog.Errorf("Failed to register HostPXEProfile struct: %v\n", err)
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

	if bookingProvisioningStatuses, err = gomysql.Register(BookingProvisioningStatus{}); err != nil {
		dbLog.Errorf("Failed to register BookingProvisioningStatus struct: %v\n", err)
		return
	}

	// Migrations
	var (
		report        *gomysql.MigrationReport
		migrationOpts gomysql.MigrationOptions
	)

	if len(os.Args) > 1 && slices.Contains(os.Args, "--allow-destructive-migrations") {
		migrationOpts.AllowDestructive = true
		dbLog.Warning("Destructive migrations are enabled!")
	}

	if report, err = Hosts.Migrate(migrationOpts); err != nil {
		return
	} else if report != nil {
		if len(report.AddedColumns) > 0 {
			dbLog.Warningf("Added columns to table for %T: %v\n", Host{}, report.AddedColumns)
		}

		if len(report.ChangedColumns) > 0 {
			dbLog.Warningf("Changed columns in table for %T: %v\n", Host{}, report.ChangedColumns)
		}

		if len(report.DroppedColumns) > 0 {
			dbLog.Warningf("Dropped columns from table for %T: %v\n", Host{}, report.DroppedColumns)
		}

		if len(report.RenamedColumns) > 0 {
			dbLog.Warningf("Renamed columns in table for %T: %v\n", Host{}, report.RenamedColumns)
		}

		if report.Rebuilt {
			dbLog.Warningf("Rebuilt table for %T\n", Host{})
		}
	}

	if report, err = StoredISOImages.Migrate(migrationOpts); err != nil {
		return
	} else if report != nil {
		if len(report.AddedColumns) > 0 {
			dbLog.Warningf("Added columns to table for %T: %v\n", StoredISOImage{}, report.AddedColumns)
		}

		if len(report.ChangedColumns) > 0 {
			dbLog.Warningf("Changed columns in table for %T: %v\n", StoredISOImage{}, report.ChangedColumns)
		}

		if len(report.DroppedColumns) > 0 {
			dbLog.Warningf("Dropped columns from table for %T: %v\n", StoredISOImage{}, report.DroppedColumns)
		}

		if len(report.RenamedColumns) > 0 {
			dbLog.Warningf("Renamed columns in table for %T: %v\n", StoredISOImage{}, report.RenamedColumns)
		}

		if report.Rebuilt {
			dbLog.Warningf("Rebuilt table for %T\n", StoredISOImage{})
		}
	}

	if report, err = HostPXEProfiles.Migrate(migrationOpts); err != nil {
		return
	} else if report != nil {
		if len(report.AddedColumns) > 0 {
			dbLog.Warningf("Added columns to table for %T: %v\n", HostPXEProfile{}, report.AddedColumns)
		}

		if len(report.ChangedColumns) > 0 {
			dbLog.Warningf("Changed columns in table for %T: %v\n", HostPXEProfile{}, report.ChangedColumns)
		}

		if len(report.DroppedColumns) > 0 {
			dbLog.Warningf("Dropped columns from table for %T: %v\n", HostPXEProfile{}, report.DroppedColumns)
		}

		if len(report.RenamedColumns) > 0 {
			dbLog.Warningf("Renamed columns in table for %T: %v\n", HostPXEProfile{}, report.RenamedColumns)
		}

		if report.Rebuilt {
			dbLog.Warningf("Rebuilt table for %T\n", HostPXEProfile{})
		}
	}

	if report, err = bookings.Migrate(migrationOpts); err != nil {
		return
	} else if report != nil {
		if len(report.AddedColumns) > 0 {
			dbLog.Warningf("Added columns to table for %T: %v\n", Booking{}, report.AddedColumns)
		}

		if len(report.ChangedColumns) > 0 {
			dbLog.Warningf("Changed columns in table for %T: %v\n", Booking{}, report.ChangedColumns)
		}

		if len(report.DroppedColumns) > 0 {
			dbLog.Warningf("Dropped columns from table for %T: %v\n", Booking{}, report.DroppedColumns)
		}

		if len(report.RenamedColumns) > 0 {
			dbLog.Warningf("Renamed columns in table for %T: %v\n", Booking{}, report.RenamedColumns)
		}

		if report.Rebuilt {
			dbLog.Warningf("Rebuilt table for %T\n", Booking{})
		}
	}

	if report, err = bookingRequests.Migrate(migrationOpts); err != nil {
		return
	} else if report != nil {
		if len(report.AddedColumns) > 0 {
			dbLog.Warningf("Added columns to table for %T: %v\n", BookingRequest{}, report.AddedColumns)
		}

		if len(report.ChangedColumns) > 0 {
			dbLog.Warningf("Changed columns in table for %T: %v\n", BookingRequest{}, report.ChangedColumns)
		}

		if len(report.DroppedColumns) > 0 {
			dbLog.Warningf("Dropped columns from table for %T: %v\n", BookingRequest{}, report.DroppedColumns)
		}

		if len(report.RenamedColumns) > 0 {
			dbLog.Warningf("Renamed columns in table for %T: %v\n", BookingRequest{}, report.RenamedColumns)
		}

		if report.Rebuilt {
			dbLog.Warningf("Rebuilt table for %T\n", BookingRequest{})
		}
	}

	if report, err = bookingPeople.Migrate(migrationOpts); err != nil {
		return
	} else if report != nil {
		if len(report.AddedColumns) > 0 {
			dbLog.Warningf("Added columns to table for %T: %v\n", BookingPerson{}, report.AddedColumns)
		}

		if len(report.ChangedColumns) > 0 {
			dbLog.Warningf("Changed columns in table for %T: %v\n", BookingPerson{}, report.ChangedColumns)
		}

		if len(report.DroppedColumns) > 0 {
			dbLog.Warningf("Dropped columns from table for %T: %v\n", BookingPerson{}, report.DroppedColumns)
		}

		if len(report.RenamedColumns) > 0 {
			dbLog.Warningf("Renamed columns in table for %T: %v\n", BookingPerson{}, report.RenamedColumns)
		}

		if report.Rebuilt {
			dbLog.Warningf("Rebuilt table for %T\n", BookingPerson{})
		}
	}

	if report, err = bookingContainers.Migrate(migrationOpts); err != nil {
		return
	} else if report != nil {
		if len(report.AddedColumns) > 0 {
			dbLog.Warningf("Added columns to table for %T: %v\n", BookingContainer{}, report.AddedColumns)
		}

		if len(report.ChangedColumns) > 0 {
			dbLog.Warningf("Changed columns in table for %T: %v\n", BookingContainer{}, report.ChangedColumns)
		}

		if len(report.DroppedColumns) > 0 {
			dbLog.Warningf("Dropped columns from table for %T: %v\n", BookingContainer{}, report.DroppedColumns)
		}

		if len(report.RenamedColumns) > 0 {
			dbLog.Warningf("Renamed columns in table for %T: %v\n", BookingContainer{}, report.RenamedColumns)
		}

		if report.Rebuilt {
			dbLog.Warningf("Rebuilt table for %T\n", BookingContainer{})
		}
	}

	if report, err = bookingVMs.Migrate(migrationOpts); err != nil {
		return
	} else if report != nil {
		if len(report.AddedColumns) > 0 {
			dbLog.Warningf("Added columns to table for %T: %v\n", BookingVM{}, report.AddedColumns)
		}

		if len(report.ChangedColumns) > 0 {
			dbLog.Warningf("Changed columns in table for %T: %v\n", BookingVM{}, report.ChangedColumns)
		}

		if len(report.DroppedColumns) > 0 {
			dbLog.Warningf("Dropped columns from table for %T: %v\n", BookingVM{}, report.DroppedColumns)
		}

		if len(report.RenamedColumns) > 0 {
			dbLog.Warningf("Renamed columns in table for %T: %v\n", BookingVM{}, report.RenamedColumns)
		}

		if report.Rebuilt {
			dbLog.Warningf("Rebuilt table for %T\n", BookingVM{})
		}
	}

	if report, err = bookingProvisioningStatuses.Migrate(migrationOpts); err != nil {
		return
	} else if report != nil {
		if len(report.AddedColumns) > 0 {
			dbLog.Warningf("Added columns to table for %T: %v\n", BookingProvisioningStatus{}, report.AddedColumns)
		}

		if len(report.ChangedColumns) > 0 {
			dbLog.Warningf("Changed columns in table for %T: %v\n", BookingProvisioningStatus{}, report.ChangedColumns)
		}

		if len(report.DroppedColumns) > 0 {
			dbLog.Warningf("Dropped columns from table for %T: %v\n", BookingProvisioningStatus{}, report.DroppedColumns)
		}

		if len(report.RenamedColumns) > 0 {
			dbLog.Warningf("Renamed columns in table for %T: %v\n", BookingProvisioningStatus{}, report.RenamedColumns)
		}

		if report.Rebuilt {
			dbLog.Warningf("Rebuilt table for %T\n", BookingProvisioningStatus{})
		}
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
