package pxe

import (
	"bytes"
	"embed"
	"fmt"
	"hash/crc32"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/db"
)

//go:embed templates/*.tmpl templates/*/*.tmpl
var embeddedTemplates embed.FS

const (
	templateKeyPXELinux          = "pxelinux.cfg"
	templateKeyCloudInitUserData = "cloudinit/user-data"
	templateKeyCloudInitMetaData = "cloudinit/meta-data"
	templateKeyKickstart         = "kickstart/ks.cfg"
)

var templatePaths map[string]string = map[string]string{
	templateKeyPXELinux:          "pxelinux.cfg.tmpl",
	templateKeyCloudInitUserData: path.Join("cloudinit", "user-data.tmpl"),
	templateKeyCloudInitMetaData: path.Join("cloudinit", "meta-data.tmpl"),
	templateKeyKickstart:         path.Join("kickstart", "ks.cfg.tmpl"),
}

var templateFuncMap = template.FuncMap{
	"default": func(value, fallback string) (out string) {
		out = value
		if strings.TrimSpace(value) == "" {
			out = fallback
		}

		return
	},
	"indent": func(spaces int, text string) (out string) {
		var (
			pad   string   = strings.Repeat(" ", spaces)
			lines []string = strings.Split(text, "\n")
		)

		for i, line := range lines {
			lines[i] = pad + line
		}

		out = strings.Join(lines, "\n")
		return
	},
	"join":    strings.Join,
	"replace": strings.ReplaceAll,
}

type TemplateContext struct {
	Host                *db.Host
	Profile             *db.HostPXEProfile
	ISO                 *db.StoredISOImage
	Identifiers         TemplateIdentifiers
	Artifacts           ArtifactPaths
	KernelArgs          []string
	KernelArgsJoined    string
	ProfileBaseRelative string
	ProfileBaseHTTP     string
	DNSServers          []string
	Templates           TemplateDefaults
}

type TemplateIdentifiers struct {
	Hostname   string
	InstanceID string
	Slug       string
}

type ArtifactPaths struct {
	KernelRelative string
	InitrdRelative string
	ISORelative    string
	Stage2Relative string
	KernelHTTP     string
	InitrdHTTP     string
	ISOHTTP        string
	Stage2HTTP     string
}

// loadTemplate loads a template by its key from the embedded templates.
func (s *Service) loadTemplate(key string) (t *template.Template, err error) {
	var (
		rel     string
		ok      bool
		content []byte
	)

	if rel, ok = templatePaths[key]; !ok {
		err = fmt.Errorf("unknown template key %s", key)
		return
	}

	if content, err = embeddedTemplates.ReadFile(filepath.ToSlash(path.Join("templates", rel))); err != nil {
		return
	}

	t, err = template.New(key).Funcs(templateFuncMap).Parse(string(content))
	return
}

// renderTemplate renders a template with the given context.
func (s *Service) renderTemplate(key string, ctx *TemplateContext) (out []byte, err error) {
	var (
		tpl *template.Template
		buf bytes.Buffer
	)

	if tpl, err = s.loadTemplate(key); err != nil {
		return
	}

	if err = tpl.Execute(&buf, ctx); err != nil {
		return
	}

	out = buf.Bytes()
	return
}

// buildTemplateContext builds a template context for the given host, profile, and ISO.
func (s *Service) buildTemplateContext(host *db.Host, profile *db.HostPXEProfile, iso *db.StoredISOImage) (ctx *TemplateContext) {
	if profile.TemplateData == nil {
		profile.TemplateData = map[string]string{}
	}

	var slug string
	if host != nil && host.ManagementIP != "" {
		slug = makeHostSlug(host.ManagementIP)
	}

	if slug == "" && profile.ManagementIP != "" {
		slug = makeHostSlug(profile.ManagementIP)
	}

	if slug == "" && profile.IPv4Address != "" {
		slug = makeHostSlug(profile.IPv4Address)
	}

	if slug == "" {
		slug = fmt.Sprintf("%08x", crc32.ChecksumIEEE(fmt.Append(nil, time.Now().UnixNano())))
	}

	var relBase string = path.Clean(path.Join("profiles", slug))
	ctx = &TemplateContext{
		Host:    host,
		Profile: profile,
		ISO:     iso,
		Identifiers: TemplateIdentifiers{
			Slug:       slug,
			Hostname:   safeHostname(host, slug),
			InstanceID: fmt.Sprintf("laas-%s", slug),
		},
		ProfileBaseRelative: relBase,
		ProfileBaseHTTP:     s.absoluteURL(relBase),
	}

	ctx.Artifacts = s.buildArtifactPaths(iso)
	ctx.KernelArgs = s.buildKernelArgs(ctx)
	ctx.KernelArgsJoined = strings.Join(ctx.KernelArgs, " ")
	ctx.DNSServers = s.dnsServersForProfile(profile)
	ctx.Templates = s.templateDefaults.Clone()
	return ctx
}

