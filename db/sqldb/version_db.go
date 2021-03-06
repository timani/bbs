package sqldb

import (
	"encoding/json"

	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/pivotal-golang/lager"
)

const VersionID = "version"

func (db *SQLDB) SetVersion(logger lager.Logger, version *models.Version) error {
	logger = logger.Session("set-version-sqldb", lager.Data{"version": version})
	logger.Debug("starting")
	defer logger.Debug("complete")

	versionJSON, err := json.Marshal(version)
	if err != nil {
		logger.Error("failed-marshalling-version", err)
		return err
	}

	return db.setConfigurationValue(logger, VersionID, string(versionJSON))
}

func (db *SQLDB) Version(logger lager.Logger) (*models.Version, error) {
	logger = logger.Session("version-sqldb")
	logger.Debug("starting")
	defer logger.Debug("complete")

	versionJSON, err := db.getConfigurationValue(logger, VersionID)
	if err != nil {
		return nil, err
	}

	var version models.Version
	err = json.Unmarshal([]byte(versionJSON), &version)
	if err != nil {
		logger.Error("failed-to-deserialize-version", err)
		return nil, models.ErrDeserialize
	}

	return &version, nil
}
