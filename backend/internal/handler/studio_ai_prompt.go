package handler

import (
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ErrStudioNoChatModel 用户没有配置对话模型的可用分组,无法使用 AI 提示词助手。
var ErrStudioNoChatModel = errors.New("没有可用的对话模型,请联系管理员为分组配置对话模型")

// AIPromptSystemMessage 按创作类型返回 AI 提示词助手的系统指令。
func AIPromptSystemMessage(kind string) string {
	if kind == "video" {
		return "你是顶级 AI 视频提示词专家。用户会给出一句视频创意,请把它扩写成一段高质量的视频生成提示词:包含主体与动作、场景环境、镜头语言(景别/运镜/角度)、光线氛围与整体风格。使用与用户输入一致的语言,40~120 字,直接输出提示词本身,不要任何解释、前缀、引号或列表。"
	}
	if kind == "svg" {
		return "你是 SVG 矢量图形专家。用户会给出画面创意,请直接在回复正文中输出一份完整的静态 SVG XML(从 <svg 开始到 </svg> 结束):精准呈现用户的创意,合理运用 path/rect/circle/ellipse/linearGradient 等元素与渐变、透明度;只允许纯静态矢量元素,不含 script 标签、事件属性、外部链接或 image/foreignObject 位图引用;根元素必须包含有效的 width/height 或 viewBox;不要输出任何解释文字、注释或代码块围栏,只输出 SVG 本身。使用与用户输入一致的语言填写 text 内容。"
	}
	return "你是顶级 AI 绘画提示词专家。用户会给出一句画面创意,请把它扩写成一段高质量的图像生成提示词:包含主体细节、构图与视角、环境与光线、艺术风格与质感。使用与用户输入一致的语言,40~120 字,直接输出提示词本身,不要任何解释、前缀、引号或列表。"
}

// StudioChatTarget 为 AI 提示词助手选择用户的对话分组与模型:
// 取第一个平台受支持且配置了非媒体(对话)模型的分组。
func (h *GatewayHandler) StudioChatTarget(c *gin.Context, apiKeyService *service.APIKeyService, userID int64) (*service.Group, string, error) {
	groups, err := apiKeyService.GetAvailableGroups(c.Request.Context(), userID)
	if err != nil {
		return nil, "", err
	}
	for i := range groups {
		g := &groups[i]
		switch g.Platform {
		case service.PlatformOpenAI, service.PlatformAnthropic, service.PlatformGrok, service.PlatformComposite:
		default:
			continue
		}
		ids := h.gatewayService.GetAvailableModels(c.Request.Context(), &g.ID, g.Platform)
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" || imageStudioModelPattern.MatchString(id) || videoStudioModelPattern.MatchString(id) {
				continue
			}
			return g, id, nil
		}
	}
	return nil, "", ErrStudioNoChatModel
}
