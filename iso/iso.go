package iso

import (
	"errors"
	"os"

	"github.com/kdomanski/iso9660"
	"github.com/opnlaas/opnlaas/db"
)

// Need: Name, DistroName, Version

func ExtractISO(sourceImage, outputStorageDirectory string) (extracted *db.StoredISOImage, err error) {
	var (
		stat  os.FileInfo
		file  *os.File
		img   *iso9660.Image
		index []string
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

	if img, err = iso9660.OpenImage(file); err != nil {
		return
	}

	if index, err = buildIndex(img); err != nil {
		if errors.Is(err, ErrUDFHybrid) {
			index, err = buildIndexExternal(sourceImage)
		}

		if err != nil {
			return
		}
	}

	if extracted.KernelPath, extracted.InitrdPath, err = findKernelAndInitrd(index); err != nil {
		return
	}

	if err = detectMetaData(extracted, img, index); err != nil {
		return
	}

	err = createOutputs(extracted, img, sourceImage, outputStorageDirectory)
	return
}
