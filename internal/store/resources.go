package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"

	"web-panel/internal/domain"
)

// Each aggregate has its own table, stable key and order. JSON is limited to a
// typed resource's nested settings; the database never stores a state snapshot.
func saveResources[T any](tx *sql.Tx, table string, values []T, key func(T) string) error {
	keys := make([]string, 0, len(values))
	seen := map[string]bool{}
	statement, err := tx.Prepare(fmt.Sprintf(`INSERT INTO %s(id,position,document) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET position=excluded.position,document=excluded.document WHERE position<>excluded.position OR document<>excluded.document`, table))
	if err != nil {
		return err
	}
	defer statement.Close()
	for position, value := range values {
		id := key(value)
		if id == "" || seen[id] {
			return fmt.Errorf("%s 包含重复或空的资源主键", table)
		}
		seen[id] = true
		keys = append(keys, id)
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if _, err = statement.Exec(id, position, string(data)); err != nil {
			return err
		}
	}
	encoded, _ := json.Marshal(keys)
	_, err = tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE id NOT IN (SELECT value FROM json_each(?))", table), string(encoded))
	return err
}
func loadResources[T any](tx *sql.Tx, table string) ([]T, error) {
	rows, err := tx.Query("SELECT document FROM " + table + " ORDER BY position,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []T{}
	for rows.Next() {
		var data []byte
		if err = rows.Scan(&data); err != nil {
			return nil, err
		}
		var value T
		if err = json.Unmarshal(data, &value); err != nil {
			return nil, fmt.Errorf("读取 %s: %w", table, err)
		}
		values = append(values, value)
	}
	return values, rows.Err()
}
func putSingleton(tx *sql.Tx, table string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO "+table+"(id,document) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET document=excluded.document WHERE document<>excluded.document", string(data))
	return err
}
func getSingleton(tx *sql.Tx, table string, value any) error {
	var data []byte
	if err := tx.QueryRow("SELECT document FROM " + table + " WHERE id=1").Scan(&data); err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
func numeric(id uint) string { return strconv.FormatUint(uint64(id), 10) }
func saveState(tx *sql.Tx, s domain.State) error {
	_, err := tx.Exec(`INSERT INTO app_state VALUES(1,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET schema_version=excluded.schema_version,revision=excluded.revision,next_site_id=excluded.next_site_id,next_ssl_id=excluded.next_ssl_id,next_mtls_id=excluded.next_mtls_id`, s.Schema, s.Revision, s.NextSiteID, s.SSL.NextID, s.MTLS.NextID)
	if err != nil {
		return err
	}
	if err = putSingleton(tx, "openresty_settings", s.OpenResty); err != nil {
		return err
	}
	if err = putSingleton(tx, "s3_settings", s.S3); err != nil {
		return err
	}
	saves := []func() error{
		func() error {
			return saveResources(tx, "websites", s.Sites, func(v domain.Site) string { return v.ID })
		},
		func() error {
			return saveResources(tx, "streams", s.Streams, func(v domain.StreamProxy) string { return v.ID })
		},
		func() error {
			return saveResources(tx, "lua_resources", s.Lua, func(v domain.Lua) string { return v.ID })
		},
		func() error {
			return saveResources(tx, "certificates", s.Certificates, func(v domain.Certificate) string { return v.ID })
		},
		func() error {
			return saveResources(tx, "ssl_records", s.SSL.Records, func(v domain.SSLRecord) string { return numeric(v.ID) })
		},
		func() error {
			return saveResources(tx, "ssl_accounts", s.SSL.Accounts, func(v domain.SSLAccount) string { return numeric(v.ID) })
		},
		func() error {
			return saveResources(tx, "ssl_dns_accounts", s.SSL.DNSAccounts, func(v domain.SSLDNSAccount) string { return numeric(v.ID) })
		},
		func() error {
			return saveResources(tx, "ssl_cas", s.SSL.CAs, func(v domain.SSLCARecord) string { return numeric(v.ID) })
		},
		func() error {
			return saveResources(tx, "mtls_authorities", s.MTLS.Authorities, func(v domain.Certificate) string { return v.ID })
		},
		func() error {
			return saveResources(tx, "mtls_clients", s.MTLS.Clients, func(v domain.Certificate) string { return v.ID })
		},
		func() error {
			return saveResources(tx, "mtls_cas", s.MTLS.CAs, func(v domain.SSLCARecord) string { return numeric(v.ID) })
		},
		func() error {
			return saveResources(tx, "mtls_bindings", s.MTLS.Bindings, func(v domain.MTLSBinding) string { return v.SiteID })
		},
	}
	for _, save := range saves {
		if err = save(); err != nil {
			return err
		}
	}
	return nil
}
func loadState(tx *sql.Tx) (s domain.State, err error) {
	if err = tx.QueryRow("SELECT schema_version,revision,next_site_id,next_ssl_id,next_mtls_id FROM app_state WHERE id=1").Scan(&s.Schema, &s.Revision, &s.NextSiteID, &s.SSL.NextID, &s.MTLS.NextID); err != nil {
		return
	}
	if err = getSingleton(tx, "openresty_settings", &s.OpenResty); err != nil {
		return
	}
	if err = getSingleton(tx, "s3_settings", &s.S3); err != nil {
		return
	}
	if s.Sites, err = loadResources[domain.Site](tx, "websites"); err != nil {
		return
	}
	if s.Streams, err = loadResources[domain.StreamProxy](tx, "streams"); err != nil {
		return
	}
	if s.Lua, err = loadResources[domain.Lua](tx, "lua_resources"); err != nil {
		return
	}
	if s.Certificates, err = loadResources[domain.Certificate](tx, "certificates"); err != nil {
		return
	}
	if s.SSL.Records, err = loadResources[domain.SSLRecord](tx, "ssl_records"); err != nil {
		return
	}
	if s.SSL.Accounts, err = loadResources[domain.SSLAccount](tx, "ssl_accounts"); err != nil {
		return
	}
	if s.SSL.DNSAccounts, err = loadResources[domain.SSLDNSAccount](tx, "ssl_dns_accounts"); err != nil {
		return
	}
	if s.SSL.CAs, err = loadResources[domain.SSLCARecord](tx, "ssl_cas"); err != nil {
		return
	}
	if s.MTLS.Authorities, err = loadResources[domain.Certificate](tx, "mtls_authorities"); err != nil {
		return
	}
	if s.MTLS.Clients, err = loadResources[domain.Certificate](tx, "mtls_clients"); err != nil {
		return
	}
	if s.MTLS.CAs, err = loadResources[domain.SSLCARecord](tx, "mtls_cas"); err != nil {
		return
	}
	if s.MTLS.Bindings, err = loadResources[domain.MTLSBinding](tx, "mtls_bindings"); err != nil {
		return
	}
	domain.NormalizeState(&s)
	err = domain.Validate(s)
	return
}
