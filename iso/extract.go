package iso

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/filesystem"

	"github.com/opnlaas/opnlaas/db"
)

/* =================================================================================
   Public API
   ================================================================================= */

func ExtractISO(rawISOPath, extractDirPath string) (*db.StoredISOImage, error) {
	if rawISOPath == "" || extractDirPath == "" {
		return nil, fmt.Errorf("invalid arguments: iso=%q out=%q", rawISOPath, extractDirPath)
	}
	if err := os.MkdirAll(extractDirPath, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", extractDirPath, err)
	}
	st, err := os.Stat(rawISOPath)
	if err != nil {
		return nil, fmt.Errorf("stat iso: %w", err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("path %q is a directory, expected an ISO file", rawISOPath)
	}

	// Keep a copy of the original ISO next to extracted bits
	dstISO := filepath.Join(extractDirPath, filepath.Base(rawISOPath))
	if err := copyFile(rawISOPath, dstISO); err != nil {
		return nil, fmt.Errorf("copy iso: %w", err)
	}

	// Preferred: mount + walk with normal FS. If that fails (non-root), fallback to diskfs.
	if img, err := extractViaMount(rawISOPath, extractDirPath, st.Size()); err == nil {
		img.FullISOPath = dstISO
		return img, nil
	}

	// Fallback path (works in unprivileged test runs)
	img, err2 := extractViaDiskFS(rawISOPath, extractDirPath, st.Size())
	if err2 != nil {
		return nil, err2
	}
	img.FullISOPath = dstISO
	return img, nil
}

// DebugISOLayout: pretty tree; prefers mounting; falls back to diskfs.
func DebugISOLayout(isoPath string, maxDepth, maxEntriesPerDir int) (string, error) {
	// Try mount first
	mnt, umount := tryMount(isoPath)
	if mnt != "" {
		defer umount()
		return printTreeLocal(mnt, maxDepth, maxEntriesPerDir)
	}
	// Fallback to diskfs tree
	d, err := diskfs.Open(isoPath)
	if err != nil {
		return "", fmt.Errorf("open iso: %w", err)
	}
	fs, err := d.GetFilesystem(0)
	if err != nil {
		return "", fmt.Errorf("open iso filesystem: %w", err)
	}
	return printTreeDiskFS(fs, maxDepth, maxEntriesPerDir)
}

/* =================================================================================
   Strategy A: Mount + local FS
   ================================================================================= */

func extractViaMount(isoPath, outDir string, isoSize int64) (*db.StoredISOImage, error) {
	mnt, umount := tryMount(isoPath)
	if mnt == "" {
		return nil, fmt.Errorf("mount not available")
	}
	defer umount()

	// Detect distro & metadata using local helpers
	dt, name, version, archMeta := detectLocal(mnt)

	kernel, initrd := pickKernelInitrdLocal(mnt, dt)
	if kernel == "" || initrd == "" {
		// Pairs that cover mixed layouts (Arch, SUSE upper, generic)
		if k, i := firstExistingPairLocal(mnt, canonicalPairs()); k != "" && i != "" {
			kernel, initrd = k, i
			if dt == db.DistroTypeOther && isSUSEPair(k, i) {
				dt, name = db.DistroTypeSUSEBased, "openSUSE/SLE"
			}
		}
	}
	if kernel == "" || initrd == "" {
		// GRUB parse on mounted FS
		if k, i := tryGrubLocal(mnt); k != "" && i != "" {
			kernel, initrd = k, i
			if dt == db.DistroTypeOther && isSUSEPair(k, i) {
				dt, name = db.DistroTypeSUSEBased, "openSUSE/SLE"
			}
		}
	}

	if kernel == "" || initrd == "" {
		return nil, fmt.Errorf("unsupported ISO layout or missing kernel/initrd (distro=%s name=%s version=%s)", dt, name, version)
	}

	// Copy out kernel & initrd
	kOut, err := copyFileFrom(mnt, kernel, outDir)
	if err != nil {
		return nil, fmt.Errorf("extract kernel: %w", err)
	}
	iOut, err := copyFileFrom(mnt, initrd, outDir)
	if err != nil {
		return nil, fmt.Errorf("extract initrd: %w", err)
	}

	arch := pickArchitecture(archMeta, kernel, initrd)
	if arch == "" {
		arch = inferArchFromName(filepath.Base(isoPath))
		if arch == "" {
			arch = db.ArchitectureX86_64
		}
	}

	pre := choosePreconfigure(dt, name)
	return &db.StoredISOImage{
		Name:         filepath.Base(isoPath),
		DistroName:   coalesce(name, familyName(dt)),
		Version:      version,
		Size:         isoSize,
		KernelPath:   kOut,
		InitrdPath:   iOut,
		Architecture: arch,
		DistroType:   dt,
		PreConfigure: pre,
	}, nil
}

func tryMount(isoPath string) (mountPoint string, umount func()) {
	// Create a private temp dir next to the ISO (so parallel tests don’t collide)
	tmp := filepath.Join(os.TempDir(), "opnlaas-mnt-"+randSuffix(6))
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return "", func() {}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	// First attempt: kernel loop mount (needs CAP_SYS_ADMIN/root)
	cmd := exec.CommandContext(ctx, "mount", "-o", "loop,ro", "-t", "auto", isoPath, tmp)
	if out, err := cmd.CombinedOutput(); err == nil {
		return tmp, func() { _ = exec.Command("umount", "-f", tmp).Run(); _ = os.RemoveAll(tmp) }
	} else {
		_ = os.RemoveAll(tmp)
		_ = out // ignore, we’ll fallback
	}

	// Optional: fuseiso fallback if present (user-space, no root). We’ll try if binary exists.
	if _, err := exec.LookPath("fuseiso"); err == nil {
		if err := os.MkdirAll(tmp, 0o755); err != nil {
			return "", func() {}
		}
		ctx2, cancel2 := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel2()
		cmd := exec.CommandContext(ctx2, "fuseiso", "-p", isoPath, tmp)
		if out, err := cmd.CombinedOutput(); err == nil {
			return tmp, func() { _ = exec.Command("fusermount", "-u", tmp).Run(); _ = os.RemoveAll(tmp) }
		} else {
			_ = exec.Command("fusermount", "-u", tmp).Run()
			_ = os.RemoveAll(tmp)
			_ = out
		}
	}

	return "", func() {}
}

/* ---- Local FS helpers ---- */

func existsLocal(root, p string) bool {
	_, err := os.Stat(filepath.Join(root, fixSlash(p)))
	return err == nil
}
func readLocal(root, p string, max int64) ([]byte, error) {
	fp := filepath.Join(root, fixSlash(p))
	f, err := os.Open(fp)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var buf bytes.Buffer
	if _, err := io.CopyN(&buf, f, max+1); err != nil && err != io.EOF {
		return nil, err
	}
	if int64(buf.Len()) > max {
		return nil, fmt.Errorf("file too large: %s (> %d bytes)", p, max)
	}
	return buf.Bytes(), nil
}
func copyFileFrom(root, p, outDir string) (string, error) {
	fp := filepath.Join(root, fixSlash(p))
	in, err := os.Open(fp)
	if err != nil {
		return "", err
	}
	defer in.Close()
	dst := filepath.Join(outDir, filepath.Base(fp))
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return "", err
	}
	return dst, nil
}

