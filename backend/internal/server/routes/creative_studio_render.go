package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/creativestudio"
)

type creativeClip struct {
	ID       string  `json:"id"`
	AssetID  string  `json:"asset_id"`
	Title    string  `json:"title"`
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
}
type creativeComposition struct {
	RequestID    string         `json:"request_id"`
	Clips        []creativeClip `json:"clips"`
	Size         string         `json:"size"`
	AudioAssetID string         `json:"audio_asset_id,omitempty"`
}

func (c creativeComposition) validate() ([]string, error) {
	if len(c.Clips) < 1 || len(c.Clips) > 12 {
		return nil, errors.New("时间线需要 1–12 个镜头 / Timeline requires 1–12 shots")
	}
	if c.Size != "1280x720" && c.Size != "720x1280" && c.Size != "720x720" {
		return nil, errors.New("unsupported export dimensions")
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(c.Clips)+1)
	total := 0.0
	for _, clip := range c.Clips {
		if clip.AssetID == "" || len(clip.Title) > 1200 || math.IsNaN(clip.Start) || math.IsInf(clip.Start, 0) || clip.Start < 0 || clip.Start > 3600 || math.IsNaN(clip.Duration) || math.IsInf(clip.Duration, 0) || clip.Duration < 1 || clip.Duration > 30 {
			return nil, errors.New("镜头时长需为 1–30 秒，起点不能为负 / Shot durations must be 1–30 seconds with nonnegative start times")
		}
		total += clip.Duration
		if !seen[clip.AssetID] {
			ids = append(ids, clip.AssetID)
			seen[clip.AssetID] = true
		}
	}
	if total > 90 {
		return nil, errors.New("成片总时长不能超过 90 秒 / Total duration cannot exceed 90 seconds")
	}
	if c.AudioAssetID != "" && !seen[c.AudioAssetID] {
		ids = append(ids, c.AudioAssetID)
	}
	return ids, nil
}

type creativeProbe struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	} `json:"streams"`
}

// All ffmpeg inputs are owned media files resolved by the store. Commands use
// fixed executable names and argv, never a shell, URL or user-provided filter.
func creativeMediaCommand(ctx context.Context, executable string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, exec.ErrNotFound) {
			return nil, errors.New("服务器未安装视频合成工具 / Video rendering tools are not installed")
		}
		// Provider assets may contain metadata; keep command output in server logs,
		// not in the user's job record or HTTP response.
		return nil, errors.New("媒体处理失败，请检查素材是否可播放 / Media processing failed; check the source media")
	}
	return output.Bytes(), nil
}

