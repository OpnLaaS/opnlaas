package iso

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kdomanski/iso9660"
	"github.com/opnlaas/opnlaas/db"
)

var ErrUDFHybrid = errors.New("udf/hybrid dvd not supported by iso9660 reader")

func isUDFMismatch(err error) bool {
	return err != nil && strings.Contains(err.Error(), "little-endian and big-endian value mismatch")
}

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

func buildIndex(image *iso9660.Image) (index []string, err error) {
	var walkFn func(*iso9660.File, string) error
	walkFn = func(file *iso9660.File, currPath string) (err error) {
		var lowerPath string = currPath
		if lowerPath == "" {
			lowerPath = "/"
		}

		index = append(index, strings.ToLower(lowerPath))

		if file.IsDir() {
			var children []*iso9660.File
			if children, err = file.GetChildren(); err != nil {
				if isUDFMismatch(err) {
					err = ErrUDFHybrid
				}

				return
			}

			for _, child := range children {
				var name string = child.Name()
				if name == "." || name == ".." {
					continue
				}

				var next string = path.Join(lowerPath, name)
				if !strings.HasPrefix(next, "/") {
					next = "/" + next
				}

				if err = walkFn(child, next); err != nil {
					return
				}
			}
		}

		return
	}

	var root *iso9660.File
	if root, err = image.RootDir(); err != nil {
		if isUDFMismatch(err) {
			err = ErrUDFHybrid
		}

		return
	}

	if err = walkFn(root, "/"); err != nil {
		return
	}

	sort.Strings(index)
	return
}

// Tries external command line tools (xorriso, bsdtar, 7z, isoinfo) to build index
func buildIndexExternal(isoPath string) (index []string, err error) {
	// Try xorriso
	if _, err = exec.LookPath("xorriso"); err == nil {
		var output []byte
		if output, err = exec.Command("xorriso", "-indev", isoPath, "-find", "/", "-print").Output(); err == nil {
			return normalizeIndexLines(strings.NewReader(string(output)), func(line string) (subPath string, ok bool) {
				subPath = strings.TrimSpace(line)
				if subPath == "" || subPath == "/" {
					ok = false
					return
				}

				subPath = strings.ToLower(path.Clean(subPath))
				ok = true
				return
			})
		}
	}

	// Try bsdtar
	if _, err = exec.LookPath("bsdtar"); err == nil {
		var output []byte
		if output, err = exec.Command("bsdtar", "-tf", isoPath).Output(); err == nil {
			return normalizeIndexLines(strings.NewReader(string(output)), func(line string) (subPath string, ok bool) {
				subPath = strings.TrimSpace(line)
				if subPath == "" {
					ok = false
					return
				}

				subPath = strings.ReplaceAll(subPath, "\\", "/")
				if !strings.HasPrefix(subPath, "/") {
					subPath = "/" + subPath
				}

				subPath = strings.ToLower(path.Clean(subPath))
				ok = true
				return
			})
		}
	}

	// Try 7z
	if _, err = exec.LookPath("7z"); err == nil {
		var output []byte
		if output, err = exec.Command("7z", "l", "-slt", "--", isoPath).Output(); err == nil {
			return normalizeIndexLines(strings.NewReader(string(output)), func(line string) (subPath string, ok bool) {
				if !strings.HasPrefix(line, "Path = ") {
					ok = false
					return
				}

				subPath = strings.TrimSpace(strings.TrimPrefix(line, "Path = "))
				if subPath == "" {
					ok = false
					return
				}

				subPath = strings.ReplaceAll(subPath, "\\", "/")
				if !strings.HasPrefix(subPath, "/") {
					subPath = "/" + subPath
				}

				subPath = strings.ToLower(path.Clean(subPath))
				ok = true
				return
			})
		}
	}

	// Try isoinfo
	if _, err = exec.LookPath("isoinfo"); err == nil {
		var output []byte
		if output, err = exec.Command("isoinfo", "-J", "-R", "-f", "-i", isoPath).Output(); err == nil {
			return normalizeIndexLines(strings.NewReader(string(output)), func(line string) (subPath string, ok bool) {
				subPath = strings.TrimSpace(line)
				if subPath == "" {
					ok = false
					return
				}

				subPath = strings.ReplaceAll(subPath, "\\", "/")
				if !strings.HasPrefix(subPath, "/") {
					subPath = "/" + subPath
				}

				subPath = strings.ToLower(path.Clean(subPath))
				ok = true
				return
			})
		}
	}

	err = fmt.Errorf("no UDF-capable lister produced results; install xorriso or bsdtar or 7z")
	return
}

