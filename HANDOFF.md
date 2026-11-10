# Exodrive 项目工程交接文档 (Handoff Document)

**项目路径**: `/Volumes/PHD/code/exodrive`
**开源协议**: Apache-2.0
**技术栈**: Go 1.27 + nsgo (`github.com/oneo-me/nsgo` 原生 macOS Cocoa 声明式 GUI)
**当前状态**: 核心引擎、CLI、GUI 全部可运行，已按 Apache-2.0 开源发布 v0.1.0。第三轮补齐 5 项 P2 产品增强（GUI 回滚、`.app` 打包签名、可取消的 `DirSize`、进程表进程锁、`--dry-run`），已在 main（尚未重新发版）。`make check` 全绿（67 个测试函数，race 检测，`golangci-lint` 0 警告，gofmt 无差异）。CLI 与 GUI 二进制均已实测启动，发布产物已下载验证。

---

## 1. 接手核查结论

原始 HANDOFF 声称"核心引擎全部通过"，复核后发现实现与声明存在多处差距，其中三处会造成数据丢失或空间不回收：

- OrbStack 迁移只改配置、不拷数据，指向刚创建的空目录；
- 所有 provider 迁移后从不删除本地副本，磁盘占用翻倍；
- 未知文件系统默认放行（fail-open）。

两轮修复已完成：第一轮修 P0 缺陷，第二轮修独立评审（换上下文）提出的问题。

---

## 2. 第一轮修复 (P0 缺陷)

| # | 位置 | 问题 | 修复 |
|---|---|---|---|
| 1 | `plugins/docker.go` | OrbStack 分支只写 `data_dir`，不拷贝数据 → 镜像/容器全丢 | 先拷贝 + `VerifyTree` 校验，再原子写 `~/.orbstack/config.json`；保留其他配置键 |
| 2 | 所有 provider | 迁移只复制，从不删除本地副本，空间不回收 | 迁移 = 拷贝 → 校验 → 切换 → 记账 → 删除本地副本 |
| 3 | `fsutil/disk.go` | 未知文件系统默认放行 | 改白名单，未知一律拒绝 |
| 4 | `core/engine.go` | 找不到 provider 时 `Preflight` 静默返回 nil | 返回错误；`Migrate` 也先查 provider |
| 5 | `core/engine.go` | 无账本去重，可重复迁移同一项 | 账本有记录即拒绝；扫描时按账本标记 `IsMigrated` |
| 6 | 全部 provider `Rollback` | `_ = cmd.Run()`、`_ = os.Remove` 吞错，失败仍删记录 | 统一 `runCommand` / `restoreInto` / `restoreSymlink`，失败保留记录 |
| 7 | `plugins/yarn.go` | `yarn config set` 缺 `--global` | 补 `--global` |
| 8 | `fsutil/copy.go` | 未保留 mtime；失败留下半成品；不拒绝 dst 在 src 内 | 保留文件与目录时间、失败清理、拷贝前 fsync、拒绝嵌套目标 |
| 9 | 全部 provider | 拷贝后未校验 | 新增 `fsutil.VerifyTree`（文件数 + 总字节） |
| 10 | `desktop/app.go` | 界面永不刷新，目标目录写死 | `core.Dynamic` / `core.Each` / `ContentFunc`；可编辑目标 + 文件夹选择器 + 确认弹窗 + 进度 |
| 11 | `core/engine.go` | `CleanOrphan` 允许删除 `~/Library` 这类浅层目录 | 拒绝 home、home 顶层子目录、深度 < 3 的路径 |
| 12 | `plugins/lifecycle_test.go` | UV 测试走真实 uv CLI，扫描真实 1.1GB 缓存 | 加 mock CLI，测试恢复隔离 |
| 13 | `cmd/exodrive` | 版本号写死 `1.0.0` | `-X main.version` 注入，默认 `dev` |

