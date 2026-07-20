# ANL API 国内灾备

本目录实现以下固定拓扑：

- 美国节点是唯一主站和唯一 OAuth 执行节点。
- 美国业务 PostgreSQL 通过单向逻辑复制同步到国内 Windows PostgreSQL。
- OAuth Token、Cookie、刷新凭据和 Vault 加密密钥只保存在美国独立 Vault PostgreSQL。
- 国内应用平时停止，`oauth_vault.mode=disabled`。
- 故障接管和恢复回切均由人工执行，不修改 Cloudflare 自动切换。

## 安全边界

1. 美国业务库完成 `--migrate-oauth-vault` 且 `--audit-oauth-vault` 返回
   `sensitive_rows=0` 后，才能建立国内订阅。
2. 发布使用当前 `public` schema 的显式表清单，不使用 `FOR ALL TABLES`。逻辑复制不会同步
   DDL；官方更新新增、删除或修改表、列、索引、约束时，必须先备份，再用
   `Initialize-AnlDrSubscriber.ps1 -Reinitialize` 重建国内库和订阅，不能只更新美国程序。
3. 国内接管前必须从服务器控制台或网络层确认美国应用已停止写入。只看到 Cloudflare
   故障不等于主站已围栏。
4. 国内接管后禁止恢复美国旧库写入。回切使用国内全量核心库恢复候选库，不做双向合并。
5. Redis 不是权威数据源。接管时清空国内 Redis，并从 PostgreSQL 重建缓存。

## 文件

- `linux/Prepare-AnlDrPublisher.sh`：配置美国 PostgreSQL、复制角色和显式 publication。
- `sql/publisher-publication.sql`：刷新显式业务表发布清单。
- `windows/Keep-AnlDrTunnel.ps1`：维持国内到美国 PostgreSQL 的 SSH 本地隧道。
- `windows/Start-AnlDrPostgres.ps1`：开机只启动国内 PostgreSQL，不启动应用或 Redis。
- `windows/Configure-AnlDrStandby.ps1`：注册数据库、备份和人工接管任务，确保应用平时停止。
- `windows/Initialize-AnlDrSubscriber.ps1`：创建国内 schema 和逻辑订阅。
- `windows/Backup-AnlDr.ps1`：国内每小时一致性备份、SHA-256 和保留策略。
- `windows/Invoke-AnlDrFailover.ps1`：人工接管，强制要求主站围栏确认。
- `windows/Export-AnlDrFailback.ps1`：停止国内写入并生成最终回切包。
- `linux/Restore-AnlDrCore.sh`：美国恢复候选核心库并在确认后原子切换。

## 推荐顺序

1. 备份美国应用、业务库和配置。
2. 部署支持外部 Vault 的新程序，建立美国独立 Vault PostgreSQL。
3. 运行 `anlapi --migrate-oauth-vault`，审计通过后关闭 legacy fallback。
4. 运行 `Prepare-AnlDrPublisher.sh configure`，允许一次 PostgreSQL 重启。
5. 运行 `Prepare-AnlDrPublisher.sh publication`。
6. 在国内运行 `Initialize-AnlDrSubscriber.ps1`，完成初始复制并注册每小时备份任务，应用保持停止。
7. 持续检查复制延迟和 `ANLAPI-DR-Backup` 最近一次执行结果。
8. 按维护手册做一次不对公网的接管演练。

以后升级 ANL API 时，若数据库迁移包含任何 DDL，国内应用必须继续保持停止。美国升级和
迁移验收后，先刷新显式 publication，再在国内重新初始化订阅并确认所有关系均为 `ready`；
完成前不能把国内节点视为可接管状态。

PostgreSQL 官方说明逻辑复制支持不同平台（例如 Linux 到 Windows），但不复制 schema 和
序列。参考：<https://www.postgresql.org/docs/current/logical-replication.html>。
