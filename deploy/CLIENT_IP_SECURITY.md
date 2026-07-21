# 客户端真实 IP 安全配置

ANL API 同时支持通过 Cloudflare 域名访问和直接访问源站 IP。默认配置会关闭 Gin 的默认代理信任，也不会信任 `CF-Connecting-IP`、`X-Real-IP` 或 `X-Forwarded-For` 等原始请求头。

## 推荐配置

如果需要在域名链路中使用 API Key IP 白名单或会话 IP 绑定，请在 `server.trusted_proxies` 填入实际反代的 CIDR/IP。只填当前真实会出现在源站连接一侧的 Cloudflare、Nginx 或负载均衡器地址范围：

```yaml
server:
  trusted_proxies:
    - "203.0.113.0/24"

security:
  trust_forwarded_ip_for_api_key_acl: false
  forwarded_client_ip_headers: []
```

`trusted_proxies` 未设置、显式设置为空数组、或包含无效 CIDR 时，服务会退回到直连 peer 地址，不会接受可伪造的转发头。

## 兼容模式

仅当源站防火墙已经只允许可信反代 IP 段访问时，管理员才能在后台打开 `trust_forwarded_ip_for_api_key_acl`。开启后可在 `forwarded_client_ip_headers` 配置 CDN 的自定义客户端 IP 请求头，列表顺序即解析优先级。

源站 IP 仍可被用户直接访问时，不要开启兼容模式。直连请求可以自行填写这些 HTTP 头，进而伪造 API Key ACL、审计日志和会话绑定使用的客户端 IP。

Cloudflare 的当前 IP 段应以其官方文档为准，变更后同步更新 `server.trusted_proxies`。
