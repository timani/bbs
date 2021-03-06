package migrations

import (
	"database/sql"

	"github.com/cloudfoundry-incubator/bbs/db/etcd"
	"github.com/cloudfoundry-incubator/bbs/encryption"
	"github.com/cloudfoundry-incubator/bbs/migration"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"
)

func init() {
	AppendMigration(NewSQLTestMigration(9999999999))
}

type SQLTestMigration struct {
	version  int64
	rawSQLDB *sql.DB
}

func NewSQLTestMigration(version int64) migration.Migration {
	return &SQLTestMigration{
		version: version,
	}
}

func (t *SQLTestMigration) SetStoreClient(storeClient etcd.StoreClient) {}

func (*SQLTestMigration) SetCryptor(cryptor encryption.Cryptor) {}

func (t *SQLTestMigration) SetRawSQLDB(rawSQLDB *sql.DB) {
	t.rawSQLDB = rawSQLDB
}

func (*SQLTestMigration) SetClock(clock.Clock) {}
func (*SQLTestMigration) SetDBFlavor(string)   {}

func (*SQLTestMigration) RequiresSQL() bool {
	return true
}

func (t *SQLTestMigration) Up(logger lager.Logger) error {
	_, err := t.rawSQLDB.Exec(`CREATE TABLE IF NOT EXISTS sweet_table (
		something VARCHAR(255) PRIMARY KEY,
		something_else INT DEFAULT 0
	);`)

	return err
}

func (t *SQLTestMigration) Down(logger lager.Logger) error {
	// do nothing until we get rollback
	return nil
}

func (t SQLTestMigration) Version() int64 {
	return t.version
}
