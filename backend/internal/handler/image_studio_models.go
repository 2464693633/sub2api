package handler

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// imageStudioModelPattern 与生图工作台前端一致的生图模型识别规则。
var imageStudioModelPattern = regexp.MustCompile(`(?i)(image|dall|flux|seedream|banana|diffusion)`)

func isImageStudioModel(id, platform string) bool {
	return imageStudioModelPattern.MatchString(id) ||
		(platform == service.PlatformGrok && xai.IsGrokImagineImageModel(id))
}

// ImageStudioModels 聚合用户全部生图分组的生图模型,按分组标注返回
// (gpt/grok 生图模型同台),供前端按模型所属分组路由生成请求。
// apiKeyService 由 routes 闭包注入(每分组一把工作台密钥已在 relay 中确保)。
func (h *GatewayHandler) ImageStudioModels(c *gin.Context, apiKeyService *service.APIKeyService) {
	if apiKeyService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "api key service unavailable"}})
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"message": "User not authenticated"}})
		return
	}
	groups, err := apiKeyService.ImageStudioGroups(c.Request.Context(), subject.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}

	type modelItem struct {
		ID        string `json:"id"`
		GroupID   int64  `json:"group_id"`
		GroupName string `json:"group_name"`
		Platform  string `json:"platform"`
	}
	out := make([]modelItem, 0)
	seen := map[string]bool{}
	for i := range groups {
		g := &groups[i]
		ids := h.studioAvailableModels(c.Request.Context(), g)
		if g.ModelAllowlistEnabled() {
			ids = g.ModelAllowlist.FilterForListing(ids)
		}
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] || !isImageStudioModel(id, g.Platform) {
				continue
			}
			seen[id] = true
			out = append(out, modelItem{ID: id, GroupID: g.ID, GroupName: g.Name, Platform: string(g.Platform)})
		}
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": out})
}
