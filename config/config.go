package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
)

type Configuration struct {
	WebServer struct {
		Address                     string   `toml:"address" default:":8080" validate:"required"`                     // Listen address for the web application server e.g. ":8080" or "0.0.0.0:8080"
		TLSDir                      string   `toml:"tls_dir" default:""`                                              // Directory containing a crt and a key file for TLS. Leave empty to use HTTP instead of HTTPS.
		ReloadTemplatesOnEachRender bool     `toml:"reload_templates_on_each_render" default:"false"`                 // For development purposes. If true, templates are reloaded from disk on each render.
		RedirectServerAddresses     []string `toml:"redirect_server_addresses" default:"[]" validate:"dive,required"` // List of addresses ("host:port" or ":port") to which HTTP requests should be redirected to HTTPS. If your web app is on ":443", you might want to redirect ":80" here.
	} `toml:"web_server"` // Web server configuration

	LDAP struct {
		Address     string   `toml:"address" default:"" validate:"required"`                   // LDAP server address (e.g. "ldaps://domain.cyber.lab:636")
		DomainSLD   string   `toml:"domain_sld" default:"" validate:"required"`                // LDAP domain second-level domain (e.g. "cyber" for "domain.cyber.lab")
		DomainTLD   string   `toml:"domain_tld" default:"" validate:"required"`                // LDAP domain top-level domain (e.g. "lab" for "domain.cyber.lab")
		AccountsCN  string   `toml:"accounts_cn" default:"accounts" validate:"required"`       // LDAP container name for accounts (usually "accounts")
		UsersCN     string   `toml:"users_cn" default:"users" validate:"required"`             // LDAP container name for users (usually "users")
		GroupsCN    string   `toml:"groups_cn" default:"groups" validate:"required"`           // LDAP container name for groups (usually "groups")
		AdminGroups []string `toml:"admin_groups" default:"[\"admins\"]" validate:"required"`  // LDAP groups whose members should have admin access to the web app
		UserGroups  []string `toml:"user_groups" default:"[\"ipausers\"]" validate:"required"` // LDAP groups whose members should have user access to the web app
	} `toml:"ldap"` // LDAP configuration

	Management struct {
		Username string `toml:"username" default:"" validate:"required"` // IPMI/Redfish operator username
		Password string `toml:"password" default:"" validate:"required"` // IPMI/Redfish operator password

		Testing struct {
			Basic struct {
				Enabled bool     `toml:"enabled" default:"false"`                   // Enable basic management interface testing mode
				IPs     []string `toml:"ips" default:"[]" validate:"dive,required"` // List of IPs to run management testing on
			} `toml:"basic"` // Basic testing mode: just a list of IPs

			Long struct {
				Enabled bool   `toml:"enabled" default:"false"` // Enable long management interface testing mode
				IP      string `toml:"ip" default:""`           // IP to run long management testing on
			} `toml:"long"` // Long testing mode: single IP with extended tests
		} `toml:"testing"` // Management interface testing configuration. If either component is enabled, when runing go tests, the management interface tests will be run.
	} `toml:"management"` // Management interface configuration. Note that the user provided should be an "operator" user with limited privileges.

	Database struct {
		File string `toml:"file" default:"laas.db" validate:"required"` // Path to the MySQL database file
	} `toml:"database"` // Database configuration

	Proxmox struct {
		Enabled  bool   `toml:"enabled" default:"false"`                // Enable Proxmox VE integration
		Hostname string `toml:"hostname" default:""`                    // Proxmox VE server hostname or IP address (e.g. "proxmox.cyber.lab")
		Port     string `toml:"port" default:""`                        // Proxmox VE API port (usually "8006")
		TokenID  string `toml:"token_id" default:""`                    // Proxmox VE API token ID (e.g. "laas-api-token-id")
		Secret   string `toml:"secret" default:"laas-api-token-secret"` // Proxmox VE API token secret

		Testing struct {
			Enabled        bool   `toml:"enabled" default:"false"`                                                            // Enable Proxmox VE integration testing mode
			SubnetCIDR     string `toml:"subnet_cidr" default:"10.255.255.0/24"`                                              // Subnet CIDR to use for testing VMs
			Storage        string `toml:"storage" default:"local-lvm"`                                                        // Proxmox VE storage to use for testing VMs
			UbuntuTemplate string `toml:"ubuntu_template" default:"local:vztmpl/ubuntu-22.04-standard_22.04-1_amd64.tar.zst"` // Proxmox VE container template to use for testing VMs
			Gateway        string `toml:"gateway" default:"10.0.0.1"`                                                         // Gateway IP for testing VMs
			DNS            string `toml:"dns" default:"10.0.0.2"`                                                             // DNS server IP for testing VMs
			SearchDomain   string `toml:"search_domain" default:"local"`                                                      // Search domain for testing VMs
		} `toml:"testing"` // Proxmox VE integration testing configuration
	} `toml:"proxmox"` // Proxmox VE integration configuration

	ISOs struct {
		SearchDir  string `toml:"search_dir" default:"./isos_search_dir"`           // Optional. Directory to search for ISO files. Whenever modifications are made to this directory, the application will pick up changes automatically.
		StorageDir string `toml:"storage_dir" default:"./isos" validate:"required"` // Directory to store ISO files for use with the application.
		Testing    bool   `toml:"testing" default:"false"`                          // Enable ISO testing mode. When running go tests, the ISO testing suite will be executed.
	} `toml:"isos"` // ISO management configuration

	PXE struct {
		Enabled bool `toml:"enabled" default:"false"` // Enable PXE services (DHCP, TFTP, HTTP)

		DHCPServer struct {
			Address             string   `toml:"address" default:":67" validate:"required"`         // DHCP server listen address (e.g. ":67")
			Interface           string   `toml:"interface" default:"eth0"`                          // Network interface to bind the DHCP server to
			ServerPublicAddress string   `toml:"server_public_address" default:""`                  // Publicly reachable IP address of the DHCP server (Used in the "Next Server" DHCP option)
			ProxyMode           bool     `toml:"proxy_mode" default:"false"`                        // If true, act as a proxy DHCP server and do not hand out leases
			IPRangeStart        string   `toml:"ip_range_start" default:""`                         // Start of the DHCP IP address range to lease to clients
			IPRangeEnd          string   `toml:"ip_range_end" default:""`                           // End of the DHCP IP address range to lease to clients
			LeaseSeconds        int      `toml:"lease_seconds" default:"7200" validate:"required"`  // DHCP lease duration in seconds
			SubnetMask          string   `toml:"subnet_mask" default:""`                            // Subnet mask to provide to DHCP clients
			Router              string   `toml:"router" default:""`                                 // Default gateway to provide to DHCP clients
			DNSServers          []string `toml:"dns_servers" default:"[]" validate:"dive,required"` // DNS servers to provide to DHCP clients
		} `toml:"dhcp_server"` // DHCP server configuration

		TFTPServer struct {
			Address   string `toml:"address" default:":69" validate:"required"`                 // TFTP server listen address (e.g. ":69")
			Directory string `toml:"directory" default:"/var/lib/tftpboot" validate:"required"` // TFTP server root directory
		} `toml:"tftp_server"` // TFTP server configuration

		HTTPServer struct {
			Address   string `toml:"address" default:":8069" validate:"required"`               // HTTP server listen address (e.g. ":8069")
			Directory string `toml:"directory" default:"/var/www/tftpboot" validate:"required"` // HTTP server root directory
			PublicURL string `toml:"public_url" default:""`                                     // Publicly reachable URL of the HTTP server (used in PXE configs) (e.g. "http://pxe.cyber.lab:8069")
		} `toml:"http_server"` // HTTP server configuration
	} `toml:"pxe"` // PXE services configuration

	Preconfigure struct {
		Locale          string   `toml:"locale" default:"en_US" validate:"required"`                                // System locale (e.g. "en_US")
		Timezone        string   `toml:"timezone" default:"UTC" validate:"required"`                                // System timezone (e.g. "UTC")
		Keyboard        string   `toml:"keyboard_layout" default:"us" validate:"required"`                          // Keyboard layout (e.g. "us")
		KeyboardVariant string   `toml:"keyboard_variant" default:""`                                               // Keyboard variant (e.g. "")
		Packages        []string `toml:"packages" default:"[\"openssh-server\"]" validate:"required,dive,required"` // List of additional packages to install
		Mirror          string   `toml:"mirror" default:""`                                                         // Package mirror URL (e.g. "http://archive.ubuntu.com/ubuntu")
		RootPassword    string   `toml:"root_password" default:""`                                                  // Root password for Kickstart-based installs
		DisableRoot     bool     `toml:"disable_root" default:"true"`                                               // Whether to disable the root account for Autoinstall-based installs

		ManagedUser struct {
			Username          string   `toml:"username" default:"laas-admin" validate:"required"`         // Managed user username
			Password          string   `toml:"password" default:"laas-admin" validate:"required"`         // Managed user password
			AllowSudo         bool     `toml:"allow_sudo" default:"true"`                                 // Whether the managed user is allowed to use sudo
			SSHAuthorizedKeys []string `toml:"ssh_authorized_keys" default:"[]" validate:"dive,required"` // SSH authorized keys for the managed user
		} `toml:"managed_user"` // The user that the LaaS system manages on the deployed systems

		GivenUser struct {
			Username          string   `toml:"username" default:"laas" validate:"required"`               // Given user username
			Password          string   `toml:"password" default:"laas" validate:"required"`               // Given user password
			AllowSudo         bool     `toml:"allow_sudo" default:"true"`                                 // Whether the given user is allowed to use sudo
			SSHAuthorizedKeys []string `toml:"ssh_authorized_keys" default:"[]" validate:"dive,required"` // SSH authorized keys for the given user
		} `toml:"given_user"` // The user that is specified at deployment time

		GlobalKernelParams []string `toml:"global_kernel_params" default:"[]" validate:"dive,required"` // Global kernel parameters to add to all preconfigure templates
		GlobalInitrdParams []string `toml:"global_initrd_params" default:"[]" validate:"dive,required"` // Global initrd parameters to add to all preconfigure templates

		ScriptingFilePaths struct {
			GlobalPreScriptFile  string `toml:"global_pre_script_file" default:""`  // Path to a global pre-script file to include in all preconfigure templates
			GlobalPostScriptFile string `toml:"global_post_script_file" default:""` // Path to a global post-script file to include in all preconfigure templates
		} `toml:"scripting_file_paths"` // File paths for global pre/post scripts to include in all preconfigure templates
	} `toml:"preconfigure"` // Preconfigure template defaults
}

