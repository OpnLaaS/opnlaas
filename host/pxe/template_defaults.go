package pxe

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// TemplateDefaults represents all configurable bits that the PXE templates can access.
// Values are loaded from a single environment variable so we can keep .env files tidy.
type TemplateDefaults struct {
	Common      TemplateCommonDefaults `json:"common"`
	Autoinstall AutoinstallDefaults    `json:"autoinstall"`
	Kickstart   KickstartDefaults      `json:"kickstart"`
}

// TemplateCommonDefaults hosts the knobs that are shared across PreConfig templates.
type TemplateCommonDefaults struct {
	Timezone          string   `json:"timezone"`
	Locale            string   `json:"locale"`
	KeyboardLayout    string   `json:"keyboard_layout"`
	KeyboardVariant   string   `json:"keyboard_variant"`
	Packages          []string `json:"packages"`
	Mirror            string   `json:"mirror"`
	SSHAuthorizedKeys []string `json:"ssh_authorized_keys"`
}

type AutoinstallDefaults struct {
	AdminUsername     string   `json:"admin_username"`
	AdminPasswordHash string   `json:"admin_password_hash"`
	DisableRoot       bool     `json:"disable_root"`
	PreScript         string   `json:"pre_script"`
	PostScript        string   `json:"post_script"`
	EarlyCommands     []string `json:"early_commands"`
	LateCommands      []string `json:"late_commands"`
}

type KickstartDefaults struct {
	RootPasswordHash  string   `json:"root_password_hash"`
	UserName          string   `json:"user_name"`
	UserPasswordHash  string   `json:"user_password_hash"`
	SSHAuthorizedKeys []string `json:"ssh_authorized_keys"`
	PreScript         string   `json:"pre_script"`
	PostScript        string   `json:"post_script"`
}

func defaultTemplateDefaults() TemplateDefaults {
	return TemplateDefaults{
		Common: TemplateCommonDefaults{
			Timezone:        "UTC",
			Locale:          "en_US",
			KeyboardLayout:  "us",
			KeyboardVariant: "",
		},
		Autoinstall: AutoinstallDefaults{
			AdminUsername: "ubuntu",
		},
		Kickstart: KickstartDefaults{
			UserName: "admin",
		},
	}
}

// Clone returns an independent copy so slices can be mutated safely by templates.
func (d TemplateDefaults) Clone() TemplateDefaults {
	out := d
	out.Common.Packages = cloneStringSlice(d.Common.Packages)
	out.Common.SSHAuthorizedKeys = cloneStringSlice(d.Common.SSHAuthorizedKeys)
	out.Autoinstall.EarlyCommands = cloneStringSlice(d.Autoinstall.EarlyCommands)
	out.Autoinstall.LateCommands = cloneStringSlice(d.Autoinstall.LateCommands)
	out.Kickstart.SSHAuthorizedKeys = cloneStringSlice(d.Kickstart.SSHAuthorizedKeys)
	return out
}

func parseTemplateDefaults(spec string) (TemplateDefaults, error) {
	defaults := defaultTemplateDefaults()
	spec = strings.TrimSpace(spec)
	if spec == "" {
		applyLegacyTemplateEnv(&defaults)
		return defaults, nil
	}

	data, err := readTemplateDefaultsData(spec)
	if err != nil {
		return TemplateDefaults{}, err
	}
	if len(data) == 0 {
		return defaults, nil
	}

	var overrides TemplateDefaults
	if err := json.Unmarshal(data, &overrides); err != nil {
		return TemplateDefaults{}, fmt.Errorf("parse template defaults: %w", err)
	}
	mergeTemplateDefaults(&defaults, overrides)
	return defaults, nil
}

func readTemplateDefaultsData(spec string) ([]byte, error) {
	if spec == "" {
		return nil, nil
	}

	if strings.HasPrefix(spec, "@") {
		path := strings.TrimSpace(strings.TrimPrefix(spec, "@"))
		return os.ReadFile(path)
	}

	if strings.HasPrefix(spec, "{") || strings.HasPrefix(spec, "[") {
		return []byte(spec), nil
	}

	if info, err := os.Stat(spec); err == nil && !info.IsDir() {
		return os.ReadFile(spec)
	}

	return []byte(spec), nil
}