// safeHostname generates a safe hostname based on the host and slug.
func safeHostname(host *db.Host, slug string) (hostname string) {
	if slug == "" && host != nil {
		slug = makeHostSlug(host.ManagementIP)
	}

	if slug == "" {
		slug = "opn"
	}

	var (
		base string = fmt.Sprintf("opn-%s", slug)
		b    strings.Builder
	)

	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}

	hostname = b.String()
	if hostname == "" {
		hostname = "opn-host"
	}

	if len(hostname) > 63 {
		hostname = hostname[:63]
	}

	return

}

// buildArtifactPaths builds the artifact paths for the given ISO.
func (s *Service) buildArtifactPaths(iso *db.StoredISOImage) (paths ArtifactPaths) {
	var (
		dir                  string = makeArtifactDirName(iso.Name)
		kernelRel, initrdRel string = path.Clean("/" + path.Join("artifacts", dir, "kernel")), path.Clean("/" + path.Join("artifacts", dir, "initrd"))
		isoRel, stage2Rel    string = path.Clean("/" + path.Join("artifacts", dir, "image.iso")), path.Clean("/" + path.Join("artifacts", dir, "stage2"))
	)

	paths = ArtifactPaths{
		KernelRelative: kernelRel,
		InitrdRelative: initrdRel,
		ISORelative:    isoRel,
		Stage2Relative: stage2Rel,
		KernelHTTP:     s.absoluteURL(kernelRel),
		InitrdHTTP:     s.absoluteURL(initrdRel),
		ISOHTTP:        s.absoluteURL(isoRel),
		Stage2HTTP:     s.absoluteURL(stage2Rel),
	}

	return
}

// buildKernelArgs builds the kernel arguments for the given template context.
func (s *Service) buildKernelArgs(ctx *TemplateContext) (args []string) {
	var data string
	args = []string{"ip=dhcp", "rd.neednet=1"}
	switch ctx.ISO.PreConfigure {
	case db.PreConfigureTypeCloudInit:
		if ctx.Artifacts.ISOHTTP != "" {
			args = append(args, fmt.Sprintf("url=%s", ctx.Artifacts.ISOHTTP))
		}

		args = append(args, "boot=casper", "autoinstall")

		if data = ctx.ProfileBaseHTTP; data == "" {
			data = ctx.ProfileBaseRelative
		}

		if !strings.HasSuffix(data, "/") {
			data += "/"
		}

		args = append(args, fmt.Sprintf("ds=nocloud-net;s=%s", data))

		if data = ctx.ProfileFileHTTP("cloud-init/user-data"); data != "" && strings.HasPrefix(data, "http") {
			args = append(args, fmt.Sprintf("autoinstall url=%s", data))
		}

	case db.PreConfigureTypeKickstart:
		args = append(args, "ksdevice=bootif")

		if data = ctx.ProfileFileHTTP("kickstart/ks.cfg"); data != "" {
			args = append(args, fmt.Sprintf("inst.ks=%s", data))
		}

		if ctx.Artifacts.Stage2HTTP != "" {
			args = append(args, fmt.Sprintf("inst.stage2=%s", ctx.Artifacts.Stage2HTTP))
		}
	}

	args = append(args, ctx.Profile.KernelParams...)
	args = compactArgs(args)
	return
}

// dnsServersForProfile returns the DNS servers for the given profile or the default ones.
func (s *Service) dnsServersForProfile(profile *db.HostPXEProfile) (servers []string) {
	servers = config.Config.PXE.DHCPServer.DNSServers
	if profile != nil && len(profile.DNSServers) > 0 {
		servers = profile.DNSServers
	}

	servers = cloneStringSlice(servers)
	return
}

// compactArgs removes empty or whitespace-only arguments from the given slice.
func compactArgs(args []string) (compact []string) {
	if len(args) == 0 {
		compact = nil
		return
	}

	compact = make([]string, 0, len(args))
	for _, arg := range args {
		if arg = strings.TrimSpace(arg); arg != "" {
			compact = append(compact, arg)
		}
	}

	return
}

// ProfileFileRelative returns the relative path for a profile file.
func (ctx *TemplateContext) ProfileFileRelative(name string) (relative string) {
	relative = path.Join(ctx.ProfileBaseRelative, strings.TrimPrefix(name, "/"))
	return
}

// ProfileFileHTTP returns the HTTP URL for a profile file.
func (ctx *TemplateContext) ProfileFileHTTP(name string) (http string) {
	var base string
	if base = strings.TrimRight(ctx.ProfileBaseHTTP, "/"); base == "" {
		http = ctx.ProfileFileRelative(name)
		return
	}

	http = fmt.Sprintf("%s/%s", base, strings.TrimPrefix(name, "/"))
	return
}
