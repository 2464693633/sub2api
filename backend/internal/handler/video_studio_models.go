package handler

import (
	"net/http"
	"regexp"
	"strings"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// videoStudioModelPattern 与视频工作台前端一致的视频模型识别规则。
var videoStudioModelPattern = regexp.MustCompile(`(?i)(video|imagine|seedance|veo|sora|kling)`)

// VideoStudioModels 聚合用户全部视频分组的视频模型,按分组标注返回
// (多个 grok/composite 视频分组同台),供前端按模型所属分组路由生成请求。
// apiKeyService 由 routes 闭包注入(每分组一把工作台密钥已在 relay 中确保)。
func (h *GatewayHandler) VideoStudioModels(c *gin.Context, apiKeyService *service.APIKeyService) {
	if apiKeyService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "api key service unavailable"}})
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"message": "User not authenticated"}})
		return
	}
	groups, err := apiKeyService.VideoStudioGroups(c.Request.Context(), subject.UserID)
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
		ids := h.gatewayService.GetAvailableModels(c.Request.Context(), &g.ID, g.Platform)
		for _, id := range ids {
			id = strings.TrimSpace(id)
			// 排除含 image 的模型名(如 grok-imagine-image-*),以及上游 v0.2.8 新增的
			// 裸 grok-imagine 图片模型别名,避免图片模型混入视频列表
			lower := strings.ToLower(id)
			if id == "" || seen[id] || strings.Contains(lower, "image") || lower == "grok-imagine" || !videoStudioModelPattern.MatchString(id) {
				continue
			}
			seen[id] = true
			out = append(out, modelItem{ID: id, GroupID: g.ID, GroupName: g.Name, Platform: string(g.Platform)})
		}
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": out})
}
