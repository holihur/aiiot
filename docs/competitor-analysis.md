# 竞品对比与改进优先级

> 定位：企业私有化、中小规模设备接入的 IoT 平台（对标 ThingsBoard CE 类产品）。
> 对比对象：ThingsBoard CE（开源对标）、EMQX（协议网关生态）、AWS IoT Core /
> Azure IoT Hub / 阿里云 IoT 平台（云服务参照）、IoT-Tree / Mainflux（轻量开源参照）。

## 1. 能力矩阵

| 能力域 | ThingsBoard CE | EMQX 生态 | AWS/Azure/阿里云 | **aiiot（现状）** | 差距 |
|---|---|---|---|---|---|
| 接入协议 | MQTT/CoAP/HTTP/LwM2M/SNMP/Modbus | 仅 MQTT(网关) + 桥接 | MQTT/AMQP/HTTP/LwM2M/OPC-UA | MQTT/CoAP/custom-TCP/HTTP-ingest | 中 |
| 规则引擎 | 可视化节点编排（Filter/Transform/Action） | 规则 SQL + 桥接 | 可视化流编排（IoT Rules/Azure Rules） | CEL 表达式 + 4 种动作（webhook/下行/命令/log） | 中 |
| 可视化仪表盘 | ⭐ 核心卖点：图表/卡片/地图/报表 | 无（配合 Grafana） | 强（IoT Analytics） | 设备曲线（raw/1m/5m/1h 间隔）+ 可保存看板（stat/trend/alerts/devices/text）；**缺多设备聚合、地图/报表** | 中 |
| 告警 | 阈值规则 + 升级 + 邮件通道 | 规则桥接 | 规则 + SNS/SES/短信 | 三级升级 + 抑制 + **webhook/钉钉/邮件通道 + 通知日志** | 小 |
| 设备管理 | 物模型/影子/批量导入导出/子设备 | 设备+ACL | 物模型/影子 | 物模型/影子/OTA/时间线（扎实） | 小 |
| 时序存储 | PostgreSQL+扩展 / Cassandra | InfluxDB 等外部 | 自研 TSDB | PostgreSQL 分区 + rollup + 最新值缓存 | 小（中规模够用） |
| 多租户/权限 | Tenant→Customer 两级 | — | 云账户体系 | project/workspace/member + JWT；**无自定义角色/资源级 ACL/审计** | 中 |
| 安全 | 双向 TLS/证书（GP） | TLS/ACL | X.509/JWT/策略 | JWT + 设备 token；MQTT TLS 已支持（env 开启）；**无设备 X.509 认证、无 DTLS** | 中 |
| HA/水平扩展 | 集群（需 Kafka 可选） | nativer 集群 | 云托管 | NATS 总线 + 副本安全后台任务（架构强，实战验证少） | 小 |
| 部署 | 复杂（3 组件） | 容器 | 云 | docker-compose/Makefile（尚无 Helm） | 小 |
| 生态/SDK/社区 | ⭐ 成熟 SDK + 庞大社区 | 强 | 丰富 | 示例代码 + 浏览器模拟器；**无正式 SDK/英文文档/OpenAPI** | 大 |

## 2. 最需要改进的 TOP 5（按优先级）

### P0-1 可视化仪表盘与图表 —— 大部分已存在，按差距补齐
- **现状（核实后修正）**：前端已有 recharts；设备详情页有曲线（raw/聚合 interval）× 时间范围；项目看板可保存（stat 统计卡 / trend 设备曲线 / alerts / devices / text 五种 panel）。此前"无图表库"的判断与事实不符。
- **本次已完成的增量**：
  1. 后端聚合查询支持**多标识**（`identifier=temperature,humidity`，聚合 SQL 按 `(bucket, identifier)` 分组，raw/rollup 双路径）；
  2. 设备详情曲线升级为**多指标多线**（复选指标、调色板区分系列、非数值指标自动过滤）。
- **下一步（价值排序）**：
  1. trend panel 支持多设备/多指标叠加（复用已就绪的聚合参数）；
  2. 平台级活跃度趋势（在线设备数、上行速率）——需新增后端指标历史存储；
  3. CSV 导出曲线数据 / 报表（依赖聚合 API，改动小）。
- **工作量**：本次增量约 1 天；后续 1/2 项各 2-3 天。

### P0-2 协议广度与 MQTT 网关可靠性
- **现状**：接入层抽象好（`access.Adapter` + 独立网关进程），但协议只有 3 种；MQTT 网关用 `mochi-mqtt`（内存实现）。
- **问题**：
  - **mochi 无持久会话**：QoS1 消息、会话状态（订阅/离线消息）进程重启即丢；客户端订阅的 retain 消息与集群场景受限——设备量大时重启=全量重连+状态重建。
  - 缺网关聚合场景：Modbus TCP/RTU、OPC-UA、LwM2M 这类工业协议未实现（`Adapter` 接口就绪，只需各写一个网关进程）。