func detectLocal(mnt string) (db.DistroType, string, string, string) {
	// Windows
	if existsLocal(mnt, "/sources/boot.wim") || existsLocal(mnt, "/sources/install.wim") || existsLocal(mnt, "/bootmgr") {
		return db.DistroTypeWindowsBased, "Windows", "", ""
	}
	// RedHat
	if existsLocal(mnt, "/.treeinfo") || existsLocal(mnt, "/.discinfo") {
		name, ver, arch := readTreeinfoLocal(mnt)
		if name == "" {
			name = "RHEL/Fedora/Rocky/Alma"
		}
		return db.DistroTypeRedHatBased, name, ver, arch
	}
	// Ubuntu
	if existsLocal(mnt, "/README.diskdefines") {
		name, ver, arch := readUbuntuLocal(mnt)
		if name == "" {
			name = "Ubuntu"
		}
		return db.DistroTypeDebianBased, name, ver, arch
	}
	// Debian
	if existsLocal(mnt, "/.disk/info") {
		name, ver, arch := readDebianLocal(mnt)
		if name == "" {
			name = "Debian"
		}
		return db.DistroTypeDebianBased, name, ver, arch
	}
	// SUSE
	if existsLocal(mnt, "/media.1/products") || existsLocal(mnt, "/content") {
		name, ver, arch := readSUSELocal(mnt)
		if name == "" {
			name = "openSUSE/SLE"
		}
		return db.DistroTypeSUSEBased, name, ver, arch
	}
	// SUSE path heuristics
	for _, archDir := range []string{"aarch64", "x86_64", "ppc64le", "s390x"} {
		k := "/boot/" + archDir + "/loader/linux"
		i := "/boot/" + archDir + "/loader/initrd"
		if existsLocal(mnt, k) && (existsLocal(mnt, i) || existsLocal(mnt, i+".gz")) {
			return db.DistroTypeSUSEBased, "openSUSE/SLE", "", normalizeArchStr(archDir)
		}
		// UPPER
		kU := "/BOOT/" + strings.ToUpper(archDir) + "/LOADER/LINUX"
		iU := "/BOOT/" + strings.ToUpper(archDir) + "/LOADER/INITRD"
		if existsLocal(mnt, kU) && (existsLocal(mnt, iU) || existsLocal(mnt, iU+".GZ")) {
			return db.DistroTypeSUSEBased, "openSUSE/SLE", "", normalizeArchStr(archDir)
		}
	}
	// Debian-ish telltales
	if existsLocal(mnt, "/casper/vmlinuz") || existsLocal(mnt, "/install/vmlinuz") || existsLocal(mnt, "/live/vmlinuz") {
		return db.DistroTypeDebianBased, "", "", ""
	}
	// RedHat-ish telltales
	if existsLocal(mnt, "/images/pxeboot/vmlinuz") || existsLocal(mnt, "/isolinux/vmlinuz") {
		return db.DistroTypeRedHatBased, "", "", ""
	}
	// Arch-ish telltales
	if existsLocal(mnt, "/arch/boot/x86_64/vmlinuz-linux") {
		return db.DistroTypeArchBased, "Arch Linux", "", ""
	}
	return db.DistroTypeOther, "", "", ""
}

