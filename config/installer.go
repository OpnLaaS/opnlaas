package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// InstallerConfiguration captures the shared defaults for automated installers.
// The YAML file intentionally stores sensitive values (like passwords) in plaintext.
// Callers are expected to hash/encrypt as needed before rendering templates.
type InstallerConfiguration struct {
	SourcePath      string   `yaml:"-"`
	Locale          string   `yaml:"locale"`
	Timezone        string   `yaml:"timezone"`
	KeyboardLayout  string   `yaml:"keyboard_layout"`
	KeyboardVariant string   `yaml:"keyboard_variant"`
	Packages        []string `yaml:"packages,omitempty"`
	Mirror          string   `yaml:"mirror,omitempty"`
	RootPassword    string   `yaml:"root_password,omitempty"`

	GivenUser   InstallerUser `yaml:"given_user"`
	ManagedUser InstallerUser `yaml:"managed_user"`

	DisableRoot bool `yaml:"disable_root"`

	ScriptingFilePaths struct {
		GlobalPreScriptFile  string `yaml:"global_pre_script_file"`
		GlobalPostScriptFile string `yaml:"global_post_script_file"`
	} `yaml:"scripting_file_paths"`

	GlobalKernelParams []string `yaml:"global_kernel_params"`
	GlobalInitrdParams []string `yaml:"global_initrd_params"`
}

type InstallerUser struct {
	Username          string   `yaml:"username"`
	Password          string   `yaml:"password,omitempty"`
	SSHAuthorizedKeys []string `yaml:"ssh_authorized_keys,omitempty"`
	AllowSudo         bool     `yaml:"allow_sudo"`
}

// LoadInstallerConfiguration reads the YAML specification at the provided path.
// If the path is empty, in-memory defaults are returned. When the file does not
// exist, a default file is generated and an error is returned so the operator
// can populate the values.
func LoadInstallerConfiguration(path string) (InstallerConfiguration, error) {
	cfg := defaultInstallerConfiguration()
	if strings.TrimSpace(path) == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if writeErr := writeInstallerConfiguration(path, cfg); writeErr != nil {
				return cfg, writeErr
			}
			return cfg, fmt.Errorf("installer configuration not found, generated template at %s", path)
		}
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	cfg.ensureDefaults()
	cfg.SourcePath = path
	return cfg, cfg.validate()
}

func defaultInstallerConfiguration() InstallerConfiguration {
	return InstallerConfiguration{
		Locale:          "en_US",
		Timezone:        "UTC",
		KeyboardLayout:  "us",
		KeyboardVariant: "",
		Packages:        []string{"openssh-server"},
		Mirror:          "",
		RootPassword:    "",
		DisableRoot:     true,
		GivenUser: InstallerUser{
			Username:          "laas",
			Password:          "laas",
			AllowSudo:         true,
			SSHAuthorizedKeys: nil,
		},
		ManagedUser: InstallerUser{
			Username:          "laas-admin",
			Password:          "laas-admin",
			AllowSudo:         true,
			SSHAuthorizedKeys: nil,
		},
		GlobalKernelParams: nil,
		GlobalInitrdParams: nil,
	}
}

func (cfg *InstallerConfiguration) ensureDefaults() {
	if strings.TrimSpace(cfg.Locale) == "" {
		cfg.Locale = "en_US"
	}
	if strings.TrimSpace(cfg.Timezone) == "" {
		cfg.Timezone = "UTC"
	}
	if strings.TrimSpace(cfg.KeyboardLayout) == "" {
		cfg.KeyboardLayout = "us"
	}
	if cfg.Packages == nil {
		cfg.Packages = []string{"openssh-server"}
	}
	if strings.TrimSpace(cfg.GivenUser.Username) == "" {
		cfg.GivenUser.Username = "ubuntu"
	}
	if strings.TrimSpace(cfg.ManagedUser.Username) == "" {
		cfg.ManagedUser.Username = "opnadmin"
	}
	if strings.TrimSpace(cfg.RootPassword) == "" {
		cfg.RootPassword = "changeme"
	}
}

func (cfg InstallerConfiguration) validate() error {
	if strings.TrimSpace(cfg.GivenUser.Username) == "" {
		return errors.New("installer configuration: given_user.username is required")
	}
	if strings.TrimSpace(cfg.ManagedUser.Username) == "" {
		return errors.New("installer configuration: managed_user.username is required")
	}
	if strings.TrimSpace(cfg.Locale) == "" {
		return errors.New("installer configuration: locale is required")
	}
	if strings.TrimSpace(cfg.Timezone) == "" {
		return errors.New("installer configuration: timezone is required")
	}
	return nil
}

func writeInstallerConfiguration(path string, cfg InstallerConfiguration) error {
	cfg.SourcePath = ""
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	return nil
}
