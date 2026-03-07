package iso

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	started := time.Now()
	storageDir := filepath.Clean(outputStorageDirectory)
	kernelISOPath := extracted.KernelPath
	initrdISOPath := extracted.InitrdPath
	isoLog.Basicf(
		"Output staging start source=%s storage_dir=%s iso_name=%s kernel_iso=%s initrd_iso=%s\n",
		sourceImage,
		storageDir,
		extracted.Name,
		kernelISOPath,
		initrdISOPath,
	)

	if err = os.MkdirAll(storageDir, 0755); err != nil {
		err = fmt.Errorf("create storage directory %s: %w", storageDir, err)
		return
	}

	var (
		httpArtifacts = ""
		tftpArtifacts = ""
	)
	if root := strings.TrimSpace(config.Config.PXE.HTTPServer.Directory); root != "" {
		httpArtifacts = filepath.Join(root, "artifacts", extracted.Name)
		if err = os.MkdirAll(httpArtifacts, 0755); err != nil {
			err = fmt.Errorf("create HTTP artifacts directory %s: %w", httpArtifacts, err)
			return
		}
	}
	if root := strings.TrimSpace(config.Config.PXE.TFTPServer.Directory); root != "" {
		tftpArtifacts = filepath.Join(root, "artifacts", extracted.Name)
		if err = os.MkdirAll(tftpArtifacts, 0755); err != nil {
			err = fmt.Errorf("create TFTP artifacts directory %s: %w", tftpArtifacts, err)
			return
		}
	}

	isoFilename := last(sourceImage, "/")
	storageISO := filepath.Join(storageDir, isoFilename)
	httpISO := ""
	tftpISO := ""
	if strings.TrimSpace(httpArtifacts) != "" {
		httpISO = filepath.Join(httpArtifacts, "image.iso")
	}
	if strings.TrimSpace(tftpArtifacts) != "" {
		tftpISO = filepath.Join(tftpArtifacts, "image.iso")
	}
	primaryISO := chooseFirstNonEmpty(httpISO, tftpISO, storageISO)

	// Copy full ISO once to reduce upload latency for large images.
	if err = copyToTargets(sourceImage, primaryISO); err != nil {
		err = fmt.Errorf("copy source ISO to primary path %s: %w", primaryISO, err)
		return
	}
	if err = copyISOEntry(img, kernelISOPath, filepath.Join(httpArtifacts, "kernel"), filepath.Join(tftpArtifacts, "kernel")); err != nil {
		err = fmt.Errorf("copy kernel entry %s to PXE artifacts: %w", kernelISOPath, err)
		return
	}
	if err = copyISOEntry(img, initrdISOPath, filepath.Join(httpArtifacts, "initrd"), filepath.Join(tftpArtifacts, "initrd")); err != nil {
		err = fmt.Errorf("copy initrd entry %s to PXE artifacts: %w", initrdISOPath, err)
		return
	}

	// if httpArtifacts != "" {
	// 	stage2Dir := filepath.Join(httpArtifacts, "stage2")
	// 	if err = copyWholeISO(sourceImage, stage2Dir); err != nil {
	// 		fmt.Println(err)
	// 		return
	// 	}
	// }

	extracted.FullISOPath = chooseFirstNonEmpty(filepath.Join(httpArtifacts, "image.iso"), filepath.Join(tftpArtifacts, "image.iso"), storageISO)
	extracted.KernelPath = chooseFirstNonEmpty(filepath.Join(tftpArtifacts, "kernel"), filepath.Join(httpArtifacts, "kernel"), filepath.Join(storageDir, filepath.Base(kernelISOPath)))
	extracted.InitrdPath = chooseFirstNonEmpty(filepath.Join(tftpArtifacts, "initrd"), filepath.Join(httpArtifacts, "initrd"), filepath.Join(storageDir, filepath.Base(initrdISOPath)))
	isoLog.Basicf(
		"Output staging complete source=%s duration=%s full_iso=%s kernel=%s initrd=%s\n",
		sourceImage,
		time.Since(started),
		extracted.FullISOPath,
		extracted.KernelPath,
		extracted.InitrdPath,
	)
	return
}

func copyToTargets(src string, dests ...string) error {
	for _, dst := range dests {
		if strings.TrimSpace(dst) == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("create target directory %s: %w", filepath.Dir(dst), err)
		}
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
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
			return fmt.Errorf("open iso entry %s: %w", isoPath, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("create target directory %s: %w", filepath.Dir(dst), err)
		}
		if err := pipeReaderToFile(reader, dst); err != nil {
			return fmt.Errorf("write iso entry %s -> %s: %w", isoPath, dst, err)
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
		return fmt.Errorf("remove stage2 directory %s: %w", dest, err)
	}
	if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("create stage2 directory %s: %w", dest, err)
	}

	if err := extractWholeISOExternal(imagePath, dest); err == nil {
		return nil
	} else {
		isoLog.Warningf("Stage2 external extract fallback source=%s dest=%s error=%v\n", imagePath, dest, err)
	}

	file, err := os.Open(imagePath)
	if err != nil {
		return fmt.Errorf("open source ISO %s: %w", imagePath, err)
	}
	defer file.Close()
	if err := iso9660util.ExtractImageToDirectory(file, dest); err != nil {
		return fmt.Errorf("extract ISO %s into %s: %w", imagePath, dest, err)
	}
	return nil
}

func extractWholeISOExternal(imagePath, dest string) error {
	var errorsSeen []string

	if _, err := exec.LookPath("bsdtar"); err == nil {
		cmd := exec.Command("bsdtar", "-xf", imagePath, "-C", dest)
		if output, runErr := cmd.CombinedOutput(); runErr == nil {
			isoLog.Basicf("Stage2 external extract success tool=bsdtar source=%s dest=%s\n", imagePath, dest)
			return nil
		} else {
			out := strings.TrimSpace(string(output))
			if len(out) > 320 {
				out = out[:320] + "..."
			}
			errorsSeen = append(errorsSeen, fmt.Sprintf("bsdtar: %v output=%q", runErr, out))
		}
	}

	if _, err := exec.LookPath("7z"); err == nil {
		cmd := exec.Command("7z", "x", "-y", "-o"+dest, "--", imagePath)
		if output, runErr := cmd.CombinedOutput(); runErr == nil {
			isoLog.Basicf("Stage2 external extract success tool=7z source=%s dest=%s\n", imagePath, dest)
			return nil
		} else {
			out := strings.TrimSpace(string(output))
			if len(out) > 320 {
				out = out[:320] + "..."
			}
			errorsSeen = append(errorsSeen, fmt.Sprintf("7z: %v output=%q", runErr, out))
		}
	}

	if len(errorsSeen) == 0 {
		return fmt.Errorf("no external extractor available (bsdtar/7z)")
	}

	return fmt.Errorf("external extractors failed: %s", strings.Join(errorsSeen, "; "))
}

func EnsureStage2Artifacts(imagePath, dest string) error {
	if strings.TrimSpace(imagePath) == "" || strings.TrimSpace(dest) == "" {
		return nil
	}
	isoLog.Basicf("Stage2 extraction start source=%s dest=%s\n", imagePath, dest)
	started := time.Now()
	if err := copyWholeISO(imagePath, dest); err != nil {
		isoLog.Errorf("Stage2 extraction failed source=%s dest=%s duration=%s error=%v\n", imagePath, dest, time.Since(started), err)
		return err
	}
	isoLog.Basicf("Stage2 extraction complete source=%s dest=%s duration=%s\n", imagePath, dest, time.Since(started))
	return nil
}