func normalizeIndexLines(r io.Reader, pick func(string) (string, bool)) (index []string, err error) {
	var (
		scanner *bufio.Scanner = bufio.NewScanner(r)
		buf     []byte         = make([]byte, 0, 1024*1024) // In case some tools print very long lines (rare), bump the buffer

	)

	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		var (
			line, subPath string = scanner.Text(), ""
			ok            bool
		)

		if subPath, ok = pick(line); ok {
			index = append(index, subPath)
		}
	}

	if err = scanner.Err(); err != nil {
		return
	}

	sort.Strings(index)

	var (
		out  []string = index[:0]
		prev string
	)

	for _, s := range index {
		if s != prev {
			out = append(out, s)
			prev = s
		}
	}

	index = out
	return
}

func findKernelAndInitrd(index []string) (kernelPath, initrdPath string, err error) {
	var ok bool

	if kernelPath, ok = indexFindFirst(index,
		// SUSE / openSUSE
		"/boot/x86_64/loader/linux",
		"/boot/aarch64/loader/linux",
		"/boot/i586/loader/linux",
		"/boot/i386/loader/linux",
		"/boot/i686/loader/linux",
		"/boot/loader/linux",
		"/boot/linux",
		"/isolinux/linux",
		"/linux",
		"/boot/aarch64/linux",
		"/boot/arm64/loader/linux",

		// RHEL-Based (RHEL/Rocky/Alma/CentOS/Fedora/Amazon)
		"/images/pxeboot/vmlinuz",
		"/images/pxeboot/vmlinuz0",
		"/isolinux/vmlinuz",
		"/isolinux/generic/vmlinuz",

		// Ubuntu / Mint (casper + HWE)
		"/casper/hwe_vmlinuz",
		"/casper/vmlinuz.efi",
		"/casper/vmlinuz",
		"/install/vmlinuz",

		// Debian (multi-arch + netboot)
		"/install.amd/vmlinuz",
		"/install.arm64/vmlinuz",
		"/install.a64/vmlinuz",
		"/install.i386/vmlinuz",
		"/install/netboot/vmlinuz",
		"/install.amd/gtk/vmlinuz",
		"/install.arm64/gtk/vmlinuz",
		"/install.a64/gtk/vmlinuz",

		// Arch Linux
		"/arch/boot/x86_64/vmlinuz-linux",
		"/arch/boot/x86_64/vmlinuz",
		"/arch/boot/vmlinuz",
		"/arch/boot/vmlinuz64",

		// Alpine
		"/boot/vmlinuz-lts",
		"/boot/vmlinuz-virt",
		"/boot/vmlinuz-edge",
		"/boot/vmlinuz",
	); !ok {
		err = fmt.Errorf("could not find kernel in ISO image")
		return
	}

	if initrdPath, ok = indexFindFirst(index,
		// SUSE / openSUSE
		"/boot/x86_64/loader/initrd",
		"/boot/aarch64/loader/initrd",
		"/boot/i586/loader/initrd",
		"/boot/i386/loader/initrd",
		"/boot/i686/loader/initrd",
		"/boot/loader/initrd",
		"/boot/initrd",
		"/isolinux/initrd",
		"/initrd",
		"/boot/aarch64/initrd",
		"/boot/arm64/loader/initrd",

		// RHEL-Based
		"/images/pxeboot/initrd.img",
		"/isolinux/initrd.img",

		// Ubuntu / Mint
		"/casper/hwe_initrd",
		"/casper/initrd.img",
		"/casper/initrd",
		"/casper/initrd.gz",
		"/casper/initrd.lz",
		"/boot/initrd.img",
		"/boot/initrd",
		"/install/initrd.gz",

		// Debian
		"/install.amd/initrd.gz",
		"/install.arm64/initrd.gz",
		"/install.a64/initrd.gz",
		"/install.i386/initrd.gz",
		"/install/netboot/initrd.gz",
		"/install.amd/gtk/initrd.gz",
		"/install.arm64/gtk/initrd.gz",
		"/install.a64/gtk/initrd.gz",

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
		"/boot/modloop-lts",
	); !ok {
		err = fmt.Errorf("could not find initrd in ISO image")
		return
	}

	return
}

