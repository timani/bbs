package db

import (
	"fmt"
	"path"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs/bbserrors"
	"github.com/cloudfoundry/gunk/workpool"
	"github.com/coreos/go-etcd/etcd"
	"github.com/pivotal-golang/lager"
)

const maxActualGroupGetterWorkPoolSize = 50
const ActualLRPSchemaRoot = DataSchemaRoot + "actual"
const ActualLRPInstanceKey = "instance"
const ActualLRPEvacuatingKey = "evacuating"

func ActualLRPProcessDir(processGuid string) string {
	return path.Join(ActualLRPSchemaRoot, processGuid)
}

func ActualLRPIndexDir(processGuid string, index int32) string {
	return path.Join(ActualLRPProcessDir(processGuid), strconv.Itoa(int(index)))
}
func ActualLRPSchemaPath(processGuid string, index int32) string {
	return path.Join(ActualLRPIndexDir(processGuid, index), ActualLRPInstanceKey)
}

func EvacuatingActualLRPSchemaPath(processGuid string, index int32) string {
	return path.Join(ActualLRPIndexDir(processGuid, index), ActualLRPEvacuatingKey)
}

func (db *ETCDDB) ActualLRPGroups(filter models.ActualLRPFilter, logger lager.Logger) (*models.ActualLRPGroups, error) {
	node, err := db.fetchRecursiveRaw(ActualLRPSchemaRoot, logger)
	if err != nil {
		return &models.ActualLRPGroups{}, nil
	}
	if node.Nodes.Len() == 0 {
		return &models.ActualLRPGroups{}, nil
	}

	var groups = &models.ActualLRPGroups{}
	groupsLock := sync.Mutex{}
	var workErr atomic.Value

	works := []func(){}

	for _, node := range node.Nodes {
		node := node

		works = append(works, func() {
			g, err := parseActualLRPGroups(node, filter, logger)
			if err != nil {
				workErr.Store(err)
				return
			}
			groupsLock.Lock()
			groups.ActualLrpGroups = append(groups.ActualLrpGroups, g.ActualLrpGroups...)
			groupsLock.Unlock()
		})
	}

	throttler, err := workpool.NewThrottler(maxActualGroupGetterWorkPoolSize, works)
	if err != nil {
		logger.Error("failed-constructing-throttler", err, lager.Data{"max-workers": maxActualGroupGetterWorkPoolSize, "num-works": len(works)})
		return &models.ActualLRPGroups{}, err
	}

	logger.Debug("performing-deserialization-work")
	throttler.Work()
	if err, ok := workErr.Load().(error); ok {
		logger.Error("failed-performing-deserialization-work", err)
		return &models.ActualLRPGroups{}, err
	}
	logger.Debug("succeeded-performing-deserialization-work", lager.Data{"num-actual-lrp-groups": len(groups.GetActualLrpGroups())})

	return groups, nil
}

func (db *ETCDDB) ActualLRPGroupsByProcessGuid(processGuid string, logger lager.Logger) (*models.ActualLRPGroups, error) {
	node, err := db.fetchRecursiveRaw(ActualLRPProcessDir(processGuid), logger)
	if err != nil {
		return &models.ActualLRPGroups{}, nil
	}
	if node.Nodes.Len() == 0 {
		return &models.ActualLRPGroups{}, nil
	}

	return parseActualLRPGroups(node, models.ActualLRPFilter{}, logger)
}

func (db *ETCDDB) ActualLRPGroupByProcessGuidAndIndex(processGuid string, index int32, logger lager.Logger) (*models.ActualLRPGroup, error) {
	node, err := db.fetchRecursiveRaw(ActualLRPIndexDir(processGuid, index), logger)
	if err != nil {
		return nil, err
	}

	group := models.ActualLRPGroup{}
	for _, instanceNode := range node.Nodes {
		var lrp models.ActualLRP
		deserializeErr := models.FromJSON([]byte(instanceNode.Value), &lrp)
		if deserializeErr != nil {
			logger.Error("failed-parsing-actual-lrp", deserializeErr)
			return nil, fmt.Errorf("cannot parse lrp JSON for key %s: %s", instanceNode.Key, deserializeErr.Error())
		}

		if isInstanceActualLRPNode(instanceNode) {
			group.Instance = &lrp
		}

		if isEvacuatingActualLRPNode(instanceNode) {
			group.Evacuating = &lrp
		}
	}

	if group.Evacuating == nil && group.Instance == nil {
		return nil, bbserrors.ErrStoreResourceNotFound
	}

	return &group, nil
}

func parseActualLRPGroups(node *etcd.Node, filter models.ActualLRPFilter, logger lager.Logger) (*models.ActualLRPGroups, error) {
	var groups = &models.ActualLRPGroups{}

	logger.Debug("performing-parsing-actual-lrp-groups")
	for _, indexNode := range node.Nodes {
		group := &models.ActualLRPGroup{}
		for _, instanceNode := range indexNode.Nodes {
			var lrp models.ActualLRP
			deserializeErr := models.FromJSON([]byte(instanceNode.Value), &lrp)
			if deserializeErr != nil {
				logger.Error("failed-parsing-actual-lrp-groups", deserializeErr)
				return &models.ActualLRPGroups{}, fmt.Errorf("cannot parse lrp JSON for key %s: %s", instanceNode.Key, deserializeErr.Error())
			}
			if filter.Domain != "" && lrp.GetDomain() != filter.Domain {
				continue
			}
			if filter.CellID != "" && lrp.GetCellId() != filter.CellID {
				continue
			}

			if isInstanceActualLRPNode(instanceNode) {
				group.Instance = &lrp
			}

			if isEvacuatingActualLRPNode(instanceNode) {
				group.Evacuating = &lrp
			}
		}

		if group.Instance != nil || group.Evacuating != nil {
			groups.ActualLrpGroups = append(groups.ActualLrpGroups, group)
		}
	}
	logger.Debug("succeeded-performing-parsing-actual-lrp-groups", lager.Data{"num-actual-lrp-groups": len(groups.GetActualLrpGroups())})

	return groups, nil
}

func isInstanceActualLRPNode(node *etcd.Node) bool {
	return path.Base(node.Key) == ActualLRPInstanceKey
}

func isEvacuatingActualLRPNode(node *etcd.Node) bool {
	return path.Base(node.Key) == ActualLRPEvacuatingKey
}