package handler

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"

	"github.com/gin-gonic/gin"
)

// studioAssetClient 供代理端点与内联取图共用的受控 HTTP 客户端:
// 仅访问受信图片 CDN,并在拨号层拒绝解析到回环/私网/链路本地地址的域名,防 SSRF。
// 禁用自动重定向:入口的域名白名单只校验初始 URL,跟随 30x 会绕过白名单
// 让后端代取任意公网资源(Codex 审核 P1)。
var studioAssetClient = &http.Client{
	Timeout: 90 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			hp, _, splitErr := net.SplitHostPort(addr)
			if splitErr != nil {
				return nil, splitErr
			}
			ips, lookupErr := net.LookupIP(hp)
			if lookupErr != nil {
				return nil, lookupErr
			}
			for _, ip := range ips {
				if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
					return nil, errors.New("resolved to forbidden address")
				}
			}
			dialer := &net.Dialer{Timeout: 10 * time.Second}
			return dialer.DialContext(ctx, network, addr)
		},
	},
}

// studioAssetMaxBytes 单个上游图片的最大字节数。
const studioAssetMaxBytes = 32 << 20

// studioAssetHostAllowed 仅允许 xAI 图片 CDN 域名。
func studioAssetHostAllowed(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", false
	}
	host := u.Hostname()
	if host != "imgen.x.ai" && !strings.HasSuffix(host, ".x.ai") {
		return "", false
	}
	return host, true
}

// fetchStudioAssetBytes 带 3 次退避重试地下载图片,返回字节与错误。
// CDN 偶发 403/404(临时 URL 未就绪或边缘反爬拦截),浏览器化请求头降低拦截概率。
func fetchStudioAssetBytes(ctx context.Context, raw string) ([]byte, error) {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
		req.Header.Set("Accept", "image/avif,image/webp,image/*,*/*;q=0.8")
		req.Header.Set("Referer", "https://x.ai/")
		resp, doErr := studioAssetClient.Do(req)
		if doErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		ct := resp.Header.Get("Content-Type")
		if resp.StatusCode == http.StatusOK && strings.HasPrefix(ct, "image/") {
			// 读 limit+1 以区分"恰好等于上限"与"超限截断":超限视为失败而非静默截断
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, studioAssetMaxBytes+1))
			_ = resp.Body.Close()
			if readErr != nil {
				continue
			}
			if len(data) > studioAssetMaxBytes {
				return nil, errors.New("upstream asset exceeds size limit")
			}
			return data, nil
		}
		_ = resp.Body.Close()
	}
	return nil, errors.New("upstream asset unavailable")
}

// ImageStudioAsset 代理下载上游生图服务返回的图片 URL(如 xAI 的 imgen.x.ai)。
// 浏览器直连这些 CDN 会被 CORS 拦截,改由后端代取。仅允许受信图片 CDN 域名,防 SSRF。
func (h *GatewayHandler) ImageStudioAsset(c *gin.Context) {
	raw := strings.TrimSpace(c.Query("u"))
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "missing u"})
		return
	}
	host, ok := studioAssetHostAllowed(raw)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid or disallowed url"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()
	data, err := fetchStudioAssetBytes(ctx, raw)
	if err != nil {
		logger.L().Warn("image studio asset proxy failed",
			zap.String("component", "handler.image_studio.asset"),
			zap.String("host", host),
			zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"message": "上游图片暂不可用,请点击重试"})
		return
	}
	ct := http.DetectContentType(data)
	if !strings.HasPrefix(ct, "image/") {
		c.JSON(http.StatusBadGateway, gin.H{"message": "上游返回的不是图片,请点击重试"})
		return
	}
	c.Header("Content-Type", ct)
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(data)
}
