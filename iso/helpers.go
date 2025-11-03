package iso

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"sort"
	"strings"

	"github.com/kdomanski/iso9660"
)

func indexContains(index []string, target string) bool {
	target = strings.ToLower(target)
	var idx = sort.SearchStrings(index, target)
	return idx < len(index) && index[idx] == target
}

func indexFindFirst(index []string, candidates ...string) (found string, ok bool) {
	for _, cand := range candidates {
		if indexContains(index, cand) {
			return cand, true
		}
	}

	return "", false
}

func indexFindAnyWithPrefix(index []string, prefixes ...string) (out []string) {
	for _, entry := range index {
		for _, prefix := range prefixes {
			if strings.HasPrefix(entry, strings.ToLower(prefix)) {
				out = append(out, entry)
				break
			}
		}
	}

	return
}

// func buildIndex(image *iso9660.Image) (index []string, err error) {
// 	var walkFn func(*iso9660.File, string) error
// 	walkFn = func(file *iso9660.File, currPath string) (err error) {
// 		var lowerPath string = currPath
// 		if lowerPath == "" {
// 			lowerPath = "/"
// 		}

// 		index = append(index, strings.ToLower(lowerPath))
// 		if file.IsDir() {
// 			var children []*iso9660.File
// 			if children, err = file.GetChildren(); err != nil {
// 				panic(err)
// 				return
// 			}

// 			for _, child := range children {
// 				var name string = child.Name()
// 				if name == "." || name == ".." {
// 					continue
// 				}

// 				var next string = path.Join(lowerPath, name)
// 				if !strings.HasPrefix(next, "/") {
// 					next = fmt.Sprintf("/%s", next)
// 				}

// 				if err = walkFn(child, next); err != nil {
// 					panic(err)
// 					return
// 				}
// 			}
// 		}

// 		return
// 	}

// 	var root *iso9660.File
// 	if root, err = image.RootDir(); err != nil {
// 					panic(err)
// 		return
// 	}

// 	if err = walkFn(root, "/"); err != nil {
// 					panic(err)
// 		return
// 	}

// 	sort.Strings(index)
// 	return
// }

// helpers.go
var ErrUDFHybrid = errors.New("udf/hybrid dvd not supported by iso9660 reader")

func isUDFMismatch(err error) bool {
	return err != nil && strings.Contains(err.Error(),
		"little-endian and big-endian value mismatch")
}

func buildIndex(image *iso9660.Image) (index []string, err error) {
	var walkFn func(*iso9660.File, string) error
	walkFn = func(file *iso9660.File, currPath string) error {
		lowerPath := currPath
		if lowerPath == "" {
			lowerPath = "/"
		}
		index = append(index, strings.ToLower(lowerPath))

		if file.IsDir() {
			children, e := file.GetChildren()
			if e != nil {
				if isUDFMismatch(e) {
					return ErrUDFHybrid
				}
				return e
			}
			for _, child := range children {
				name := child.Name()
				if name == "." || name == ".." {
					continue
				}
				next := path.Join(lowerPath, name)
				if !strings.HasPrefix(next, "/") {
					next = "/" + next
				}
				if e := walkFn(child, next); e != nil {
					return e
				}
			}
		}
		return nil
	}

	root, e := image.RootDir()
	if e != nil {
		if isUDFMismatch(e) {
			return nil, ErrUDFHybrid
		}
		return nil, e
	}
	if e := walkFn(root, "/"); e != nil {
		return nil, e
	}
	sort.Strings(index)
	return index, nil
}