func pickKernelInitrdLocal(mnt string, dt db.DistroType) (string, string) {
	switch dt {
	case db.DistroTypeRedHatBased:
		return firstLocal(mnt, "/images/pxeboot/vmlinuz", "/isolinux/vmlinuz"),
			firstLocal(mnt, "/images/pxeboot/initrd.img", "/isolinux/initrd.img")
	case db.DistroTypeDebianBased:
		return firstLocal(mnt, "/casper/vmlinuz", "/install/vmlinuz", "/live/vmlinuz", "/linux"),
			firstLocal(mnt, "/casper/initrd", "/install/initrd.gz", "/live/initrd.img", "/initrd", "/initrd.img")
	case db.DistroTypeSUSEBased:
		for _, archDir := range []string{"aarch64", "x86_64", "ppc64le", "s390x"} {
			k := "/boot/" + archDir + "/loader/linux"
			i := "/boot/" + archDir + "/loader/initrd"
			if existsLocal(mnt, k) && existsLocal(mnt, i) {
				return k, i
			}
			if existsLocal(mnt, k) && existsLocal(mnt, i+".gz") {
				return k, i + ".gz"
			}
			kU := "/BOOT/" + strings.ToUpper(archDir) + "/LOADER/LINUX"
			iU := "/BOOT/" + strings.ToUpper(archDir) + "/LOADER/INITRD"
			if existsLocal(mnt, kU) && existsLocal(mnt, iU) {
				return kU, iU
			}
			if existsLocal(mnt, kU) && existsLocal(mnt, iU+".GZ") {
				return kU, iU + ".GZ"
			}
		}
		k := firstLocal(mnt, "/boot/linux", "/BOOT/LINUX")
		if k != "" {
			i := firstLocal(mnt, "/boot/initrd", "/boot/initrd.gz", "/boot/initrd.img", "/BOOT/INITRD", "/BOOT/INITRD.GZ", "/BOOT/INITRD.IMG")
			if i != "" {
				return k, i
			}
		}
	case db.DistroTypeArchBased:
		// Arch: vmlinuz-linux + initramfs-linux.img (as seen in your dump)
		return firstLocal(mnt, "/arch/boot/x86_64/vmlinuz-linux", "/boot/vmlinuz-linux", "/arch/boot/vmlinuz-linux"),
			firstLocal(mnt, "/arch/boot/x86_64/initramfs-linux.img", "/boot/initramfs-linux.img", "/arch/boot/x86_64/archiso.img")
	}
	return "", ""
}

func tryGrubLocal(mnt string) (string, string) {
	cfgs := []string{
		"/boot/grub2/grub.cfg", "/boot/grub/grub.cfg",
		"/EFI/BOOT/grub.cfg", "/EFI/BOOT/grub/grub.cfg", "/EFI/opensuse/grub.cfg",
		"/BOOT/GRUB2/GRUB.CFG", "/BOOT/GRUB/GRUB.CFG", "/EFI/BOOT/GRUB.CFG", "/EFI/OPENSUSE/GRUB.CFG",
	}
	reLinux := regexp.MustCompile(`(?m)^\s*linux(?:efi)?\s+(\S+)(?:\s|$)`)
	reInit := regexp.MustCompile(`(?m)^\s*initrd(?:efi)?\s+(\S+)(?:\s|$)`)
	for _, p := range cfgs {
		if !existsLocal(mnt, p) {
			continue
		}
		b, err := readLocal(mnt, p, 4<<20)
		if err != nil || len(b) == 0 {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(p))
		text := string(b)
		km := reLinux.FindStringSubmatch(text)
		im := reInit.FindStringSubmatch(text)
		if len(km) >= 2 {
			k := sanitizeGrubPath(km[1], dir)
			if existsLocal(mnt, k) {
				var i string
				if len(im) >= 2 {
					i = sanitizeGrubPath(im[1], dir)
				}
				if i == "" {
					for _, c := range []string{"initrd", "initrd.gz", "initrd.img", "INITRD", "INITRD.GZ", "INITRD.IMG"} {
						pp := filepath.ToSlash(filepath.Join(filepath.Dir(k), c))
						if existsLocal(mnt, pp) {
							i = pp
							break
						}
					}
				}
				if i != "" && existsLocal(mnt, i) {
					return k, i
				}
			}
		}
	}
	return "", ""
}

func firstLocal(mnt string, paths ...string) string {
	for _, p := range paths {
		if existsLocal(mnt, p) {
			return p
		}
		if up := strings.ToUpper(p); up != p && existsLocal(mnt, up) {
			return up
		}
	}
	return ""
}

func firstExistingPairLocal(mnt string, pairs []struct{ k, i string }) (string, string) {
	for _, p := range pairs {
		if existsLocal(mnt, p.k) && existsLocal(mnt, p.i) {
			return p.k, p.i
		}
	}
	return "", ""
}

/* =================================================================================
   Strategy B: diskfs fallback (existing logic, trimmed)
   ================================================================================= */

type pair struct{ k, i string }

func extractViaDiskFS(rawISOPath, outDir string, isoSize int64) (*db.StoredISOImage, error) {
	d, err := diskfs.Open(rawISOPath)
	if err != nil {
		return nil, fmt.Errorf("open iso: %w", err)
	}
	fs, err := d.GetFilesystem(0)
	if err != nil {
		return nil, fmt.Errorf("open iso filesystem: %w", err)
	}

	dt, name, version, archMeta := detectFamilyAndMeta(fs)
	if dt == db.DistroTypeWindowsBased {
		return nil, fmt.Errorf("windows ISO detected (%s %s) — PXE here needs boot.wim/BCD, not kernel/initrd", name, version)
	}

	kernelISOPath, initrdISOPath := pickKernelInitrd(fs, dt)
	if kernelISOPath == "" || initrdISOPath == "" {
		kernelISOPath, initrdISOPath = firstExistingPairExact(fs, canonicalPairs())
		if dt == db.DistroTypeOther && isSUSEPair(kernelISOPath, initrdISOPath) {
			dt = db.DistroTypeSUSEBased
			if name == "" {
				name = "openSUSE/SLE"
			}
		}
	}
	if kernelISOPath == "" || initrdISOPath == "" {
		if k, i := tryGrubConfigs(fs); k != "" && i != "" {
			kernelISOPath, initrdISOPath = k, i
			if dt == db.DistroTypeOther && isSUSEPair(k, i) {
				dt = db.DistroTypeSUSEBased
				if name == "" {
					name = "openSUSE/SLE"
				}
			}
		}
	}
	if kernelISOPath == "" || initrdISOPath == "" {
		return nil, fmt.Errorf("unsupported ISO layout or missing kernel/initrd (distro=%s name=%s version=%s)", dt, name, version)
	}

	kOut, err := extractOne(fs, kernelISOPath, outDir)
	if err != nil {
		return nil, fmt.Errorf("extract kernel: %w", err)
	}
	iOut, err := extractOne(fs, initrdISOPath, outDir)
	if err != nil {
		return nil, fmt.Errorf("extract initrd: %w", err)
	}

	arch := pickArchitecture(archMeta, kernelISOPath, initrdISOPath)
	if arch == "" {
		arch = db.ArchitectureX86_64
	}

	pre := choosePreconfigure(dt, name)
	return &db.StoredISOImage{
		Name:         filepath.Base(rawISOPath),
		DistroName:   coalesce(name, familyName(dt)),
		Version:      version,
		Size:         isoSize,
		KernelPath:   kOut,
		InitrdPath:   iOut,
		Architecture: arch,
		DistroType:   dt,
		PreConfigure: pre,
	}, nil
}

