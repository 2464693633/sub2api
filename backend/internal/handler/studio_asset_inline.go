package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 生图工作台的内联取图缓冲层。
//
// xAI 生图返回的是 imgen.x.ai 临时图片 URL,浏览器因 CORS 无法直接下载,代理又
// 会被 CDN 间歇性拦截。此缓冲层装在 BillableUsageResponse 计费 writer 之外:
// target 写出的 JSON 先落入缓冲,完成后把其中每个图片 URL 下载并改写为
// b64_json(与 OpenAI 响应同构),前端从此不再接触 imgen.x.ai。
// 计费与用量记录基于上游解析结果(result.ImageCount),不读响应体,不受改写影响;
// 任一图片下载失败时原样透传缓冲内容,前端仍走代理兜底。

const studioAssetBufferLimit = 32 << 20

// StudioAssetBuffer 拦截 gin 响应写入,延迟落盘。
type StudioAssetBuffer struct {
	gin.ResponseWriter
	buf        bytes.Buffer
	status     int
	passthrough bool
}

// InstallStudioAssetBuffer 用缓冲 writer 替换 c.Writer,返回缓冲供 Finalize 使用。
func InstallStudioAssetBuffer(c *gin.Context) *StudioAssetBuffer {
	buf := &StudioAssetBuffer{ResponseWriter: c.Writer}
	c.Writer = buf
	return buf
}

// FinalizeStudioAssetInline 恢复原 writer,必要时内联图片后写出最终响应。
func FinalizeStudioAssetInline(c *gin.Context, buf *StudioAssetBuffer) {
	c.Writer = buf.ResponseWriter
	if buf.passthrough || buf.buf.Len() == 0 {
		return
	}
	body := buf.buf.Bytes()
	status := buf.status
	if status == 0 {
		status = http.StatusOK
	}

	if final, changed := inlineGrokImageAssets(c, body); changed {
		body = final
		buf.ResponseWriter.Header().Del("Content-Length")
	}

	header := buf.ResponseWriter.Header()
	if header.Get("Content-Type") == "" {
		header.Set("Content-Type", "application/json")
	}
	header.Del("Content-Length")
	buf.ResponseWriter.WriteHeader(status)
	_, _ = buf.ResponseWriter.Write(body)
}

// Size 供网关 failover 逻辑比较写入量:缓冲层字节 + 底层 writer 字节。
func (b *StudioAssetBuffer) Size() int {
	return b.buf.Len() + b.ResponseWriter.Size()
}

// Status 返回缓冲的状态码(未显式设置时回退底层)。
func (b *StudioAssetBuffer) Status() int {
	if b.status != 0 {
		return b.status
	}
	return b.ResponseWriter.Status()
}

// Written 底层未写则视为未提交,错误路径仍可正常改写响应。
func (b *StudioAssetBuffer) Written() bool {
	return b.ResponseWriter.Written()
}

// Flush 图片响应为一次性 JSON,无需推进;保留空实现避免 SSE flush 直写底层。
func (b *StudioAssetBuffer) Flush() {}

func (b *StudioAssetBuffer) Write(data []byte) (int, error) {
	if b.passthrough {
		return b.ResponseWriter.Write(data)
	}
	if b.buf.Len()+len(data) > studioAssetBufferLimit {
		// 超限保护:先把已缓冲内容按原序写出,再切换纯透传,放弃内联改写。
		// (直接写底层而留旧缓冲会造成 finalize 时前后段乱序——Codex 审核 P1)
		if _, err := b.ResponseWriter.Write(b.buf.Bytes()); err != nil {
			return 0, err
		}
		b.buf.Reset()
		b.passthrough = true
		return b.ResponseWriter.Write(data)
	}
	return b.buf.Write(data)
}

func (b *StudioAssetBuffer) WriteString(data string) (int, error) {
	return b.Write([]byte(data))
}

func (b *StudioAssetBuffer) WriteHeader(code int) {
	if b.status == 0 {
		b.status = code
	}
}

// inlineGrokImageAssets 识别工作台生图成功响应中的 xAI 图片 URL,
// 下载并改写为 b64_json;任何失败都原样返回(changed=false),前端走代理兜底。
// 用 map 泛型操作保留上游响应的全部未知字段。
func inlineGrokImageAssets(c *gin.Context, body []byte) ([]byte, bool) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' || !bytes.Contains(body, []byte("imgen.")) {
		return body, false
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return body, false
	}
	dataArr, ok := root["data"].([]any)
	if !ok || len(dataArr) == 0 {
		return body, false
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()
	changed := false
	for _, item := range dataArr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		u, _ := m["url"].(string)
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if _, hasB64 := m["b64_json"]; hasB64 {
			continue
		}
		if _, allowed := studioAssetHostAllowed(u); !allowed {
			continue
		}
		data, err := fetchStudioAssetBytes(ctx, u)
		if err != nil {
			return body, false
		}
		delete(m, "url")
		m["b64_json"] = base64.StdEncoding.EncodeToString(data)
		changed = true
	}
	if !changed {
		return body, false
	}
	out, err := json.Marshal(root)
	if err != nil {
		return body, false
	}
	return out, true
}
