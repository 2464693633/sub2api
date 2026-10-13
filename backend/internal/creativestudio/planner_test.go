package creativestudio

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestProjectSettingsInheritanceAndFrozenJobInputs(t *testing.T) {
	s := testStore(t, unusedExecutor(), Options{})
	shared := card("shared", "image")
	shared.Data.ParamsMode = "project"
	legacy := card("legacy", "image")
	snap := saveGraph(t, s, 7, []Node{shared, legacy}, nil)
	snap.Project.Settings = &ProjectSettings{Subject: "white bag", Style: "soft daylight", ImageParams: json.RawMessage(`{"model":"gpt-image-2","group_id":20,"size":"1536x1024"}`)}
	snap, err := s.Save(7, snap.Project.ID, snap.Project)
	if err != nil {
		t.Fatal(err)
	}
	report, err := s.Preflight(7, snap.Project.ID, []string{"shared", "legacy"}, snap.Project.Revision)
	if err != nil || !report.Valid || report.Counts.Total != 2 {
		t.Fatalf("invalid preflight: %+v %v", report, err)
	}
	for _, node := range report.Nodes {
		if !strings.Contains(node.Prompt, "white bag") || !strings.Contains(node.Prompt, "soft daylight") {
			t.Fatalf("missing shared direction: %+v", node)
		}
		want := "example"
		if node.NodeID == "shared" {
			want = "gpt-image-2"
		}
		if !strings.Contains(string(node.Params), want) {
			t.Fatalf("incorrect inheritance: %+v", node)
		}
	}
	before, _ := s.Get(7, snap.Project.ID)
	if len(before.Jobs) != 0 || before.Project.Revision != snap.Project.Revision {
		t.Fatal("preflight mutated project")
	}
	run, err := s.RunAtRevision(7, snap.Project.ID, "first", []string{"shared", "legacy"}, snap.Project.Revision)
	if err != nil {
		t.Fatal(err)
	}
	run.Project.Settings.Subject = "changed subject"
	run.Project.Settings.ImageParams = json.RawMessage(`{"model":"new-model","group_id":50}`)
	saved, err := s.Save(7, snap.Project.ID, run.Project)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(saved.Jobs[0].Prompt, "changed subject") || strings.Contains(string(saved.Jobs[0].Params), "new-model") {
		t.Fatal("running job parameters changed with project settings")
	}
	// Idempotent replay still returns the original run after newer settings exist.
	replay, err := s.RunAtRevision(7, snap.Project.ID, "first", []string{"shared", "legacy"}, snap.Project.Revision)
	if err != nil || len(replay.Jobs) != 2 {
		t.Fatalf("replay duplicated jobs: %+v %v", replay.Jobs, err)
	}
}

func TestPreflightUsesPinnedVersionsAndCountsDependencies(t *testing.T) {
	s := testStore(t, unusedExecutor(), Options{})
	snap := saveGraph(t, s, 7, []Node{card("text", "text"), card("overview", "image"), card("detail", "image"), card("video", "video")}, []Edge{{ID: "a", Source: "text", Target: "overview"}, {ID: "b", Source: "overview", Target: "detail"}, {ID: "c", Source: "detail", Target: "video"}})
	report, err := s.Preflight(7, snap.Project.ID, []string{"video"}, snap.Project.Revision)
	if err != nil || !report.Valid || report.Counts.Image != 2 || report.Counts.Video != 1 {
		t.Fatalf("dependency counts: %+v %v", report, err)
	}
	if report.Nodes[2].Inputs[0].Kind != "job" || report.Nodes[2].Inputs[0].NodeID != "detail" {
		t.Fatalf("missing planned reference: %+v", report.Nodes)
	}
	asset, err := s.PutAsset(7, snap.Project.ID, "chosen.png", "image/png", pngData(t))
	if err != nil {
		t.Fatal(err)
	}
	snap.Project.Nodes[2].Data.AssetID = asset.ID
	snap, err = s.Save(7, snap.Project.ID, snap.Project)
	if err != nil {
		t.Fatal(err)
	}
	report, err = s.Preflight(7, snap.Project.ID, []string{"video"}, snap.Project.Revision)
	if err != nil || !report.Valid || report.Counts.Total != 1 || report.Nodes[0].Inputs[0].AssetID != asset.ID {
		t.Fatalf("chosen version not pinned: %+v %v", report, err)
	}
	run, err := s.RunAtRevision(7, snap.Project.ID, "pin", []string{"video"}, snap.Project.Revision)
	if err != nil || len(run.Jobs) != 1 || run.Jobs[0].Inputs[0].AssetID != asset.ID {
		t.Fatalf("run drifted from preflight: %+v %v", run.Jobs, err)
	}
}