var (
	Config           Configuration
	loadedConfigPath string
)

func LoadedConfigPath() string {
	return loadedConfigPath
}

func loadConfig(path string) (err error) {
	// Apply struct defaults BEFORE loading TOML (so TOML overrides)
	if err = defaults.Set(&Config); err != nil {
		err = fmt.Errorf("set defaults: %w", err)
		return
	}

	// Decode TOML file into struct
	if _, err = toml.DecodeFile(path, &Config); err != nil {
		err = fmt.Errorf("decode toml: %w", err)
		return
	}

	// Validate required fields
	if err = validator.New(validator.WithRequiredStructEnabled()).Struct(Config); err != nil {
		err = fmt.Errorf("validate config: %w", err)
	}

	return
}

// generateDefaultConfig writes a config.toml with all default values filled in.
// It will overwrite any existing file at path.
func generateDefaultConfig(path string) (err error) {
	var cfg Configuration

	// 1. Apply struct defaults
	if err = defaults.Set(&cfg); err != nil {
		err = fmt.Errorf("set defaults: %w", err)
		return
	}

	// NOTE: Do NOT validate here.
	// The default config is allowed to be "invalid" from a required-fields POV;
	// it's just a template for the user to fill in.
	// Validation happens in LoadConfig() when we actually load the file.

	// 2. Create / truncate the file
	var file *os.File
	if file, err = os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644); err != nil {
		err = fmt.Errorf("create config file: %w", err)
		return
	}

	defer file.Close()

	// 3. Encode as TOML
	var encoder *toml.Encoder = toml.NewEncoder(file)
	encoder.Indent = "    "
	if err = encoder.Encode(cfg); err != nil {
		err = fmt.Errorf("encode toml: %w", err)
	}

	return
}

func Init(path string) (err error) {
	if !filepath.IsAbs(path) {
		if path, err = filepath.Abs(path); err != nil {
			return err
		}
	}
	loadedConfigPath = path

	if _, err = os.Stat(path); err != nil {
		if err = generateDefaultConfig(path); err != nil {
			return
		}

		err = fmt.Errorf("no config file found, created a default config at %s. Please fill in the required values and try again", path)
		return
	}

	if err = loadConfig(path); err != nil {
		return err
	}

	return nil
}