/* =================================================================================
   Shared helpers (both strategies)
   ================================================================================= */

func printTreeLocal(root string, maxDepth, maxEntries int) (string, error) {
	var b strings.Builder
	writeln := func(s string) { b.WriteString(s); b.WriteByte('\n') }
	writeln("/")

	type frame struct {
		dir    string
		prefix string
		depth  int
	}
	stack := []frame{{dir: "/", prefix: "", depth: 0}}
	for len(stack) > 0 {
		fr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if fr.depth > maxDepth {
			continue
		}
		ents, err := os.ReadDir(filepath.Join(root, strings.TrimPrefix(fr.dir, "/")))
		if err != nil {
			if fr.dir != "/" {
				writeln(fmt.Sprintf("%s└── [ERR] %s", fr.prefix, fr.dir))
			}
			continue
		}
		sort.Slice(ents, func(i, j int) bool {
			return ents[i].IsDir() && !ents[j].IsDir() || strings.ToLower(ents[i].Name()) < strings.ToLower(ents[j].Name())
		})
		if maxEntries > 0 && len(ents) > maxEntries {
			ents = ents[:maxEntries]
		}
		for i := len(ents) - 1; i >= 0; i-- {
			e := ents[i]
			last := i == 0
			conn := "├── "
			nextPrefix := fr.prefix + "│   "
			if last {
				conn = "└── "
				nextPrefix = fr.prefix + "    "
			}
			name := e.Name()
			full := filepath.ToSlash(filepath.Join(fr.dir, name))
			if e.IsDir() {
				writeln(fmt.Sprintf("%s%s%s/", fr.prefix, conn, name))
				if fr.depth < maxDepth {
					stack = append(stack, frame{dir: full, prefix: nextPrefix, depth: fr.depth + 1})
				}
			} else {
				fi, _ := os.Stat(filepath.Join(root, strings.TrimPrefix(full, "/")))
				writeln(fmt.Sprintf("%s%s%s (%s)", fr.prefix, conn, name, human(fiSize(fi))))
			}
		}
	}
	return b.String(), nil
}

func printTreeDiskFS(fs filesystem.FileSystem, maxDepth, maxEntries int) (string, error) {
	var b strings.Builder
	writeLine := func(s string) { b.WriteString(s); b.WriteByte('\n') }
	writeLine("/")

	type frame struct {
		path   string
		prefix string
		depth  int
	}
	stack := []frame{{path: "", prefix: "", depth: 0}}

	for len(stack) > 0 {
		fr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if fr.depth > maxDepth {
			continue
		}

		entries, err := readDirAny(fs, fr.path)
		if err != nil {
			if fr.path != "" {
				writeLine(fmt.Sprintf("%s└── [ERR] %s", fr.prefix, fr.path))
			}
			continue
		}
		sort.Slice(entries, func(i, j int) bool {
			di := entries[i].IsDir()
			dj := entries[j].IsDir()
			if di != dj {
				return di && !dj
			}
			return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
		})
		if maxEntries > 0 && len(entries) > maxEntries {
			entries = entries[:maxEntries]
		}
		for idx := len(entries) - 1; idx >= 0; idx-- {
			fi := entries[idx]
			name := fi.Name()
			full := name
			if fr.path != "" {
				full = filepath.ToSlash(filepath.Join(fr.path, name))
			}
			last := idx == 0
			conn := "├── "
			nextPrefix := fr.prefix + "│   "
			if last {
				conn = "└── "
				nextPrefix = fr.prefix + "    "
			}
			if fi.IsDir() {
				writeLine(fmt.Sprintf("%s%s%s/", fr.prefix, conn, name))
				if fr.depth < maxDepth {
					stack = append(stack, frame{path: full, prefix: nextPrefix, depth: fr.depth + 1})
				}
			} else {
				writeLine(fmt.Sprintf("%s%s%s", fr.prefix, conn, name))
			}
		}
	}
	return b.String(), nil
}

