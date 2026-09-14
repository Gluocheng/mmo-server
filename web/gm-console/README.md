# GM 控制台前端

Vue 3 + Vite + Naive UI。生产构建写入 `internal/gmapp/ui`，由 GM 进程嵌入托管。

登录走账号密码（Cookie 会话），不再使用共享 token。开发默认管理员 `admin` / `admin123`。

```powershell
cd web/gm-console
npm install
npm run build
```

本地热更新（代理到本机 GM `:9080`）：

```powershell
npm run dev
```
