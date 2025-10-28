package auth

import (
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/opnlaas/laas/config"
)

type authPerms uint8

const (
	AuthPermsNone          authPerms = iota // No permissions, cannot log in
	AuthPermsUser                           // Can view but not edit
	AuthPermsAdministrator                  // Can do everything
)

type AuthUser struct {
	LDAPConn *LDAPConn
	Token    *jwt.Token
	Expiry   time.Time
	perms    authPerms
}

func (user *AuthUser) Permissions() authPerms {
	if user.perms != AuthPermsNone {
		return user.perms
	}

	if user.LDAPConn == nil {
		return AuthPermsNone
	}

	if groups, err := user.LDAPConn.Groups(); err != nil || len(groups) == 0 {
		return AuthPermsNone
	} else {
		for _, gName := range config.Config.LDAP.AdminGroups {
			if slices.Contains(groups, gName) {
				user.perms = AuthPermsAdministrator
				return user.perms
			}
		}

		for _, gName := range config.Config.LDAP.UserGroups {
			if slices.Contains(groups, gName) {
				user.perms = AuthPermsUser
				return user.perms
			}
		}
	}

	user.perms = AuthPermsNone
	return user.perms
}

var activeUsers = make(map[string]*AuthUser)
var usersLock *sync.RWMutex = &sync.RWMutex{}

func GetActiveUser(username string) *AuthUser {
	usersLock.RLock()
	defer usersLock.RUnlock()

	if user, ok := activeUsers[username]; ok {
		if user.Expiry.After(time.Now()) {
			return user
		}
	}

	return nil
}

func RefreshToken(user *AuthUser) {
	user.Expiry = time.Now().Add(time.Hour)
}

func WithAuth(w http.ResponseWriter, r *http.Request, jwtSecret []byte) bool {
	var authToken string

	if cookie, err := r.Cookie("Authorization"); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	} else {
		authToken = cookie.Value
	}

	if authToken == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}

	parsedToken, err := jwt.Parse(authToken, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}

		return jwtSecret, nil
	})

	if err != nil || !parsedToken.Valid {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}

	if claims, ok := parsedToken.Claims.(jwt.MapClaims); ok {
		if username, ok := claims["username"].(string); ok {
			usersLock.Lock()
			defer usersLock.Unlock()

			user, ok := activeUsers[username]
			if ok && user.Expiry.After(time.Now()) {
				RefreshToken(user)
				return true
			}
		}
	}

	w.WriteHeader(http.StatusUnauthorized)
	return false
}

func IsAuthenticated(r *fiber.Ctx, jwtSecret []byte) *AuthUser {
	var authToken string = r.Cookies("Authorization")

	if authToken == "" {
		return nil
	}

	parsedToken, err := jwt.Parse(authToken, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}

		return jwtSecret, nil
	})

	if err != nil || !parsedToken.Valid {
		return nil
	}

	if claims, ok := parsedToken.Claims.(jwt.MapClaims); ok {
		if username, ok := claims["username"].(string); ok {
			usersLock.RLock()
			defer usersLock.RUnlock()

			user, ok := activeUsers[username]
			if ok && user.Expiry.After(time.Now()) {
				return user
			}
		}
	}

	return nil
}

func Authenticate(username, password string) (*AuthUser, error) {
	ldapConn, err := NewLDAPConn(username, password)
	if err != nil {
		return nil, err
	}

	if !ldapConn.IsAuthenticated {
		ldapConn.Close()
		return nil, ErrUnauthorized
	}

	user := &AuthUser{
		LDAPConn: ldapConn,
		Token:    jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"username": username}),
		Expiry:   time.Now().Add(time.Hour),
	}

	if user.Permissions() == AuthPermsNone {
		ldapConn.Close()
		return nil, fmt.Errorf("user is unauthorized to use this application")
	}

	usersLock.Lock()
	defer usersLock.Unlock()

	activeUsers[username] = user

	return user, nil
}

func Logout(username string) {
	usersLock.Lock()
	defer usersLock.Unlock()

	if user, ok := activeUsers[username]; ok {
		user.LDAPConn.Close()
		delete(activeUsers, username)
	}
}