func openPath(image *iso9660.Image, isoPath string) (reader io.Reader, err error) {
	isoPath = path.Clean(isoPath)
	var parts []string = strings.Split(isoPath, "/")
	var currDir *iso9660.File

	if currDir, err = image.RootDir(); err != nil {
		return
	}

	for i, part := range parts {
		if part == "" {
			continue
		}

		var children []*iso9660.File
		if children, err = currDir.GetChildren(); err != nil {
			return
		}

		var next *iso9660.File
		var lp string = strings.ToLower(part)

		for _, child := range children {
			if strings.ToLower(child.Name()) == lp {
				next = child
				break
			}
		}

		if next == nil {
			err = fmt.Errorf("path not found in ISO image: %s", isoPath)
			return
		}

		if i == len(parts)-1 {
			reader = next.Reader()
			return
		}

		currDir = next
	}

	return
}

func readFileLines(image *iso9660.Image, path string) (lines []string, err error) {
	var (
		reader       io.Reader
		bufferReader *bufio.Reader
	)

	if reader, err = openPath(image, path); err != nil {
		return
	}

	bufferReader = bufio.NewReader(reader)
	for {
		var line string
		if line, err = bufferReader.ReadString('\n'); err != nil && err != io.EOF {
			return
		}

		line = strings.TrimRight(line, "\r\n")
		lines = append(lines, line)

		if err == io.EOF {
			err = nil
			break
		}
	}

	return
}

func loadConfigs(index []string) (configs []string, err error) {
	var cfgs []string = append([]string{}, indexFindAnyWithPrefix(index, "/boot/grub", "/boot/grub2", "/efi/boot", "/isolinux", "/syslinux")...)
	for _, cfg := range cfgs {
		if strings.HasSuffix(cfg, ".cfg") || strings.HasSuffix(cfg, ".conf") {
			configs = append(configs, cfg)
		} else if path.Base(cfg) == "grub.cfg" || path.Base(cfg) == "grub2.cfg" || path.Base(cfg) == "syslinux.cfg" || path.Base(cfg) == "isolinux.cfg" {
			configs = append(configs, cfg)
		}
	}

	return
}

func readConfigs(image *iso9660.Image, configs []string) (allLines map[string][]string, err error) {
	allLines = make(map[string][]string)
	for _, cfgPath := range configs {
		var lines []string
		if lines, err = readFileLines(image, cfgPath); err != nil {
			return
		}

		allLines[cfgPath] = lines
	}

	return
}

