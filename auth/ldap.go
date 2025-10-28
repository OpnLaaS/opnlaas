package auth

import (
	"crypto/tls"
	"fmt"

	"github.com/go-ldap/ldap/v3"
	"github.com/opnlaas/laas/config"
)

const LDAP_USER = "uid=%s,cn=%s,cn=accounts,dc=%s,dc=%s"

var ErrUnauthorized error = fmt.Errorf("unauthorized")

func getUsername(s string) string {
	return fmt.Sprintf(LDAP_USER, s, config.Config.LDAP.UsersCN, config.Config.LDAP.DomainSLD, config.Config.LDAP.DomainTLD)
}

func getGroupName(s string) string {
	return fmt.Sprintf("cn=%s,cn=%s,cn=accounts,dc=%s,dc=%s", s, config.Config.LDAP.GroupsCN, config.Config.LDAP.DomainSLD, config.Config.LDAP.DomainTLD)
}

func getFilter() string {
	return fmt.Sprintf("cn=%s,cn=accounts,dc=%s,dc=%s", config.Config.LDAP.UsersCN, config.Config.LDAP.DomainSLD, config.Config.LDAP.DomainTLD)
}

func getGroupsFilter() string {
	return fmt.Sprintf("cn=%s,cn=accounts,dc=%s,dc=%s", config.Config.LDAP.GroupsCN, config.Config.LDAP.DomainSLD, config.Config.LDAP.DomainTLD)
}

func UserExists(username string) (exists bool, err error) {
	var conn *ldap.Conn
	if conn, err = ldap.DialURL(config.Config.LDAP.Address, ldap.DialWithTLSConfig(&tls.Config{InsecureSkipVerify: true})); err != nil {
		return
	}

	defer conn.Close()

	var result *ldap.SearchResult
	if result, err = conn.Search(ldap.NewSearchRequest(
		fmt.Sprintf(getFilter(), config.Config.LDAP.UsersCN, config.Config.LDAP.DomainSLD, config.Config.LDAP.DomainTLD),
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1, 0, false,
		fmt.Sprintf("(uid=%s)", username),
		[]string{"dn"},
		nil,
	)); err != nil {
		return
	}

	exists = len(result.Entries) > 0
	return
}

type LDAPConn struct {
	conn            *ldap.Conn
	Username        string
	IsAuthenticated bool
}

func NewLDAPConn(username, password string) (conn *LDAPConn, err error) {
	var socket *ldap.Conn
	if socket, err = ldap.DialURL(config.Config.LDAP.Address, ldap.DialWithTLSConfig(&tls.Config{InsecureSkipVerify: true})); err != nil {
		return
	}

	conn = &LDAPConn{
		conn:            socket,
		Username:        username,
		IsAuthenticated: socket.Bind(getUsername(username), password) == nil,
	}

	return
}

func (l *LDAPConn) Close() {
	if l.conn != nil {
		l.conn.Close()
	}
}

func (l *LDAPConn) WhoAmI() (id string, err error) {
	if !l.IsAuthenticated {
		err = ErrUnauthorized
		return
	}

	var who *ldap.WhoAmIResult
	if who, err = l.conn.WhoAmI(nil); err != nil {
		return
	}

	id = who.AuthzID
	return
}

func (l *LDAPConn) Groups() (groups []string, err error) {
	if !l.IsAuthenticated {
		err = ErrUnauthorized
		return
	}

	var result *ldap.SearchResult
	if result, err = l.conn.Search(ldap.NewSearchRequest(
		getGroupsFilter(), ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(&(objectClass=groupOfNames)(member=%s))", getUsername(l.Username)),
		[]string{"cn"}, nil,
	)); err != nil {
		return nil, err
	}

	for _, entry := range result.Entries {
		groups = append(groups, entry.GetAttributeValue("cn"))
	}

	return
}

func (l *LDAPConn) GetAttributes(attrs ...string) (attributes map[string]string, err error) {
	if !l.IsAuthenticated {
		err = ErrUnauthorized
		return
	}

	var result *ldap.SearchResult
	if result, err = l.conn.Search(ldap.NewSearchRequest(
		getFilter(), ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1, 0, false,
		fmt.Sprintf("(uid=%s)", l.Username), attrs, nil,
	)); err != nil {
		return
	}

	if len(result.Entries) == 0 {
		err = fmt.Errorf("no entries found")
		return
	}

	var entry *ldap.Entry = result.Entries[0]
	attributes = make(map[string]string)
	for _, attr := range attrs {
		attributes[attr] = entry.GetAttributeValue(attr)
	}

	return
}

func (l *LDAPConn) IsMemberOf(groupName string) (isMember bool, err error) {
	if !l.IsAuthenticated {
		err = ErrUnauthorized
		return
	}

	var result *ldap.SearchResult
	if result, err = l.conn.Search(ldap.NewSearchRequest(
		getGroupName(groupName), ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		fmt.Sprintf("(member=%s)", getUsername(l.Username)), []string{"cn"}, nil,
	)); err != nil {
		return false, err
	}

	isMember = len(result.Entries) > 0
	return
}

func (l *LDAPConn) DisplayName() (displayName string, err error) {
	var attributes map[string]string
	if attributes, err = l.GetAttributes("displayName"); err == nil {
		displayName = attributes["displayName"]
	}

	return
}

func (l *LDAPConn) Email() (email string, err error) {
	var attributes map[string]string
	if attributes, err = l.GetAttributes("mail"); err == nil {
		email = attributes["mail"]
	}

	return
}

func (l *LDAPConn) UID() (uid uint64, err error) {
	var attributes map[string]string
	if attributes, err = l.GetAttributes("uidNumber"); err == nil {
		_, err = fmt.Sscanf(attributes["uidNumber"], "%d", &uid)
	}

	return
}
