# 开发任务:sub2api 新增 Kling(可灵)平台,对接 kling-multi 官方兼容层

## 背景

同机已部署 kling-multi(可灵国际站多账号反代,systemd 服务,端口 8020),并已实现**可灵官方开放平台 API 兼容层**(已上线验收过)。现要在 sub2api 里新增 Kling 平台,渠道指向 kling-multi,让 sub2api 的用户/key 体系能分发可灵视频/图片生成。

## 上游契约(已核对过的事实,直接用)

- 兼容层完整契约报告:`/root/tasks/kling-multi/OFFICIAL-API-REPORT.md`(必读);new-api 适配器源码快照在 `/root/tasks/kling-multi/debug/official-api/new-api/plugins/tasks/kling/plugin.js`(只读参考)
- **渠道 key 格式**: `accessKey|secretKey`(new-api 同款)。kling-multi 的实值在本机 `/root/tasks/kling-multi/config.json` 的 `officialAccessKey`/`officialSecretKey`(测试用;不要写死进代码)
- **鉴权**: 每请求生成 HS256 JWT:claims `{iss:<accessKey>, exp:now+1800, nbf:now-5}`,header `Authorization: Bearer <jwt>`(Go 里用 go.mod 已有的 jwt 库,没有就加 golang-jwt/jwt/v5)
- **端点**(base_url 形如 `http://127.0.0.1:8020`,渠道可配):
  - POST `/v1/videos/text2video`、POST `/v1/videos/image2video`、GET `/v1/videos/text2video/{task_id}`、GET `/v1/videos/image2video/{task_id}`
  - POST `/v1/images/generations`、GET `/v1/images/generations/{task_id}`
  - 4.0 新协议:POST `/text-to-video/kling-4.0`、`/image-to-video/kling-4.0`、`/omni-video/kling-4.0`,GET `/tasks?task_ids=...`(成功态 `succeeded`,输出在 `outputs[]`)
  - 传统端点成功态是 `succeed`(不是 completed!);信封 `{"code":0,"message":"","data":{...}}`
- model_name 集合:`kling-v1`~`kling-v4` 系列(视频)、`kling-v3`/kolors 系(图像);mode: std/pro;duration 5/10

## 实现要求(照本仓库 grok_media 先例的模式做)

1. **平台注册**:`backend/internal/domain/platforms.go` 登记 Kling 平台(决策:归哪个网关族——建议照 grok_media 挂 OpenAI 网关族,handler 挂新路径)
2. **渠道类型**:Channel/Account 模型加 kling 类型与必要字段(base_url、access_key、secret_key;或 key 一根 `ak|sk` 拼串,与 new-api 习惯一致);ent schema 需要的话走**增量迁移**,绝不破坏既有渠道数据
3. **对外端点**:参考 grok 媒体端点的挂载方式,**路径不得与 grok 的 /v1/videos/* 冲突**(grok 已占)。建议采用 new-api 风格 `/kling/v1/...` 前缀,并在 README/报告里写清最终路径
4. **转发逻辑**:handler → service 层生成 JWT → 转发提交/查询 → 状态映射(submitted/processing/succeed|succeeded/failed)→ 结果 URL 透传;上游 code!=0 映射为合适 HTTP 状态(1102→402 等,见契约报告错误码表)
5. **计费**:参考 grok 媒体端点在本仓库的按次计费机制,给 kling 模型配置按次价格(图像/视频分开;管理端可改)
6. **前端**:平台/渠道表单支持 kling 类型(base_url + ak|sk 字段);模型列表展示
7. **测试**:handler+service 单测(JWT 构造、状态映射、错误码透传;用 httptest 假上游);**一次 `go build ./...` 必须过**;前端如需构建注意内存(2C4G,单构建)
8. 真实联调:渠道配好后用真实 kling-multi 查询一个已存在任务(如 326401578300834,GET /v1/images/generations/{id},**不产生新扣费**);不得真实提交视频

## 不变量

- 不得破坏既有平台/渠道/端点行为;grok_media 与 chat-draw 等近期功能不受影响
- 数据库迁移只增不删不改既有列
- 部署:本服务由 `sub2api-dev.service` 托管(systemd),你沙箱连不了 systemd——构建出 `backend/bin/server` 后报告里写明"宿主需 systemctl restart sub2api-dev"
- 只写本目录;真实生成提交 0 次
- 完成后写 KLING-CHANNEL-REPORT.md(实现/测试/联调证据/最终对外路径与渠道配置说明),git 提交
