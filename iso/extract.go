package iso

import (
	"os"

	"github.com/kdomanski/iso9660"
	"github.com/opnlaas/opnlaas/db"
)

func ExtractISO(sourceImage, outputStorageDirectory string) (extracted *db.StoredISOImage, err error) {
	var (
		stat  os.FileInfo
		file  *os.File
		image *iso9660.Image
	)

	if stat, err = os.Stat(sourceImage); err != nil {
		return
	}

	extracted = &db.StoredISOImage{
		Size: stat.Size(),
	}

	if file, err = os.Open(sourceImage); err != nil {
		return
	}

	defer file.Close()

	if image, err = iso9660.OpenImage(file); err != nil {
		return
	}

	

	return
}