func detectMetaData(extracted *db.StoredISOImage, image *iso9660.Image, index []string) (err error) {
	// 1) Gather content from bootloader configs + the index itself
	configPaths, err := loadConfigs(index)
	if err != nil {
		return
	}
	configLines, err := readConfigs(image, configPaths)
	if err != nil {
		return
	}

	var blobBuilder strings.Builder
	if len(configPaths) > 0 {
		blobBuilder.WriteString(strings.Join(configPaths, " "))
		blobBuilder.WriteByte('\n')
	}
	for _, lines := range configLines {
		blobBuilder.WriteString(strings.Join(lines, "\n"))
		blobBuilder.WriteByte('\n')
	}
	for _, p := range index {
		blobBuilder.WriteString(p)
		blobBuilder.WriteByte('\n')
	}
	blob := strings.ToLower(blobBuilder.String())

	// tiny helpers (style-matched)
	hasAny := func(s string, subs ...string) bool {
		for _, sub := range subs {
			if strings.Contains(s, strings.ToLower(sub)) {
				return true
			}
		}
		return false
	}
	indexHasPrefix := func(prefix string) bool {
		lp := strings.ToLower(prefix)
		for _, p := range index {
			if strings.HasPrefix(p, lp) {
				return true
			}
		}
		return false
	}
	firstLineOrEmpty := func(lines []string) string {
		if len(lines) == 0 {
			return ""
		}
		return strings.TrimSpace(lines[0])
	}
	readIf := func(p string) ([]string, bool) {
		if indexContains(index, p) {
			if ls, e := readFileLines(image, p); e == nil {
				return ls, true
			}
		}
		return nil, false
	}
	archTag := func(a db.Architecture) string {
		switch a {
		case db.ArchitectureX86_64:
			return "(x86_64)"
		case db.ArchitectureARM64:
			return "(aarch64)"
		default:
			return ""
		}
	}
	title := func(s string) string {
		if s == "" {
			return s
		}
		r := []rune(s)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		return string(r)
	}

	// 2) Distro detection (order matters!)
	switch {
	case indexHasPrefix("/boot/x86_64/loader") || indexHasPrefix("/boot/aarch64/loader") ||
		indexHasPrefix("/boot/i586/loader") || indexHasPrefix("/boot/i386/loader") ||
		indexHasPrefix("/suse/") ||
		hasAny(blob, "autoyast", "opensuse", "control.xml", "yast"):
		extracted.DistroType = db.DistroTypeSUSEBased

	case indexHasPrefix("/repodata/") || indexHasPrefix("/images/pxeboot/") ||
		hasAny(blob, ".treeinfo", "anaconda", "fedora", "rhel", "rocky", "alma", "centos", "red hat", "amazon linux"):
		extracted.DistroType = db.DistroTypeRedHatBased

	case indexHasPrefix("/dists/") || indexHasPrefix("/pool/") ||
		indexHasPrefix("/install.") || hasAny(blob, "debian", "ubuntu", "mint", "kali", "pop!_os", "elementary os", "preseed"):
		extracted.DistroType = db.DistroTypeDebianBased

	case indexHasPrefix("/arch/") || hasAny(blob, "archiso", "arch linux", "manjaro"):
		extracted.DistroType = db.DistroTypeArchBased

	case indexHasPrefix("/apks/") || hasAny(blob, "alpine "):
		extracted.DistroType = db.DistroTypeAlpineBased

	default:
		extracted.DistroType = db.DistroTypeOther
	}

	// 3) Architecture detection (first match wins)
	extracted.Architecture = db.Architecture("")
	for _, p := range index {
		if strings.Contains(p, "x86_64") || strings.Contains(p, "amd64") {
			extracted.Architecture = db.ArchitectureX86_64
			break
		}
		if strings.Contains(p, "aarch64") || strings.Contains(p, "arm64") {
			extracted.Architecture = db.ArchitectureARM64
			break
		}
	}

	// 4) Preconfigure detection
	switch {
	case hasAny(blob, " inst.ks=", " ks=", "/ks.cfg", "anaconda", "ksdevice=", "append initrd=initrd.img inst.ks", "append initrd=initrd.img ks="):
		extracted.PreConfigure = db.PreConfigureTypeKickstart
	case hasAny(blob, " autoyast=", "autoyast=", "autoyast.xml", "autoinst.xml", "y2update="):
		extracted.PreConfigure = db.PreConfigureTypeAutoYaST
	case hasAny(blob, " preseed/", "preseed/file=", "file=/cdrom/preseed", "preseed/url=", "auto=true", "priority=critical", "debian-installer", "preseed.cfg"):
		extracted.PreConfigure = db.PreConfigureTypePreseed
	case hasAny(blob, " autoinstall", " ds=nocloud", " nocloud-net", "/nocloud/", "user-data", "meta-data", "cidata"):
		extracted.PreConfigure = db.PreConfigureTypeCloudInit
	case hasAny(blob, "/user-data", "/meta-data", "cloud-init", "seedfrom=", "datasource="):
		extracted.PreConfigure = db.PreConfigureTypeCloudInit
	case hasAny(blob, "archinstall", "/usr/lib/archinstall", "archinstall-guided"):
		extracted.PreConfigure = db.PreConfigureTypeArchInstallAuto
	default:
		extracted.PreConfigure = db.PreConfigureTypeNone
	}
	if extracted.PreConfigure == db.PreConfigureTypeNone {
		if extracted.DistroType == db.DistroTypeDebianBased && (indexHasPrefix("/casper/") || hasAny(blob, "subiquity")) {
			extracted.PreConfigure = db.PreConfigureTypeCloudInit
		}
		if extracted.DistroType == db.DistroTypeRedHatBased {
			extracted.PreConfigure = db.PreConfigureTypeKickstart
		}
		if extracted.DistroType == db.DistroTypeSUSEBased {
			extracted.PreConfigure = db.PreConfigureTypeAutoYaST
		}
	}

	// 5) Extract Name, DistroName, Version (with cleaner rules)
	base := strings.ToLower(filepath.Base(extracted.FullISOPath))
	clean := strings.TrimSuffix(base, filepath.Ext(base))

	if extracted.Name == "" {
		extracted.Name = clean
	}
	if extracted.DistroName == "" {
		extracted.DistroName = "Unknown"
	}

	// version fallback helper that avoids arch tokens
	versionFromName := func(s string) string {
		// remove arch tokens before searching for digits
		s = strings.ReplaceAll(s, "x86_64", "")
		s = strings.ReplaceAll(s, "aarch64", "")
		s = strings.ReplaceAll(s, "arm64", "")
		s = strings.ReplaceAll(s, "i386", "")
		s = strings.ReplaceAll(s, "i686", "")
		s = strings.ReplaceAll(s, "i586", "")
		s = strings.ReplaceAll(s, "_", "-")
		// prefer date-like or dotted semantic versions
		if m := regexp.MustCompile(`\d{4}\.\d{2}\.\d{2}`).FindString(s); m != "" {
			return m
		}
		if m := regexp.MustCompile(`\d+\.\d+(\.\d+)?`).FindString(s); m != "" {
			return m
		}
		// Tumbleweed snapshot/build handled elsewhere
		return ""
	}

	switch extracted.DistroType {

	case db.DistroTypeSUSEBased:
		// /content preferred
		if ls, ok := readIf("/content"); ok {
			if prod, ver := parseSUSEContent(ls); prod != "" || ver != "" {
				// prod is often "openSUSE Tumbleweed" or "openSUSE Leap"
				if prod != "" {
					extracted.DistroName = prod
				}
				// ignore arch-leakage into version (VERSION(arch) filtered in parseSUSEContent)
				if ver != "" && ver != "x86_64" && ver != "aarch64" && ver != "arm64" && ver != "i386" && ver != "i586" && ver != "i686" {
					extracted.Version = ver
				}
			}
		}
		// Label: /media.1/media (clean, human)
		var mediaLabel string
		if ls, ok := readIf("/media.1/media"); ok {
			mediaLabel = firstLineOrEmpty(ls)
		}
		lcLabel := strings.ToLower(mediaLabel)
		lcClean := strings.ToLower(clean)

		// Normalize Tumbleweed media labels: "openSUSE - openSUSE-Tumbleweed-DVD-x86_64-Build4545.1-Media"
		if strings.Contains(lcLabel, "tumbleweed") || strings.Contains(lcClean, "tumbleweed") {
			extracted.DistroName = "openSUSE Tumbleweed"

			// find snapshot/build
			var snap, build string
			if m := regexp.MustCompile(`snapshot(\d{8})`).FindStringSubmatch(lcClean); len(m) == 2 {
				snap = m[1]
			}
			if snap == "" {
				if m := regexp.MustCompile(`snapshot(\d{8})`).FindStringSubmatch(lcLabel); len(m) == 2 {
					snap = m[1]
				}
			}
			if m := regexp.MustCompile(`build(\d+(?:\.\d+)*)`).FindStringSubmatch(lcLabel); len(m) == 2 {
				build = m[1]
			}
			if build == "" {
				if m := regexp.MustCompile(`build(\d+(?:\.\d+)*)`).FindStringSubmatch(lcClean); len(m) == 2 {
					build = m[1]
				}
			}

			// disc type
			disc := ""
			switch {
			case strings.Contains(lcLabel, "dvd") || strings.Contains(lcClean, "dvd"):
				disc = "DVD"
			case strings.Contains(lcLabel, "net") || strings.Contains(lcClean, "net"):
				disc = "NET"
			}

			// version preference: snapshot > build > keep as-is (but never arch)
			if extracted.Version == "" || extracted.Version == "x86_64" || extracted.Version == "aarch64" {
				if snap != "" {
					extracted.Version = "Snapshot" + snap
				} else if build != "" {
					extracted.Version = "Build" + build
				}
			}

			// friendly name
			var parts []string
			parts = append(parts, "openSUSE Tumbleweed")
			if extracted.Version != "" {
				parts = append(parts, extracted.Version)
			}
			if disc != "" {
				parts = append(parts, disc)
			}
			if at := archTag(extracted.Architecture); at != "" {
				parts = append(parts, at)
			}
			extracted.Name = strings.TrimSpace(strings.Join(parts, " "))
		}

		// Normalize Leap installers (online/offline)
		if strings.Contains(strings.ToLower(extracted.DistroName), "leap") {
			kind := ""
			if strings.Contains(lcClean, "offline") || strings.Contains(lcLabel, "offline") {
				kind = "Offline Installer"
			} else if strings.Contains(lcClean, "online") || strings.Contains(lcLabel, "online") {
				kind = "Online Installer"
			}
			if kind != "" {
				var parts []string
				parts = append(parts, "openSUSE Leap")
				if extracted.Version != "" {
					parts = append(parts, extracted.Version)
				}
				parts = append(parts, kind)
				if at := archTag(extracted.Architecture); at != "" {
					parts = append(parts, at)
				}
				extracted.Name = strings.TrimSpace(strings.Join(parts, " "))
				extracted.DistroName = "openSUSE Leap"
			}
		}

		// If still Unknown, use filename hints
		if extracted.DistroName == "Unknown" || extracted.DistroName == "" {
			switch {
			case strings.Contains(lcClean, "tumbleweed"):
				extracted.DistroName = "openSUSE Tumbleweed"
			case strings.Contains(lcClean, "leap"):
				extracted.DistroName = "openSUSE Leap"
			default:
				extracted.DistroName = "SUSE-Based"
			}
		}

		if strings.Contains(strings.ToLower(extracted.DistroName), "leap") {
			nl := strings.ToLower(extracted.Name)
			if nl == strings.ToLower(clean) || strings.Contains(nl, "installer") {
				kind := ""
				lcLabel := ""
				if ls, ok := readIf("/media.1/media"); ok {
					lcLabel = strings.ToLower(firstLineOrEmpty(ls))
				}
				if strings.Contains(strings.ToLower(clean), "offline") || strings.Contains(lcLabel, "offline") {
					kind = "Offline Installer"
				} else if strings.Contains(strings.ToLower(clean), "online") || strings.Contains(lcLabel, "online") {
					kind = "Online Installer"
				}
				if kind != "" {
					var parts []string
					parts = append(parts, "openSUSE Leap")
					if extracted.Version != "" {
						parts = append(parts, extracted.Version)
					}
					parts = append(parts, kind)
					if at := archTag(extracted.Architecture); at != "" {
						parts = append(parts, at)
					}
					extracted.Name = strings.TrimSpace(strings.Join(parts, " "))
					extracted.DistroName = "openSUSE Leap"
				}
			}
		}

	case db.DistroTypeRedHatBased:
		// .treeinfo
		if ls, ok := readIf("/.treeinfo"); ok {
			if name, fam, ver := parseTreeinfo(ls); name != "" || fam != "" || ver != "" {
				if name != "" {
					extracted.Name = name
				}
				if fam != "" {
					extracted.DistroName = fam
				}
				if ver != "" {
					extracted.Version = ver
				}
			}
		}
		// .discinfo fallback (guarded)
		if extracted.DistroName == "Unknown" || extracted.DistroName == "" {
			if ls, ok := readIf("/.discinfo"); ok {
				if prod := parseDiscInfo(ls); prod != "" {
					extracted.Name = prod
					l := strings.ToLower(prod)
					switch {
					case strings.Contains(l, "fedora"):
						extracted.DistroName = "Fedora"
					case strings.Contains(l, "rocky"):
						extracted.DistroName = "Rocky Linux"
					case strings.Contains(l, "alma"):
						extracted.DistroName = "AlmaLinux"
					case strings.Contains(l, "centos"):
						extracted.DistroName = "CentOS"
					case strings.Contains(l, "amazon linux"):
						extracted.DistroName = "Amazon Linux"
					default:
						extracted.DistroName = "RHEL-Based"
					}
				}
			}
		}
		// Friendly names for variants
		if extracted.DistroName != "" && extracted.DistroName != "Unknown" {
			ln := strings.ToLower(strings.TrimSpace(extracted.Name))
			if ln == "baseos" || ln == "server" || ln == "minimal" || ln == "appstream" {
				var parts []string
				parts = append(parts, extracted.DistroName)
				v := title(ln)
				if strings.EqualFold(v, "Baseos") {
					v = "BaseOS"
				}
				if strings.EqualFold(v, "Appstream") {
					v = "AppStream"
				}
				parts = append(parts, v)
				if extracted.Version != "" {
					parts = append(parts, extracted.Version)
				}
				if at := archTag(extracted.Architecture); at != "" {
					parts = append(parts, at)
				}
				extracted.Name = strings.TrimSpace(strings.Join(parts, " "))
			}
		}

	case db.DistroTypeDebianBased:
		// .disk/info (e.g., "ubuntu-server 24.04.3 lts")
		if ls, ok := readIf("/.disk/info"); ok {
			line := strings.TrimSpace(firstLineOrEmpty(ls))
			if line != "" {
				low := strings.ToLower(line)
				i := strings.IndexFunc(low, func(r rune) bool { return r >= '0' && r <= '9' })
				if i > 0 {
					name := strings.TrimSpace(line[:i])
					ver := strings.TrimSpace(line[i:])
					if sp := strings.Fields(ver); len(sp) > 0 {
						ver = sp[0]
					}
					nl := strings.ToLower(name)
					switch {
					case strings.HasPrefix(nl, "kali"):
						name = "Kali Linux"
					case strings.HasPrefix(nl, "ubuntu-server"):
						name = "Ubuntu-Server"
					}
					extracted.DistroName = name
					extracted.Version = ver
					if extracted.Name == "" || extracted.Name == clean {
						extracted.Name = strings.ReplaceAll(name, " ", "-") + "-" + ver
					}
				}
			}
		}
		// .disk/cd_label fallback
		if extracted.Name == clean || extracted.DistroName == "Unknown" {
			if ls, ok := readIf("/.disk/cd_label"); ok {
				if lbl := strings.TrimSpace(firstLineOrEmpty(ls)); lbl != "" {
					extracted.Name = lbl
					if strings.Contains(strings.ToLower(lbl), "ubuntu") && extracted.DistroName == "Unknown" {
						extracted.DistroName = "Ubuntu"
					}
				}
			}
		}
		if extracted.DistroName == "Unknown" || extracted.DistroName == "" {
			switch {
			case strings.Contains(clean, "ubuntu"):
				extracted.DistroName = "Ubuntu"
			case strings.Contains(clean, "debian"):
				extracted.DistroName = "Debian"
			case strings.Contains(clean, "kali"):
				extracted.DistroName = "Kali Linux"
			case strings.Contains(clean, "mint"):
				extracted.DistroName = "Linux Mint"
			default:
				extracted.DistroName = "Debian-Based"
			}
		}

	case db.DistroTypeArchBased:
		extracted.DistroName = "Arch Linux"
		// Arch puts a date in /arch/version or filename
		if ls, ok := readIf("/arch/version"); ok {
			if v := strings.TrimSpace(firstLineOrEmpty(ls)); v != "" {
				extracted.Version = v
			}
		}
		if extracted.Version == "" {
			extracted.Version = versionFromName(clean) // e.g., 2025.11.01
		}
		// Friendly name
		var parts []string
		parts = append(parts, "Arch Linux")
		if extracted.Version != "" {
			parts = append(parts, extracted.Version)
		}
		if at := archTag(extracted.Architecture); at != "" {
			parts = append(parts, at)
		}
		extracted.Name = strings.TrimSpace(strings.Join(parts, " "))

	case db.DistroTypeAlpineBased:
		extracted.DistroName = "Alpine Linux"

		// Prefer on-media release files (pure "x.y.z")
		gotVer := false
		if ls, ok := readIf("/.alpine-release"); ok {
			if v := strings.TrimSpace(firstLineOrEmpty(ls)); v != "" {
				extracted.Version = v
				gotVer = true
			}
		} else if ls, ok := readIf("/alpine-release"); ok {
			if v := strings.TrimSpace(firstLineOrEmpty(ls)); v != "" {
				extracted.Version = v
				gotVer = true
			}
		}

		// Fallback: semver from basename; ignore 6-digit build tags (e.g. 250513)
		if !gotVer {
			// Pick a clean x.y or x.y.z (prefer x.y.z when present)
			if m := regexp.MustCompile(`\b\d+\.\d+\.\d+\b`).FindString(clean); m != "" {
				extracted.Version = m
			} else if m := regexp.MustCompile(`\b\d+\.\d+\b`).FindString(clean); m != "" {
				extracted.Version = m
			} else {
				extracted.Version = "" // leave blank rather than polluting
			}
		}

		// Friendly name: "Alpine Linux 3.22.2 (x86_64)"
		{
			var parts []string
			parts = append(parts, "Alpine Linux")
			if extracted.Version != "" {
				parts = append(parts, extracted.Version)
			}
			if at := archTag(extracted.Architecture); at != "" {
				parts = append(parts, at)
			}
			extracted.Name = strings.TrimSpace(strings.Join(parts, " "))
		}

	}

	// 6) Final fallback for version (safe; avoids arch tokens)
	if extracted.Version == "" {
		if v := versionFromName(clean); v != "" {
			extracted.Version = v
		}
	}

	return
}

