package repository

import (
	"icarus-auth-ms/internal/models"
	"database/sql"
	"errors"
)

func (r *SQLRepository) GetLDAPConfig() (*models.LDAPConfig, error) {
	var cfg models.LDAPConfig
	var enabledVal bool
	row := r.db.QueryRow(`
		SELECT id, enabled, server_url, bind_dn, bind_password, search_base, 
		       username_attribute, mail_attribute, first_name_attribute, last_name_attribute 
		FROM ldap_config 
		WHERE id = 1
	`)
	err := row.Scan(
		&cfg.ID, &enabledVal, &cfg.ServerURL, &cfg.BindDN, &cfg.BindPassword, &cfg.SearchBase,
		&cfg.UsernameAttribute, &cfg.MailAttribute, &cfg.FirstNameAttribute, &cfg.LastNameAttribute,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &models.LDAPConfig{
				ID:                 1,
				Enabled:            false,
				ServerURL:          "",
				BindDN:             "",
				BindPassword:       "",
				SearchBase:         "",
				UsernameAttribute:  "uid",
				MailAttribute:      "mail",
				FirstNameAttribute: "givenName",
				LastNameAttribute:  "sn",
			}, nil
		}
		return nil, err
	}
	cfg.Enabled = enabledVal
	return &cfg, nil
}

func (r *SQLRepository) UpdateLDAPConfig(cfg *models.LDAPConfig) error {
	var err error
	if r.driver == "postgres" {
		_, err = r.db.Exec(`
			INSERT INTO ldap_config (
				id, enabled, server_url, bind_dn, bind_password, search_base, 
				username_attribute, mail_attribute, first_name_attribute, last_name_attribute
			) VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO UPDATE SET
				enabled = EXCLUDED.enabled,
				server_url = EXCLUDED.server_url,
				bind_dn = EXCLUDED.bind_dn,
				bind_password = EXCLUDED.bind_password,
				search_base = EXCLUDED.search_base,
				username_attribute = EXCLUDED.username_attribute,
				mail_attribute = EXCLUDED.mail_attribute,
				first_name_attribute = EXCLUDED.first_name_attribute,
				last_name_attribute = EXCLUDED.last_name_attribute
		`, cfg.Enabled, cfg.ServerURL, cfg.BindDN, cfg.BindPassword, cfg.SearchBase,
			cfg.UsernameAttribute, cfg.MailAttribute, cfg.FirstNameAttribute, cfg.LastNameAttribute)
	} else {
		_, err = r.db.Exec(`
			INSERT INTO ldap_config (
				id, enabled, server_url, bind_dn, bind_password, search_base, 
				username_attribute, mail_attribute, first_name_attribute, last_name_attribute
			) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET
				enabled = excluded.enabled,
				server_url = excluded.server_url,
				bind_dn = excluded.bind_dn,
				bind_password = excluded.bind_password,
				search_base = excluded.search_base,
				username_attribute = excluded.username_attribute,
				mail_attribute = excluded.mail_attribute,
				first_name_attribute = excluded.first_name_attribute,
				last_name_attribute = excluded.last_name_attribute
		`, cfg.Enabled, cfg.ServerURL, cfg.BindDN, cfg.BindPassword, cfg.SearchBase,
			cfg.UsernameAttribute, cfg.MailAttribute, cfg.FirstNameAttribute, cfg.LastNameAttribute)
	}
	return err
}
