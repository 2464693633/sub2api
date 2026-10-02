package routes

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/creativestudio"
)

func TestCreativeCompositionValidatesBoundsAndFreezesAssetIDs(t *testing.T) {
	valid := creativeComposition{Size: "1280x720", Clips: []creativeClip{{AssetID: "a", Duration: 3}, {AssetID: "a", Start: 1, Duration: 2}, {AssetID: "b", Duration: 4}}, AudioAssetID: "music"}
	ids, err := valid.validate()
	if err != nil || strings.Join(ids, ",") != "a,b,music" {
		t.Fatalf("incorrect asset selection: %+v %v", ids, err)
	}
	for name, change := range map[string]func(*creativeComposition){
		"empty":              func(c *creativeComposition) { c.Clips = nil },
		"unknown resolution": func(c *creativeComposition) { c.Size = "99999x99999" },
		"negative start":     func(c *creativeComposition) { c.Clips[0].Start = -1 },
		"too short":          func(c *creativeComposition) { c.Clips[0].Duration = .5 },
		"too long":           func(c *creativeComposition) { c.Clips[0].Duration = 31 },
		"not finite":         func(c *creativeComposition) { c.Clips[0].Duration = math.Inf(1) },
		"total limit": func(c *creativeComposition) {
			c.Clips = []creativeClip{{AssetID: "a", Duration: 30}, {AssetID: "b", Duration: 30}, {AssetID: "c", Duration: 30}, {AssetID: "d", Duration: 1}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.Clips = append([]creativeClip(nil), valid.Clips...)
			change(&candidate)
			if _, err := candidate.validate(); err == nil {
				t.Fatal("invalid composition accepted")
			}
		})
	}
}

func TestCreativeCaptionWrappingKeepsLiteralText(t *testing.T) {
	source := "100% %{eif:1+1} ' [literal] 中文"
	result := creativeWrapCaption(source)
	if strings.ReplaceAll(result, "\n", "") != source {
		t.Fatalf("caption content changed: %q", result)
	}
	long := creativeWrapCaption(strings.Repeat("图", 200))
	if len([]rune(strings.ReplaceAll(long, "\n", ""))) != 160 {
		t.Fatal("caption was not bounded")
	}
}

// This integration check exercises actual media commands when tools are installed;
// it uses local synthetic media and makes no provider calls.
func TestCreativeRenderImageVideoAndSoundtrack(t *testing.T) {
	for _, executable := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(executable); err != nil {
			t.Skip("media rendering tools unavailable")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "image.png")
	if err := os.WriteFile(imagePath, creativeTestPNG(t), 0600); err != nil {
		t.Fatal(err)
	}
	videoPath := filepath.Join(dir, "source.mp4")
	if _, err := creativeMediaCommand(ctx, "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=blue:s=32x32:r=30", "-t", "1.5", "-threads", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p", videoPath); err != nil {
		t.Fatal(err)
	}
	audioPath := filepath.Join(dir, "music.wav")
	if _, err := creativeMediaCommand(ctx, "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "1", "-threads", "1", audioPath); err != nil {
		t.Fatal(err)
	}
	composition := creativeComposition{Size: "720x720", Clips: []creativeClip{{AssetID: "image", Duration: 1}, {AssetID: "video", Start: .2, Duration: 1}}, AudioAssetID: "audio"}
	if creativeCaptionFont() != "" {
		composition.Clips[0].Title = "100% %{eif:1+1} ' [safe]"
	}
	params, err := json.Marshal(composition)
	if err != nil {
		t.Fatal(err)
	}
	e := &creativeStudioExecutor{renders: make(chan struct{}, 1)}
	result, err := e.render(ctx, creativestudio.Job{ID: "test", Params: params}, []creativestudio.AssetFile{
		{Asset: creativestudio.Asset{ID: "image", Kind: "image"}, Path: imagePath},
		{Asset: creativestudio.Asset{ID: "video", Kind: "video"}, Path: videoPath},
		{Asset: creativestudio.Asset{ID: "audio", Kind: "audio"}, Path: audioPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.MIME != "video/mp4" || len(result.Data) == 0 {
		t.Fatalf("empty render: %+v", result)
	}
	output := filepath.Join(dir, "render.mp4")
	if err := os.WriteFile(output, result.Data, 0600); err != nil {
		t.Fatal(err)
	}
	probe, err := creativeProbeFile(ctx, output)
	if err != nil {
		t.Fatal(err)
	}
	duration, err := strconv.ParseFloat(probe.Format.Duration, 64)
	if err != nil || duration < 1.9 || duration > 2.3 {
		t.Fatalf("incorrect output duration: %s", probe.Format.Duration)
	}
	video, audio := false, false
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" {
			video = stream.Width == 720 && stream.Height == 720
		}
		if stream.CodecType == "audio" {
			audio = true
		}
	}
	if !video || !audio {
		t.Fatalf("missing output streams: %+v", probe.Streams)
	}
}
