package iso

import (
	"errors"
	"os"

	"github.com/kdomanski/iso9660"
	"github.com/opnlaas/opnlaas/db"
)

// extract.go
func ExtractISO(sourceImage, outputStorageDirectory string) (*db.StoredISOImage, error) {
	stat, err := os.Stat(sourceImage)
	if err != nil {
		return nil, err
	}

	out := &db.StoredISOImage{Size: stat.Size()}

	f, err := os.Open(sourceImage)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, err := iso9660.OpenImage(f)
	if err != nil {
		return nil, err
	}

	index, err := buildIndex(img)
	if err != nil {
		if errors.Is(err, ErrUDFHybrid) {
			// try external fallback (no root required)
			index, err = buildIndexExternal(sourceImage)
		}
		if err != nil {
			return nil, err
		}
	}

	k, i, err := findKernelAndInitrd(index)
	if err != nil {
		return nil, err
	}
	out.KernelPath, out.InitrdPath = k, i
	return out, nil
}
