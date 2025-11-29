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

// AutoinstallDefaults hosts the knobs for cloud-init autoinstall templates.
type AutoinstallDefaults struct {
	AdminUsername     string
	AdminPasswordHash string
	DisableRoot       bool
	PreScript         string
	PostScript        string
	EarlyCommands     []string
	LateCommands      []string
}

// KickstartDefaults hosts the knobs for kickstart templates.
type KickstartDefaults struct {
	RootPasswordHash  string
	UserName          string
	UserPasswordHash  string
	SSHAuthorizedKeys []string
	PreScript         string
	PostScript        string
}

// TemplateUser represents a user configuration for templates.
type TemplateUser struct {
	Username          string
	PasswordHash      string
	SSHAuthorizedKeys []string
	AllowSudo         bool
}

// defaultTemplateDefaults returns the built-in default template defaults.
func defaultTemplateDefaults() (tds TemplateDefaults) {
	tds = TemplateDefaults{
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

	return
}

// Clone returns an independent copy so slices can be mutated safely by templates.
func (d TemplateDefaults) Clone() (clone TemplateDefaults) {
	clone = d
	clone.Common.Packages = cloneStringSlice(d.Common.Packages)
	clone.Common.SSHAuthorizedKeys = cloneStringSlice(d.Common.SSHAuthorizedKeys)
	clone.Autoinstall.EarlyCommands = cloneStringSlice(d.Autoinstall.EarlyCommands)
	clone.Autoinstall.LateCommands = cloneStringSlice(d.Autoinstall.LateCommands)
	clone.Kickstart.SSHAuthorizedKeys = cloneStringSlice(d.Kickstart.SSHAuthorizedKeys)
	clone.GivenUser.SSHAuthorizedKeys = cloneStringSlice(d.GivenUser.SSHAuthorizedKeys)
	clone.ManagedUser.SSHAuthorizedKeys = cloneStringSlice(d.ManagedUser.SSHAuthorizedKeys)
	return
}

// loadTemplateDefaults loads the template defaults from the global configuration.
func loadTemplateDefaults() (defaults TemplateDefaults, err error) {
	var hash string
	defaults = defaultTemplateDefaults()

	defaults.Common.Timezone = config.Config.Preconfigure.Timezone
	defaults.Common.Locale = config.Config.Preconfigure.Locale
	defaults.Common.KeyboardLayout = config.Config.Preconfigure.Keyboard
	defaults.Common.KeyboardVariant = config.Config.Preconfigure.KeyboardVariant
	defaults.Common.Packages = cloneStringSlice(config.Config.Preconfigure.Packages)
	defaults.Common.Mirror = config.Config.Preconfigure.Mirror
	defaults.Common.SSHAuthorizedKeys = cloneStringSlice(config.Config.Preconfigure.GivenUser.SSHAuthorizedKeys)

	defaults.Autoinstall.AdminUsername = config.Config.Preconfigure.GivenUser.Username
	if hash, err = hashPassword(config.Config.Preconfigure.GivenUser.Password); err != nil {
		return
	}

	defaults.Autoinstall.AdminPasswordHash = hash
	defaults.Autoinstall.DisableRoot = config.Config.Preconfigure.DisableRoot
	defaults.GivenUser = TemplateUser{
		Username:          config.Config.Preconfigure.GivenUser.Username,
		PasswordHash:      hash,
		SSHAuthorizedKeys: cloneStringSlice(config.Config.Preconfigure.GivenUser.SSHAuthorizedKeys),
		AllowSudo:         config.Config.Preconfigure.GivenUser.AllowSudo,
	}

	if defaults.Autoinstall.PreScript, err = loadScriptFile(config.LoadedConfigPath(), config.Config.Preconfigure.ScriptingFilePaths.GlobalPreScriptFile); err != nil {
		return
	}

	if defaults.Autoinstall.PostScript, err = loadScriptFile(config.LoadedConfigPath(), config.Config.Preconfigure.ScriptingFilePaths.GlobalPostScriptFile); err != nil {
		return
	}

	if hash, err = hashPassword(config.Config.Preconfigure.RootPassword); err != nil {
		return
	}

	defaults.Kickstart.RootPasswordHash = hash

	defaults.Kickstart.UserName = config.Config.Preconfigure.ManagedUser.Username
	if hash, err = hashPassword(config.Config.Preconfigure.ManagedUser.Password); err != nil {
		return
	}

	defaults.Kickstart.UserPasswordHash = hash
	defaults.Kickstart.SSHAuthorizedKeys = cloneStringSlice(config.Config.Preconfigure.ManagedUser.SSHAuthorizedKeys)
	defaults.Kickstart.PreScript = defaults.Autoinstall.PreScript
	defaults.Kickstart.PostScript = defaults.Autoinstall.PostScript

	defaults.ManagedUser = TemplateUser{
		Username:          config.Config.Preconfigure.ManagedUser.Username,
		PasswordHash:      hash,
		SSHAuthorizedKeys: cloneStringSlice(config.Config.Preconfigure.ManagedUser.SSHAuthorizedKeys),
		AllowSudo:         config.Config.Preconfigure.ManagedUser.AllowSudo,
	}

	return
}

// loadScriptFile loads a script file, resolving relative paths based on the config path.
func loadScriptFile(configPath, scriptPath string) (data string, err error) {
	if scriptPath = strings.TrimSpace(scriptPath); scriptPath == "" {
		return
	}

	if !filepath.IsAbs(scriptPath) && configPath != "" {
		scriptPath = filepath.Join(filepath.Dir(configPath), scriptPath)
	}

	var content []byte
	if content, err = os.ReadFile(scriptPath); err != nil {
		return
	}

	data = strings.TrimSpace(string(content))
	return
}

// hashPassword hashes a password using openssl with SHA-512.
func hashPassword(value string) (hash string, err error) {
	if value = strings.TrimSpace(value); value == "" {
		return
	}

	var command *exec.Cmd = exec.Command("openssl", "passwd", "-6", "-stdin")
	command.Stdin = strings.NewReader(value + "\n")

	var out bytes.Buffer
	command.Stdout = &out

	if err = command.Run(); err != nil {
		err = fmt.Errorf("hash password with openssl: %w", err)
		return
	}

	hash = strings.TrimSpace(out.String())
	return
}