第一轮同时产出开源面：`README.md` / `README.zh.md`、`CONTRIBUTING.md`、`SECURITY.md`、重写的 `SPEC.md`、`ci.yml`（macOS + Ubuntu 矩阵）、`release.yml`（四平台 + SHA256SUMS）、`Makefile`。

---

## 3. 第二轮修复 (独立评审)

评审要求换上下文执行，报告按 P0/P1/P2 分级。以下 14 项全部落地：

| # | 级别 | 问题 | 修复 |
|---|---|---|---|
| 14 | **P0** | `restoreNativeData` 见原路径存在就 `return nil`：回滚报告成功并删掉账本记录，数据只剩外置副本 | 改为树比对：一致才删外置副本；不一致报错并保留记录与两份数据 |
| 15 | P1 | GUI 扫描/迁移卡死：后台 `Post` 走 `Dynamic` 分支作用域，分支重绘后作用域失效 | `AppState.Root` 保存根作用域，后台一律 post 到根；新增 `TestWindowRescanRecovers`（旧实现下该测试为红，已变异验证） |
| 16 | P1 | 环境变量（`GOCACHE`、`GOMODCACHE`、`PIP_CACHE_DIR`、`YARN_CACHE_FOLDER`、`npm_config_cache`、`npm_config_store_dir`、`ORBSTACK_DATA_DIR`）优先级高于工具配置，切换不生效却仍删本地 | Preflight 拒绝迁移并指出变量名 |
| 17 | P1 | 账本无跨进程锁，临时文件名固定，无 fsync | `manifest.json.lock` 加 `flock`；唯一临时文件 + 文件 fsync + 目录 fsync |
| 18 | P1 | 崩溃窗口：切换与记账之间断点 → 数据已在外置盘但无记录 | 记账前置到切换之前；切换失败补偿删除记录；回滚时按树比对 reconcile |
| 19 | P2 | 迁移后行徽章与勾选状态不刷新 | 徽章文字/样式改用 `ContentFunc` / `ClassFunc` 读 `item()`；测试改为按行断言 |
| 20 | P2 | provider 的 `Detect` 错误被吞 | `ScanAll` 返回 items + `errors.Join`；CLI 打警告，GUI 显示在状态栏 |
| 21 | P2 | Linux 所有文件系统都返回 `"unix"`，白名单形同虚设 | 按 `Statfs_t.Type` 魔数映射；无映射的平台报未知名称（被拒绝） |
| 22 | P2 | 文件型回滚不校验字节数；`CopyFile` 可自我覆盖；`CopyDir` 不保留目录权限 | 落位前校验字节数；拒绝同一路径；恢复目录 mode |
| 23 | P2 | 迁移后 `DirSize` 对软链接根返回 0 B | `DirSize` 解析软链接根 |
| 24 | P2 | `RecordMigration` 覆盖同 ID 旧记录 | 拒绝重复 ID，保留原记录 |
| 25 | P2 | orphan 检测只读 `$GOCACHE`，漏掉 `go env -w` 写入的值 | 改为询问 `go env GOCACHE` |
| 26 | P2 | GUI 迁移可重入；迁移后扫描失败被忽略 | 扫描与迁移互斥；扫描失败单独上报并保留旧列表 |
| 27 | 文档 | 文档与实现不一致（四步流程、文件系统清单、账本原子性、回滚语义） | `SPEC.md` / `README*.md` / `CONTRIBUTING.md` 全部对齐 |

### 3.1 测试隔离

测试曾因沿用 shell 环境变量而开始复制真实的 2.7 GB cargo registry。现在 `setupMockEnv` 把 `CARGO_HOME`、`GOCACHE`、`GOMODCACHE`、`UV_CACHE_DIR`、`PIP_CACHE_DIR`、`BUN_INSTALL`、`YARN_CACHE_FOLDER`、`ORBSTACK_DATA_DIR` 全部指向临时 home，并用 mock CLI 回答查询。除一个只读的检测冒烟测试外，测试不读取真实缓存。

