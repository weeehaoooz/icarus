package models

type LDAPConfig struct {
	ID                 int64  `json:"id"`
	Enabled            bool   `json:"enabled"`
	ServerURL          string `json:"server_url"`
	BindDN             string `json:"bind_dn"`
	BindPassword       string `json:"bind_password"`
	SearchBase         string `json:"search_base"`
	UsernameAttribute  string `json:"username_attribute"`
	MailAttribute      string `json:"mail_attribute"`
	FirstNameAttribute string `json:"first_name_attribute"`
	LastNameAttribute  string `json:"last_name_attribute"`
}
