package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/opnlaas/opnlaas/app"
	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/db"
	"github.com/opnlaas/opnlaas/host/iso"
	"github.com/opnlaas/opnlaas/host/pxe"
	"github.com/z46-dev/go-logger"
)

var (
	log        *logger.Logger
	addISOPath = flag.String("add-iso", "", "Path to a local ISO image to import and index")
)

func init() {
	log = logger.NewLogger().SetPrefix("[MAIN]", logger.BoldPurple)

	var err error
	if err = config.Init("config.toml"); err != nil {
		log.Errorf("Failed to initialize environment: %v\n", err)
		panic(err)
	}
}

func main() {
	flag.Parse()

	var err error

	if err = db.InitDB(); err != nil {
		log.Errorf("Failed to initialize database: %v\n", err)
		panic(err)
	}

	if len(*addISOPath) > 0 {
		var isoRecord *db.StoredISOImage
		if isoRecord, err = importLocalISO(*addISOPath); err != nil {
			log.Errorf("Failed to import ISO %s: %v\n", *addISOPath, err)
			os.Exit(1)
		}

		log.Successf("Imported ISO %s (%s): %+v\n", isoRecord.Name, isoRecord.FullISOPath, isoRecord)
		return
	}

	if err = pxe.InitPXE(); err != nil {
		log.Errorf("Failed to initialize PXE services: %v\n", err)
		panic(err)
	}

	defer pxe.Shutdown()

	if err = app.StartApp(); err != nil {
		log.Errorf("Failed to run web server: %v\n", err)
		panic(err)
	}
}

// importLocalISO handles running the ISO extraction locally without invoking the API
func importLocalISO(imagePath string) (isoRecord *db.StoredISOImage, err error) {
	if isoRecord, err = iso.ExtractISO(imagePath, fmt.Sprintf("%s/%s", config.Config.ISOs.StorageDir, filepath.Base(imagePath))); err != nil {
		return
	}

	err = db.StoredISOImages.Insert(isoRecord)
	return
}
