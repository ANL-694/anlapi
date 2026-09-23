package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type systemImageGenerationGroupRequest struct {
	GroupID int64 `json:"group_id"`
}

type systemImageGenerationGroupResponse struct {
	GroupID int64 `json:"group_id"`
}

// GetSystemImageGenerationGroup returns the selected public group id. Zero
// means the system-managed image-key feature is disabled.
func (h *SettingHandler) GetSystemImageGenerationGroup(c *gin.Context) {
	groupID, err := h.settingService.GetSystemImageGenerationGroupID(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, systemImageGenerationGroupResponse{GroupID: groupID})
}

// UpdateSystemImageGenerationGroup validates and stores the selected group.
// Sending group_id=0 disables automatic system image-key provisioning.
func (h *SettingHandler) UpdateSystemImageGenerationGroup(c *gin.Context) {
	var req systemImageGenerationGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, service.ErrSystemImageGenerationGroupInvalid.Error())
		return
	}
	if err := h.settingService.SetSystemImageGenerationGroupID(c.Request.Context(), req.GroupID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, systemImageGenerationGroupResponse{GroupID: req.GroupID})
}