func detectFamilyAndMeta(fs filesystem.FileSystem) (db.DistroType, string, string, string) {
	// Windows
	if fileExists(fs, "/sources/boot.wim") || fileExists(fs, "/sources/install.wim") || fileExists(fs, "/bootmgr") {
		return db.DistroTypeWindowsBased, "Windows", "", ""
	}
	// RedHat
	if fileExists(fs, "/.treeinfo") || fileExists(fs, "/.discinfo") {
		name, ver, arch := readTreeinfo(fs)
		if name == "" {
			name = "RHEL/Fedora/Rocky/Alma"
		}
		return db.DistroTypeRedHatBased, name, ver, arch
	}
	// Ubuntu
	if fileExists(fs, "/README.diskdefines") {
		name, ver, arch := readUbuntu(fs)
		if name == "" {
			name = "Ubuntu"
		}
		return db.DistroTypeDebianBased, name, ver, arch
	}
	// Debian
	if fileExists(fs, "/.disk/info") {
		name, ver, arch := readDebianDiskInfo(fs)
		if name == "" {
			name = "Debian"
		}
		return db.DistroTypeDebianBased, name, ver, arch
	}
	// SUSE
	if fileExists(fs, "/media.1/products") || fileExists(fs, "/content") {
		name, ver, arch := readSUSE(fs)
		if name == "" {
			name = "openSUSE/SLE"
		}
		return db.DistroTypeSUSEBased, name, ver, arch
	}
	// SUSE path heuristics
	for _, archDir := range []string{"aarch64", "x86_64", "ppc64le", "s390x"} {
		k := "/boot/" + archDir + "/loader/linux"
		i := "/boot/" + archDir + "/loader/initrd"
		if fileExists(fs, k) && (fileExists(fs, i) || fileExists(fs, i+".gz")) {
			return db.DistroTypeSUSEBased, "openSUSE/SLE", "", normalizeArchStr(archDir)
		}
		kU := "/BOOT/" + strings.ToUpper(archDir) + "/LOADER/LINUX"
		iU := "/BOOT/" + strings.ToUpper(archDir) + "/LOADER/INITRD"
		if fileExists(fs, kU) && (fileExists(fs, iU) || fileExists(fs, iU+".GZ")) {
			return db.DistroTypeSUSEBased, "openSUSE/SLE", "", normalizeArchStr(archDir)
		}
	}
	// Debian-ish telltales
	if fileExists(fs, "/casper/vmlinuz") || fileExists(fs, "/install/vmlinuz") || fileExists(fs, "/live/vmlinuz") {
		return db.DistroTypeDebianBased, "", "", ""
	}
	// RedHat-ish telltales
	if fileExists(fs, "/images/pxeboot/vmlinuz") || fileExists(fs, "/isolinux/vmlinuz") {
		return db.DistroTypeRedHatBased, "", "", ""
	}
	// Arch-ish
	if fileExists(fs, "/arch/boot/x86_64/vmlinuz-linux") {
		return db.DistroTypeArchBased, "Arch Linux", "", ""
	}
	return db.DistroTypeOther, "", "", ""
}

/* ---- canonical checks (shared) ---- */

func canonicalPairs() []struct{ k, i string } {
	return []struct{ k, i string }{
		// RedHat
		{"/images/pxeboot/vmlinuz", "/images/pxeboot/initrd.img"},
		{"/isolinux/vmlinuz", "/isolinux/initrd.img"},
		// Debian/Ubuntu
		{"/casper/vmlinuz", "/casper/initrd"},
		{"/install/vmlinuz", "/install/initrd.gz"},
		{"/live/vmlinuz", "/live/initrd.img"},
		// Arch
		{"/arch/boot/x86_64/vmlinuz-linux", "/arch/boot/x86_64/initramfs-linux.img"},
		{"/arch/boot/x86_64/vmlinuz-linux", "/arch/boot/x86_64/archiso.img"},
		// SUSE lower
		{"/boot/x86_64/loader/linux", "/boot/x86_64/loader/initrd"},
		{"/boot/x86_64/loader/linux", "/boot/x86_64/loader/initrd.gz"},
		{"/boot/aarch64/loader/linux", "/boot/aarch64/loader/initrd"},
		{"/boot/aarch64/loader/linux", "/boot/aarch64/loader/initrd.gz"},
		// SUSE UPPER
		{"/BOOT/X86_64/LOADER/LINUX", "/BOOT/X86_64/LOADER/INITRD"},
		{"/BOOT/X86_64/LOADER/LINUX", "/BOOT/X86_64/LOADER/INITRD.GZ"},
		{"/BOOT/AARCH64/LOADER/LINUX", "/BOOT/AARCH64/LOADER/INITRD"},
		{"/BOOT/AARCH64/LOADER/LINUX", "/BOOT/AARCH64/LOADER/INITRD.GZ"},
		// SUSE flat
		{"/boot/linux", "/boot/initrd"},
		{"/boot/linux", "/boot/initrd.gz"},
		{"/boot/linux", "/boot/initrd.img"},
		{"/BOOT/LINUX", "/BOOT/INITRD"},
		{"/BOOT/LINUX", "/BOOT/INITRD.GZ"},
		{"/BOOT/LINUX", "/BOOT/INITRD.IMG"},
		// Generic
		{"/linux", "/initrd"},
		{"/linux", "/initrd.img"},
		{"/LINUX", "/INITRD"},
		{"/LINUX", "/INITRD.IMG"},
	}
}

/* =================================================================================
   DiskFS helpers (unchanged from our previous impl)
   ================================================================================= */

func readDirAny(fs filesystem.FileSystem, p string) ([]os.FileInfo, error) {
	pp := strings.TrimPrefix(p, "/")
	if entries, err := fs.ReadDir(pp); err == nil {
		return entries, nil
	}
	if entries, err := fs.ReadDir(p); err == nil {
		return entries, nil
	}
	if p == "" || p == "/" {
		if entries, err := fs.ReadDir(""); err == nil {
			return entries, nil
		}
	}
	return nil, fmt.Errorf("readdir failed for %q", p)
}

// open, exists, read, extract for diskfs:

func openFileAny(fs filesystem.FileSystem, p string) (filesystem.File, string, error) {
	try := func(q string) (filesystem.File, string, error) {
		if f, err := fs.OpenFile(strings.TrimPrefix(q, "/"), os.O_RDONLY); err == nil {
			return f, strings.TrimPrefix(q, "/"), nil
		}
		if f, err := fs.OpenFile(q, os.O_RDONLY); err == nil {
			return f, q, nil
		}
		return nil, "", fmt.Errorf("no")
	}
	if f, rp, err := try(p); err == nil {
		return f, rp, nil
	}
	if f, rp, err := try(strings.ToLower(p)); err == nil {
		return f, rp, nil
	}
	if f, rp, err := try(strings.ToUpper(p)); err == nil {
		return f, rp, nil
	}
	// UPPER each component
	upp := make([]string, 0, 8)
	for _, seg := range strings.Split(filepath.ToSlash(p), "/") {
		if seg == "" {
			continue
		}
		upp = append(upp, strings.ToUpper(seg))
	}
	if len(upp) > 0 {
		upPath := "/" + strings.Join(upp, "/")
		if f, rp, err := try(upPath); err == nil {
			return f, rp, nil
		}
	}
	return nil, "", fmt.Errorf("open failed: %s", p)
}

