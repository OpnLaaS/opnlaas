package iso

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kdomanski/iso9660"
	iso9660util "github.com/kdomanski/iso9660/util"
	"github.com/opnlaas/opnlaas/config"
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
	storageDir := filepath.Clean(outputStorageDirectory)
	kernelISOPath := extracted.KernelPath
	initrdISOPath := extracted.InitrdPath

	if err = os.MkdirAll(storageDir, 0755); err != nil {
		return
	}

	var (
		httpArtifacts = ""
		tftpArtifacts = ""
	)
	if root := strings.TrimSpace(config.Config.PXE.HTTPServer.Directory); root != "" {
		httpArtifacts = filepath.Join(root, "artifacts", extracted.Name)
		if err = os.MkdirAll(httpArtifacts, 0755); err != nil {
			return
		}
	}
	if root := strings.TrimSpace(config.Config.PXE.TFTPServer.Directory); root != "" {
		tftpArtifacts = filepath.Join(root, "artifacts", extracted.Name)
		if err = os.MkdirAll(tftpArtifacts, 0755); err != nil {
			return
		}
	}

	isoFilename := last(sourceImage, "/")
	storageISO := filepath.Join(storageDir, isoFilename)
	if err = copyFile(sourceImage, storageISO); err != nil {
		return
	}
	if err = copyToTargets(sourceImage, filepath.Join(httpArtifacts, "image.iso"), filepath.Join(tftpArtifacts, "image.iso")); err != nil {
		return
	}

	if err = copyISOEntry(img, kernelISOPath, filepath.Join(storageDir, filepath.Base(kernelISOPath))); err != nil {
		return
	}
	if err = copyISOEntry(img, kernelISOPath, filepath.Join(httpArtifacts, "kernel"), filepath.Join(tftpArtifacts, "kernel")); err != nil {
		return
	}

	if err = copyISOEntry(img, initrdISOPath, filepath.Join(storageDir, filepath.Base(initrdISOPath))); err != nil {
		return
	}
	if err = copyISOEntry(img, initrdISOPath, filepath.Join(httpArtifacts, "initrd"), filepath.Join(tftpArtifacts, "initrd")); err != nil {
		return
	}

	if httpArtifacts != "" {
		stage2Dir := filepath.Join(httpArtifacts, "stage2")
		if err = copyWholeISO(sourceImage, stage2Dir); err != nil {
			return
		}
	}

	extracted.FullISOPath = chooseFirstNonEmpty(filepath.Join(httpArtifacts, "image.iso"), filepath.Join(tftpArtifacts, "image.iso"), storageISO)
	extracted.KernelPath = chooseFirstNonEmpty(filepath.Join(tftpArtifacts, "kernel"), filepath.Join(httpArtifacts, "kernel"), filepath.Join(storageDir, filepath.Base(kernelISOPath)))
	extracted.InitrdPath = chooseFirstNonEmpty(filepath.Join(tftpArtifacts, "initrd"), filepath.Join(httpArtifacts, "initrd"), filepath.Join(storageDir, filepath.Base(initrdISOPath)))
	return
}

func copyToTargets(src string, dests ...string) error {
	for _, dst := range dests {
		if strings.TrimSpace(dst) == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		if err := copyFile(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func copyISOEntry(img *iso9660.Image, isoPath string, dests ...string) error {
	for _, dst := range dests {
		if strings.TrimSpace(dst) == "" {
			continue
		}
		reader, err := openPath(img, isoPath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		if err := pipeReaderToFile(reader, dst); err != nil {
			return err
		}
	}
	return nil
}

func chooseFirstNonEmpty(paths ...string) string {
	for _, p := range paths {
		if strings.TrimSpace(p) != "" {
			return p
		}
	}
	return ""
}

func copyWholeISO(imagePath, dest string) error {
	if strings.TrimSpace(imagePath) == "" || strings.TrimSpace(dest) == "" {
		return nil
	}
	if err := os.RemoveAll(dest); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	file, err := os.Open(imagePath)
	if err != nil {
		return err
	}
	defer file.Close()
	return iso9660util.ExtractImageToDirectory(file, dest)
}

func EnsureStage2Artifacts(imagePath, dest string) error {
	if strings.TrimSpace(imagePath) == "" || strings.TrimSpace(dest) == "" {
		return nil
	}
	return copyWholeISO(imagePath, dest)
}