// helpers.go
func buildIndexExternal(isoPath string) ([]string, error) {
	var (
		cmd *exec.Cmd
		out []byte
		err error
	)
	if _, e := exec.LookPath("bsdtar"); e == nil {
		cmd = exec.Command("bsdtar", "-tf", isoPath)
		out, err = cmd.Output()
	} else if _, e := exec.LookPath("7z"); e == nil {
		cmd = exec.Command("7z", "l", "-ba", isoPath)
		// 7z prints a table; keep only lines that look like file paths
		out, err = cmd.Output()
	} else {
		return nil, fmt.Errorf("no UDF-capable lister found: install bsdtar or 7z")
	}
	if err != nil {
		return nil, err
	}

	// Normalize into your index format: absolute, lowercase, sorted, dedup.
	var idx []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		// For bsdtar: lines are relative paths. For 7z: filter columns.
		p := strings.TrimSpace(line)
		if p == "" {
			continue
		}
		// For 7z, best-effort: last “column” tends to be the path.
		if strings.Contains(p, " D ") || strings.Contains(p, " .D.. ") {
			// directories will also appear; that’s fine
		}
		// Heuristic: keep entries that look like paths
		if strings.Contains(p, "/") || strings.Contains(p, "\\") {
			p = strings.ReplaceAll(p, "\\", "/")
			if !strings.HasPrefix(p, "/") {
				p = "/" + p
			}
			idx = append(idx, strings.ToLower(path.Clean(p)))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	sort.Strings(idx)
	// dedup
	outIdx := idx[:0]
	var prev string
	for _, s := range idx {
		if s != prev {
			outIdx = append(outIdx, s)
			prev = s
		}
	}
	return outIdx, nil
}

func findPrefixOfIndex(index []string, prefix string) (subsetIndex []string) {
	for _, entry := range index {
		if strings.HasPrefix(entry, strings.ToLower(prefix)) {
			subsetIndex = append(subsetIndex, entry)
		}
	}

	return
}

func findKernelAndInitrd(index []string) (kernelPath, initrdPath string, err error) {
	var ok bool

	// --- Kernel candidates ---
	if kernelPath, ok = indexFindFirst(index,
		// SUSE / openSUSE (x86_64, aarch64, 32-bit variants + legacy fallbacks)
		"/boot/x86_64/loader/linux",
		"/boot/aarch64/loader/linux",
		"/boot/i586/loader/linux",
		"/boot/i386/loader/linux",
		"/boot/i686/loader/linux",
		"/boot/loader/linux", // some spins omit arch subdir
		"/boot/linux",        // legacy fallback
		"/isolinux/linux",
		"/linux", // very old media

		// RHEL-family (RHEL/Rocky/Alma/CentOS/Fedora/Amazon)
		"/images/pxeboot/vmlinuz",
		"/images/pxeboot/vmlinuz0",
		"/isolinux/vmlinuz",
		"/isolinux/generic/vmlinuz", // rare vendor ISOs

		// Ubuntu / Mint (casper + HWE)
		"/casper/hwe_vmlinuz", // <== present on your 24.04 live-server
		"/casper/vmlinuz.efi",
		"/casper/vmlinuz",
		"/install/vmlinuz", // some alt/live variants

		// Debian (multi-arch + netboot)
		"/install.amd/vmlinuz",
		"/install.arm64/vmlinuz",
		"/install.a64/vmlinuz",
		"/install.i386/vmlinuz",
		"/install/netboot/vmlinuz",

		// Arch Linux
		"/arch/boot/x86_64/vmlinuz-linux",
		"/arch/boot/x86_64/vmlinuz",
		"/arch/boot/vmlinuz",
		"/arch/boot/vmlinuz64", // older spins

		// Alpine
		"/boot/vmlinuz-lts",
		"/boot/vmlinuz-virt",
		"/boot/vmlinuz-edge",
		"/boot/vmlinuz",
	); !ok {
		err = fmt.Errorf("could not find kernel in ISO image")
		return
	}

	// --- Initrd candidates ---
	if initrdPath, ok = indexFindFirst(index,
		// SUSE / openSUSE
		"/boot/x86_64/loader/initrd",
		"/boot/aarch64/loader/initrd",
		"/boot/i586/loader/initrd",
		"/boot/i386/loader/initrd",
		"/boot/i686/loader/initrd",
		"/boot/loader/initrd",
		"/boot/initrd", // legacy fallback
		"/isolinux/initrd",
		"/initrd", // very old media

		// RHEL-family
		"/images/pxeboot/initrd.img",
		"/isolinux/initrd.img",

		// Ubuntu / Mint (casper + HWE + fallbacks)
		"/casper/hwe_initrd", // <== present on your 24.04 live-server
		"/casper/initrd.img",
		"/casper/initrd",
		"/casper/initrd.gz",
		"/casper/initrd.lz",
		"/boot/initrd.img", // occasionally exposed at /boot
		"/boot/initrd",
		"/install/initrd.gz", // some alt/live variants

		// Debian (multi-arch + netboot)
		"/install.amd/initrd.gz",
		"/install.arm64/initrd.gz",
		"/install.a64/initrd.gz",
		"/install.i386/initrd.gz",
		"/install/netboot/initrd.gz",

		// Arch Linux
		"/arch/boot/x86_64/initramfs-linux.img",
		"/arch/boot/x86_64/initramfs-linux-fallback.img",
		"/arch/boot/archiso.img",

		// Alpine
		"/boot/initramfs-lts",
		"/boot/initramfs-virt",
		"/boot/initramfs-edge",
		"/boot/initramfs-generic",
		"/boot/initramfs",
		"/boot/modloop-lts", // not the initrd, but leave as last-ditch clue
	); !ok {
		// keep your helpful logging for casper visibility
		err = fmt.Errorf("could not find initrd in ISO image, casper paths: %v", findPrefixOfIndex(index, "/casper/"))
		return
	}

	return
}
