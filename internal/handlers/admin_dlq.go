package handlers

import (
	"net/http"

	"go.uber.org/zap"
)

type DLQViewerHandler struct {
	reader MsgDLQReader
	logger *zap.Logger
}

func NewDLQViewerHandler(reader MsgDLQReader, logger *zap.Logger) *DLQViewerHandler {
	return &DLQViewerHandler{reader: reader, logger: logger}
}

func (v *DLQViewerHandler) ListMessages(w http.ResponseWriter, r *http.Request) {

	list, err := v.reader.GetMessages(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "faild to fetch messages")
		return
	}

	writeJSON(w, http.StatusOK, list)

}
