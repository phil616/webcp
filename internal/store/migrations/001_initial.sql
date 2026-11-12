CREATE TABLE app_state (
 id INTEGER PRIMARY KEY CHECK(id=1), schema_version INTEGER NOT NULL,
 revision INTEGER NOT NULL CHECK(revision>0), next_site_id INTEGER NOT NULL,
 next_ssl_id INTEGER NOT NULL, next_mtls_id INTEGER NOT NULL
) STRICT;
CREATE TABLE openresty_settings (id INTEGER PRIMARY KEY CHECK(id=1), document TEXT NOT NULL CHECK(json_valid(document))) STRICT;
CREATE TABLE s3_settings (id INTEGER PRIMARY KEY CHECK(id=1), document TEXT NOT NULL CHECK(json_valid(document))) STRICT;
CREATE TABLE runtime_connection (id INTEGER PRIMARY KEY CHECK(id=1), version INTEGER NOT NULL, document TEXT NOT NULL CHECK(json_valid(document))) STRICT;
CREATE TABLE backup_records (id TEXT PRIMARY KEY, created_at TEXT NOT NULL, document TEXT NOT NULL CHECK(json_valid(document))) STRICT;
CREATE TABLE restore_operations (id INTEGER PRIMARY KEY CHECK(id=1), document TEXT NOT NULL CHECK(json_valid(document))) STRICT;

CREATE TABLE certificates (
 id TEXT PRIMARY KEY, position INTEGER NOT NULL,
 document TEXT NOT NULL CHECK(json_valid(document)),
 issuer_id TEXT GENERATED ALWAYS AS (NULLIF(json_extract(document,'$.caId'),'')) VIRTUAL
 REFERENCES certificates(id) DEFERRABLE INITIALLY DEFERRED
) STRICT;
CREATE TABLE websites (
 id TEXT PRIMARY KEY, position INTEGER NOT NULL,
 document TEXT NOT NULL CHECK(json_valid(document) AND json_type(document,'$.pages') IS NULL),
 tls_id TEXT GENERATED ALWAYS AS (NULLIF(json_extract(document,'$.tlsId'),'')) VIRTUAL
 REFERENCES certificates(id) DEFERRABLE INITIALLY DEFERRED
) STRICT;
CREATE INDEX websites_tls_id ON websites(tls_id);
CREATE TABLE streams (id TEXT PRIMARY KEY, position INTEGER NOT NULL, document TEXT NOT NULL CHECK(json_valid(document))) STRICT;
CREATE TABLE lua_resources (id TEXT PRIMARY KEY, position INTEGER NOT NULL, document TEXT NOT NULL CHECK(json_valid(document))) STRICT;
CREATE TABLE ssl_records (
 id TEXT PRIMARY KEY, position INTEGER NOT NULL,
 document TEXT NOT NULL CHECK(json_valid(document)),
 material_id TEXT GENERATED ALWAYS AS (NULLIF(json_extract(document,'$.materialId'),'')) VIRTUAL
 REFERENCES certificates(id) DEFERRABLE INITIALLY DEFERRED
) STRICT;
CREATE TABLE ssl_accounts (id TEXT PRIMARY KEY, position INTEGER NOT NULL, document TEXT NOT NULL CHECK(json_valid(document))) STRICT;
CREATE TABLE ssl_dns_accounts (id TEXT PRIMARY KEY, position INTEGER NOT NULL, document TEXT NOT NULL CHECK(json_valid(document))) STRICT;
CREATE TABLE ssl_cas (
 id TEXT PRIMARY KEY, position INTEGER NOT NULL,
 document TEXT NOT NULL CHECK(json_valid(document)),
 material_id TEXT GENERATED ALWAYS AS (NULLIF(json_extract(document,'$.materialId'),'')) VIRTUAL
 REFERENCES certificates(id) DEFERRABLE INITIALLY DEFERRED
) STRICT;
CREATE TABLE mtls_authorities (id TEXT PRIMARY KEY, position INTEGER NOT NULL, document TEXT NOT NULL CHECK(json_valid(document))) STRICT;
CREATE TABLE mtls_clients (
 id TEXT PRIMARY KEY, position INTEGER NOT NULL,
 document TEXT NOT NULL CHECK(json_valid(document)),
 authority_id TEXT GENERATED ALWAYS AS (json_extract(document,'$.caId')) VIRTUAL
 REFERENCES mtls_authorities(id) DEFERRABLE INITIALLY DEFERRED
) STRICT;
CREATE INDEX mtls_clients_authority_id ON mtls_clients(authority_id);
CREATE TABLE mtls_cas (
 id TEXT PRIMARY KEY, position INTEGER NOT NULL,
 document TEXT NOT NULL CHECK(json_valid(document)),
 material_id TEXT GENERATED ALWAYS AS (NULLIF(json_extract(document,'$.materialId'),'')) VIRTUAL
 REFERENCES mtls_authorities(id) DEFERRABLE INITIALLY DEFERRED
) STRICT;
CREATE TABLE mtls_bindings (
 id TEXT PRIMARY KEY REFERENCES websites(id) DEFERRABLE INITIALLY DEFERRED,
 position INTEGER NOT NULL, document TEXT NOT NULL CHECK(json_valid(document)),
 authority_id TEXT GENERATED ALWAYS AS (json_extract(document,'$.caId')) VIRTUAL
 REFERENCES mtls_authorities(id) DEFERRABLE INITIALLY DEFERRED
) STRICT;
CREATE INDEX mtls_bindings_authority_id ON mtls_bindings(authority_id);
