package config

import (
	"fmt"
	"os"

	"github.com/Netflix/go-env"
	"github.com/joho/godotenv"
)

type Configuration struct {
	Database struct {
		File string `env:"DB_FILE,default=laas.db"`
	}

	TFTP struct {
		Enabled           bool     `env:"TFTP_ENABLED,default=false"`
		TFTP_RootDir      string   `env:"TFTP_ROOT_DIR,default=/var/lib/tftpboot"`
		TFTP_Address      string   `env:"TFTP_ADDRESS,default=:69"`
		ServeHTTPFallback bool     `env:"TFTP_SERVE_HTTP_FALLBACK,default=true"`
		HTTP_RootDir      string   `env:"TFTP_HTTP_ROOT_DIR,default=/var/www/tftpboot"`
		HTTP_Address      string   `env:"TFTP_HTTP_ADDRESS,default=:8069"`
		HTTP_PublicURL    string   `env:"TFTP_HTTP_PUBLIC_URL,default="`
		DHCP_Address      string   `env:"TFTP_DHCP_ADDRESS,default="`
		DHCP_Interface    string   `env:"TFTP_DHCP_INTERFACE,default="`
		DHCP_ServerIP     string   `env:"TFTP_DHCP_SERVER_IP,default="`
		DHCP_LeaseSeconds int      `env:"TFTP_DHCP_LEASE_SECONDS,default=7200"`
		DHCP_SubnetMask   string   `env:"TFTP_DHCP_SUBNET_MASK,default="`
		DHCP_Router       string   `env:"TFTP_DHCP_ROUTER,default="`
		DHCP_DNSServers   []string `env:"TFTP_DHCP_DNS_SERVERS,default="`
		DHCP_AllowedMACs  []string `env:"TFTP_DHCP_ALLOWED_MACS,default="`
		TemplateDir       string   `env:"PXE_TEMPLATE_DIR,default=./pxe/templates"`
		DefaultProfile    struct {
			ISOName      string   `env:"TFTP_DEFAULT_ISO_NAME,default="`
			BootFilename string   `env:"TFTP_DEFAULT_BOOT_FILENAME,default=pxelinux.0"`
			KernelParams []string `env:"TFTP_DEFAULT_KERNEL_PARAMS,default="`
			InitrdParams []string `env:"TFTP_DEFAULT_INITRD_PARAMS,default="`
			TemplateData []string `env:"TFTP_DEFAULT_TEMPLATE_DATA,default="`
			IPv4Address  string   `env:"TFTP_DEFAULT_IPV4_ADDRESS,default="`
			SubnetMask   string   `env:"TFTP_DEFAULT_SUBNET_MASK,default="`
			Gateway      string   `env:"TFTP_DEFAULT_GATEWAY,default="`
			DNSServers   []string `env:"TFTP_DEFAULT_DNS_SERVERS,default="`
			DomainName   string   `env:"TFTP_DEFAULT_DOMAIN_NAME,default="`
			NextServer   string   `env:"TFTP_DEFAULT_NEXT_SERVER,default="`
		}
	}

	LDAP struct {
		Address    string `env:"LDAP_ADDRESS,required=true"`
		DomainSLD  string `env:"LDAP_DOMAIN_SLD,required=true"`
		DomainTLD  string `env:"LDAP_DOMAIN_TLD,required=true"`
		AccountsCN string `env:"LDAP_ACCOUNTS_CN,default=accounts"`
		UsersCN    string `env:"LDAP_USERS_CN,default=users"`
		GroupsCN   string `env:"LDAP_GROUPS_CN,default=groups"`

		// Array values are separated with "|" in the .env file (e.g. LDAP_ADMIN_GROUPS=admins|laasAdmins)
		AdminGroups         []string `env:"LDAP_ADMIN_GROUPS,required=true"`
		CreateBookingGroups []string `env:"LDAP_CREATE_BOOKING_GROUPS,required=true"`
		UserGroups          []string `env:"LDAP_USER_GROUPS,required=true"`
	}

	ISOs struct {
		SearchDir   string `env:"ISOS_SEARCH_DIR,default=./iso_search"`
		StorageDir  string `env:"ISOS_STORAGE_DIR,default=./isos"`
		TestingISOs bool   `env:"ISOS_TESTING,default=false"`
	}

	WebServer struct {
		Address                     string `env:"WEB_ADDRESS,default=:8080"`
		TlsDir                      string `env:"WEB_TLS_DIR"`
		ReloadTemplatesOnEachRender bool   `env:"WEB_RELOAD_TEMPLATES_ON_EACH_RENDER,default=false"`
		// Array values are separated with "|" in the .env file (e.g. WEB_REDIRECT_ADDRESSES=:80|:80)
		RedirectAddresses []string `env:"WEB_REDIRECT_ADDRESSES,default="`
	}

	Management struct {
		DefaultIPMIUser string `env:"MGMT_DEFAULT_IPMI_USER,default=ipmi-user"`
		DefaultIPMIPass string `env:"MGMT_DEFAULT_IPMI_PASS,default=ipmiUserPassword"`

		// Array values are separated with "|" in the .env file (e.g. LDAP_ADMIN_GROUPS=admins|laasAdmins)
		TestingManagementIPs     []string `env:"MGMT_TESTING_IPS,default="`
		TestingRunManagement     bool     `env:"MGMT_TESTING_RUN_MGMT,default=false"`
		TestingRunLongManagement bool     `env:"MGMT_TESTING_RUN_LONG_MGMT,default=false"`
		TestingLongManagementIP  string   `env:"MGMT_TESTING_LONG_MGMT_IP,default="`
	}

	Proxmox struct {
		Enabled bool   `env:"PROXMOX_ENABLED,default=false"`
		Host    string `env:"PROXMOX_HOST,default=proxmox.local"`
		Port    string `env:"PROXMOX_PORT,default=8006"`
		TokenID string `env:"PROXMOX_API_TOKEN_ID,default=root@pam!laas-api-token"`
		Secret  string `env:"PROXMOX_API_TOKEN_SECRET,default=supersecretproxmoxapitokensecret"`

		Testing struct {
			Enabled        bool   `env:"PROXMOX_TESTING_ENABLED,default=false"`
			Subnet         string `env:"PROXMOX_TESTING_SUBNET,default=10.255.255.0/24"`
			Storage        string `env:"PROXMOX_TESTING_STORAGE,default=local-lvm"`
			UbuntuTemplate string `env:"PROXMOX_TESTING_UBUNTU_TEMPLATE,default=local:vztmpl/ubuntu-22.04-standard_22.04-1_amd64.tar.zst"`
			Gateway        string `env:"PROXMOX_TESTING_GATEWAY,default=10.0.0.1"`
			DNS            string `env:"PROXMOX_TESTING_DNS,default=10.0.0.2"`
			SearchDomain   string `env:"PROXMOX_TESTING_SEARCH_DOMAIN,default=local"`
		}
	}

	JWT struct {
		Secret string `env:"JWT_SECRET,required=true"`
	}
}

var Config Configuration

// Try to initialize the environment variables from a .env in the directory the program is run from.
// If the .env file is not present, we will create a sample .env file based on the Configuration struct.
// You can then use config.Config globally
func InitEnv(path string) error {
	if _, err := os.Stat(path); err != nil {
		if e := GenerateSampleEnvFile(path); e != nil {
			return e
		}

		return fmt.Errorf("no .env file found, created a sample .env file. Please fill in the required values and try again")
	}

	if err := godotenv.Load(path); err != nil {
		return err
	}

	_, err := env.UnmarshalFromEnviron(&Config)
	if err != nil {
		return err
	}

	MustHelpfulHippo()

	return nil
}
