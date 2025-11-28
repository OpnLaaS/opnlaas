package pxe

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/opnlaas/opnlaas/config"
)

// TemplateDefaults represents all configurable bits that the PXE templates can access.
// Values are loaded from a single environment variable so we can keep .env files tidy.
type TemplateDefaults struct {
	Common      TemplateCommonDefaults
	Autoinstall AutoinstallDefaults
	Kickstart   KickstartDefaults
	GivenUser   TemplateUser
	ManagedUser TemplateUser
}

// TemplateCommonDefaults hosts the knobs that are shared across PreConfig templates.
type TemplateCommonDefaults struct {
	Timezone          string
	Locale            string
	KeyboardLayout    string
	KeyboardVariant   string
	Packages          []string
	Mirror            string
	SSHAuthorizedKeys []string
}

type AutoinstallDefaults struct {
	AdminUsername     string
	AdminPasswordHash string
	DisableRoot       bool
	PreScript         string
	PostScript        string
	EarlyCommands     []string
	LateCommands      []string
}

type KickstartDefaults struct {
	RootPasswordHash  string
	UserName          string
	UserPasswordHash  string
	SSHAuthorizedKeys []string
	PreScript         string
	PostScript        string
}

type TemplateUser struct {
	Username          string
	PasswordHash      string
	SSHAuthorizedKeys []string
	AllowSudo         bool
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
		GivenUser: TemplateUser{
			Username: "ubuntu",
		},
		ManagedUser: TemplateUser{
			Username: "opnadmin",
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
	out.GivenUser.SSHAuthorizedKeys = cloneStringSlice(d.GivenUser.SSHAuthorizedKeys)
	out.ManagedUser.SSHAuthorizedKeys = cloneStringSlice(d.ManagedUser.SSHAuthorizedKeys)
	return out
}

func parseTemplateDefaults(spec string) (TemplateDefaults, error) {
	defaults := defaultTemplateDefaults()
	spec = strings.TrimSpace(spec)
	if spec == "" {
		applyLegacyTemplateEnv(&defaults)
		return defaults, nil
	}

	cfg, err := config.LoadInstallerConfiguration(spec)
	if err != nil {
		return TemplateDefaults{}, err
	}
	return buildTemplateDefaultsFromInstaller(cfg)
}

func buildTemplateDefaultsFromInstaller(cfg config.InstallerConfiguration) (TemplateDefaults, error) {
	defaults := defaultTemplateDefaults()
	defaults.Common.Timezone = cfg.Timezone
	defaults.Common.Locale = cfg.Locale
	defaults.Common.KeyboardLayout = cfg.KeyboardLayout
	defaults.Common.KeyboardVariant = cfg.KeyboardVariant
	defaults.Common.Packages = cloneStringSlice(cfg.Packages)
	defaults.Common.Mirror = cfg.Mirror
	defaults.Common.SSHAuthorizedKeys = cloneStringSlice(cfg.GivenUser.SSHAuthorizedKeys)

	defaults.Autoinstall.AdminUsername = cfg.GivenUser.Username
	adminHash, err := hashPassword(cfg.GivenUser.Password)
	if err != nil {
		return TemplateDefaults{}, err
	}
	defaults.Autoinstall.AdminPasswordHash = adminHash
	defaults.Autoinstall.DisableRoot = cfg.DisableRoot

	defaults.GivenUser = TemplateUser{
		Username:          cfg.GivenUser.Username,
		PasswordHash:      adminHash,
		SSHAuthorizedKeys: cloneStringSlice(cfg.GivenUser.SSHAuthorizedKeys),
		AllowSudo:         cfg.GivenUser.AllowSudo,
	}

	preScript, err := loadScriptFile(cfg.SourcePath, cfg.ScriptingFilePaths.GlobalPreScriptFile)
	if err != nil {
		return TemplateDefaults{}, err
	}
	postScript, err := loadScriptFile(cfg.SourcePath, cfg.ScriptingFilePaths.GlobalPostScriptFile)
	if err != nil {
		return TemplateDefaults{}, err
	}
	defaults.Autoinstall.PreScript = preScript
	defaults.Autoinstall.PostScript = postScript

	rootHash, err := hashPassword(cfg.RootPassword)
	if err != nil {
		return TemplateDefaults{}, err
	}
	defaults.Kickstart.RootPasswordHash = rootHash

	defaults.Kickstart.UserName = cfg.ManagedUser.Username
	userHash, err := hashPassword(cfg.ManagedUser.Password)
	if err != nil {
		return TemplateDefaults{}, err
	}
	defaults.Kickstart.UserPasswordHash = userHash
	defaults.Kickstart.SSHAuthorizedKeys = cloneStringSlice(cfg.ManagedUser.SSHAuthorizedKeys)
	defaults.Kickstart.PreScript = preScript
	defaults.Kickstart.PostScript = postScript

	defaults.ManagedUser = TemplateUser{
		Username:          cfg.ManagedUser.Username,
		PasswordHash:      userHash,
		SSHAuthorizedKeys: cloneStringSlice(cfg.ManagedUser.SSHAuthorizedKeys),
		AllowSudo:         cfg.ManagedUser.AllowSudo,
	}

	return defaults, nil
}

func loadScriptFile(configPath, scriptPath string) (string, error) {
	scriptPath = strings.TrimSpace(scriptPath)
	if scriptPath == "" {
		return "", nil
	}

	if !filepath.IsAbs(scriptPath) && configPath != "" {
		dir := filepath.Dir(configPath)
		scriptPath = filepath.Join(dir, scriptPath)
	}

	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func hashPassword(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	cmd := exec.Command("openssl", "passwd", "-6", "-stdin")
	cmd.Stdin = strings.NewReader(value + "\n")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("hash password with openssl: %w", err)
	}
	return strings.TrimSpace(out.String()), nil
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
