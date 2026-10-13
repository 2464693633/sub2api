package routes

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/creativestudio"
)

func TestCreativePlanChecksEveryGroupWithoutRejectingModelAliases(t *testing.T) {
	access := creativeStudioGroupAccess{"image": {2: true}, "video": {4: true}}
	nodes := []creativestudio.PreflightNode{
		{NodeID: "image", Title: "Overview", Kind: "image", Params: json.RawMessage(`{"model":"custom-alias","group_id":2}`)},
		{NodeID: "video", Title: "Motion", Kind: "video", Params: json.RawMessage(`{"model":"custom-video-alias","group_id":2}`)},
	}
	issues := access.issues(nodes)
	if len(issues) != 1 || issues[0].NodeID != "video" || issues[0].Code != "group_unavailable" {
		t.Fatalf("wrong authorization issues: %+v", issues)
	}
	if err := access.validate(nodes); !errors.Is(err, creativestudio.ErrInvalid) {
		t.Fatalf("unauthorized downstream accepted: %v", err)
	}
	nodes[1].Params = json.RawMessage(`{"model":"custom-video-alias","group_id":4}`)
	if err := access.validate(nodes); err != nil {
		t.Fatalf("authorized custom aliases rejected: %v", err)
	}
	// A removed group fails even when its numeric ID and model are otherwise valid.
	delete(access["video"], 4)
	if err := access.validate(nodes); err == nil {
		t.Fatal("removed group accepted")
	}
}