func TestPreflightReportsAllInvalidCardsAndRunRemainsAtomic(t *testing.T) {
	s := testStore(t, unusedExecutor(), Options{})
	one := card("one", "image")
	one.Data.ParamsMode = "project"
	two := card("two", "video")
	two.Data.Params = json.RawMessage(`{"model":"grok-imagine-video","group_id":1,"seconds":99}`)
	snap := saveGraph(t, s, 7, []Node{one, two}, nil)
	report, err := s.Preflight(7, snap.Project.ID, []string{"one", "two"}, snap.Project.Revision)
	if err != nil || report.Valid || len(report.Errors) != 2 {
		t.Fatalf("missing independent errors: %+v %v", report, err)
	}
	if _, err = s.RunAtRevision(7, snap.Project.ID, "bad", []string{"one", "two"}, snap.Project.Revision); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid run accepted: %v", err)
	}
	after, _ := s.Get(7, snap.Project.ID)
	if len(after.Jobs) != 0 {
		t.Fatal("invalid run created partial work")
	}
	if _, err = s.Preflight(7, snap.Project.ID, []string{"one"}, snap.Project.Revision+1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale preview accepted: %v", err)
	}
	if _, err = s.Preflight(8, snap.Project.ID, []string{"one"}, snap.Project.Revision); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other owner previewed project: %v", err)
	}
}

func TestPreferredVersionAndSettingsValidation(t *testing.T) {
	s := testStore(t, unusedExecutor(), Options{})
	snap := saveGraph(t, s, 7, []Node{card("image", "image")}, nil)
	snap.Project.Nodes[0].Data.PreferredAssetID = "foreign"
	if _, err := s.Save(7, snap.Project.ID, snap.Project); !errors.Is(err, ErrInvalid) {
		t.Fatalf("foreign preferred asset accepted: %v", err)
	}
	asset, err := s.PutAsset(7, snap.Project.ID, "chosen.png", "image/png", pngData(t))
	if err != nil {
		t.Fatal(err)
	}
	snap.Project.Nodes[0].Data.PreferredAssetID = asset.ID
	snap, err = s.Save(7, snap.Project.ID, snap.Project)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Project.Nodes[0].Data.PreferredAssetID != asset.ID {
		t.Fatal("favorite lost")
	}
	snap.Project.Settings = &ProjectSettings{ImageParams: json.RawMessage(`[]`)}
	if _, err = s.Save(7, snap.Project.ID, snap.Project); !errors.Is(err, ErrInvalid) {
		t.Fatalf("non-object parameters accepted: %v", err)
	}
}