func fileExists(fs filesystem.FileSystem, p string) bool {
	f, _, err := openFileAny(fs, p)
	if err == nil {
		_ = f.Close()
		return true
	}
	return false
}

func readSmall(fs filesystem.FileSystem, p string, max int64) ([]byte, error) {
	f, _, err := openFileAny(fs, p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var b []byte
	buf := make([]byte, 64*1024)
	var n int64
	for {
		k, er := f.Read(buf)
		if k > 0 {
			n += int64(k)
			if n > max {
				return nil, fmt.Errorf("file too large: %s (> %d bytes)", p, max)
			}
			b = append(b, buf[:k]...)
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			return nil, er
		}
	}
	return b, nil
}

func extractOne(fs filesystem.FileSystem, isoPath, outDir string) (string, error) {
	in, resolved, err := openFileAny(fs, isoPath)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out := filepath.Join(outDir, filepath.Base(resolved))
	fo, err := os.Create(out)
	if err != nil {
		return "", err
	}
	defer fo.Close()
	if _, err := io.Copy(fo, in); err != nil {
		return "", err
	}
	return out, nil
}

/* =================================================================================
   Parsing helpers and misc
   ================================================================================= */

func readTreeinfo(fs filesystem.FileSystem) (name, version, arch string) {
	if b, err := readSmall(fs, "/.treeinfo", 1<<20); err == nil {
		s := string(b)
		name = kv(s, "family")
		if name == "" {
			name = kv(s, "name")
		}
		version = kv(s, "version")
		arch = normalizeArchStr(kv(s, "arch"))
	}
	if (version == "" || arch == "") && fileExists(fs, "/.discinfo") {
		if b, err := readSmall(fs, "/.discinfo", 1<<20); err == nil {
			lines := strings.Split(strings.TrimSpace(string(b)), "\n")
			if version == "" && len(lines) >= 2 {
				version = strings.TrimSpace(lines[1])
			}
			if arch == "" && len(lines) >= 3 {
				arch = normalizeArchStr(strings.TrimSpace(lines[2]))
			}
		}
	}
	return
}

func readDebianDiskInfo(fs filesystem.FileSystem) (name, version, arch string) {
	if b, err := readSmall(fs, "/.disk/info", 1<<20); err == nil {
		line := strings.TrimSpace(string(b))
		name = "Debian"
		version = firstMatch(line, `([0-9]+(\.[0-9]+)*)`)
		arch = normalizeArchStr(archFromString(line))
	}
	return
}

func readUbuntu(fs filesystem.FileSystem) (name, version, arch string) {
	if b, err := readSmall(fs, "/README.diskdefines", 1<<20); err == nil {
		s := string(b)
		name = "Ubuntu"
		version = firstMatch(s, `([0-9]{2}\.[0-9]{2}(\.[0-9]+)?)`)
		arch = normalizeArchStr(archFromString(s))
	}
	return
}

func readSUSE(fs filesystem.FileSystem) (name, version, arch string) {
	if b, err := readSmall(fs, "/media.1/products", 1<<20); err == nil {
		name = strings.TrimSpace(string(b))
	}
	if b, err := readSmall(fs, "/content", 2<<20); err == nil {
		s := string(b)
		if version == "" {
			version = firstMatch(s, `VERSION\s*=\s*([0-9.]+)`)
		}
		if arch == "" {
			arch = normalizeArchStr(firstMatch(s, `ARCH\s*=\s*([A-Za-z0-9_]+)`))
		}
	}
	return
}

func readTreeinfoLocal(mnt string) (name, version, arch string) {
	if b, err := readLocal(mnt, "/.treeinfo", 1<<20); err == nil {
		s := string(b)
		name = kv(s, "family")
		if name == "" {
			name = kv(s, "name")
		}
		version = kv(s, "version")
		arch = normalizeArchStr(kv(s, "arch"))
	}
	if (version == "" || arch == "") && existsLocal(mnt, "/.discinfo") {
		if b, err := readLocal(mnt, "/.discinfo", 1<<20); err == nil {
			lines := strings.Split(strings.TrimSpace(string(b)), "\n")
			if version == "" && len(lines) >= 2 {
				version = strings.TrimSpace(lines[1])
			}
			if arch == "" && len(lines) >= 3 {
				arch = normalizeArchStr(strings.TrimSpace(lines[2]))
			}
		}
	}
	return
}

func readDebianLocal(mnt string) (name, version, arch string) {
	if b, err := readLocal(mnt, "/.disk/info", 1<<20); err == nil {
		line := strings.TrimSpace(string(b))
		name = "Debian"
		version = firstMatch(line, `([0-9]+(\.[0-9]+)*)`)
		arch = normalizeArchStr(archFromString(line))
	}
	return
}
func readUbuntuLocal(mnt string) (name, version, arch string) {
	if b, err := readLocal(mnt, "/README.diskdefines", 1<<20); err == nil {
		s := string(b)
		name = "Ubuntu"
		version = firstMatch(s, `([0-9]{2}\.[0-9]{2}(\.[0-9]+)?)`)
		arch = normalizeArchStr(archFromString(s))
	}
	return
}
func readSUSELocal(mnt string) (name, version, arch string) {
	if b, err := readLocal(mnt, "/media.1/products", 1<<20); err == nil {
		name = strings.TrimSpace(string(b))
	}
	if b, err := readLocal(mnt, "/content", 2<<20); err == nil {
		s := string(b)
		if version == "" {
			version = firstMatch(s, `VERSION\s*=\s*([0-9.]+)`)
		}
		if arch == "" {
			arch = normalizeArchStr(firstMatch(s, `ARCH\s*=\s*([A-Za-z0-9_]+)`))
		}
	}
	return
}

func tryGrubConfigs(fs filesystem.FileSystem) (kernel, initrd string) {
	cfgs := []string{
		"/boot/grub2/grub.cfg",
		"/boot/grub/grub.cfg",
		"/EFI/BOOT/grub.cfg",
		"/EFI/BOOT/grub/grub.cfg",
		"/EFI/opensuse/grub.cfg",
		"/BOOT/GRUB2/GRUB.CFG",
		"/BOOT/GRUB/GRUB.CFG",
		"/EFI/BOOT/GRUB.CFG",
		"/EFI/OPENSUSE/GRUB.CFG",
	}
	for _, p := range cfgs {
		b, err := readSmall(fs, p, 4<<20)
		if err != nil || len(b) == 0 {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(p))
		text := string(b)
		reLinux := regexp.MustCompile(`(?m)^\s*linux(?:efi)?\s+(\S+)(?:\s|$)`)
		reInit := regexp.MustCompile(`(?m)^\s*initrd(?:efi)?\s+(\S+)(?:\s|$)`)
		km := reLinux.FindStringSubmatch(text)
		im := reInit.FindStringSubmatch(text)
		if len(km) >= 2 {
			k := sanitizeGrubPath(km[1], dir)
			if fileExists(fs, k) {
				var i string
				if len(im) >= 2 {
					i = sanitizeGrubPath(im[1], dir)
				}
				if i == "" {
					for _, c := range []string{"initrd", "initrd.gz", "initrd.img", "INITRD", "INITRD.GZ", "INITRD.IMG"} {
						pp := filepath.ToSlash(filepath.Join(filepath.Dir(k), c))
						if fileExists(fs, pp) {
							i = pp
							break
						}
					}
				}
				if i != "" && fileExists(fs, i) {
					return k, i
				}
			}
		}
	}
	return "", ""
}

/* ---- tiny utils ---- */

func sanitizeGrubPath(p, cfgDir string) string {
	pp := strings.Fields(p)[0]
	if strings.HasPrefix(pp, "(") {
		if idx := strings.Index(pp, ")"); idx >= 0 && idx+1 < len(pp) {
			pp = pp[idx+1:]
		}
	}
	pp = strings.TrimPrefix(pp, "$root")
	pp = strings.TrimPrefix(pp, "${root}")
	if !strings.HasPrefix(pp, "/") {
		pp = filepath.ToSlash(filepath.Join(cfgDir, pp))
	}
	return filepath.ToSlash(pp)
}

func kv(s, key string) string {
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(strings.ToLower(ln), strings.ToLower(key)+"=") {
			return strings.Trim(strings.SplitN(ln, "=", 2)[1], " \t\"'")
		}
	}
	return ""
}

