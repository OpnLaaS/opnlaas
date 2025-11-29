package pxe

import (
	"bytes"
	"embed"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"text/template"

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

var templatePaths = map[string]string{
	templateKeyPXELinux:          "pxelinux.cfg.tmpl",
	templateKeyCloudInitUserData: path.Join("cloudinit", "user-data.tmpl"),
	templateKeyCloudInitMetaData: path.Join("cloudinit", "meta-data.tmpl"),
	templateKeyKickstart:         path.Join("kickstart", "ks.cfg.tmpl"),
}

var templateFuncMap = template.FuncMap{
	"default": func(value, fallback string) string {
		if strings.TrimSpace(value) == "" {
			return fallback
		}
		return value
	},
	"indent": func(spaces int, text string) string {
		pad := strings.Repeat(" ", spaces)
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			lines[i] = pad + line
		}
		return strings.Join(lines, "\n")
	},
	"join":    strings.Join,
	"replace": strings.ReplaceAll,
}

type TemplateContext struct {
	Host        *db.Host
	Profile     *db.HostPXEProfile
	ISO         *db.StoredISOImage
	Identifiers TemplateIdentifiers
	Artifacts   ArtifactPaths

	KernelArgs       []string
	KernelArgsJoined string

	ProfileBaseRelative string
	ProfileBaseHTTP     string

	DNSServers []string

	Templates TemplateDefaults
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

	KernelHTTP string
	InitrdHTTP string
	ISOHTTP    string
	Stage2HTTP string
}

func (s *Service) loadTemplate(key string) (*template.Template, error) {
	rel, ok := templatePaths[key]
	if !ok {
		return nil, fmt.Errorf("unknown template key %s", key)
	}

	var content []byte
	// if dir := strings.TrimSpace(s.cfg.TFTP.TemplateDir); dir != "" {
	// 	candidate := filepath.Join(dir, rel)
	// 	if data, err := os.ReadFile(candidate); err == nil {
	// 		content = data
	// 	}
	// }

	if len(content) == 0 {
		embeddedPath := filepath.ToSlash(path.Join("templates", rel))
		data, err := embeddedTemplates.ReadFile(embeddedPath)
		if err != nil {
			return nil, err
		}
		content = data
	}

	return template.New(key).Funcs(templateFuncMap).Parse(string(content))
}

func (s *Service) renderTemplate(key string, ctx *TemplateContext) ([]byte, error) {
	tpl, err := s.loadTemplate(key)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ctx); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *Service) buildTemplateContext(host *db.Host, profile *db.HostPXEProfile, iso *db.StoredISOImage) *TemplateContext {
	if profile.TemplateData == nil {
		profile.TemplateData = map[string]string{}
	}

	slug := ""
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
		slug = "default"
	}

	relBase := pathClean(path.Join("profiles", slug))
	ctx := &TemplateContext{
		Host:    host,
		Profile: profile,
		ISO:     iso,
		Identifiers: TemplateIdentifiers{
			Slug:       slug,
			Hostname:   safeHostname(host, slug),
			InstanceID: fmt.Sprintf("opn-%s", slug),
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

func safeHostname(host *db.Host, slug string) string {
	if slug == "" && host != nil {
		slug = makeHostSlug(host.ManagementIP)
	}
	if slug == "" {
		slug = "opn"
	}
	base := fmt.Sprintf("opn-%s", slug)
	var b strings.Builder
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	cleaned := b.String()
	if cleaned == "" {
		return "opn-host"
	}
	return cleaned
}

func (s *Service) buildArtifactPaths(iso *db.StoredISOImage) ArtifactPaths {
	dir := makeArtifactDirName(iso.Name)
	kernelRel := path.Clean("/" + path.Join("artifacts", dir, "kernel"))
	initrdRel := path.Clean("/" + path.Join("artifacts", dir, "initrd"))
	isoRel := path.Clean("/" + path.Join("artifacts", dir, "image.iso"))
	stage2Rel := path.Clean("/" + path.Join("artifacts", dir, "stage2"))
	return ArtifactPaths{
		KernelRelative: kernelRel,
		InitrdRelative: initrdRel,
		ISORelative:    isoRel,
		Stage2Relative: stage2Rel,
		KernelHTTP:     s.absoluteURL(kernelRel),
		InitrdHTTP:     s.absoluteURL(initrdRel),
		ISOHTTP:        s.absoluteURL(isoRel),
		Stage2HTTP:     s.absoluteURL(stage2Rel),
	}
}

func relativeToRoot(root, full string) string {
	if root == "" || full == "" {
		return ""
	}
	if rel, err := filepath.Rel(root, full); err == nil {
		return "/" + filepath.ToSlash(rel)
	}
	return "/" + filepath.Base(full)
}

func (s *Service) buildKernelArgs(ctx *TemplateContext) []string {
	args := []string{"ip=dhcp", "rd.neednet=1"}
	switch ctx.ISO.PreConfigure {
	case db.PreConfigureTypeCloudInit:
		if isUbuntuISO(ctx.ISO) {
			if ctx.Artifacts.ISOHTTP != "" {
				args = append(args, fmt.Sprintf("url=%s", ctx.Artifacts.ISOHTTP))
			}
			args = append(args, "boot=casper", "autoinstall")
			seed := ctx.ProfileBaseHTTP
			if seed == "" {
				seed = ctx.ProfileBaseRelative
			}
			if !strings.HasSuffix(seed, "/") {
				seed += "/"
			}
			args = append(args, fmt.Sprintf("ds=nocloud-net;s=%s", seed))
			if userData := ctx.ProfileFileHTTP("cloud-init/user-data"); userData != "" && strings.HasPrefix(userData, "http") {
				args = append(args, fmt.Sprintf("autoinstall url=%s", userData))
			}
		}
	case db.PreConfigureTypeKickstart:
		args = append(args, "ksdevice=bootif")
		if ks := ctx.ProfileFileHTTP("kickstart/ks.cfg"); ks != "" {
			args = append(args, fmt.Sprintf("inst.ks=%s", ks))
		}
		if ctx.Artifacts.Stage2HTTP != "" {
			args = append(args, fmt.Sprintf("inst.stage2=%s", ctx.Artifacts.Stage2HTTP))
		}
	}
	args = append(args, ctx.Profile.KernelParams...)
	return compactArgs(args)
}

func (s *Service) dnsServersForProfile(profile *db.HostPXEProfile) []string {
	if profile != nil && len(profile.DNSServers) > 0 {
		return cloneStringSlice(profile.DNSServers)
	}
	return cloneStringSlice(s.cfg.PXE.DHCPServer.DNSServers)
}

func compactArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func (ctx *TemplateContext) ProfileFileRelative(name string) string {
	clean := strings.TrimPrefix(name, "/")
	return path.Clean(path.Join(ctx.ProfileBaseRelative, clean))
}

func (ctx *TemplateContext) ProfileFileHTTP(name string) string {
	rel := strings.TrimPrefix(name, "/")
	base := strings.TrimRight(ctx.ProfileBaseHTTP, "/")
	if base == "" {
		return ctx.ProfileFileRelative(name)
	}
	return base + "/" + rel
}

func isUbuntuISO(iso *db.StoredISOImage) bool {
	if iso == nil {
		return false
	}
	name := strings.ToLower(iso.DistroName)
	if name == "" {
		name = strings.ToLower(iso.Name)
	}
	return strings.Contains(name, "ubuntu")
}
