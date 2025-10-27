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
		TFTP_RootDir      string `env:"TFTP_ROOT_DIR,default=/var/lib/tftpboot"`
		TFTP_Address      string `env:"TFTP_ADDRESS,default=:69"`
		ServeHTTPFallback bool   `env:"TFTP_SERVE_HTTP_FALLBACK,default=true"`
		HTTP_RootDir      string `env:"HTTP_ROOT_DIR,default=/var/www/tftpboot"`
		HTTP_Address      string `env:"HTTP_ADDRESS,default=:8069"`
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
		SearchDir string `env:"ISOS_SEARCH_DIR,default=./isos"`
	}

	WebServer struct {
		Address string `env:"WEB_ADDRESS,default=:8080"`
		TlsDir  string `env:"WEB_TLS_DIR"`
	}

	Management struct {
		DefaultIPMIUser string `env:"MGMT_DEFAULT_IPMI_USER,default=ipmi-user"`
		DefaultIPMIPass string `env:"MGMT_DEFAULT_IPMI_PASS,default=ipmiUserPassword"`
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

	return nil
}
