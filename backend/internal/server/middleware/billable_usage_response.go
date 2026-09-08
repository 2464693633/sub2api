package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// BillableUsageResponse rewrites HTTP JSON and SSE usage payloads for groups
// that explicitly expose customer-billed token counts downstream. WebSocket
// frames bypass Gin's ResponseWriter and intentionally retain raw usage.
func BillableUsageResponse() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey, ok := GetAPIKeyFromContext(c)
		if !ok || apiKey == nil || apiKey.Group == nil {
			c.Next()
			return
		}

		writer := &billableUsageWriter{ResponseWriter: c.Writer, context: c, group: apiKey.Group}
		c.Writer = writer
		c.Next()
		_ = writer.flushPending()
	}
}

const billableUsageResponseGroupContextKey = "billable_usage_response_group"
const billableUsageResponseForceCacheContextKey = "billable_usage_response_force_cache"
const maxBillableUsageSSEEventBytes = 32 << 20

// SetBillableUsageResponseGroup keeps response usage aligned with the group
// that ultimately owns billing when a handler switches to a fallback group.
func SetBillableUsageResponseGroup(c *gin.Context, group *service.Group) {
	if c == nil || group == nil {
		return
	}
	c.Set(billableUsageResponseGroupContextKey, group)
}

// SetBillableUsageResponseForceCache applies the same sticky-session account
// switch classification used by billing before the successful response writes.
func SetBillableUsageResponseForceCache(c *gin.Context) {
	if c != nil {
		c.Set(billableUsageResponseForceCacheContextKey, true)
	}
}

type billableUsageWriter struct {
	gin.ResponseWriter
	context *gin.Context
	group   *service.Group
	pending []byte
	// Oversized unframed events fall back to raw streaming to preserve the
	// response without retaining unbounded memory.
	ssePassthrough bool
}

func (w *billableUsageWriter) responseGroup() *service.Group {
	if w.context != nil {
		if value, ok := w.context.Get(billableUsageResponseGroupContextKey); ok {
			if group, ok := value.(*service.Group); ok && group != nil {
				return group
			}
		}
	}
	return w.group
}

func (w *billableUsageWriter) forceCacheBilling() bool {
	if w.context == nil {
		return false
	}
	value, _ := w.context.Get(billableUsageResponseForceCacheContextKey)
	enabled, _ := value.(bool)
	return enabled
}

