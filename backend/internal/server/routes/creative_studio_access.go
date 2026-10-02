package routes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/creativestudio"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type creativeStudioGroupAccess map[string]map[int64]bool

// These read-only methods share the gateway studio permission/platform filters;
// they do not allocate managed API keys or make provider/model-discovery calls.
func loadCreativeStudioGroups(ctx context.Context, apiKeys *service.APIKeyService, userID int64) (creativeStudioGroupAccess, error) {
	images, err := apiKeys.ImageStudioGroups(ctx, userID)
	if err != nil && !errors.Is(err, service.ErrImageStudioNoImageGroup) {
		return nil, err
	}
	videos, err := apiKeys.VideoStudioGroups(ctx, userID)
	if err != nil && !errors.Is(err, service.ErrVideoStudioNoVideoGroup) {
		return nil, err
	}
	access := creativeStudioGroupAccess{"image": {}, "video": {}}
	for _, group := range images {
		access["image"][group.ID] = true
	}
	for _, group := range videos {
		access["video"][group.ID] = true
	}
	return access, nil
}

func (access creativeStudioGroupAccess) issues(nodes []creativestudio.PreflightNode) []creativestudio.PreflightError {
	issues := make([]creativestudio.PreflightError, 0)
	for _, node := range nodes {
		var params struct {
			GroupID int64 `json:"group_id"`
		}
		// Invalid/missing parameter errors are already reported by the shared planner.
		if json.Unmarshal(node.Params, &params) != nil || params.GroupID <= 0 {
			continue
		}
		if !access[node.Kind][params.GroupID] {
			issues = append(issues, creativestudio.PreflightError{NodeID: node.NodeID, Code: "group_unavailable", Message: fmt.Sprintf("卡片 %q 的生成分组已不可用，请重新选择 / The generation group for this card is unavailable; choose another group", node.Title)})
		}
	}
	return issues
}

func (access creativeStudioGroupAccess) validate(nodes []creativestudio.PreflightNode) error {
	if issues := access.issues(nodes); len(issues) > 0 {
		return fmt.Errorf("%w: %s", creativestudio.ErrInvalid, issues[0].Message)
	}
	return nil
}