func mergeTemplateDefaults(base *TemplateDefaults, overrides TemplateDefaults) {
	if overrides.Common.Timezone != "" {
		base.Common.Timezone = overrides.Common.Timezone
	}
	if overrides.Common.Locale != "" {
		base.Common.Locale = overrides.Common.Locale
	}
	if overrides.Common.KeyboardLayout != "" {
		base.Common.KeyboardLayout = overrides.Common.KeyboardLayout
	}
	if overrides.Common.KeyboardVariant != "" {
		base.Common.KeyboardVariant = overrides.Common.KeyboardVariant
	}
	if len(overrides.Common.Packages) > 0 {
		base.Common.Packages = cloneStringSlice(overrides.Common.Packages)
	}
	if overrides.Common.Mirror != "" {
		base.Common.Mirror = overrides.Common.Mirror
	}
	if len(overrides.Common.SSHAuthorizedKeys) > 0 {
		base.Common.SSHAuthorizedKeys = cloneStringSlice(overrides.Common.SSHAuthorizedKeys)
	}

	if overrides.Autoinstall.AdminUsername != "" {
		base.Autoinstall.AdminUsername = overrides.Autoinstall.AdminUsername
	}
	if overrides.Autoinstall.AdminPasswordHash != "" {
		base.Autoinstall.AdminPasswordHash = overrides.Autoinstall.AdminPasswordHash
	}
	if overrides.Autoinstall.DisableRoot {
		base.Autoinstall.DisableRoot = true
	}
	if overrides.Autoinstall.PreScript != "" {
		base.Autoinstall.PreScript = overrides.Autoinstall.PreScript
	}
	if overrides.Autoinstall.PostScript != "" {
		base.Autoinstall.PostScript = overrides.Autoinstall.PostScript
	}
	if len(overrides.Autoinstall.EarlyCommands) > 0 {
		base.Autoinstall.EarlyCommands = cloneStringSlice(overrides.Autoinstall.EarlyCommands)
	}
	if len(overrides.Autoinstall.LateCommands) > 0 {
		base.Autoinstall.LateCommands = cloneStringSlice(overrides.Autoinstall.LateCommands)
	}

	if overrides.Kickstart.RootPasswordHash != "" {
		base.Kickstart.RootPasswordHash = overrides.Kickstart.RootPasswordHash
	}
	if overrides.Kickstart.UserName != "" {
		base.Kickstart.UserName = overrides.Kickstart.UserName
	}
	if overrides.Kickstart.UserPasswordHash != "" {
		base.Kickstart.UserPasswordHash = overrides.Kickstart.UserPasswordHash
	}
	if len(overrides.Kickstart.SSHAuthorizedKeys) > 0 {
		base.Kickstart.SSHAuthorizedKeys = cloneStringSlice(overrides.Kickstart.SSHAuthorizedKeys)
	}
	if overrides.Kickstart.PreScript != "" {
		base.Kickstart.PreScript = overrides.Kickstart.PreScript
	}
	if overrides.Kickstart.PostScript != "" {
		base.Kickstart.PostScript = overrides.Kickstart.PostScript
	}
}

func applyLegacyTemplateEnv(base *TemplateDefaults) {
	if v := strings.TrimSpace(os.Getenv("TFTP_AUTOINSTALL_ADMIN_USERNAME")); v != "" {
		base.Autoinstall.AdminUsername = v
	}
	if v := strings.TrimSpace(os.Getenv("TFTP_AUTOINSTALL_ADMIN_PASSWORD_HASH")); v != "" {
		base.Autoinstall.AdminPasswordHash = v
	}
	if v := strings.TrimSpace(os.Getenv("TFTP_AUTOINSTALL_PRE_SCRIPT")); v != "" {
		base.Autoinstall.PreScript = v
	}
	if v := strings.TrimSpace(os.Getenv("TFTP_AUTOINSTALL_POST_SCRIPT")); v != "" {
		base.Autoinstall.PostScript = v
	}
	if v := strings.TrimSpace(os.Getenv("TFTP_AUTOINSTALL_TIMEZONE")); v != "" {
		base.Common.Timezone = v
	}
	if v := strings.TrimSpace(os.Getenv("TFTP_AUTOINSTALL_MIRROR")); v != "" {
		base.Common.Mirror = v
	}
	if keys := legacyEnvList("TFTP_AUTOINSTALL_SSH_AUTHORIZED_KEYS"); len(keys) > 0 {
		base.Common.SSHAuthorizedKeys = keys
	}
	if pkgs := legacyEnvList("TFTP_AUTOINSTALL_PACKAGES"); len(pkgs) > 0 {
		base.Common.Packages = pkgs
	}

	if v := strings.TrimSpace(os.Getenv("TFTP_KICKSTART_ROOT_PASSWORD_HASH")); v != "" {
		base.Kickstart.RootPasswordHash = v
	}
	if v := strings.TrimSpace(os.Getenv("TFTP_KICKSTART_USER")); v != "" {
		base.Kickstart.UserName = v
	}
	if v := strings.TrimSpace(os.Getenv("TFTP_KICKSTART_USER_PASSWORD_HASH")); v != "" {
		base.Kickstart.UserPasswordHash = v
	}
	if keys := legacyEnvList("TFTP_KICKSTART_SSH_AUTHORIZED_KEYS"); len(keys) > 0 {
		base.Kickstart.SSHAuthorizedKeys = keys
	}
	if v := strings.TrimSpace(os.Getenv("TFTP_KICKSTART_PRE_SCRIPT")); v != "" {
		base.Kickstart.PreScript = v
	}
	if v := strings.TrimSpace(os.Getenv("TFTP_KICKSTART_POST_SCRIPT")); v != "" {
		base.Kickstart.PostScript = v
	}
}

func legacyEnvList(key string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}
	parts := strings.Split(value, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
