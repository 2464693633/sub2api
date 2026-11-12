package handler

import (
	"io"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *OpenAIGatewayHandler) KlingStudioGeneration(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Unable to read request body")
		return
	}
	endpoint, payload, model, err := service.BuildKlingStudioRequest(body)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	h.handleKlingResponse(c, endpoint, "", payload, model, "studio")
}

func (h *OpenAIGatewayHandler) KlingStudioStatus(c *gin.Context) {
	h.klingStudioQuery(c, "studio")
}

func (h *OpenAIGatewayHandler) KlingStudioContent(c *gin.Context) {
	h.klingStudioQuery(c, "content")
}

func (h *OpenAIGatewayHandler) klingStudioQuery(c *gin.Context, responseMode string) {
	endpoint, taskID, err := service.ParseKlingStudioTaskID(c.Param("request_id"))
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	h.handleKlingResponse(c, endpoint, taskID, nil, "", responseMode)
}
