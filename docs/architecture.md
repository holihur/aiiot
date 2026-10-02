# AI IoT Platform — Architecture (Mermaid)

> 可在 GitLab、语雀、GitHub 等支持 Mermaid 的 Markdown 平台直接渲染。
> 与 README 中的 ASCII 图对应；更详细的接入协议说明见 [custom-protocol.md](custom-protocol.md)。

```mermaid
flowchart TD
    %% ── L1: Devices ───────────────────────────────────
    subgraph L1["Devices"]
        D1["MQTT"]
        D2["CoAP / UDP"]
        D3["Custom TCP"]
        D4["HTTP（桥接/直连）"]
    end

    %% ── L2: Access layer ──────────────────────────────
    subgraph L2["接入层 · 独立网关进程"]
        G1["mqtt-gw（内置 broker）"]
        G2["coap-gw（UDP）"]
        G3["custom-gw（TCP）"]
        H1["HTTP ingest<br/>/api/v1/ingest/…"]
    end

    %% ── L3: Message bus ───────────────────────────────
    BUS["NATS JetStream（必选总线）<br/>stream AIOT_UPLINK · subject aiiot.uplink<br/>durable consumer core-uplink（队列组）"]

    %% ── L4: Core ──────────────────────────────────────
    subgraph L4["核心 · 控制平面"]
        C["REST API · ingest · resolver · 设备影子<br/>规则引擎 CEL · OTA · 网关注册表 · SSE"]
        A["管理控制台（独立账号体系）<br/>NATS 运维页 · 存储 · 审计"]
        M["/metrics（Prometheus）"]
    end

    %% ── L5: Storage & targets ─────────────────────────
    PG[("PostgreSQL<br/>遥测按月分区 + 小时级 rollup")]
    NT["通知目标<br/>webhook · 钉钉 · 邮件（SMTP）"]

    %% Data path: device → gateway → bus → core
    D1 -->|MQTT| G1
    D2 -->|CoAP| G2
    D3 -->|TCP| G3
    D4 -->|HTTP| H1

    G1 -->|"上行 · UplinkEnvelope JSON"| BUS
    G2 -->|上行| BUS
    G3 -->|上行| BUS
    H1 -->|进程内 ingest| C

    BUS -->|"durable 队列组消费者"| C

    %% Control plane: gateway → core (HTTP)
    G1 -->|"注册 · 心跳 · 认证"| C
    G2 -->|注册 · 心跳 · 认证| C
    G3 -->|注册 · 心跳 · 认证| C

    %% Downlink: core → gateway → device
    C -->|下行 HTTP| G1
    C -->|下行 HTTP| G2
    C -->|下行 HTTP| G3
    G1 -->|"property_set · service_call · OTA · peer"| D1
    G2 -->|下行| D2
    G3 -->|下行| D3

    %% Storage & notify
    C -->|遥测写入| PG
    C -->|notify 动作| NT

    %% Observability
    M --> PROM["Prometheus / Grafana"]
    G1 -->|"/metrics（9101）"| PROM
    G2 -->|"/metrics（9102）"| PROM
    G3 -->|"/metrics（9103）"| PROM
    A -->|"GET /admin/nats-stats ← bus.Stats"| BUS

    %% Frontend
    SPA["SPA · 业务控制台"] -->|HTTP :8080| C
    A -->|HTTP :8080 /admin/*| C

    classDef device fill:#eef2ff,stroke:#6366f1,stroke-width:1.5px;
    classDef gateway fill:#f0fdf4,stroke:#16a34a,stroke-width:1.5px;
    classDef bus fill:#fff7ed,stroke:#f97316,stroke-width:2px;
    classDef core fill:#fef2f2,stroke:#dc2626,stroke-width:1.5px;
    classDef store fill:#f8fafc,stroke:#64748b,stroke-width:1.5px;
    classDef obs fill:#fdf4ff,stroke:#c026d3,stroke-width:1.5px;

    class D1,D2,D3,D4 device;
    class G1,G2,G3,H1 gateway;
    class BUS bus;
    class C,A,M core;
    class PG,NT store;
    class PROM,SPA obs;
```

## 关键数据流

1. **数据平面（上行）**：设备 → 网关 → `NATS JetStream (aiiot.uplink)` → core 队列组消费 → ingest → PostgreSQL + 规则引擎。HTTP 直连上传不经 NATS，进程内直接进入 ingest。
2. **控制平面（HTTP，请求-响应）**：网关注册 / 心跳 / 设备认证走 HTTP。
3. **下行**：core → 网关 `9101..9103/downlink` → 设备（规则 `set_desired`、服务调用、OTA、workspace 内 peer 路由）。
4. **观测**：core 与每个网关各自暴露 `/metrics`；管理控制台 `/admin/nats` 直接读取 core 自身 NATS 连接的 stream/consumer 状态（轻量 nats-top，无需额外部署）。