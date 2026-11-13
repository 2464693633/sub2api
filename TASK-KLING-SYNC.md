# 开发任务:sub2api Kling 账号支持"同步上游模型"

## 现象

管理端 kling 账号没有同步上游模型能力。根因(已定位):
- `backend/internal/service/upstream_models.go` 的 `buildUpstreamModelsRequest`(约 991 行 switch)没有 kling 分支,默认返回 "Unsupported platform for upstream model sync"
- 前端同步按钮/入口可能按平台隐藏(EditAccountModal.vue 里 antigravity 有专用块约 1580 行;kling 需要等价入口)

## 上游数据源(已就绪)

kling-multi(本机 http://127.0.0.1:8020,systemd 服务)的官方兼容层已提供:
- `GET /v1/models`,Bearer JWT 鉴权(HS256,claims {iss:<accessKey>,exp:now+1800,nbf:now-5}——**JWT 生成逻辑复用你上期实现的 service/kling.go**)
- 响应:`{"code":0,"message":"","request_id":"...","data":{"models":[{"id":"kling-v4"},...]}}`
- 账号凭据:credentials 里 base_url + api_key(`accessKey|secretKey`),解析函数上期已有

## 要做的

1. `buildUpstreamModelsRequest` 加 kling 分支:GET {base_url}/v1/models + JWT,复用凭据解析与 URL 校验逻辑(validateUpstreamBaseURL 一类)
2. 响应解析:在模型列表解析处支持 kling 信封(data.models[].id);空的 models 返回上游错误
3. 前端:kling 账号编辑页出现"同步上游模型"按钮(参照 antigravity 的交互),同步结果写入账号模型字段;若 kling 账号有静态模型目录,同步后替换
4. 测试:service 层 httptest 假上游(正常/404/坏凭据);前端相应 spec;`go build ./...` 过
5. 真实联调:对一个真实 kling 账号(库里有)执行同步,确认模型列表落库——**只读操作,不产生扣费**
6. 前端构建注意:vue-tsc 在本机会 OOM,前端验证用 `npx vitest run <spec>` + 语法检查即可,**最终 vite build 和部署由宿主做**
7. 完成后更新 KLING-CHANNEL-REPORT.md 追加"同步上游模型"一节;git 由宿主提交

## 不变量

- 其他平台同步逻辑零改动;既有 kling 端点/工作台行为不变
- 只写本目录;每 10 分钟必须有代码产出,纯阅读超时就直接按本任务书写代码
