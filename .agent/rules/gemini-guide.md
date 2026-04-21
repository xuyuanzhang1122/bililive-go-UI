# AI 开发指南

本文档为在此项目中工作的 AI 助手（Claude、GitHub Copilot、Gemini 等）提供指导。

> **注意**：本文件是 AI 指示的唯一源文件。修改后请运行 `make sync-agents` 同步到其他位置：
> - `.github/copilot-instructions.md` (GitHub Copilot)
> - `.agent/rules/gemini-guide.md` (Gemini CLI)
> - `.gemini/GEMINI.md` (Gemini Antigravity)

## 项目概览

这是 **bililive-go 优化版**，一个多平台直播录制服务。

- **后端**：Go，位于 `src/`
- **前端**：React + TypeScript + Vite，位于 `src/webapp/src/`
- **部署**：Docker（`xuniubi/bililive-go`）或本地二进制
- **完整架构与改动说明**：见 [`docs/PROJECT.md`](docs/PROJECT.md)
- **版本历史**：见 [`CHANGELOG.md`](CHANGELOG.md)

## 语言要求

永远使用中文进行交流，包括代码注释和 AI 生成的 markdown 文本。

## 核心规则

1. **编译验证**：修改代码后必须运行 `make dev` 验证编译通过
2. **不要使用** `go build ./...`，必须使用 Make 命令（`make dev` 或 `make build-web dev`）
3. **提交前检查**：确保 `make build-web dev`、`make lint`、`make test` 全部通过
4. **禁止擅自提交**：不要主动执行 `git commit`、`git push` 等 git 操作，除非用户明确要求
5. **配置变更**：修改 `Config` 结构体必须同步在 `src/pkg/migration/` 添加迁移函数
6. **平台扩展**：新平台只需在 `src/live/<platform>/` 添加 Parser，不修改主循环
7. **API 新增**：路由注册在 `src/servers/server.go`，handler 实现在 `src/servers/handler.go`

## 当前路线图（优先级顺序）

1. 修复 `config path not set`（Docker 保存设置报错）
2. 隐藏更新管理入口（指向上游，数据无意义）
3. 修复外部工具下载（GitHub 直连失败，需镜像 fallback）
4. URL 转换 resolver 模块（`src/pkg/urlresolver/`）
5. iOS App 后端支持（API Key 鉴权 + WebSocket/SSE + HLS 转封装）

## 详细指南（Skills）

| Skill | 说明 |
|-------|------|
| `build` | 编译命令、build tags、代码检查 |
| `config-modification` | 配置修改同步、层级配置系统 |
| `test-e2e` | Playwright E2E 测试 |
| `version-switching` | 不停机版本切换设计规范 |

## 快速参考

```bash
# 编译后端
make dev

# 编译前端 + 后端
make build-web dev

# 代码检查
make lint

# 单元测试
make test

# E2E 测试
npx playwright test

# 同步 AI 指示文件
make sync-agents
```