- **改法**：P0-2a 补 Modbus TCP + OPC-UA 两个 `Adapter`（复用 custom 网关框架，各约 1 周）；P0-2b MQTT 网关加持久化（`mochi` 的 persistence 插件或迁移到 `emqx/kuiper` 类嵌入式——后者工作量小，推荐先做 mochi persistence）。

### P0-3 通知通道与告警闭环
- **现状**：webhook/钉钉/邮件（SMTP）+ 企微（markdown 机器人）+ 飞书（自定义机器人）通道已可用；`notification_logs` 全量记录 + 聚合去重。
- **本次完成**：企微/飞书通道（含裸 key 拼接）；统一 `postJSON` 发送封装——网络错误/5xx 指数退避重试（300ms/900ms）、4xx 快速失败；新增指标 `aiiot_notify_attempts_total{channel,result}`；UI 测试按钮（`POST /channels/:id/test`）联动。端到端验证（本地接收器 + 指标 + 日志）。
- **提醒**：通知链路是告警闭环的出口；如需 dead-letter 告警可在 `postJSON` 重试耗尽后追加一条 `aiiot_notify_failed` 业务事件，当前日志已可追踪。

### P1-4 安全加固：设备级认证 + 通道加密
- **现状**：MQTT 网关已有 TLS 监听（`MQTT_TLS_*` env）；但设备仍用静态 token，无 X.509 客户端证书认证，CoAP 无 DTLS，平台用户登录无限流。
- **改法**：
  1. ✅ 设备证书（X.509）认证：平台 CA（自签、私钥 0600 不落盘）、一次性签发 API、吊销（重签自动吊销旧证书）、MQTT mTLS（`MQTT_TLS_CA_FILE`）；CN 绑定设备 + `device_certs` 查询；全链路实测（接受/无证书 TLS 拒/吊销拒）；
  2. ✅ CoAP DTLS-PSK（pion/dtls + plgd；`COAP_DTLS_PSK_ADDR` 监听，identity=deviceKey、PSK=设备 secret，core 内部端点下发+5min 缓存；错误 PSK 握手不建立）——全链路实测（正确 PSK POST 落库、错误 PSK 拒绝）；
  3. 登录限流已存在（30/min）；TOTP——待办。

### P1-5 审计与批量管理
- **现状**：有时间线/下行日志（devices/events、downlinks 很扎实），但**管理面操作无审计日志**（谁在何时改了什么规则/固件/成员），企业交付审计要求难满足。
- **改法**：加 `audit_logs` 表 + 中间件在写路径（rules/ota/devices/members）记录 actor/action/对象/前后值；管理页可查。约 1 周。同期把"设备批量导入/导出（CSV）"补上（TB 标配，交付常被点名）。

## 3. 可暂缓（P2）
- ✅ 正式 SDK 包（Python/TS，零依赖）+ OpenAPI 3.0 导出（`/api/v1/openapi.yaml`）+ `docs/api.md` 英文文档（原 3 分项，已消除）。
- ✅ 规模化钥匙：`tsdb.Store` 时序后端抽象（可插拔）+ `docs/scaling.md`（Timescale 迁移、持久化 MQTT 策略）。
- Helm chart / Kubernetes 部署——docker-compose 够用，等真实集群需求。
- 规则引擎可视化节点编排——CEL 已够用且比 TB 简单；但**定时触发（cron）**值得提前做（常见需求，改动小）。
- 报表/导出数据分析——依赖 P0-1 的聚合 API，放其后。

## 4. 建议路线（3 周节奏）
| 周 | 主题 | 交付 |
|---|---|---|
| W1 | **可视化** | ✅ 完成：多标识聚合 API + 设备多指标多线曲线（基础能力复用现有 recharts 看板） |
| W2 | **协议 + 通道** | ✅ Modbus TCP 网关 + 企微/飞书通知通道 + 通知重试(5xx退避)/指标 全部完成 |
| W3 | **安全 + 运营** | ✅ 设备 X.509 证书认证 + ✅ CoAP DTLS-PSK 全部完成（均全链路实测）；登录限流/审计核实已存在；CSV 批量导入导出、TOTP 待办 |

> 原则：优先补"用户能直接看到/客户会点名"的能力（仪表盘、通知、批量），再补"安全合规"（证书、审计），协议广度按项目需求插队（`Adapter` 框架已把成本降到单协议 1 周内）。