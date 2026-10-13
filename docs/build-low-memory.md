# 低内存前端打包

本机 Node 堆上限已调整为 2 GB，可在 `frontend` 目录运行：

```bash
NODE_OPTIONS=--max-old-space-size=2048 pnpm run build:bundle:low-memory
```

默认写入 `backend/internal/web/dist`，供 Go 的 `-tags=embed` 构建嵌入。
也可传入独立输出目录，避免覆盖已有资源：

```bash
NODE_OPTIONS=--max-old-space-size=2048 pnpm run build:bundle:low-memory /tmp/sub2api-frontend-dist
```

此命令保留正式构建的压缩、树摇优化、分包和 Vue 编译行为。在全部模块转换结束后，释放仅供后续热更新比较使用的 Vue 语法树缓存，再进行 Rollup 分包和输出。文件操作并发限制为 1，并关闭额外的 gzip 大小统计。只适用于一次性生产打包，不用于开发热更新或 watch。

该命令只打包，不执行 TypeScript 类型检查或测试。常规 `pnpm run build` 保持原有检查流程；使用此备用命令时，必须单独记录类型检查与测试的结果，不能将打包成功视为全部检查通过。此前完整 `vue-tsc` 检查触及 1 GB 上限；2026-09-24 调整为 2 GB 后，当前工作区完整检查已通过。

Go 构建应使用 `go build -p 1 -tags=embed`，并与前端构建、测试串行执行。

## 完整类型检查与正式发布

本机 Node 堆上限由 `/root/AGENTS.md` 的构建资源规则规定，已按用户要求从 1 GB 调整为 2 GB；服务器有 4 GB 物理内存和 4 GB 交换分区。该数值是 Node 老生代堆上限，进程总内存会高于此值，因此仍须串行执行构建和测试。增加交换分区不会解除 Node 的堆上限。

本机完整类型检查命令：

```bash
NODE_OPTIONS=--max-old-space-size=2048 pnpm exec vue-tsc -b --force --pretty false
```

2026-09-24 实测退出码为 0，耗时 34.80 秒，最大 RSS 为 2,064,432 KiB（约 1.97 GiB）。检查覆盖当前 tsconfig 定义的前端源码和引用的 Vite 配置项目，测试文件仍按原 tsconfig 排除；本次结果仅代表该时点的工作区源码。

也可在独立的 GitHub Actions 托管 runner 上执行完整检查，其 4096 MiB 配置独立于本机限制。

仓库的 `CI` 工作流在推送和 PR 时运行前端检查：使用 4096 MiB Node 堆，先执行 `pnpm exec vue-tsc -b --force --pretty false`，强制检查前端源码及引用的 Vite 配置项目，再执行 ESLint 和关键测试。类型错误会令检查失败，需要修复后重跑；提高堆上限本身不代表检查通过。

`Release` 工作流也使用 4096 MiB Node 堆，保留 `pnpm run build` 中的语言完整性检查和 `vue-tsc -b`；失败时后续发布 job 不会运行。Vite 的后台类型检查器只在开发服务器启动，避免生产打包时再启动一个 TypeScript 进程。直接调用 `vite build` 仅打包，完整验证应使用 `pnpm run build`。

建议发布步骤：

1. 将待发布修复和上述配置提交、推送到 GitHub，等待该提交的 `CI / frontend` 检查通过。
2. 为同一提交创建新版本标签（例如 `v2.8.1`）并推送，触发 `Release`，生成安装包、校验文件及镜像。当前本地 V2.8.0 包包含未提交修改，旧 `v2.8.0` 标签不包含这些修改，直接重跑旧标签不会打进修复。
3. 若从 Actions 手动运行 `Release`，工作流分支选择含本次配置的分支，`tag` 填写新版本标签，`simple_release` 设为 `false`；同时确认仓库变量 `SIMPLE_RELEASE` 未设为 `true`，否则只发布简化镜像，不生成完整安装包。版本文件、前端和后端都检出该标签。
4. 发布完成后，在目标服务器安装对应架构的包，或更新到对应版本的容器镜像，并验证版本与健康检查。发布 GitHub Release 不会自动更新正在运行的服务。

如必须覆盖现有 V2.8.0 资产，应先在同一份待发布源码上完成检查，再保存旧资产并一起更新安装包和 `checksums.txt`。直接用本地两份 Linux 包替换时，还需单独处理容器镜像；替换 Release 附件不会更新 GHCR 镜像。新版本发布更便于追溯修复和回滚。

本节描述的 CI 配置须提交到 GitHub 后才会执行；本地配置验证不能作为远程完整类型检查通过记录。此前本地包内的验证信息仍记录原构建结果。