func (w *billableUsageWriter) WriteHeader(code int) {
	if group := w.responseGroup(); group != nil && group.ReturnBillableUsage {
		// Rewritten JSON can change byte length.
		w.Header().Del("Content-Length")
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *billableUsageWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	group := w.responseGroup()
	if group == nil || !group.ReturnBillableUsage {
		if _, err := w.ResponseWriter.Write(data); err != nil {
			return 0, err
		}
		return len(data), nil
	}
	w.Header().Del("Content-Length")
	if strings.Contains(strings.ToLower(w.Header().Get("Content-Type")), "text/event-stream") {
		if w.ssePassthrough {
			if _, err := w.ResponseWriter.Write(data); err != nil {
				return 0, err
			}
			return len(data), nil
		}
		if len(data) > maxBillableUsageSSEEventBytes-len(w.pending) {
			if _, err := w.ResponseWriter.Write(w.pending); err != nil {
				return 0, err
			}
			w.pending = nil
			w.ssePassthrough = true
			if _, err := w.ResponseWriter.Write(data); err != nil {
				return 0, err
			}
			return len(data), nil
		}
		w.pending = append(w.pending, data...)
		if err := w.flushSSEEvents(false); err != nil {
			return 0, err
		}
		return len(data), nil
	}

	rewritten, changed := rewriteBillableUsageJSON(data, group, w.forceCacheBilling())
	if !changed {
		rewritten = data
	}
	if _, err := w.ResponseWriter.Write(rewritten); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (w *billableUsageWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func (w *billableUsageWriter) Flush() {
	_ = w.flushSSEEvents(false)
	w.ResponseWriter.Flush()
}

func (w *billableUsageWriter) Size() int {
	return w.ResponseWriter.Size() + len(w.pending)
}

func (w *billableUsageWriter) flushPending() error {
	if len(w.pending) == 0 {
		return nil
	}
	if strings.Contains(strings.ToLower(w.Header().Get("Content-Type")), "text/event-stream") {
		return w.flushSSEEvents(true)
	}
	_, err := w.ResponseWriter.Write(w.pending)
	w.pending = nil
	return err
}

func (w *billableUsageWriter) flushSSEEvents(final bool) error {
	group := w.responseGroup()
	for {
		end, separatorLen := nextSSEEventBoundary(w.pending)
		if end < 0 {
			break
		}
		event := w.pending[:end]
		separator := w.pending[end : end+separatorLen]
		if _, err := w.ResponseWriter.Write(rewriteBillableSSEEvent(event, group, w.forceCacheBilling())); err != nil {
			return err
		}
		if _, err := w.ResponseWriter.Write(separator); err != nil {
			return err
		}
		w.pending = w.pending[end+separatorLen:]
	}
	if final && len(w.pending) > 0 {
		_, err := w.ResponseWriter.Write(rewriteBillableSSEEvent(w.pending, group, w.forceCacheBilling()))
		w.pending = nil
		return err
	}
	return nil
}

func nextSSEEventBoundary(data []byte) (int, int) {
	lf := bytes.Index(data, []byte("\n\n"))
	crlf := bytes.Index(data, []byte("\r\n\r\n"))
	switch {
	case lf < 0:
		if crlf < 0 {
			return -1, 0
		}
		return crlf, 4
	case crlf < 0 || lf < crlf:
		return lf, 2
	default:
		return crlf, 4
	}
}

func rewriteBillableSSEEvent(event []byte, group *service.Group, forceCacheBilling ...bool) []byte {
	var out bytes.Buffer
	for index, line := range bytes.Split(event, []byte{'\n'}) {
		if index > 0 {
			_ = out.WriteByte('\n')
		}
		trimmed := bytes.TrimSpace(line)
		if !bytes.HasPrefix(trimmed, []byte("data:")) {
			_, _ = out.Write(line)
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(trimmed, []byte("data:")))
		if bytes.Equal(payload, []byte("[DONE]")) {
			_, _ = out.Write(line)
			continue
		}
		rewritten, changed := rewriteBillableUsageJSON(payload, group, firstBool(forceCacheBilling))
		if !changed {
			_, _ = out.Write(line)
			continue
		}
		indent := line[:len(line)-len(bytes.TrimLeft(line, " \t"))]
		_, _ = out.Write(indent)
		_, _ = out.WriteString("data: ")
		_, _ = out.Write(rewritten)
	}
	return out.Bytes()
}

func rewriteBillableUsageJSON(payload []byte, group *service.Group, forceCacheBilling ...bool) ([]byte, bool) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return payload, false
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return payload, false
	}
	if !rewriteBillableUsageNodes(value, group, firstBool(forceCacheBilling)) {
		return payload, false
	}
	rewritten, err := json.Marshal(value)
	if err != nil {
		return payload, false
	}
	return rewritten, true
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}

func firstBool(values []bool) bool {
	return len(values) > 0 && values[0]
}

func rewriteBillableUsageNodes(value any, group *service.Group, forceCacheBilling bool) bool {
	changed := false
	switch node := value.(type) {
	case map[string]any:
		if usage, ok := node["usage"].(map[string]any); ok && rewriteTokenUsageMap(usage, group, forceCacheBilling) {
			changed = true
		}
		if usage, ok := node["usageMetadata"].(map[string]any); ok && rewriteGeminiUsageMap(usage, group, forceCacheBilling) {
			changed = true
		}
		for _, containerKey := range []string{"response", "message"} {
			container, ok := node[containerKey].(map[string]any)
			if !ok {
				continue
			}
			if usage, ok := container["usage"].(map[string]any); ok && rewriteTokenUsageMap(usage, group, forceCacheBilling) {
				changed = true
			}
		}
		// Some OpenAI-compatible providers wrap the response in a single data
		// object. Limit handling to that documented wrapper and its direct
		// response child so tool arguments remain opaque.
		if data, ok := node["data"].(map[string]any); ok {
			if usage, ok := data["usage"].(map[string]any); ok && rewriteTokenUsageMap(usage, group, forceCacheBilling) {
				changed = true
			}
			if response, ok := data["response"].(map[string]any); ok {
				if usage, ok := response["usage"].(map[string]any); ok && rewriteTokenUsageMap(usage, group, forceCacheBilling) {
					changed = true
				}
			}
		}
	case []any:
		for _, child := range node {
			if rewriteBillableUsageNodes(child, group, forceCacheBilling) {
				changed = true
			}
		}
	}
	return changed
}

func rewriteTokenUsageMap(usage map[string]any, group *service.Group, forceCacheBilling bool) bool {
	promptTotal, hasPrompt := tokenField(usage, "prompt_tokens")
	inputTotal, hasInput := tokenField(usage, "input_tokens")
	if !hasInput {
		inputTotal = promptTotal
		hasInput = hasPrompt
	}
	_, outputTotal, hasOutput := firstTokenField(usage, "completion_tokens", "output_tokens")
	cacheRead, hasCacheRead := nestedTokenField(usage,
		"cache_read_input_tokens", "cache_read_tokens", "cached_tokens",
		"input_tokens_details.cached_tokens", "prompt_tokens_details.cached_tokens")
	promptCacheHit, hasPromptCacheHit := tokenField(usage, "prompt_cache_hit_tokens")
	if (!hasCacheRead || cacheRead == 0) && hasPromptCacheHit {
		cacheRead = promptCacheHit
		hasCacheRead = true
	}
	cacheCreation, hasCacheCreation := nestedTokenField(usage,
		"cache_creation_input_tokens", "cache_creation_tokens", "cache_write_input_tokens", "cache_write_tokens",
		"input_tokens_details.cache_write_tokens",
		"input_tokens_details.cache_creation_tokens", "prompt_tokens_details.cache_creation_tokens")
	cacheCreation5m, hasCacheCreation5m := nestedTokenField(usage, "cache_creation.ephemeral_5m_input_tokens")
	cacheCreation1h, hasCacheCreation1h := nestedTokenField(usage, "cache_creation.ephemeral_1h_input_tokens")
	if (!hasCacheCreation || cacheCreation == 0) && (hasCacheCreation5m || hasCacheCreation1h) {
		cacheCreation = cacheCreation5m + cacheCreation1h
		hasCacheCreation = true
	}
	promptCacheMiss, hasPromptCacheMiss := tokenField(usage, "prompt_cache_miss_tokens")
	if !hasInput && hasPromptCacheMiss {
		inputTotal = promptCacheMiss
		hasInput = true
	}
	if !hasInput && !hasOutput && !hasCacheRead && !hasCacheCreation && !hasPromptCacheMiss {
		return false
	}

	aggregateInput := hasTokenField(usage, "prompt_tokens") || mapAtPath(usage, "input_tokens_details") != nil ||
		mapAtPath(usage, "prompt_tokens_details") != nil || hasTokenField(usage, "total_tokens")
	rawInput := inputTotal
	if hasPromptCacheMiss {
		rawInput = promptCacheMiss
	} else if hasPrompt {
		rawInput = promptTotal - cacheRead - cacheCreation
		if rawInput < 0 {
			rawInput = 0
		}
	} else if aggregateInput {
		rawInput -= cacheRead + cacheCreation
		if rawInput < 0 {
			rawInput = 0
		}
	}
	if forceCacheBilling && rawInput > 0 {
		cacheRead += rawInput
		rawInput = 0
	}
	imageInput, _ := nestedTokenField(usage, "input_tokens_details.image_tokens", "prompt_tokens_details.image_tokens")
	imageOutput, _ := nestedTokenField(usage, "output_tokens_details.image_tokens", "completion_tokens_details.image_tokens")
	reasoning, _ := nestedTokenField(usage,
		"completion_tokens_details.reasoning_tokens", "output_tokens_details.reasoning_tokens")
	rawInputTotal := rawInput + cacheRead + cacheCreation
	outputTotal = int(xai.IncludeIndependentReasoningTokens(
		int64(rawInputTotal), int64(outputTotal), int64(tokenFieldOrZero(usage, "total_tokens")), int64(reasoning),
	))
	raw := service.UsageTokens{
		InputTokens: rawInput, ImageInputTokens: imageInput, OutputTokens: outputTotal,
		CacheCreationTokens: cacheCreation, CacheReadTokens: cacheRead, ImageOutputTokens: imageOutput,
		CacheCreation5mTokens: cacheCreation5m, CacheCreation1hTokens: cacheCreation1h,
	}
	billable, multipliers := service.ResolveBillableUsageTokens(raw, group)

	billableInputTotal := billable.InputTokens + billable.CacheCreationTokens + billable.CacheReadTokens
	if hasTokenField(usage, "prompt_tokens") {
		usage["prompt_tokens"] = billableInputTotal
	}
	if originalInput, ok := tokenField(usage, "input_tokens"); ok {
		if hasPromptCacheMiss && originalInput == promptCacheMiss {
			usage["input_tokens"] = billable.InputTokens
		} else if hasPrompt && originalInput == rawInput && originalInput != promptTotal {
			usage["input_tokens"] = billable.InputTokens
		} else if aggregateInput {
			usage["input_tokens"] = billableInputTotal
		} else {
			usage["input_tokens"] = billable.InputTokens
		}
	}
	if hasOutput {
		setExistingTokenFields(usage, billable.OutputTokens, "completion_tokens", "output_tokens")
	}
	setExistingTokenFields(usage, billable.CacheReadTokens,
		"cache_read_input_tokens", "cache_read_tokens", "cached_tokens", "prompt_cache_hit_tokens",
		"input_tokens_details.cached_tokens", "prompt_tokens_details.cached_tokens")
	if forceCacheBilling && !aggregateInput && !hasTokenField(usage, "cache_read_input_tokens") {
		usage["cache_read_input_tokens"] = billable.CacheReadTokens
	}
	setExistingTokenFields(usage, billable.CacheCreationTokens,
		"cache_creation_input_tokens", "cache_creation_tokens", "cache_write_input_tokens", "cache_write_tokens",
		"input_tokens_details.cache_write_tokens",
		"input_tokens_details.cache_creation_tokens", "prompt_tokens_details.cache_creation_tokens")
	setExistingTokenFields(usage, billable.InputTokens, "prompt_cache_miss_tokens")
	setExistingTokenFields(usage, billable.ImageInputTokens,
		"input_tokens_details.image_tokens", "prompt_tokens_details.image_tokens")
	setExistingTokenFields(usage, billable.ImageOutputTokens,
		"output_tokens_details.image_tokens", "completion_tokens_details.image_tokens")
	scaleExistingDetailFields(usage, multipliers.Output,
		"output_tokens_details.reasoning_tokens", "completion_tokens_details.reasoning_tokens")
	if hasTokenField(usage, "total_tokens") {
		usage["total_tokens"] = billableInputTotal + billable.OutputTokens
	}
	if cacheCreationDetails, ok := usage["cache_creation"].(map[string]any); ok {
		if hasTokenField(cacheCreationDetails, "ephemeral_5m_input_tokens") {
			cacheCreationDetails["ephemeral_5m_input_tokens"] = billable.CacheCreation5mTokens
		}
		if hasTokenField(cacheCreationDetails, "ephemeral_1h_input_tokens") {
			cacheCreationDetails["ephemeral_1h_input_tokens"] = billable.CacheCreation1hTokens
		}
	}
	return true
}

func tokenFieldOrZero(values map[string]any, key string) int {
	value, _ := tokenField(values, key)
	return value
}

func rewriteGeminiUsageMap(usage map[string]any, group *service.Group, forceCacheBilling bool) bool {
	prompt, hasPrompt := tokenField(usage, "promptTokenCount")
	candidates, hasCandidates := tokenField(usage, "candidatesTokenCount")
	thoughts, hasThoughts := tokenField(usage, "thoughtsTokenCount")
	cached, hasCached := tokenField(usage, "cachedContentTokenCount")
	if !hasPrompt && !hasCandidates && !hasThoughts && !hasCached {
		return false
	}
	rawInput := prompt - cached
	if rawInput < 0 {
		rawInput = 0
	}
	if forceCacheBilling {
		cached += rawInput
		rawInput = 0
	}
	billable, multipliers := service.ResolveBillableUsageTokens(service.UsageTokens{
		InputTokens: rawInput, OutputTokens: candidates + thoughts, CacheReadTokens: cached,
	}, group)
	if hasPrompt {
		usage["promptTokenCount"] = billable.InputTokens + billable.CacheReadTokens
		rewriteGeminiTokenDetails(
			usage,
			"promptTokensDetails",
			prompt,
			billable.InputTokens+billable.CacheReadTokens,
		)
	}
	if hasCached {
		usage["cachedContentTokenCount"] = billable.CacheReadTokens
	} else if forceCacheBilling && billable.CacheReadTokens > 0 {
		usage["cachedContentTokenCount"] = billable.CacheReadTokens
	}
	if hasCandidates || hasThoughts {
		billableCandidates := roundedResponseTokenCount(candidates, multipliers.Output)
		if billableCandidates > billable.OutputTokens {
			billableCandidates = billable.OutputTokens
		}
		if hasCandidates {
			usage["candidatesTokenCount"] = billableCandidates
		}
		if hasThoughts {
			usage["thoughtsTokenCount"] = billable.OutputTokens - billableCandidates
		}
		rewriteGeminiTokenDetails(usage, "candidatesTokensDetails", candidates, billableCandidates)
	}
	if hasTokenField(usage, "totalTokenCount") {
		usage["totalTokenCount"] = billable.InputTokens + billable.CacheReadTokens + billable.OutputTokens
	}
	return true
}

func rewriteGeminiTokenDetails(usage map[string]any, key string, rawAggregate, billableAggregate int) {
	details, ok := usage[key].([]any)
	if !ok || len(details) == 0 {
		return
	}
	rawCounts := make([]int, len(details))
	rawTotal := 0
	for index, item := range details {
		detail, ok := item.(map[string]any)
		if !ok {
			continue
		}
		count, ok := tokenField(detail, "tokenCount")
		if !ok {
			continue
		}
		rawCounts[index] = count
		rawTotal += count
	}
	if rawTotal <= 0 {
		return
	}
	if rawAggregate <= 0 {
		rawAggregate = rawTotal
	}
	target := int(math.Round(float64(billableAggregate) * float64(rawTotal) / float64(rawAggregate)))
	if target > billableAggregate {
		target = billableAggregate
	}
	allocated := 0
	cumulativeRaw := 0
	for index, item := range details {
		detail, ok := item.(map[string]any)
		if !ok || !hasTokenField(detail, "tokenCount") {
			continue
		}
		cumulativeRaw += rawCounts[index]
		cumulativeTarget := int(math.Round(float64(target) * float64(cumulativeRaw) / float64(rawTotal)))
		if cumulativeTarget > target {
			cumulativeTarget = target
		}
		detail["tokenCount"] = cumulativeTarget - allocated
		allocated = cumulativeTarget
	}
}

func firstTokenField(values map[string]any, keys ...string) (string, int, bool) {
	for _, key := range keys {
		if value, ok := tokenField(values, key); ok {
			return key, value, true
		}
	}
	return "", 0, false
}

func tokenField(values map[string]any, key string) (int, bool) {
	return tokenNumber(values[key])
}

func nestedTokenField(values map[string]any, paths ...string) (int, bool) {
	for _, path := range paths {
		if value, ok := tokenNumber(valueAtPath(values, path)); ok {
			return value, true
		}
	}
	return 0, false
}

func tokenNumber(value any) (int, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseInt(number.String(), 10, 64)
		if err == nil && parsed >= 0 {
			return int(parsed), true
		}
	case float64:
		if number >= 0 {
			return int(number), true
		}
	case int:
		if number >= 0 {
			return number, true
		}
	}
	return 0, false
}

