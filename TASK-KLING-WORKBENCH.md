# 开发任务:视频工作台接入 Kling 平台

## 现象(用户实测)

用户建了 Kling 平台的账号和分组,但视频工作台(无限画布 → Video Studio)模型下拉里没有任何 kling 模型。

## 根因(已定位,直接改)

1. `backend/internal/service/video_studio_key.go` 的 `VideoStudioGroups`:硬编码只收 `PlatformGrok`/`PlatformComposite` 分组,kling 分组入口被过滤
2. 工作台生成链路只走 grok:`routes/video_studio.go` 里 `/generations`→`handler.EndpointVideos`、`/tasks/:id`→`/v1/videos`、`/tasks/:id/content`→`/v1/videos` content;kling 需要自己的提交/查询/内容获取路径
3. 前端 `useVideoStudioEngine.ts` 的模型正则已含 kling,无需大改;但注意生成参数映射(prompt 为主,duration/aspect_ratio 给合理默认或从 UI 透传)

## 要做的

1. **分组聚合**:VideoStudioGroups 纳入 kling 平台(仍要求 AllowImageGeneration 开启,与 grok 一致);错误文案里的平台清单同步更新
2. **模型列表**:`studioAvailableModels` 对 kling 分组返回 kling 视频模型(从 kling 平台静态模型目录/账号可用模型取,只含视频系:kling-v1~v4/4.0;**不要**把含 image 的模型放进来,`isVideoStudioModel` 也会过滤,但源头就别给)
3. **生成链路**:按选中模型所属分组的平台分发——grok 走现状;kling 走 kling service(直接内部函数调用,不要绕 HTTP):
   - 提交:text2video(或按模型判断 4.0 走 `/text-to-video/kling-4.0`),prompt+model_name+mode=std+duration 默认 "5"、aspect_ratio 默认 "16:9"
   - 查询:任务状态映射成工作台所需字段(submitted/processing/succeed→工作台的任务进度语义,与 grok 一致)
   - 内容:工作台靠 `/tasks/:id/content` 拿视频 blob;kling 的结果是外部 URL(s15-kling.klingai.com / v15-*),服务端代理下载并流式返回,Content-Type 用上游的(参考 grok_media_content 的做法)
4. **计费**:沿用 kling 已有的按次计费(上期已实现),确保工作台链路也过计费
5. **测试**:VideoStudioGroups 纳入 kling 的单测;klink 分发的 handler/service 单测(httptest 假上游覆盖提交/查询/内容代理);`go build ./...` 过
6. 真实联调:只读查询既有任务 326401578300834 验证 kling 查询路径;**不真实提交视频**;内容代理可用既有任务的图片 URL 做下载代理测试(图片不算生成,读已有结果不扣费)
7. 完成后 git 由宿主提交(沙箱 .git 只读);更新 KLING-CHANNEL-REPORT.md 追加"工作台接入"一节

## 不变量

- grok/composite 工作台行为不变;既有 /kling/v1/* 对外 API 不变
- 只写本目录;不改数据库既有列
- 前端如需小改(参数透传),保持现有 UI 结构
