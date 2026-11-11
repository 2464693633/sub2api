# Kling 平台接入报告

## 实现

- 在 `domain/platforms.go` 注册 `kling`（OpenAI 网关族），账号凭据使用现有 JSON `credentials` 字段：`base_url`、`api_key`（`accessKey|secretKey`），同时兼容 `access_key`/`secret_key`。
- 新增 Kling service/handler：每次请求生成 HS256 JWT（`iss`、`exp=now+1800`、`nbf=now-5`），实现传统视频、图像和 Kling 4.0 提交/查询，透传成功信封与结果 URL，按官方错误码映射 HTTP 状态。
- 新增管理端 Kling 平台选项、`base_url` 与 `accessKey|secretKey` 表单占位符及静态模型目录。数据库无需迁移：平台与凭据字段均为既有字符串/JSON 扩展。

## 最终对外路径

所有路径均带 `/kling/v1` 前缀，不占用 Grok 的 `/v1/videos/*`：

`POST /videos/text2video`、`POST /videos/image2video`、`GET /videos/text2video/:task_id`、`GET /videos/image2video/:task_id`、`POST /images/generations`、`GET /images/generations/:task_id`、`POST /text-to-video/kling-4.0`、`POST /image-to-video/kling-4.0`、`POST /omni-video/kling-4.0`、`GET /tasks?task_ids=:id`（另有 `/tasks/:task_id` 兼容形式）、`GET /models`。

请求仍使用 sub2api API Key；Key 所属分组的平台应配置为 `kling`，账号类型选择 API Key，`base_url` 例如 `http://127.0.0.1:8020`，API Key 字段填 `accessKey|secretKey`。

## 测试与联调

- 新增 service 单测覆盖 JWT claims/HS256、传统与 4.0 状态及结果 URL 解析、httptest 上游转发和错误码映射。
- `GOCACHE=/tmp/sub2api-gocache go test ./internal/service ./internal/handler ./internal/domain -run 'Test(BuildKlingJWTClaims|KlingStatusAndResultParsing|ForwardKlingJWTAndErrorMapping|KlingErrorHTTPStatus|KlingModelsHandler|PlatformList)'` 通过。
- `GOCACHE=/tmp/sub2api-gocache go build ./...` 通过；`backend/bin/server` 已构建（165600208 bytes）。前端 `npm run typecheck` 与平台目录 Vitest（5 tests）通过；`git diff --check` 与前端 JSON 校验通过。
- 真实只读联调已完成：`GET http://127.0.0.1:8020/v1/images/generations/326401578300834` 返回 HTTP 200、`code=0`、`task_status=succeed`，并得到 `task_result.images[0].url`（未提交任何新任务）。
- 计费取舍：Kling 提交成功后按模型的图像/视频按次价格写入一次 usage；查询只透传状态与结果 URL，不重复计费。提交前复用现有 inflight reservation 做余额预留，查询路径不做预留，避免已提交任务因余额变化无法取回；视频时长从请求/结果读取，缺省 5 秒。该实现优先保证异步任务不重复扣费，未引入独立 Kling 任务表。

部署时构建 `backend/bin/server` 后，宿主需执行：

`systemctl restart sub2api-dev`

本沙箱 `.git` 目录只读，执行 `git add`/`git commit` 时无法创建 `.git/index.lock`，因此改动保留在工作区，尚未生成提交。
