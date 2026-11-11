# simple-webhook-auto-deploy

单实例管理多个目录的 webhook 自动部署服务:一个二进制同时托管多个仓库的拉取、构建、发布,内置 Web 管理界面(项目配置、手动部署、分支切换、日志查看与筛选)。

## 特性

- **单实例多项目**:一个服务、一个端口,通过 `[[projects]]` 配置任意多个仓库目录
- **Web 管理界面**:Vue 3 前端嵌入二进制,配置项目、手动触发部署、切换分支
- **日志中心**:结构化日志(时间/级别/阶段/部署会话),支持级别、关键词、时间范围、部署会话筛选,历史回放 + SSE 实时流
- **webhook 触发**:GitLab token / GitHub HMAC-SHA256 签名,支持按分支过滤
- **部署安全**:命令非零退出码立即中断,绝不清空发布目录;同一项目串行排队,不同项目并行

## 快速开始

1. 构建二进制(前端产物已提交,无需 Node 环境):

   ```bash
   go build -o webhook-service .
   ```

2. 准备配置:

   ```bash
   cp config.toml.example config.toml
   # 编辑 config.toml,填入要管理的仓库路径
   ```

3. 启动:

   ```bash
   ./webhook-service -c config.toml
   ```

4. 打开 `http://localhost:8079`(配置了 `password` 则需登录),在界面上完成:
   - 新增/编辑项目(路径、分支、构建命令、发布目录等)
   - 手动部署、切换分支
   - 查看与筛选部署日志

## 配置说明

| 键 | 说明 | 默认 |
|---|---|---|
| `port` | 服务端口 | `8079` |
| `host` | 监听地址:留空或 `0.0.0.0` 监听全部地址;`localhost` / `127.0.0.1` / `::1` 只监听本机;也可填具体 IP(如 `192.168.1.10`)只监听该网卡。修改需重启 | 空(全部地址) |
| `password` | Web 界面登录密码,留空免登录 | 空 |
| `token` | 全局 webhook token(项目未单独配置时兜底) | 空 |
| `log-dir` | 日志目录 | `logs` |
| `[[projects]]` | 项目数组,见下 | |

每个 `[[projects]]`:

| 键 | 说明 | 默认 |
|---|---|---|
| `name` | 项目唯一标识(字母/数字/`_`/`-`),用于 URL、日志文件名 | 必填 |
| `path` | 仓库工作目录(绝对路径) | 必填 |
| `branch` | 只部署该分支的 webhook 推送,留空不过滤 | 空 |
| `script` / `package-manager` | 构建命令 `<pm> run <script>` | `build` / `npm` |
| `dist` | 构建产物目录(相对仓库) | `./dist` |
| `output` | 发布目标目录,`rm -rf` 后移入产物;留空只构建不发布 | 空 |
| `token` | 项目独立 webhook token | 空(用全局) |
| `additional-script` / `additional-script-args` | 构建前附加脚本及参数 | 空 |
| `clean-ignore` | 发布清理时保留的输出目录顶层条目,支持通配(如 Syncthing 的 `.stfolder`、`.stignore`) | 空(全清) |

配置也可以完全在 Web 界面里维护,保存时会整体写回 `config.toml`。

## webhook 接入

```
POST http://<host>:<port>/webhook/<name>
```

- GitLab:Webhook 令牌填入项目 token(或全局 token),以 `X-Gitlab-Token` 头发送
- GitHub:Secret 同上,服务端校验 `X-Hub-Signature-256` HMAC-SHA256 签名
- 请求体带 `ref`(如 `refs/heads/main`)或 `branch` 字段时按项目 `branch` 配置过滤
- 恰好只有一个项目时,兼容旧的 `POST /webhook`

## 部署流程

每次部署(会话 ID 形如 `20261102-153001-a1b2c3`)依次执行,输出实时进入日志:

1. `git clean -f` + `git reset --hard`(清理工作区)
2. `git pull`(拉取代码)
3. 附加脚本(可选)
4. `<package-manager> run <script>`(构建)
5. `rm -rf <output>` + `mv <dist> <output>`(发布,未配置 output 时跳过)

任一步失败即中断,项目状态标记为失败。

## 前端开发

```bash
cd frontend
npm install
BACKEND_PORT=8079 npm run dev   # Vite :5173,代理 /api 与 /webhook 到后端
npm run build                   # 产物输出 frontend/dist,由 go:embed 嵌入
```

> `frontend/dist` 提交进仓库,保证无 Node 环境也能 `go build` 出完整二进制。修改前端后记得重新 build 并提交。

## 日志文件

运行后 `log-dir` 目录下:

- `<name>.log` — 项目部署日志(JSONL,单文件超 10MB 滚动为 `.1`)
- `<name>.history.json` — 项目部署历史记录
- `system.log` — 服务自身日志