---

## 4. 验证证据

```text
$ make check
go test -race -count=1 ./...        # PASS, 6 packages, 67 test functions
go vet ./...                        # PASS
golangci-lint run ./...             # 0 issues
$ gofmt -l .                        # 空

$ make build
$ ./bin/exodrive version
Exodrive v0.1.0-dirty (darwin/arm64)

$ ./bin/exodrive check
... 9 items（含 4 个已在 /Volumes 的条目，状态 Migrated）
Total Local SSD Cache: 316.7 MB | Already on External: 52.3 GB

$ ./bin/exodrive-gui
running pid=82027 (UN, foreground app), stderr 为空

$ go test ./desktop/ -v
--- PASS: TestWindowListsDetectedCaches
--- PASS: TestWindowMigratesSelectedCaches
--- PASS: TestWindowRescanRecovers
--- PASS: TestWindowRollsBackMigratedCache

$ make app
make-app: bin/Exodrive.app (version 0.1.0-1-g845180c-dirty, identity: -)
$ codesign -dv --verbose=2 bin/Exodrive.app
Identifier=net.jungley.exodrive
Format=app bundle with Mach-O thin (arm64)
Signature=adhoc

$ ./bin/exodrive help
  ... --dry-run    Run every check and print the plan without moving data

$ ./bin/exodrive migrate --tools=cargo --target=$T --dry-run     # 无工具运行时
Target external directory: $T/target
Dry run. 1 cache(s) selected. No data moves.
[ready] Cargo Crate Registry: $T/home/.cargo/registry -> $T/target/cargo/registry (4.0 KB)
Dry run complete. 1 cache(s) ready. 4.0 KB would be free on the local disk.
exit=0   registry 仍是真目录，target 0 个条目，账本不存在

$ cp /bin/sleep $T/fakebin/cargo && $T/fakebin/cargo 45 &
$ ./bin/exodrive migrate --tools=cargo --target=$T --dry-run     # 工具运行时
[blocked] Cargo Crate Registry: cargo or rustc is running (PID: 16082): stop active builds before migration
Dry run complete. 0 cache(s) ready. 0 B would be free on the local disk.
exit=1   registry 仍是真目录，target 0 个条目，账本不存在

$ gh release download v0.1.0 && shasum -a 256 -c SHA256SUMS
全部 4 个产物 OK
$ ./exodrive_darwin_arm64/exodrive version
Exodrive v0.1.0 (darwin/arm64)
```

窗口测试用 `nsgotest` 无头挂载真实窗口并模拟点击，覆盖：扫描结果渲染、无目标时 Migrate 禁用、输入目标后可点、确认弹窗、迁移写入账本、迁移后徽章变为 Migrated、重扫期间按钮禁用并在完成后恢复、已迁移行出现 Roll back 并完成回滚。

进程锁的匹配规则另有单测：`node .../npm-cli.js` 命中 `npm`，`--tools=cargo` 不命中，`CARGO_HOME=/tmp/cargo exodrive migrate` 不命中，`GOFLAGS=-mod=mod go build` 命中 `go`。

---

## 5. 未完成事项

### 5.1 远端仓库与首发 (P1，✅ 2026-11-10 完成)

- [x] 创建 GitHub 仓库：<https://github.com/Jungley8/exodrive>（public，Apache-2.0）
- [x] DCO 签署提交（`Jungley Yeh <yejing@live.cn>`）并推送 main
- [x] 打 tag `v0.1.0`，release workflow 发布四个平台的 tar.gz 与 SHA256SUMS

发布地址：<https://github.com/Jungley8/exodrive/releases/tag/v0.1.0>

首发验证：四个产物 `shasum -a 256 -c SHA256SUMS` 全部 OK；下载 `darwin_arm64` 产物运行 `exodrive version` 输出 `Exodrive v0.1.0 (darwin/arm64)`。

