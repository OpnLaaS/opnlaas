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

func loadTemplateDefaults() (TemplateDefaults, error) {
	defaults := defaultTemplateDefaults()
	cfg := config.Config.Preconfigure

	defaults.Common.Timezone = cfg.Timezone
	defaults.Common.Locale = cfg.Locale
	defaults.Common.KeyboardLayout = cfg.Keyboard
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

	preScript, err := loadScriptFile(config.LoadedConfigPath(), cfg.ScriptingFilePaths.GlobalPreScriptFile)
	if err != nil {
		return TemplateDefaults{}, err
	}
	postScript, err := loadScriptFile(config.LoadedConfigPath(), cfg.ScriptingFilePaths.GlobalPostScriptFile)
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
