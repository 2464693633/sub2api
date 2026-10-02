package creativestudio

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Suggestions describe the adapter's controls, not a guarantee for every mapped
// provider. Unknown models remain configurable and gateway authorization applies.
type Capabilities struct {
	Profile          string   `json:"profile"`
	MaxReferences    int      `json:"max_references"`
	Sizes            []string `json:"sizes"`
	Qualities        []string `json:"qualities"`
	Formats          []string `json:"formats"`
	Seconds          []int    `json:"seconds"`
	Resolutions      []string `json:"resolutions"`
	CustomSize       bool     `json:"custom_size"`
	CustomQuality    bool     `json:"custom_quality"`
	CustomFormat     bool     `json:"custom_format"`
	CustomSeconds    bool     `json:"custom_seconds"`
	CustomResolution bool     `json:"custom_resolution"`
	Notes            []string `json:"notes"`
}

func ModelCapabilities(kind, model, platform string) Capabilities {
	model = strings.ToLower(strings.TrimSpace(model))
	caps := Capabilities{Profile: "compatible", MaxReferences: 8, Sizes: []string{"auto", "1024x1024", "1536x1024", "1024x1536"}, Qualities: []string{"auto", "low", "medium", "high"}, Formats: []string{"png", "jpeg", "webp"}, Seconds: []int{}, Resolutions: []string{}, CustomSize: true, CustomQuality: true, CustomFormat: true, Notes: []string{"建议值以当前网关参数为准，上游模型映射可能影响支持范围 / Suggestions reflect the gateway; mapped providers may differ"}}
	if kind == "video" {
		caps.Profile = "compatible-video"
		caps.MaxReferences = 1
		caps.Sizes = []string{"auto", "1280x720", "720x1280", "720x720"}
		caps.Qualities = []string{}
		caps.Formats = []string{}
		caps.CustomQuality = false
		caps.CustomFormat = false
		caps.Seconds = []int{4, 6, 8, 10}
		caps.Resolutions = []string{"480p", "720p", "1080p"}
		caps.CustomSeconds = true
		caps.CustomResolution = true
		caps.Notes = []string{"当前视频网关使用 Grok 接口，工作台时长范围为 1–30 秒；未知模型由上游判断 / Video uses the Grok gateway; studio duration is 1–30 seconds; unknown models depend on the provider"}
		if platform == "grok" || strings.HasPrefix(model, "grok-") {
			caps.Profile = "grok-video"
		}
		return caps
	}
	if platform == "grok" || strings.HasPrefix(model, "grok-") {
		caps.Profile = "grok-image"
		caps.Qualities = []string{}
		caps.Formats = []string{}
		caps.CustomQuality = false
		caps.CustomFormat = false
		caps.Notes = []string{"尺寸会转换为 Grok 的画幅与 1K/2K 分辨率；质量由模型决定 / Dimensions map to Grok aspect ratio and 1K/2K resolution; quality is selected by model"}
	} else if strings.HasPrefix(model, "gpt-image-") {
		caps.Profile = "gpt-image"
		caps.Sizes = append(caps.Sizes, "2048x1152", "1152x2048", "2048x2048", "3840x2160", "2160x3840")
		if strings.HasPrefix(model, "gpt-image-2.5") {
			caps.Qualities = append(caps.Qualities, "xhigh")
		}
	} else if model == "dall-e-3" {
		caps.Profile = "dall-e-3"
		caps.MaxReferences = 0
		caps.Sizes = []string{"auto", "1024x1024", "1792x1024", "1024x1792"}
		caps.Qualities = []string{"auto", "standard", "hd"}
		caps.Formats = []string{}
		caps.CustomSize = false
		caps.CustomQuality = false
		caps.CustomFormat = false
	} else if model == "dall-e-2" {
		caps.Profile = "dall-e-2"
		caps.MaxReferences = 1
		caps.Sizes = []string{"auto", "256x256", "512x512", "1024x1024"}
		caps.Qualities = []string{}
		caps.Formats = []string{}
		caps.CustomSize = false
		caps.CustomQuality = false
		caps.CustomFormat = false
	}
	return caps
}

func ValidateGenerationParams(kind string, raw json.RawMessage, references int) error {
	if err := checkParams(raw); err != nil {
		return err
	}
	var params struct {
		Model      string `json:"model"`
		Platform   string `json:"platform"`
		Size       string `json:"size"`
		Quality    string `json:"quality"`
		Format     string `json:"format"`
		Seconds    int    `json:"seconds"`
		Resolution string `json:"resolution"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("%w: invalid generation parameters", ErrInvalid)
	}
	// Client platform cannot relax validation; execution resolves its authorized group.
	caps := ModelCapabilities(kind, params.Model, "")
	if references > caps.MaxReferences {
		return fmt.Errorf("%w: 此模型最多使用 %d 张参考图 / This model accepts at most %d reference images", ErrInvalid, caps.MaxReferences, caps.MaxReferences)
	}
	if len(params.Model) > 240 || len(params.Size) > 80 || len(params.Quality) > 80 || len(params.Format) > 80 || len(params.Resolution) > 80 {
		return fmt.Errorf("%w: generation parameter is too long", ErrInvalid)
	}
	allowed := func(value string, values []string) bool {
		if value == "" || value == "auto" {
			return true
		}
		for _, v := range values {
			if value == v {
				return true
			}
		}
		return false
	}
	if !caps.CustomSize && !allowed(params.Size, caps.Sizes) {
		return fmt.Errorf("%w: 模型不支持所选尺寸 / Unsupported model size", ErrInvalid)
	}
	if len(caps.Qualities) > 0 && !caps.CustomQuality && !allowed(params.Quality, caps.Qualities) {
		return fmt.Errorf("%w: 模型不支持所选质量 / Unsupported model quality", ErrInvalid)
	}
	if kind == "video" && (params.Seconds < 1 || params.Seconds > 30) {
		return fmt.Errorf("%w: 视频时长必须为 1–30 秒 / Video duration must be 1–30 seconds", ErrInvalid)
	}
	return nil
}

// VideoAspectRatio converts pixel dimensions into the upstream ratio field;
// resolution is independent. Unknown values remain available to mapped providers.
func VideoAspectRatio(size string) string {
	value := strings.TrimSpace(size)
	if value == "" || value == "auto" {
		return ""
	}
	parts := strings.Split(strings.ToLower(value), "x")
	if len(parts) != 2 {
		return value
	}
	width, e1 := strconv.Atoi(parts[0])
	height, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil || width <= 0 || height <= 0 {
		return value
	}
	a, b := width, height
	for b != 0 {
		a, b = b, a%b
	}
	return fmt.Sprintf("%d:%d", width/a, height/a)
}