// --- metadata helpers ---
func parseTreeinfo(lines []string) (name, family, version string) {
	for _, ln := range lines {
		kv := strings.SplitN(ln, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(kv[0]))
		v := strings.TrimSpace(kv[1])
		switch k {
		case "name":
			name = v
		case "family":
			family = v
		case "version":
			version = v
		}
	}
	return
}

func parseSUSEContent(lines []string) (prod, ver string) {
	for _, ln := range lines {
		l := strings.TrimSpace(ln)
		u := strings.ToUpper(l)

		// PRODUCT or PRODUCT(arch) is fine; value RHS is the same string
		if strings.HasPrefix(u, "PRODUCT") && prod == "" {
			if i := strings.Index(l, "="); i >= 0 {
				prod = strings.Trim(strings.TrimSpace(l[i+1:]), `"'`)
			} else {
				fs := strings.Fields(l)
				if len(fs) > 1 {
					prod = strings.Trim(fs[1], `"'`)
				}
			}
		}

		// VERSION must be the bare key; ignore VERSION(arch)=...
		if strings.HasPrefix(u, "VERSION") && ver == "" {
			// reject VERSION( ... ) by checking the next rune
			after := strings.TrimSpace(l[len("VERSION"):])
			if strings.HasPrefix(after, "(") {
				continue
			}
			if i := strings.Index(l, "="); i >= 0 {
				ver = strings.Trim(strings.TrimSpace(l[i+1:]), `"'`)
			} else {
				fs := strings.Fields(l)
				if len(fs) > 1 {
					ver = strings.Trim(fs[1], `"'`)
				}
			}
		}
	}
	return
}

var discInfoTextRe = regexp.MustCompile(`[A-Za-z]`)

func parseDiscInfo(lines []string) (prod string) {
	// Prefer the last non-empty line that contains alphabetic characters
	// (skip purely numeric tokens like "1", "ALL", timestamps, etc.)
	for i := len(lines) - 1; i >= 0; i-- {
		s := strings.TrimSpace(lines[i])
		if s == "" {
			continue
		}
		if discInfoTextRe.MatchString(s) && len(s) > 2 {
			return s
		}
	}
	return ""
}
