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

## 同步上游模型

- 管理端 Kling API Key 账号编辑页新增“同步上游模型”按钮，调用既有账号模型同步 API。后端复用 `GetKlingAccessSecret`、`GetKlingBaseURL`、`BuildKlingJWT` 与 URL 校验，发送 `GET {base_url}/v1/models`，支持基址末尾已有 `/v1` 或 `/v1/models`。
- 解析 Kling 的 `code=0` / `data.models[].id` 信封，去空白、去重并排序；404、鉴权失败、错误信封、无效 JSON 和空模型列表均返回同步错误。Kling 媒体目录跳过聊天模型能力补全及静态配置回退，其他平台的同步分支维持原行为。
- 同步成功会替换当前表单的旧白名单、旧映射及静态候选目录；“填充相关模型”也使用本次同步目录。点击编辑页“保存”后按现有账号更新流程写入 `credentials.model_mapping`，重新打开编辑页时从已保存映射恢复模型候选。失败时保留原配置；请求中禁用重复点击，切换账号或关闭弹窗后忽略迟到结果。
- 新增 service `httptest` 覆盖 JWT/HS256/claims、两种凭据格式、正常信封、404、401、坏凭据、空列表、错误信封及 URL 校验；新增前端 spec 覆盖模型替换、保存字段、错误保留和切换账号期间的并发请求。
- 后端专项测试已通过：`GOCACHE=/tmp/sub2api-gocache GOGC=20 GOMEMLIMIT=1400MiB GOMAXPROCS=2 go test -p 1 internal/service/upstream_models_kling_test.go -count=1 -v`。测试通过公开 service API 调用实际生产包，独立于现有 service 包内部的大量测试文件，以降低本机编译内存占用。
- 全量后端构建已通过：同一组内存参数下执行 `go build -p 1 ./...`。
- 前端专项及模型选择器回归 43 项通过，既有账号编辑及 Grok 同步回归 137 项通过（最终一次运行共 180 项）；两个修改过的 Vue 组件通过 SFC 脚本及模板语法编译检查。未执行 `vue-tsc`、`vite build` 或部署，最终前端构建和部署由宿主执行。
- 真实联调通过：使用数据库账号 **7**，经实际 `SyncUpstreamModelCatalog` 请求 `GET http://127.0.0.1:8020/v1/models`，返回 **HTTP 200**、成功信封及 **42 个模型 ID**（包含上游提供的别名）。通过现有账号仓储保存 `credentials.model_mapping`，重新读取数据库并与同步列表逐项核对一致。
- 真实账号配置修正：账号 7 原来保存的是该上游旧版管理 API Key，无法用于正式 JWT 接口；已核对它与本机 kling-multi 配置的管理 Key 一致，随后改用同一上游配置中的正式 AK/SK，以 `accessKey|secretKey` 格式保存。保留原 `base_url`（`https://kling.192.255.220.75.sslip.io`）及其他账号设置；账号 8 未修改。密钥未写入报告或命令输出。
- 联调上游仅执行模型列表 GET，不提交生成任务、不产生生成扣费。前端修改已通过 ESLint；`git diff --check` 通过。临时联调程序已删除，改动保留工作区，git 提交及部署由宿主完成。
