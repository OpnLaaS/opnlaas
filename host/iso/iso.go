package iso

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

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
	started := time.Now()

	isoLog.Basicf("Extract start source=%s output=%s\n", sourceImage, outputStorageDirectory)
	defer func() {
		if err != nil {
			isoLog.Errorf("Extract failed source=%s duration=%s error=%v\n", sourceImage, time.Since(started), err)
			return
		}
		if extracted != nil {
			isoLog.Successf(
				"Extract done source=%s duration=%s name=%s distro=%s version=%s arch=%s kernel=%s initrd=%s\n",
				sourceImage,
				time.Since(started),
				extracted.Name,
				extracted.DistroName,
				extracted.Version,
				extracted.Architecture,
				extracted.KernelPath,
				extracted.InitrdPath,
			)
		}
	}()
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic while extracting %s: %v", sourceImage, rec)
			isoLog.Errorf("Extract panic source=%s panic=%v stack=%s\n", sourceImage, rec, debug.Stack())
		}
	}()

	if stat, err = os.Stat(sourceImage); err != nil {
		err = fmt.Errorf("stat source image %s: %w", sourceImage, err)
		return
	}
	isoLog.Basicf("Extract source stat file=%s size_bytes=%d\n", filepath.Base(sourceImage), stat.Size())

	extracted = &db.StoredISOImage{
		Size:        stat.Size(),
		FullISOPath: sourceImage,
	}

	if file, err = os.Open(sourceImage); err != nil {
		err = fmt.Errorf("open source image %s: %w", sourceImage, err)
		return
	}

	defer file.Close()

	if img, err = iso9660.OpenImage(file); err != nil {
		err = fmt.Errorf("open iso9660 image %s: %w", sourceImage, err)
		return
	}
	isoLog.Basicf("Extract iso9660 image opened source=%s\n", sourceImage)

	if index, err = buildIndex(img); err != nil {
		if errors.Is(err, ErrUDFHybrid) {
			isoLog.Warningf("Extract iso9660 index unsupported source=%s; falling back to external tools\n", sourceImage)
			index, err = buildIndexExternal(sourceImage)
		}

		if err != nil {
			err = fmt.Errorf("build index for %s: %w", sourceImage, err)
			return
		}
	}
	isoLog.Basicf("Extract index ready source=%s entries=%d\n", sourceImage, len(index))

	if extracted.KernelPath, extracted.InitrdPath, err = findKernelAndInitrd(index); err != nil {
		err = fmt.Errorf("detect kernel/initrd in %s: %w", sourceImage, err)
		return
	}
	isoLog.Basicf("Extract boot artifacts source=%s kernel=%s initrd=%s\n", sourceImage, extracted.KernelPath, extracted.InitrdPath)

	if err = detectMetaData(extracted, img, index); err != nil {
		err = fmt.Errorf("detect metadata for %s: %w", sourceImage, err)
		return
	}
	isoLog.Basicf(
		"Extract metadata source=%s name=%s distro=%s type=%s version=%s arch=%s preconfigure=%s\n",
		sourceImage,
		extracted.Name,
		extracted.DistroName,
		extracted.DistroType,
		extracted.Version,
		extracted.Architecture,
		extracted.PreConfigure,
	)

	err = createOutputs(extracted, img, sourceImage, outputStorageDirectory)
	if err != nil {
		err = fmt.Errorf("create output artifacts for %s: %w", sourceImage, err)
	}
	return
}