func creativeProbeFile(ctx context.Context, path string) (creativeProbe, error) {
	data, err := creativeMediaCommand(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file,pipe", "-show_entries", "format=duration:stream=codec_type,width,height", "-of", "json", path)
	var result creativeProbe
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(data, &result)
	return result, err
}

func (e *creativeStudioExecutor) render(ctx context.Context, job creativestudio.Job, inputs []creativestudio.AssetFile) (creativestudio.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	select {
	case e.renders <- struct{}{}:
		defer func() { <-e.renders }()
	case <-ctx.Done():
		return creativestudio.Result{}, ctx.Err()
	}
	var composition creativeComposition
	if err := json.Unmarshal(job.Params, &composition); err != nil {
		return creativestudio.Result{}, err
	}
	if _, err := composition.validate(); err != nil {
		return creativestudio.Result{}, err
	}
	assets := make(map[string]creativestudio.AssetFile, len(inputs))
	for _, asset := range inputs {
		assets[asset.ID] = asset
	}
	dir, err := os.MkdirTemp("", "sub2api-studio-render-")
	if err != nil {
		return creativestudio.Result{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	dimensions := strings.Split(composition.Size, "x")
	width, height := dimensions[0], dimensions[1]
	var concat strings.Builder
	total := 0.0
	for index, clip := range composition.Clips {
		asset, ok := assets[clip.AssetID]
		if !ok || (asset.Kind != "image" && asset.Kind != "video") {
			return creativestudio.Result{}, errors.New("镜头素材不存在 / Shot media is missing")
		}
		args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-threads", "1", "-filter_threads", "1", "-protocol_whitelist", "file,pipe"}
		hasAudio := false
		probe, err := creativeProbeFile(ctx, asset.Path)
		if err != nil {
			return creativestudio.Result{}, err
		}
		hasVideo := false
		for _, stream := range probe.Streams {
			if stream.CodecType == "video" {
				if stream.Width <= 0 || stream.Height <= 0 || stream.Width > 8192 || stream.Height > 8192 || int64(stream.Width)*int64(stream.Height) > 32_000_000 {
					return creativestudio.Result{}, errors.New("素材分辨率过大，单边需不超过 8192 像素且总像素不超过 3200 万 / Source exceeds the 8192-pixel or 32-megapixel rendering limit")
				}
				hasVideo = true
			}
			if stream.CodecType == "audio" {
				hasAudio = true
			}
		}
		if !hasVideo {
			return creativestudio.Result{}, errors.New("素材缺少可用画面 / Source has no video or image stream")
		}
		if asset.Kind == "image" {
			hasAudio = false
			args = append(args, "-loop", "1", "-i", asset.Path)
		} else {
			length, err := strconv.ParseFloat(probe.Format.Duration, 64)
			if err != nil || math.IsNaN(length) || math.IsInf(length, 0) || length <= 0 || clip.Start+clip.Duration > length+0.15 {
				return creativestudio.Result{}, errors.New("镜头选取范围超出原视频时长 / Shot extends beyond the source video's duration")
			}
			args = append(args, "-ss", strconv.FormatFloat(clip.Start, 'f', 3, 64), "-i", asset.Path)
		}
		if !hasAudio {
			args = append(args, "-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo")
		}
		filter := fmt.Sprintf("scale=%s:%s:force_original_aspect_ratio=decrease,pad=%s:%s:(ow-iw)/2:(oh-ih)/2:color=black,setsar=1,fps=30,format=yuv420p", width, height, width, height)
		if strings.TrimSpace(clip.Title) != "" {
			captionFile := filepath.Join(dir, fmt.Sprintf("caption-%02d.txt", index))
			if err := os.WriteFile(captionFile, []byte(creativeWrapCaption(clip.Title)), 0600); err != nil {
				return creativestudio.Result{}, err
			}
			font := creativeCaptionFont()
			if font == "" {
				return creativestudio.Result{}, errors.New("服务器缺少字幕字体 / No caption font is installed")
			}
			// Text lives in a file and expansion is disabled, so it is never parsed as
			// filter syntax or ffmpeg expressions. Both paths are generated/trusted.
			filter += fmt.Sprintf(",drawtext=fontfile='%s':textfile='%s':expansion=none:fontcolor=white:fontsize=28:line_spacing=8:x=(w-text_w)/2:y=h-text_h-32:box=1:boxcolor=black@0.55:boxborderw=10", font, captionFile)
		}
		name := fmt.Sprintf("clip-%02d.mp4", index)
		output := filepath.Join(dir, name)
		args = append(args, "-map", "0:v:0")
		if hasAudio {
			args = append(args, "-map", "0:a:0")
		} else {
			args = append(args, "-map", "1:a:0")
		}
		args = append(args, "-vf", filter, "-af", "apad", "-t", strconv.FormatFloat(clip.Duration, 'f', 3, 64), "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-maxrate", "8M", "-bufsize", "16M", "-threads", "1", "-c:a", "aac", "-ar", "48000", "-ac", "2", "-movflags", "+faststart", output)
		if _, err := creativeMediaCommand(ctx, "ffmpeg", args...); err != nil {
			return creativestudio.Result{}, err
		}
		fmt.Fprintf(&concat, "file '%s'\n", name)
		total += clip.Duration
	}
	list := filepath.Join(dir, "clips.txt")
	if err := os.WriteFile(list, []byte(concat.String()), 0600); err != nil {
		return creativestudio.Result{}, err
	}
	output := filepath.Join(dir, "sequence.mp4")
	if _, err := creativeMediaCommand(ctx, "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-threads", "1", "-protocol_whitelist", "file,pipe", "-f", "concat", "-safe", "1", "-i", list, "-c", "copy", "-movflags", "+faststart", output); err != nil {
		return creativestudio.Result{}, err
	}
	if composition.AudioAssetID != "" {
		music, ok := assets[composition.AudioAssetID]
		if !ok || music.Kind != "audio" {
			return creativestudio.Result{}, errors.New("配乐素材不存在 / Soundtrack is missing")
		}
		final := filepath.Join(dir, "final.mp4")
		if _, err := creativeMediaCommand(ctx, "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-threads", "1", "-filter_complex_threads", "1", "-protocol_whitelist", "file,pipe", "-i", output, "-stream_loop", "-1", "-protocol_whitelist", "file,pipe", "-i", music.Path,
			"-filter_complex", "[1:a]volume=0.2[bg];[0:a][bg]amix=inputs=2:duration=first:normalize=0[mix]", "-map", "0:v:0", "-map", "[mix]", "-t", strconv.FormatFloat(total, 'f', 3, 64), "-c:v", "copy", "-c:a", "aac", "-ar", "48000", "-ac", "2", "-movflags", "+faststart", final); err != nil {
			return creativestudio.Result{}, err
		}
		output = final
	}
	f, err := os.Open(output)
	if err != nil {
		return creativestudio.Result{}, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, (128<<20)+1))
	if err != nil {
		return creativestudio.Result{}, err
	}
	if len(data) > 128<<20 {
		return creativestudio.Result{}, errors.New("导出结果超过 128 MB / Export exceeds 128 MB")
	}
	return creativestudio.Result{Data: data, MIME: "video/mp4", Name: "studio-" + job.ID + ".mp4"}, nil
}

func creativeCaptionFont() string {
	for _, path := range []string{"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc", "/usr/share/fonts/wqy-zenhei/wqy-zenhei.ttc", "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc", "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"} {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return ""
}
func creativeWrapCaption(text string) string {
	text = strings.ReplaceAll(text, "\r", "")
	chars := []rune(text)
	if len(chars) > 160 {
		chars = chars[:160]
	}
	var out strings.Builder
	line := 0
	for _, char := range chars {
		if char == '\n' {
			line = 0
		} else if line >= 24 {
			_ = out.WriteByte('\n')
			line = 0
		}
		_, _ = out.WriteRune(char)
		line++
	}
	return out.String()
}
