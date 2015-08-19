package handlers

import (
	"net/http"

	"github.com/cloudfoundry-incubator/bbs/db"
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/gogo/protobuf/proto"
	"github.com/pivotal-golang/lager"
)

type EvacuationHandler struct {
	db     db.EvacuationDB
	logger lager.Logger
}

func NewEvacuationHandler(logger lager.Logger, db db.EvacuationDB) *EvacuationHandler {
	return &EvacuationHandler{
		db:     db,
		logger: logger.Session("evacuation-handler"),
	}
}

type MessageValidator interface {
	proto.Message
	Validate() error
	Unmarshal(data []byte) error
}

func (h *EvacuationHandler) RemoveEvacuatingActualLRP(w http.ResponseWriter, req *http.Request) {
	logger := h.logger.Session("remove-evacuating-actual-lrp")

	request := &models.RemoveEvacuatingActualLRPRequest{}
	response := &models.RemoveEvacuatingActualLRPResponse{}

	response.Error = parseRequest(logger, req, request)
	if response.Error == nil {
		response.Error = h.db.RemoveEvacuatingActualLRP(logger, request.ActualLrpKey, request.ActualLrpInstanceKey)
	}

	writeResponse(w, response)
}

func (h *EvacuationHandler) EvacuateClaimedActualLRP(w http.ResponseWriter, req *http.Request) {
	logger := h.logger.Session("evacuate-claimed-actual-lrp")

	request := &models.EvacuateClaimedActualLRPRequest{}
	if !parseRequestAndWrite(logger, w, req, request) {
		return
	}

	keepContainer, bbsErr := h.db.EvacuateClaimedActualLRP(logger, request.ActualLrpKey, request.ActualLrpInstanceKey)

	writeProtoResponse(w, http.StatusOK, &models.EvacuationResponse{
		KeepContainer: keepContainer,
		Error:         bbsErr,
	})
}

func (h *EvacuationHandler) EvacuateCrashedActualLRP(w http.ResponseWriter, req *http.Request) {
	logger := h.logger.Session("evacuate-crashed-actual-lrp")

	request := &models.EvacuateCrashedActualLRPRequest{}
	if !parseRequestAndWrite(logger, w, req, request) {
		return
	}

	keepContainer, bbsErr := h.db.EvacuateCrashedActualLRP(logger, request.ActualLrpKey, request.ActualLrpInstanceKey, request.ErrorMessage)

	writeProtoResponse(w, http.StatusOK, &models.EvacuationResponse{
		KeepContainer: keepContainer,
		Error:         bbsErr,
	})
}

func (h *EvacuationHandler) EvacuateRunningActualLRP(w http.ResponseWriter, req *http.Request) {
	logger := h.logger.Session("evacuate-running-actual-lrp")

	request := &models.EvacuateRunningActualLRPRequest{}
	if !parseRequestAndWrite(logger, w, req, request) {
		return
	}

	keepContainer, bbsErr := h.db.EvacuateRunningActualLRP(logger, request.ActualLrpKey, request.ActualLrpInstanceKey, request.ActualLrpNetInfo, request.Ttl)

	writeProtoResponse(w, http.StatusOK, &models.EvacuationResponse{
		KeepContainer: keepContainer,
		Error:         bbsErr,
	})
}

func (h *EvacuationHandler) EvacuateStoppedActualLRP(w http.ResponseWriter, req *http.Request) {
	logger := h.logger.Session("evacuate-stopped-actual-lrp")

	request := &models.EvacuateStoppedActualLRPRequest{}
	if !parseRequestAndWrite(logger, w, req, request) {
		return
	}

	keepContainer, bbsErr := h.db.EvacuateStoppedActualLRP(logger, request.ActualLrpKey, request.ActualLrpInstanceKey)

	writeProtoResponse(w, http.StatusOK, &models.EvacuationResponse{
		KeepContainer: keepContainer,
		Error:         bbsErr,
	})
}
