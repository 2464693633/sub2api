package creativestudio

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestVideoDurationLimitsRejectUnsupportedGenerations(t *testing.T) {
	for _, tc := range []struct {
		model string
		max   int
	}{
		{model: "grok-imagine-video", max: 15},
		{model: " GROK-IMAGINE-VIDEO ", max: 15},
		{model: "grok-imagine-video-1.5", max: 15},
		{model: "custom-provider-video", max: 30},
	} {
		t.Run(tc.model, func(t *testing.T) {
			caps := ModelCapabilities("video", tc.model, "grok")
			if caps.MaxSeconds != tc.max {
				t.Fatalf("advertised duration limit = %d, want %d", caps.MaxSeconds, tc.max)
			}
			for _, duration := range []int{0, 1, tc.max, tc.max + 1} {
				raw, err := json.Marshal(map[string]any{"model": tc.model, "group_id": 1, "platform": "compatible", "seconds": duration, "resolution": "480p"})
				if err != nil {
					t.Fatal(err)
				}
				err = ValidateGenerationParams("video", raw, 1)
				invalid := duration < 1 || duration > tc.max
				if invalid && !errors.Is(err, ErrInvalid) {
					t.Fatalf("duration %d accepted: %v", duration, err)
				}
				if !invalid && err != nil {
					t.Fatalf("duration %d rejected: %v", duration, err)
				}
			}
		})
	}
}

func TestOverlongGrokVideoDoesNotEnqueuePrerequisite(t *testing.T) {
	s := testStore(t, unusedExecutor(), Options{})
	video := card("video", "video")
	video.Data.Params = json.RawMessage(`{"model":"grok-imagine-video","group_id":1,"seconds":16,"resolution":"480p"}`)
	snap := saveGraph(t, s, 7, []Node{card("image", "image"), video}, []Edge{{ID: "reference", Source: "image", Target: "video"}})
	if _, err := s.RunAtRevision(7, snap.Project.ID, "duration-rejected", []string{"video"}, snap.Project.Revision); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overlong video request was not rejected: %v", err)
	}
	after, err := s.Get(7, snap.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Jobs) != 0 {
		t.Fatalf("unsupported video enqueued %d jobs, including a potentially billable prerequisite", len(after.Jobs))
	}
}
