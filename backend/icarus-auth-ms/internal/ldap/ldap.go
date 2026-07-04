package ldap

import (
	"icarus-auth-ms/internal/models"
	"fmt"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

type LDAPUser struct {
	Username  string
	Email     string
	FirstName string
	LastName  string
	Groups    []string
}

// DialAndBind connects to the LDAP server and binds with service credentials if specified.
func DialAndBind(cfg *models.LDAPConfig) (*ldap.Conn, error) {
	if cfg.ServerURL == "" {
		return nil, fmt.Errorf("LDAP server URL is empty")
	}

	// Dial the LDAP server
	l, err := ldap.DialURL(cfg.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to dial LDAP server: %w", err)
	}

	// If using start TLS, check if server URL does not start with ldaps://
	if !strings.HasPrefix(strings.ToLower(cfg.ServerURL), "ldaps://") {
		// Try to perform StartTLS if supported, but ignore error if server doesn't enforce TLS
		_ = l.StartTLS(nil)
	}

	// Bind with read-only admin/service account if BindDN is configured
	if cfg.BindDN != "" {
		err = l.Bind(cfg.BindDN, cfg.BindPassword)
		if err != nil {
			l.Close()
			return nil, fmt.Errorf("LDAP bind failed for BindDN: %w", err)
		}
	}

	return l, nil
}

// AuthenticateLDAP attempts to authenticate a user using LDAP credentials and fetches their profile.
func AuthenticateLDAP(cfg *models.LDAPConfig, username, password string) (*LDAPUser, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("LDAP is disabled")
	}

	l, err := DialAndBind(cfg)
	if err != nil {
		return nil, err
	}
	defer l.Close()

	// Search for the user to find their full DN
	userAttr := cfg.UsernameAttribute
	if userAttr == "" {
		userAttr = "uid"
	}
	filter := fmt.Sprintf("(%s=%s)", userAttr, ldap.EscapeFilter(username))

	attrs := []string{"dn", "memberOf", "memberof"}
	if cfg.MailAttribute != "" {
		attrs = append(attrs, cfg.MailAttribute)
	}
	if cfg.FirstNameAttribute != "" {
		attrs = append(attrs, cfg.FirstNameAttribute)
	}
	if cfg.LastNameAttribute != "" {
		attrs = append(attrs, cfg.LastNameAttribute)
	}

	searchReq := ldap.NewSearchRequest(
		cfg.SearchBase,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, 0, false,
		filter,
		attrs,
		nil,
	)

	sr, err := l.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("LDAP search failed: %w", err)
	}

	if len(sr.Entries) == 0 {
		return nil, fmt.Errorf("user not found in LDAP")
	}
	if len(sr.Entries) > 1 {
		return nil, fmt.Errorf("multiple LDAP entries found matching username")
	}

	userEntry := sr.Entries[0]
	userDN := userEntry.DN

	// Bind as the target user to verify password
	err = l.Bind(userDN, password)
	if err != nil {
		return nil, fmt.Errorf("invalid LDAP credentials: %w", err)
	}

	// Retrieve user profile attributes
	email := ""
	if cfg.MailAttribute != "" {
		email = userEntry.GetAttributeValue(cfg.MailAttribute)
	}
	firstName := ""
	if cfg.FirstNameAttribute != "" {
		firstName = userEntry.GetAttributeValue(cfg.FirstNameAttribute)
	}
	lastName := ""
	if cfg.LastNameAttribute != "" {
		lastName = userEntry.GetAttributeValue(cfg.LastNameAttribute)
	}

	// Retrieve and parse user groups (memberOf)
	var groups []string
	groupsDNs := userEntry.GetAttributeValues("memberOf")
	if len(groupsDNs) == 0 {
		groupsDNs = userEntry.GetAttributeValues("memberof")
	}
	for _, dn := range groupsDNs {
		cn := dn
		parsedDN, err := ldap.ParseDN(dn)
		if err == nil {
			for _, rdn := range parsedDN.RDNs {
				for _, attr := range rdn.Attributes {
					if strings.ToLower(attr.Type) == "cn" {
						cn = attr.Value
						break
					}
				}
			}
		} else {
			// Fallback: simple split
			parts := strings.Split(dn, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(strings.ToLower(part), "cn=") {
					cn = part[3:]
					break
				}
			}
		}
		groups = append(groups, cn)
	}

	return &LDAPUser{
		Username:  username,
		Email:     email,
		FirstName: firstName,
		LastName:  lastName,
		Groups:    groups,
	}, nil
}

// TestLDAPConnection tests connecting to the server and binding with the service account.
func TestLDAPConnection(cfg *models.LDAPConfig) error {
	l, err := DialAndBind(cfg)
	if err != nil {
		return err
	}
	l.Close()
	return nil
}
