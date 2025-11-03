package iso

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/kdomanski/iso9660"
)

type PreconfigureType string

const (
	PreconfigureNone      PreconfigureType = "none"
	PreconfigureKickstart PreconfigureType = "kickstart"   // RHEL/Fedora/Rocky/CentOS
	PreconfigurePreseed   PreconfigureType = "preseed"     // Debian/Ubuntu (legacy)
	PreconfigureAutoinst  PreconfigureType = "autoinstall" // Ubuntu 20.04+ (cloud-init)
	PreconfigureNoCloud   PreconfigureType = "cloud-init"  // generic NoCloud (cidata)
	PreconfigureAutoYaST  PreconfigureType = "autoyast"    // SUSE/openSUSE
)

type BootArtifacts struct {
	DistroGuess  string           // best-effort name from paths/configs
	KernelPath   string           // ISO-internal absolute path
	InitrdPath   string           // ISO-internal absolute path
	KernelArgs   []string         // aggregate from grub/syslinux, deduped
	ConfigPaths  []string         // grub.cfg, syslinux/isolinux cfgs we parsed
	Preconfigure PreconfigureType // inferred from files/args
	ArchGuess    string
}

func ScanISOForBootArtifactsISO9660(img *iso9660.Image) (*BootArtifacts, error) {
	root, err := img.RootDir()
	if err != nil {
		return nil, err
	}

	// 1) Build a case-insensitive index of all files
	all := make([]string, 0, 4096)
	type node struct {
		f *iso9660.File
		p string // lowercased path
	}
	var walk func(*iso9660.File, string) error
	walk = func(f *iso9660.File, p string) error {
		lp := p
		if lp == "" {
			lp = "/"
		}
		all = append(all, lp)
		if f.IsDir() {
			children, err := f.GetChildren()
			if err != nil {
				return err
			}
			for _, c := range children {
				name := c.Name()
				if name == "." || name == ".." {
					continue
				}
				next := path.Join(lp, name)
				if !strings.HasPrefix(next, "/") {
					next = "/" + next
				}
				if err := walk(c, strings.ToLower(next)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(root, "/"); err != nil {
		return nil, err
	}
	sort.Strings(all)

	contains := func(p string) bool {
		p = strings.ToLower(p)
		i := sort.SearchStrings(all, p)
		return i < len(all) && all[i] == p
	}
	findFirst := func(cands ...string) string {
		for _, c := range cands {
			if contains(c) {
				return c
			}
		}
		return ""
	}
	findAnyWithPrefix := func(prefixes ...string) []string {
		out := []string{}
		for _, e := range all {
			for _, p := range prefixes {
				if strings.HasPrefix(e, strings.ToLower(p)) {
					out = append(out, e)
					break
				}
			}
		}
		return out
	}

	// 2) Try well-known kernel/initrd locations across distros
	// SUSE/openSUSE
	kernel := findFirst(
		"/boot/x86_64/loader/linux",
		"/boot/i586/loader/linux",
	)
	initrd := findFirst(
		"/boot/x86_64/loader/initrd",
		"/boot/i586/loader/initrd",
	)

	// RHEL/Fedora/Rocky/CentOS
	if kernel == "" {
		kernel = findFirst("/images/pxeboot/vmlinuz", "/isolinux/vmlinuz")
	}
	if initrd == "" {
		initrd = findFirst("/images/pxeboot/initrd.img", "/isolinux/initrd.img")
	}

	// Debian/Ubuntu
	if kernel == "" {
		kernel = findFirst("/install.amd/vmlinuz", "/install/vmlinuz", "/casper/vmlinuz", "/casper/vmlinuz.efi")
	}
	if initrd == "" {
		initrd = findFirst("/install.amd/initrd.gz", "/install/initrd.gz", "/casper/initrd", "/casper/initrd.gz")
	}

	// Arch
	if kernel == "" {
		kernel = findFirst("/arch/boot/x86_64/vmlinuz", "/arch/boot/vmlinuz")
	}
	if initrd == "" {
		initrd = findFirst("/arch/boot/x86_64/archiso.img", "/arch/boot/archiso.img")
	}

	// 3) Parse bootloader configs for better paths & args
	cfgs := append([]string{}, findAnyWithPrefix(
		"/boot/grub", "/boot/grub2", "/efi/boot",
		"/isolinux", "/syslinux",
	)...)
	// We’ll scan only files that look like configs
	candidateCfgs := []string{}
	for _, c := range cfgs {
		if strings.HasSuffix(c, ".cfg") || strings.HasSuffix(c, ".conf") {
			candidateCfgs = append(candidateCfgs, c)
		} else if path.Base(c) == "grub.cfg" || path.Base(c) == "grub.conf" ||
			path.Base(c) == "isolinux.cfg" || path.Base(c) == "syslinux.cfg" {
			candidateCfgs = append(candidateCfgs, c)
		}
	}

	lines := []string{}
	readFileLines := func(p string) ([]string, error) {
		// Open the iso file by path and read its contents as text
		f, err := openPath(img, p)
		if err != nil {
			return nil, err
		}
		br := bufio.NewReader(f)
		out := []string{}
		for {
			l, e := br.ReadString('\n')
			if l != "" {
				out = append(out, strings.TrimRight(l, "\r\n"))
			}
			if e != nil {
				if len(out) == 0 && l == "" {
					return out, e
				}
				break
			}
		}
		return out, nil
	}

	// Regex for grub/syslinux menuentries, linux/linuxefi/kernel/initrd lines
	reLinux := regexp.MustCompile(`(?i)^\s*(linux|linuxefi|kernel)\s+(\S+)(?:\s+(.*))?$`)
	reInitrd := regexp.MustCompile(`(?i)^\s*(initrd|initrdefi)\s+(\S+)(?:\s+(.*))?$`)

	aggArgs := map[string]struct{}{}
	addArgs := func(s string) {
		for _, a := range splitArgsPreserveQuotes(s) {
			if a == "" {
				continue
			}
			aggArgs[a] = struct{}{}
		}
	}

	foundCfgs := []string{}
	for _, cfg := range candidateCfgs {
		cfgLines, err := readFileLines(cfg)
		if err != nil || len(cfgLines) == 0 {
			continue
		}
		foundCfgs = append(foundCfgs, cfg)
		lines = append(lines, cfgLines...)

		for _, l := range cfgLines {
			if m := reLinux.FindStringSubmatch(l); m != nil {
				p := normalizeJoin(path.Dir(cfg), m[2])
				if kernel == "" || looksBetterThan(kernel, p) {
					if contains(p) {
						kernel = p
					}
				}
				if len(m) > 3 && m[3] != "" {
					addArgs(m[3])
				}
			}
			if m := reInitrd.FindStringSubmatch(l); m != nil {
				p := normalizeJoin(path.Dir(cfg), m[2])
				if initrd == "" || looksBetterThan(initrd, p) {
					if contains(p) {
						initrd = p
					}
				}
				if len(m) > 3 && m[3] != "" {
					addArgs(m[3])
				}
			}
		}
	}

	// 4) Infer distro + arch from paths
	distro := guessDistro(all, lines)
	arch := guessArch(all)

	// 5) Infer preconfigure type from files + args
	pre := guessPreconfigure(all, aggArgs, lines)

	// finalize args
	finalArgs := make([]string, 0, len(aggArgs))
	for a := range aggArgs {
		finalArgs = append(finalArgs, a)
	}
	sort.Strings(finalArgs)

	return &BootArtifacts{
		DistroGuess:  distro,
		KernelPath:   kernel,
		InitrdPath:   initrd,
		KernelArgs:   finalArgs,
		ConfigPaths:  foundCfgs,
		Preconfigure: pre,
		ArchGuess:    arch,
	}, nil
}

func openPath(img *iso9660.Image, isoPath string) (io.Reader, error) {
	// iso9660 lib expects a walk from root; we already indexed names,
	// so we just traverse components.
	isoPath = strings.TrimPrefix(isoPath, "/")
	parts := strings.Split(isoPath, "/")
	cur, err := img.RootDir()
	if err != nil {
		return nil, err
	}
	for i, p := range parts {
		if p == "" {
			continue
		}
		children, err := cur.GetChildren()
		if err != nil {
			return nil, err
		}
		var next *iso9660.File
		lp := strings.ToLower(p)
		for _, c := range children {
			if strings.ToLower(c.Name()) == lp {
				next = c
				break
			}
		}
		if next == nil {
			return nil, fmt.Errorf("not found: %s", isoPath)
		}
		if i == len(parts)-1 {
			rd := next.Reader()
			return rd, nil
		}
		cur = next
	}
	// root
	return cur.Reader(), nil
}

// Helpers

func splitArgsPreserveQuotes(s string) []string {
	// very lightweight splitter; good enough for kernel lines
	out := []string{}
	cur := strings.Builder{}
	inQuote := rune(0)
	for _, r := range s {
		switch r {
		case '"', '\'':
			if inQuote == 0 {
				inQuote = r
			} else if inQuote == r {
				inQuote = 0
			} else {
				cur.WriteRune(r)
			}
		case ' ', '\t':
			if inQuote != 0 {
				cur.WriteRune(r)
			} else if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func looksBetterThan(old, cand string) bool {
	// prefer non-/isolinux when we already have /isolinux; prefer pxeboot dir
	if strings.Contains(old, "/isolinux/") && strings.Contains(cand, "/images/pxeboot/") {
		return true
	}
	return false
}

func normalizeJoin(dir, p string) string {
	if strings.HasPrefix(p, "/") {
		return strings.ToLower(path.Clean(p))
	}
	return strings.ToLower(path.Clean(path.Join(dir, p)))
}

func guessDistro(all []string, cfgLines []string) string {
	blob := strings.Join(all, " ")
	cfg := strings.Join(cfgLines, "\n")
	s := blob + "\n" + cfg
	switch {
	case strings.Contains(s, "suse") || strings.Contains(s, "opensuse") || strings.Contains(s, "autoyast"):
		return "SUSE"
	case strings.Contains(s, "fedora"):
		return "Fedora"
	case strings.Contains(s, "centos") || strings.Contains(s, "rocky"):
		return "RHEL-family"
	case strings.Contains(s, "rhel"):
		return "RHEL"
	case strings.Contains(s, "ubuntu") || strings.Contains(s, "casper"):
		return "Ubuntu"
	case strings.Contains(s, "debian") || strings.Contains(s, "/install.amd/"):
		return "Debian"
	case strings.Contains(s, "arch/boot"):
		return "Arch"
	}
	return ""
}

func guessArch(all []string) string {
	for _, p := range all {
		switch {
		case strings.Contains(p, "x86_64"), strings.Contains(p, "amd64"):
			return "x86_64"
		case strings.Contains(p, "aarch64"), strings.Contains(p, "arm64"):
			return "aarch64"
		case strings.Contains(p, "ppc64le"):
			return "ppc64le"
		}
	}
	return ""
}

func guessPreconfigure(all []string, args map[string]struct{}, cfgLines []string) PreconfigureType {
	// kernel arg clues
	hasArg := func(prefix string) bool {
		for a := range args {
			if strings.HasPrefix(strings.ToLower(a), prefix) {
				return true
			}
		}
		return false
	}
	// file clues
	hasAny := func(prefixes ...string) bool {
		for _, p := range all {
			for _, pre := range prefixes {
				if strings.HasPrefix(p, strings.ToLower(pre)) {
					return true
				}
			}
		}
		return false
	}

	// Ubuntu autoinstall
	if hasArg("autoinstall") || hasAny("/autoinstall/", "/nocloud/", "/cidata/") {
		return PreconfigureAutoinst
	}
	// Generic cloud-init / NoCloud
	if hasArg("ds=nocloud") || hasAny("/nocloud/", "/cidata/", "/user-data", "/meta-data") {
		return PreconfigureNoCloud
	}
	// RHEL-family kickstart
	if hasArg("inst.ks") || hasArg("ks=") || hasAny("/ks.cfg", "/ks/", "/kickstart") {
		return PreconfigureKickstart
	}
	// Debian/Ubuntu preseed
	if hasArg("preseed/") || hasAny("/preseed/", "/preseed.cfg") {
		return PreconfigurePreseed
	}
	// SUSE AutoYaST
	if hasArg("autoyast") || hasAny("/autoinst.xml", "/autoyast/", "/control.xml") {
		return PreconfigureAutoYaST
	}
	return PreconfigureNone
}