func valueAtPath(values map[string]any, path string) any {
	parts := strings.Split(path, ".")
	var current any = values
	for _, part := range parts {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[part]
	}
	return current
}

func mapAtPath(values map[string]any, path string) map[string]any {
	value, _ := valueAtPath(values, path).(map[string]any)
	return value
}

func hasTokenField(values map[string]any, path string) bool {
	_, ok := tokenNumber(valueAtPath(values, path))
	return ok
}

func setExistingTokenFields(values map[string]any, value int, paths ...string) {
	for _, path := range paths {
		parts := strings.Split(path, ".")
		object := values
		valid := true
		for _, part := range parts[:len(parts)-1] {
			next, ok := object[part].(map[string]any)
			if !ok {
				valid = false
				break
			}
			object = next
		}
		if valid {
			key := parts[len(parts)-1]
			if hasTokenField(object, key) {
				object[key] = value
			}
		}
	}
}

func scaleExistingDetailFields(values map[string]any, multiplier float64, paths ...string) {
	for _, path := range paths {
		if raw, ok := nestedTokenField(values, path); ok {
			setExistingTokenFields(values, roundedResponseTokenCount(raw, multiplier), path)
		}
	}
}

func roundedResponseTokenCount(raw int, multiplier float64) int {
	billable, _ := service.ResolveBillableUsageTokens(service.UsageTokens{InputTokens: raw}, &service.Group{
		InputTokenMultiplier:         multiplier,
		OutputTokenMultiplier:        1,
		CacheCreationTokenMultiplier: 1,
		CacheReadTokenMultiplier:     1,
		TokenMultipliersConfigured:   true,
	})
	return billable.InputTokens
}
