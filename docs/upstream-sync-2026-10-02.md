# 上游同步记录（2026-10-02）

本次将 `Wei-Shaw/sub2api` 的 `main` 合并到 `codex/api-key-group-failover`，固定目标提交为 `5106065716e494204fc0e8db16f68f6e9d576be0`。它包含正式版 v0.2.9、v0.2.10、v0.2.11，以及后续 Grok 426 修复、TypeSafe System One、充值赠送/折扣、支付验证加固、API Key 分组排序和账号优先级快捷调整。

合并前已将部署中的工作台修改保存为 `f517a9b62`，并创建 `backup/pre-upstream-20261002` 分支。二开版本号沿用现有编号规则，更新为 `2.11.0`；没有创建上游同名标签。

## 二开兼容处理

- 保留 Lyozc 品牌、图片/视频工作台、整体图加细节组图、用户隔离的历史恢复、终态重试与空分组模型过滤。
- 保留分组故障转移、RPM override、Token 倍率和账号健康调度配置。
- 跨分组故障转移重新估算在途余额，并通过同一 Redis 预留 ID 原子调整金额，避免重复计算当前请求或先释放再申请的窗口；订阅组与余额组切换时正确移交预留句柄。
- 余额预估应用分组 Token 倍率；新增图片缓存 Token 明细也应用缓存读取倍率，并保持独立价格。
- 固定金额/币种的充值优惠档位优先于上游新增的赠送与折扣阶梯，不叠加优惠；其他金额沿用上游阶梯。服务端报价、快捷金额预览、手续费、到账金额和订单免费额度保持一致。
- 各支付渠道按自己的币种报价后再判断限额，避免固定档位与跨币种折扣组合导致误禁用或反复切换支付方式。
- 图片、视频和提示词助手的工作台密钥复用分页查询，避免旧密钥在第二页时误创建新密钥并触发创建上限。
- 用户趋势统计新增消费金额排序时，保留二开的计费 Token 统计口径。
- 修复原有调度重置时间打分遗漏：实际排序和后台快照统一读取规范化 Codex 窗口，与既有回归断言保持一致。
- 补齐 Compose 的二开镜像和更新源配置；使用 `SUB2API_IMAGE`、`SUB2API_IMAGE_TAG`、`UPDATE_GITHUB_REPO` 仍可覆盖默认来源。
- 前端固定 `pnpm@9.15.9`，与 CI、Docker 的 pnpm 9 保持兼容，修复工作区配置并重新生成包含安全依赖覆盖规则的锁文件。

## 新的默认行为和数据库变更

- `api_key_create.max_active_per_user=200`，`api_key_create.max_per_user_per_hour=60`。设为 0 可关闭对应限制；工作台自动创建的密钥同样计入。
- `billing.inflight_reservation.enabled=true`，低余额并发请求可能因预留额度而被拒绝；估价失败及 Redis 故障仍遵循上游的降级配置。
- `241_add_payment_order_bonus_amount.sql` 为订单新增 `bonus_amount`，默认 0。
- `241_add_typesafe_platform.sql` 扩展平台约束以允许 `typesafe`。两份迁移使用不同完整文件名，由现有迁移框架分别记录。

固定档位详情见 [充值优惠档位](recharge-offers.md)。低内存构建及验证命令见 [构建说明](build-low-memory.md)。

## 验证记录

- 全量 Go 单元测试 `GOMAXPROCS=2 go test -p 1 -tags=unit -json ./...` 通过：58 个含测试的软件包、22,042 项测试及子用例通过，17 项按测试自身条件跳过。
- 与 CI 一致的 `golangci-lint v2.13.0` 全库检查通过，结果为 `0 issues`；本机设置 `GOFLAGS=-p=1`、`GOMAXPROCS=2`、`--concurrency=1`。
- `pnpm@9.15.9 install --frozen-lockfile --ignore-scripts` 通过。
- 完整 Vue/TypeScript 检查 `vue-tsc -b --force --pretty false`、前端 ESLint 通过。
- 2 GiB Node 堆限制下的前端生产构建、`CGO_ENABLED=0 go build -p 1 -tags=embed -trimpath` 后端构建通过；候选程序 `--version` 显示 `2.11.0`。
- 前端关键流程与工作台回归共 31 个测试文件、411 项用例通过（分批运行；包含修正后的 API Key 多分组测试和监控页测试）。
- 两条新增 SQL 迁移在仅复制线上表结构的独立 PostgreSQL 测试库各执行两次，验证字段、约束和幂等性；测试库与临时文件已清理，线上数据未改动。
- Linux 可运行的五项部署配置检查通过：Compose 安全选项、网关环境变量、二开发布来源、运行资源配置和 Caddy 缓存策略。Apple container 检查依赖 macOS 的 `stat -f`，留给仓库 macOS CI 执行。

服务器存在 `/app/data/config.yaml`，默认配置单元测试会读取该文件。验证使用独立 mount namespace 将空目录绑定到测试进程的 `/app/data`，隔离真实部署配置；没有重命名、修改或挂载覆盖宿主服务的配置文件。

本次完成本地合并和构建验证，尚未推送远端或部署此版本。候选程序位于 `/tmp/sub2api-merge-2.11.0-server`，当前运行的服务未切换，生产数据库未执行本次新增迁移。