func firstMatch(s, pattern string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(s)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func archFromString(s string) string {
	s = strings.ToLower(s)
	for _, a := range []string{"x86_64", "amd64", "aarch64", "arm64", "ppc64le", "s390x", "i386", "i686"} {
		if strings.Contains(s, a) {
			return a
		}
	}
	return ""
}

func normalizeArchStr(a string) string {
	switch strings.ToLower(a) {
	case "amd64", "x86_64":
		return "x86_64"
	case "aarch64", "arm64":
		return "aarch64"
	default:
		return a
	}
}

func normalizeArch(a string) db.Architecture {
	switch strings.ToLower(a) {
	case "x86_64", "amd64":
		return db.ArchitectureX86_64
	case "aarch64", "arm64":
		return db.ArchitectureARM64
	default:
		return ""
	}
}

func pickArchitecture(archMeta, kernelPath, initrdPath string) db.Architecture {
	if a := normalizeArch(archMeta); a != "" {
		return a
	}
	joined := strings.ToLower(kernelPath + ":" + initrdPath)
	switch {
	case strings.Contains(joined, "aarch64") || strings.Contains(joined, "arm64"):
		return db.ArchitectureARM64
	case strings.Contains(joined, "x86_64") || strings.Contains(joined, "amd64"):
		return db.ArchitectureX86_64
	}
	return db.ArchitectureX86_64
}

func familyName(d db.DistroType) string {
	switch d {
	case db.DistroTypeDebianBased:
		return "Debian/Ubuntu"
	case db.DistroTypeRedHatBased:
		return "RHEL/Fedora"
	case db.DistroTypeArchBased:
		return "Arch"
	case db.DistroTypeSUSEBased:
		return "SUSE"
	case db.DistroTypeAlpineBased:
		return "Alpine"
	case db.DistroTypeWindowsBased:
		return "Windows"
	default:
		return "Other"
	}
}

func choosePreconfigure(d db.DistroType, name string) db.PreConfigureType {
	switch d {
	case db.DistroTypeRedHatBased:
		return db.PreConfigureTypeKickstart
	case db.DistroTypeSUSEBased:
		return db.PreConfigureTypeAutoYaST
	case db.DistroTypeArchBased:
		return db.PreConfigureTypeArchInstallAuto
	case db.DistroTypeAlpineBased:
		return db.PreConfigureTypeCloudInit
	case db.DistroTypeDebianBased:
		if strings.Contains(strings.ToLower(name), "ubuntu") {
			return db.PreConfigureTypeCloudInit
		}
		return db.PreConfigureTypePreseed
	default:
		return db.PreConfigureTypeNone
	}
}

func coalesce(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func human(n int64) string {
	const KB = 1024
	const MB = 1024 * KB
	const GB = 1024 * MB
	switch {
	case n >= GB:
		return fmt.Sprintf("%.1fG", float64(n)/float64(GB))
	case n >= MB:
		return fmt.Sprintf("%.1fM", float64(n)/float64(MB))
	case n >= KB:
		return fmt.Sprintf("%.1fK", float64(n)/float64(KB))
	default:
		return fmt.Sprintf("%dB", n)
	}
}
func fiSize(fi os.FileInfo) int64 {
	if fi == nil {
		return 0
	}
	return fi.Size()
}
func fixSlash(p string) string { return filepath.ToSlash(strings.TrimPrefix(p, "/")) }

func randSuffix(n int) string {
	const alpha = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	f, _ := os.Open("/dev/urandom")
	defer func() { _ = f.Close() }()
	_, _ = io.ReadFull(f, b)
	for i := range b {
		b[i] = alpha[int(b[i])%len(alpha)]
	}
	return string(b)
}

// ------------------------------ Missing helpers (add these) ------------------------------

// Classify SUSE-ish kernel/initrd pairs by path shape.
func isSUSEPair(k, i string) bool {
	k = strings.ToLower(k)
	i = strings.ToLower(i)
	// canonical loader layout
	if (strings.Contains(k, "/boot/x86_64/loader/") && strings.Contains(i, "/boot/x86_64/loader/")) ||
		(strings.Contains(k, "/boot/aarch64/loader/") && strings.Contains(i, "/boot/aarch64/loader/")) {
		return true
	}
	// uppercase variants
	if (strings.Contains(k, "/boot/") && strings.Contains(k, "/loader/")) ||
		(strings.Contains(i, "/boot/") && strings.Contains(i, "/loader/")) {
		return true
	}
	// flat /boot layout sometimes used by SUSE media
	if strings.Contains(k, "/boot/") && strings.Contains(i, "/boot/") {
		return true
	}
	return false
}

// Infer architecture from ISO filename when metadata is missing.
func inferArchFromName(name string) db.Architecture {
	ln := strings.ToLower(name)
	switch {
	case strings.Contains(ln, "aarch64") || strings.Contains(ln, "arm64"):
		return db.ArchitectureARM64
	case strings.Contains(ln, "x86_64") || strings.Contains(ln, "amd64"):
		return db.ArchitectureX86_64
	default:
		return ""
	}
}

// Family-specific kernel/initrd picker for the diskfs fallback path.
func pickKernelInitrd(fs filesystem.FileSystem, dt db.DistroType) (kernel, initrd string) {
	switch dt {
	case db.DistroTypeRedHatBased:
		kernel = firstExisting(fs, "/images/pxeboot/vmlinuz", "/isolinux/vmlinuz")
		initrd = firstExisting(fs, "/images/pxeboot/initrd.img", "/isolinux/initrd.img")
	case db.DistroTypeDebianBased:
		kernel = firstExisting(fs, "/casper/vmlinuz", "/install/vmlinuz", "/live/vmlinuz", "/linux")
		initrd = firstExisting(fs, "/casper/initrd", "/install/initrd.gz", "/live/initrd.img", "/initrd", "/initrd.img")
	case db.DistroTypeSUSEBased:
		// canonical loader layout (lowercase + UPPERCASE)
		for _, archDir := range []string{"aarch64", "x86_64", "ppc64le", "s390x"} {
			k := "/boot/" + archDir + "/loader/linux"
			i := "/boot/" + archDir + "/loader/initrd"
			if fileExists(fs, k) && fileExists(fs, i) {
				return k, i
			}
			if fileExists(fs, k) && fileExists(fs, i+".gz") {
				return k, i + ".gz"
			}
			kU := "/BOOT/" + strings.ToUpper(archDir) + "/LOADER/LINUX"
			iU := "/BOOT/" + strings.ToUpper(archDir) + "/LOADER/INITRD"
			if fileExists(fs, kU) && fileExists(fs, iU) {
				return kU, iU
			}
			if fileExists(fs, kU) && fileExists(fs, iU+".GZ") {
				return kU, iU + ".GZ"
			}
		}
		// flat /boot fallback
		k := firstExisting(fs, "/boot/linux", "/BOOT/LINUX")
		if k != "" {
			i := firstExisting(fs, "/boot/initrd", "/boot/initrd.gz", "/boot/initrd.img",
				"/BOOT/INITRD", "/BOOT/INITRD.GZ", "/BOOT/INITRD.IMG")
			if i != "" {
				return k, i
			}
		}
	case db.DistroTypeArchBased:
		// Arch: vmlinuz-linux + initramfs-linux.img (as seen in your layout)
		kernel = firstExisting(fs, "/arch/boot/x86_64/vmlinuz-linux", "/boot/vmlinuz-linux", "/arch/boot/vmlinuz-linux")
		initrd = firstExisting(fs, "/arch/boot/x86_64/initramfs-linux.img", "/boot/initramfs-linux.img", "/arch/boot/x86_64/archiso.img")
	case db.DistroTypeAlpineBased:
		kernel = firstExisting(fs, "/boot/vmlinuz-lts", "/boot/vmlinuz", "/boot/vmlinuz-*")
		initrd = firstExisting(fs, "/boot/initramfs-lts", "/boot/initramfs-*", "/boot/initramfs")
	}
	return kernel, initrd
}

// Return the first pair that exists (diskfs variant).
func firstExistingPairExact(fs filesystem.FileSystem, pairs []struct{ k, i string }) (string, string) {
	for _, p := range pairs {
		if fileExists(fs, p.k) && fileExists(fs, p.i) {
			return p.k, p.i
		}
	}
	return "", ""
}

// Return the first existing path from the list.
func firstExisting(fs filesystem.FileSystem, paths ...string) string {
	for _, p := range paths {
		if fileExists(fs, p) {
			return p
		}
	}
	return ""
}
