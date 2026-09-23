package admin

import (
	"sort"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/plugin"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/gin-gonic/gin"
)

type moduleStatusSource interface {
	Snapshot() []plugin.ModuleStatus
}

// ModuleHandler exposes read-only runtime module state to administrators.
type ModuleHandler struct{ runtime moduleStatusSource }

func NewModuleHandler(runtime *plugin.Runtime) *ModuleHandler {
	return &ModuleHandler{runtime: runtime}
}

type ModuleStatusItem struct {
	ID        string `json:"id"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	State     string `json:"state"`
	Error     string `json:"error"`
}

func (h *ModuleHandler) List(c *gin.Context) {
	statuses := h.runtime.Snapshot()
	modules := make([]ModuleStatusItem, 0, len(statuses))
	for _, status := range statuses {
		errText := logredact.RedactText(status.Err)
		modules = append(modules, ModuleStatusItem{
			ID: string(status.ID), Namespace: status.ID.Namespace(), Name: status.ID.Name(),
			Enabled: status.Enabled, State: string(status.State), Error: errText,
		})
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].ID < modules[j].ID })
	response.Success(c, gin.H{"modules": modules})
}
