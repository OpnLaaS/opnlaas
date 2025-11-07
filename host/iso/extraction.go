package iso

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kdomanski/iso9660"
	"github.com/opnlaas/opnlaas/db"
)

func copyFile(src, dst string) (err error) {
	var (
		srcFile *os.File
		dstFile *os.File
		stat    os.FileInfo
	)

	if srcFile, err = os.Open(src); err != nil {
		return
	}

	defer srcFile.Close()

	if stat, err = srcFile.Stat(); err != nil {
		return
	}

	if dstFile, err = os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, stat.Mode()); err != nil {
		return
	}

	defer dstFile.Close()

	if _, err = srcFile.Seek(0, 0); err != nil {
		return
	}

	_, err = dstFile.ReadFrom(srcFile)
	return
}

func pipeReaderToFile(reader io.Reader, dst string) (err error) {
	var dstFile *os.File

	if dstFile, err = os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0644); err != nil {
		return
	}

	defer dstFile.Close()

	_, err = dstFile.ReadFrom(reader)
	return
}

func last(s string, sep string) string {
	var parts []string = strings.Split(s, sep)
	return parts[len(parts)-1]
}

func createOutputs(extracted *db.StoredISOImage, img *iso9660.Image, sourceImage, outputStorageDirectory string) (err error) {
	var (
		reader        io.Reader
		newImagePath  string = fmt.Sprintf("%s/%s", outputStorageDirectory, last(sourceImage, "/"))
		newKernelPath string = fmt.Sprintf("%s/%s", outputStorageDirectory, last(extracted.KernelPath, "/"))
		newInitrdPath string = fmt.Sprintf("%s/%s", outputStorageDirectory, last(extracted.InitrdPath, "/"))
	)

	if err = os.MkdirAll(outputStorageDirectory, 0755); err != nil {
		return
	}

	// Copy the ISO image to the output storage directory
	if err = copyFile(sourceImage, newImagePath); err != nil {
		return
	}

	// Copy the kernel to the output storage directory
	if reader, err = openPath(img, extracted.KernelPath); err != nil {
		return
	}

	if err = pipeReaderToFile(reader, newKernelPath); err != nil {
		return
	}

	// Copy the initrd to the output storage directory
	if reader, err = openPath(img, extracted.InitrdPath); err != nil {
		return
	}

	if err = pipeReaderToFile(reader, newInitrdPath); err != nil {
		return
	}

	extracted.FullISOPath = newImagePath
	extracted.KernelPath = newKernelPath
	extracted.InitrdPath = newInitrdPath
	return
}