func TestModelCapabilitiesPreserveUnknownParameters(t *testing.T) {
	raw := json.RawMessage(`{"model":"custom-image","group_id":1,"size":"4096x1234","quality":"vendor-quality","format":"vendor-format"}`)
	if err := ValidateGenerationParams("image", raw, 8); err != nil {
		t.Fatal(err)
	}
	if err := ValidateGenerationParams("image", raw, 9); !errors.Is(err, ErrInvalid) {
		t.Fatal("reference cap missing")
	}
	dall := json.RawMessage(`{"model":"dall-e-3","group_id":1,"size":"1024x1024","quality":"hd"}`)
	if err := ValidateGenerationParams("image", dall, 1); !errors.Is(err, ErrInvalid) {
		t.Fatal("unsupported edit accepted")
	}
	if err := ValidateGenerationParams("image", json.RawMessage(`{"model":"dall-e-3","group_id":1,"quality":"high"}`), 0); !errors.Is(err, ErrInvalid) {
		t.Fatal("wrong DALL-E quality accepted")
	}
	grok := ModelCapabilities("image", "grok-imagine-image-2.0", "grok")
	if grok.CustomFormat || grok.CustomQuality || len(grok.Sizes) == 0 {
		t.Fatalf("wrong Grok controls: %+v", grok)
	}
	if caps := ModelCapabilities("video", "sora-2", "composite"); caps.Profile != "compatible-video" || !caps.CustomSeconds {
		t.Fatalf("invented native Sora capability: %+v", caps)
	}
	for input, want := range map[string]string{"1280x720": "16:9", "720x1280": "9:16", "720x720": "1:1", "auto": "", "16:9": "16:9"} {
		if got := VideoAspectRatio(input); got != want {
			t.Fatalf("%s→%s, want %s", input, got, want)
		}
	}
}

func TestSavedTimelineCoverMustBeOwnedImage(t *testing.T) {
	s := testStore(t, unusedExecutor(), Options{})
	snap := saveGraph(t, s, 7, []Node{card("image", "image")}, nil)
	snap.Project.Timeline = json.RawMessage(`{"cover_asset_id":"foreign"}`)
	if _, err := s.Save(7, snap.Project.ID, snap.Project); !errors.Is(err, ErrInvalid) {
		t.Fatalf("foreign cover accepted: %v", err)
	}
	asset, err := s.PutAsset(7, snap.Project.ID, "cover.png", "image/png", pngData(t))
	if err != nil {
		t.Fatal(err)
	}
	snap.Project.Timeline = json.RawMessage(`{"cover_asset_id":"` + asset.ID + `"}`)
	if _, err = s.Save(7, snap.Project.ID, snap.Project); err != nil {
		t.Fatal(err)
	}
}

func TestRunAuthorizationValidatesWholePlanBeforeCreatingJobs(t *testing.T) {
	s := testStore(t, unusedExecutor(), Options{})
	snap := saveGraph(t, s, 7, []Node{card("image", "image"), card("video", "video")}, []Edge{{ID: "edge", Source: "image", Target: "video"}})
	checked := false
	denyVideo := func(nodes []PreflightNode) error {
		checked = true
		if len(nodes) != 2 || nodes[0].NodeID != "image" || nodes[1].NodeID != "video" {
			t.Fatalf("validator omitted prerequisite: %+v", nodes)
		}
		return ErrInvalid
	}
	if _, err := s.RunAtRevisionValidated(7, snap.Project.ID, "same-request", []string{"video"}, snap.Project.Revision, denyVideo); !errors.Is(err, ErrInvalid) {
		t.Fatalf("denied branch accepted: %v", err)
	}
	if !checked {
		t.Fatal("authorization was not checked")
	}
	after, _ := s.Get(7, snap.Project.ID)
	if len(after.Jobs) != 0 {
		t.Fatal("allowed prerequisite billed before downstream permission check")
	}
	run, err := s.RunAtRevisionValidated(7, snap.Project.ID, "same-request", []string{"video"}, snap.Project.Revision, func([]PreflightNode) error { return nil })
	if err != nil || len(run.Jobs) != 2 {
		t.Fatalf("valid retry failed: %+v %v", run.Jobs, err)
	}
	run.Project.Name = "New revision"
	if _, err = s.Save(7, snap.Project.ID, run.Project); err != nil {
		t.Fatal(err)
	}
	checked = false
	replay, err := s.RunAtRevisionValidated(7, snap.Project.ID, "same-request", []string{"video"}, snap.Project.Revision, denyVideo)
	if err != nil || len(replay.Jobs) != 2 || checked {
		t.Fatalf("idempotent replay was subject to newer graph/permissions: %+v %v checked=%v", replay.Jobs, err, checked)
	}
}