首发时发现并修复：workflow 把 tag 名（`v0.1.0`）直接注入 `main.version`，CLI 又加了一次前缀，二进制显示 `vv0.1.0`。修复后重新发同一版本（旧 tag 与 release 已删除重建，无外部使用者）。

### 5.2 产品增强 (P2，✅ 2026-11-10 完成)

- [x] **GUI 回滚入口**：已迁移行显示 **Roll back** 按钮（`ContentFunc` 随 signal 重绘），二次确认后执行 `Engine.Rollback`；`busy` 把扫描、迁移、回滚三者互斥。
- [x] **macOS `.app` 打包与签名**：`packaging/Info.plist.in` + `scripts/make-app.sh`（`net.jungley.exodrive`，最低 13.0）；`make app` 构建 `bin/Exodrive.app`，`codesign --force` 签名（默认 ad-hoc `-`，`SIGN_IDENTITY` 可换 Developer ID），随后 `codesign --verify --strict` 失败即中止。CI 的 macOS job 跑同一条命令，release 的 `bundle` job 用 `ditto` 打成 zip 附在每个 macOS 产物旁。
- [x] **`DirSize` 支持取消**：`fsutil.DirSize(ctx, root)` 在每次遍历前检查 `ctx.Err()`，入口先 `EvalSymlinks`，避免统计到链接目标。
- [x] **进程锁改读进程表**：`ps -axo pid=,args=`，按 token 的 basename 匹配可执行文件，因此 `node .../npm-cli.js` 这类解释器启动也能命中；flag token 跳过（`--tools=cargo` 不算），shell 赋值 token 跳过（`CARGO_HOME=/tmp/cargo exodrive migrate` 不算）。进程表读不出来是错误，不是空列表。
- [x] **`migrate --dry-run`**：跑完目标文件系统、空间、进程锁、环境变量四项检查，逐条打印 `[ready]` / `[blocked]` 与计划路径，然后给出合计。不拷贝、不切换、不写账本；有条目被拦时退出码 1。

### 5.3 已知限制（README 已声明）
- 迁移需要目标空间容纳完整副本；回滚需要外置副本仍存在
- 进程锁读进程表并按名字匹配。改用异常名字启动的工具可能绕过检查，迁移前请自行停止该工具
- 跨盘时 pnpm 与 uv 退化为复制（硬链接无法跨卷）
- `.app` 为 ad-hoc 签名，首次打开需右键选择「打开」；Developer ID 需自备证书，公证未做

---

## 6. 接手操作指南

```bash
cd /Volumes/PHD/code/exodrive

make build          # 构建 bin/exodrive 与 bin/exodrive-gui
make check          # race 全量测试 + gofmt/vet/golangci-lint 门禁

./bin/exodrive check
./bin/exodrive preflight npm.cache --target=/Volumes/External/exodrive
./bin/exodrive migrate --target=/Volumes/External/exodrive --tools=npm
./bin/exodrive rollback npm.cache
```

环境变量：`EXODRIVE_MANIFEST_PATH`（账本路径）、`HOME`（测试自建临时 home）。

账本位置：`~/.exodrive/manifest.json`，同目录的 `manifest.json.lock` 用于跨进程加锁。

---

## 7. 文档地图

| 文件 | 内容 |
|---|---|
| `README.md` / `README.zh.md` | 用户向：迁移流程、安全规则、工具链表、用法、限制 |
| `SPEC.md` | 工程向：接口契约、迁移/回滚时序、不变量、失败矩阵、窗口契约 |
| `CONTRIBUTING.md` | 贡献者向：DCO、代码规则、8 条安全不变量、测试要求、新增 provider 指南 |
| `SECURITY.md` | 威胁模型、报告渠道、设计防线 |
| `HANDOFF.md` | 本文档：接手状态、两轮修复、证据、待办 |
