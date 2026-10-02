package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"text/template"
	"time"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/metrics"
	"github.com/aiiot/server/internal/models"
	"github.com/google/cel-go/cel"
	"gorm.io/gorm"
)

// RuleEvent is the normalized input to the rule engine.
type RuleEvent struct {
	Kind        access.UplinkKind
	State       string
	ProjectID   uint
	WorkspaceID uint
	ProductID   uint
	DeviceID    uint
	DeviceKey   string
	Identifier  string
	Value       any
	Params      map[string]any
	Payload     any
	Now         time.Time
	Device      *models.Device
	// OutOfRange reports whether the reported value fell outside the
	// thing-model min/max range for this identifier.
	OutOfRange bool
}

// RuleEngine compiles CEL conditions and executes rule actions.
type RuleEngine struct {
	db       *gorm.DB
	downlink *DownlinkService
	shadow   *ShadowService
	resolver *Resolver
	notifier *Notifier
	alerter  *Alerter
	log      *slog.Logger
	env      *cel.Env
	client   *http.Client

	mu       sync.RWMutex
	programs map[uint]*compiledRule

	pcacheMu sync.RWMutex
	pcache   map[string]cel.Program
}

type compiledRule struct {
	rule *models.Rule
	prg  cel.Program
}

// NewRuleEngine builds the CEL environment shared by all rules.
func NewRuleEngine(db *gorm.DB, downlink *DownlinkService, shadow *ShadowService, resolver *Resolver, notifier *Notifier, alerter *Alerter, log *slog.Logger) (*RuleEngine, error) {
	env, err := cel.NewEnv(
		cel.Variable("project_id", cel.IntType),
		cel.Variable("workspace_id", cel.IntType),
		cel.Variable("product_id", cel.IntType),
		cel.Variable("device_id", cel.IntType),
		cel.Variable("device_key", cel.StringType),
		cel.Variable("identifier", cel.StringType),
		cel.Variable("kind", cel.StringType),
		cel.Variable("state", cel.StringType),
		cel.Variable("value", cel.DynType),
		cel.Variable("params", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("payload", cel.DynType),
		cel.Variable("now", cel.TimestampType),
		cel.Variable("out_of_range", cel.BoolType), // thing-model min/max violation
	)
	if err != nil {
		return nil, fmt.Errorf("cel env: %w", err)
	}
	return &RuleEngine{
		db:       db,
		downlink: downlink,
		shadow:   shadow,
		resolver: resolver,
		notifier: notifier,
		alerter:  alerter,
		log:      log,
		env:      env,
		client:   &http.Client{Timeout: 10 * time.Second},
		programs: map[uint]*compiledRule{},
		pcache:   map[string]cel.Program{},
	}, nil
}

// Validate reports whether a CEL expression compiles in the rule environment.
func (e *RuleEngine) Validate(expr string) error {
	ast, iss := e.env.Compile(expr)
	if iss.Err() != nil {
		return iss.Err()
	}
	_, err := e.env.Program(ast)
	return err
}

// Reload recompiles all enabled rules. Call at startup and after rule writes.
func (e *RuleEngine) Reload(ctx context.Context) error {
	var rules []models.Rule
	if err := e.db.WithContext(ctx).Where("enabled = ?", true).Find(&rules).Error; err != nil {
		return err
	}
	next := make(map[uint]*compiledRule, len(rules))
	for i := range rules {
		r := rules[i]
		ast, iss := e.env.Compile(r.Condition)
		if iss.Err() != nil {
			e.log.Error("rule compile failed", "rule", r.ID, "name", r.Name, "error", iss.Err())
			continue
		}
		prg, err := e.env.Program(ast)
		if err != nil {
			e.log.Error("rule program failed", "rule", r.ID, "error", err)
			continue
		}
		next[r.ID] = &compiledRule{rule: &r, prg: prg}
	}
	e.mu.Lock()
	e.programs = next
	e.mu.Unlock()
	e.log.Info("rules reloaded", "count", len(next))
	return nil
}

// Evaluate runs all matching rules for an event.
func (e *RuleEngine) Evaluate(ctx context.Context, ev *RuleEvent) {
	if ev.Now.IsZero() {
		ev.Now = time.Now().UTC()
	}
	metrics.ObserveRuleEvaluation(string(ev.Kind))
	e.mu.RLock()
	candidates := make([]*compiledRule, 0, len(e.programs))
	for _, cr := range e.programs {
		if e.match(cr.rule, ev) {
			candidates = append(candidates, cr)
		}
	}
	e.mu.RUnlock()

	if len(candidates) == 0 {
		return
	}

	activation := e.activation(ev)
	for _, cr := range candidates {
		out, _, err := cr.prg.Eval(activation)
		if err != nil {
			e.log.Warn("rule eval error", "rule", cr.rule.ID, "error", err)
			continue
		}
		matched, _ := out.Value().(bool)
		if !matched {
			// A telemetry condition that no longer holds auto-resolves the
			// open alert for this rule + device.
			if e.alerter != nil && cr.rule.TriggerType == models.TriggerTelemetry && ev.DeviceID != 0 {
				if err := e.alerter.ResolveIf(ctx, cr.rule, ev); err != nil {
					e.log.Warn("resolve alert failed", "rule", cr.rule.ID, "error", err)
				}
			}
			continue
		}
		if e.alerter != nil && ev.DeviceID != 0 {
			if err := e.alerter.Raise(ctx, cr.rule, ev); err != nil {
				e.log.Warn("raise alert failed", "rule", cr.rule.ID, "error", err)
			}
		}
		metrics.ObserveRuleTrigger(cr.rule.Name)
		results, actionErr := e.runActions(ctx, cr.rule, ev, activation)
		e.record(ctx, cr.rule, ev, results, actionErr)
	}
}

func (e *RuleEngine) match(rule *models.Rule, ev *RuleEvent) bool {
	if rule.ProjectID != ev.ProjectID {
		return false
	}
	if rule.ProductID != nil && *rule.ProductID != ev.ProductID {
		return false
	}
	if rule.WorkspaceID != nil && *rule.WorkspaceID != ev.WorkspaceID {
		return false
	}
	if rule.TriggerSource != "" && rule.TriggerSource != "*" && rule.TriggerSource != ev.Identifier {
		return false
	}
	return triggerMatches(rule.TriggerType, ev)
}

func triggerMatches(trigger string, ev *RuleEvent) bool {
	switch ev.Kind {
	case access.KindProperty:
		return trigger == models.TriggerTelemetry
	case access.KindEvent, access.KindServiceReply:
		return trigger == models.TriggerTelemetry || trigger == models.TriggerEvent
	case access.KindLifecycle:
		if ev.State == access.StateOnline {
			return trigger == models.TriggerDeviceOnline
		}
		return trigger == models.TriggerDeviceOffline
	case access.KindPeer:
		return trigger == models.TriggerPeerMessage || trigger == models.TriggerDeviceMessage
	case access.KindShadowDelta:
		return trigger == models.TriggerShadowDelta
	}
	return false
}

func (e *RuleEngine) activation(ev *RuleEvent) map[string]any {
	params := ev.Params
	if params == nil {
		params = map[string]any{}
	}
	return map[string]any{
		"project_id":   int64(ev.ProjectID),
		"workspace_id": int64(ev.WorkspaceID),
		"product_id":   int64(ev.ProductID),
		"device_id":    int64(ev.DeviceID),
		"device_key":   ev.DeviceKey,
		"identifier":   ev.Identifier,
		"kind":         string(ev.Kind),
		"state":        ev.State,
		"value":        ev.Value,
		"params":       params,
		"payload":      ev.Payload,
		"now":          ev.Now,
		"out_of_range": ev.OutOfRange,
	}
}

func (e *RuleEngine) compileExpr(src string) (cel.Program, error) {
	e.pcacheMu.RLock()
	p, ok := e.pcache[src]
	e.pcacheMu.RUnlock()
	if ok {
		return p, nil
	}
	ast, iss := e.env.Compile(src)
	if iss.Err() != nil {
		return nil, iss.Err()
	}
	prg, err := e.env.Program(ast)
	if err != nil {
		return nil, err
	}
	e.pcacheMu.Lock()
	e.pcache[src] = prg
	e.pcacheMu.Unlock()
	return prg, nil
}

func (e *RuleEngine) runActions(ctx context.Context, rule *models.Rule, ev *RuleEvent, activation map[string]any) (models.JSONList, error) {
	results := make(models.JSONList, 0, len(rule.Actions))
	var firstErr error
	for _, action := range rule.Actions {
		res := map[string]any{"type": action["type"]}
		if err := e.runAction(ctx, rule, action, ev, activation); err != nil {
			res["ok"] = false
			res["error"] = err.Error()
			if firstErr == nil {
				firstErr = err
			}
		} else {
			res["ok"] = true
		}
		results = append(results, res)
	}
	return results, firstErr
}

func (e *RuleEngine) runAction(ctx context.Context, rule *models.Rule, action map[string]any, ev *RuleEvent, activation map[string]any) error {
	kind, _ := action["type"].(string)
	switch kind {
	case models.ActionLog:
		e.log.Info("rule action log", "device", ev.DeviceKey, "identifier", ev.Identifier, "value", ev.Value)
		return nil
	case models.ActionWebhook:
		return e.runWebhook(ctx, action, ev, activation)
	case models.ActionSetDesired:
		return e.runSetDesired(ctx, rule, action, ev, activation)
	case models.ActionNotify:
		return e.runNotify(ctx, rule, action, ev, activation)
	case models.ActionMQTTPublish, models.ActionDeviceCommand, "downlink", "publish":
		return e.runDownlink(ctx, action, ev, activation)
	default:
		return fmt.Errorf("unknown action type %q", kind)
	}
}

// runNotify sends an alert through a configured notification channel.
func (e *RuleEngine) runNotify(ctx context.Context, rule *models.Rule, action map[string]any, ev *RuleEvent, activation map[string]any) error {
	if e.notifier == nil {
		return fmt.Errorf("notifier unavailable")
	}
	channelID := uint(asNumber(action["channelId"]))
	if channelID == 0 {
		return fmt.Errorf("notify requires channelId")
	}
	data := templateData(rule, ev)
	title := ""
	if action["titleTemplate"] != nil {
		rendered, err := renderTemplate(fmt.Sprint(action["titleTemplate"]), data)
		if err != nil {
			return fmt.Errorf("titleTemplate: %w", err)
		}
		title = rendered
	} else {
		title = fmt.Sprint(action["title"])
	}
	if title == "" || title == "<nil>" {
		title = "Alert: " + rule.Name
	}
	var body string
	switch {
	case action["bodyTemplate"] != nil:
		rendered, err := renderTemplate(fmt.Sprint(action["bodyTemplate"]), data)
		if err != nil {
			return fmt.Errorf("bodyTemplate: %w", err)
		}
		body = rendered
	case action["messageExpr"] != nil:
		prg, err := e.compileExpr(fmt.Sprint(action["messageExpr"]))
		if err != nil {
			return err
		}
		out, _, err := prg.Eval(activation)
		if err != nil {
			return err
		}
		body = fmt.Sprint(out.Value())
	case action["message"] != nil:
		body = fmt.Sprint(action["message"])
	default:
		enc, _ := json.Marshal(map[string]any{
			"device":     ev.DeviceKey,
			"identifier": ev.Identifier,
			"value":      ev.Value,
			"params":     ev.Params,
			"time":       ev.Now,
		})
		body = string(enc)
	}
	opts := SendOptions{
		SilenceSeconds:   int(asNumber(action["silenceSeconds"])),
		AggregateSeconds: int(asNumber(action["aggregateSeconds"])),
	}
	return e.notifier.Send(ctx, channelID, title, body, &rule.ID, ev.DeviceID, opts)
}

// templateData exposes the event to notification templates.
func templateData(rule *models.Rule, ev *RuleEvent) map[string]any {
	return map[string]any{
		"ruleName":   rule.Name,
		"deviceKey":  ev.DeviceKey,
		"deviceId":   ev.DeviceID,
		"identifier": ev.Identifier,
		"value":      ev.Value,
		"params":     ev.Params,
		"kind":       string(ev.Kind),
		"state":      ev.State,
		"now":        ev.Now,
	}
}

// renderTemplate renders a Go text/template against the event data.
func renderTemplate(src string, data map[string]any) (string, error) {
	t, err := template.New("notify").Option("missingkey=zero").Parse(src)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func asNumber(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

// shadow service then pushes any delta to the device.
func (e *RuleEngine) runSetDesired(ctx context.Context, rule *models.Rule, action map[string]any, ev *RuleEvent, activation map[string]any) error {
	if e.shadow == nil || e.resolver == nil {
		return fmt.Errorf("shadow service unavailable")
	}
	var patch map[string]any
	switch {
	case action["desiredExpr"] != nil:
		prg, err := e.compileExpr(fmt.Sprint(action["desiredExpr"]))
		if err != nil {
			return fmt.Errorf("desiredExpr: %w", err)
		}
		out, _, err := prg.Eval(activation)
		if err != nil {
			return fmt.Errorf("desiredExpr eval: %w", err)
		}
		if m, ok := out.Value().(map[string]any); ok {
			patch = m
		} else {
			return fmt.Errorf("desiredExpr must evaluate to an object")
		}
	case action["desired"] != nil:
		switch d := action["desired"].(type) {
		case map[string]any:
			patch = d
		default:
			enc, _ := json.Marshal(d)
			if err := json.Unmarshal(enc, &patch); err != nil {
				return fmt.Errorf("invalid desired patch: %w", err)
			}
		}
	default:
		return fmt.Errorf("set_desired requires a desired object or desiredExpr")
	}
	dc, err := e.resolver.ResolveByDeviceID(ctx, ev.DeviceID)
	if err != nil {
		return err
	}
	_, err = e.shadow.ApplyDesired(ctx, dc, patch, models.ShadowSourceRule, &rule.ID)
	return err
}

func (e *RuleEngine) runWebhook(ctx context.Context, action map[string]any, ev *RuleEvent, activation map[string]any) error {
	url, _ := action["url"].(string)
	if url == "" {
		return fmt.Errorf("webhook action missing url")
	}
	method, _ := action["method"].(string)
	if method == "" {
		method = http.MethodPost
	}

	var body []byte
	switch {
	case action["bodyExpr"] != nil:
		prg, err := e.compileExpr(fmt.Sprint(action["bodyExpr"]))
		if err != nil {
			return fmt.Errorf("bodyExpr: %w", err)
		}
		out, _, err := prg.Eval(activation)
		if err != nil {
			return fmt.Errorf("bodyExpr eval: %w", err)
		}
		body, _ = json.Marshal(out.Value())
	case action["body"] != nil:
		switch b := action["body"].(type) {
		case string:
			body = []byte(b)
		default:
			body, _ = json.Marshal(b)
		}
	default:
		body, _ = json.Marshal(map[string]any{
			"rule_id":    action["ruleId"],
			"device_key": ev.DeviceKey,
			"device_id":  ev.DeviceID,
			"identifier": ev.Identifier,
			"kind":       string(ev.Kind),
			"value":      ev.Value,
			"params":     ev.Params,
			"payload":    ev.Payload,
			"time":       ev.Now,
		})
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if headers, ok := action["headers"].(map[string]any); ok {
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprint(v))
		}
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook status %d", resp.StatusCode)
	}
	return nil
}

func (e *RuleEngine) runDownlink(ctx context.Context, action map[string]any, ev *RuleEvent, activation map[string]any) error {
	var payload []byte
	switch {
	case action["payloadExpr"] != nil:
		prg, err := e.compileExpr(fmt.Sprint(action["payloadExpr"]))
		if err != nil {
			return fmt.Errorf("payloadExpr: %w", err)
		}
		out, _, err := prg.Eval(activation)
		if err != nil {
			return fmt.Errorf("payloadExpr eval: %w", err)
		}
		payload, _ = json.Marshal(out.Value())
	case action["payload"] != nil:
		switch p := action["payload"].(type) {
		case string:
			payload = []byte(p)
		default:
			payload, _ = json.Marshal(p)
		}
	default:
		payload, _ = json.Marshal(map[string]any{"value": ev.Value, "params": ev.Params})
	}

	kind := access.KindProperty
	if k, ok := action["kind"].(string); ok && k != "" {
		kind = access.UplinkKind(k)
	}
	identifier, _ := action["identifier"].(string)
	meta := map[string]string{"rule": fmt.Sprint(action["ruleId"]), "source": models.DownlinkSourceRule}

	// Target: explicit deviceKey (within the same workspace) or the source device.
	if dk, ok := action["deviceKey"].(string); ok && dk != "" && dk != ev.DeviceKey {
		target, err := e.downlink.ResolveDeviceKey(ctx, ev.WorkspaceID, dk)
		if err != nil {
			return err
		}
		return e.downlink.SendTo(ctx, target, kind, identifier, payload, meta)
	}
	return e.downlink.Send(ctx, ev.DeviceID, kind, identifier, payload, meta)
}

func (e *RuleEngine) record(ctx context.Context, rule *models.Rule, ev *RuleEvent, results models.JSONList, actionErr error) {
	now := time.Now().UTC()
	logRow := &models.RuleExecutionLog{
		RuleID:        rule.ID,
		ProjectID:     rule.ProjectID,
		DeviceID:      ev.DeviceID,
		Identifier:    ev.Identifier,
		Matched:       true,
		Success:       actionErr == nil,
		Context:       models.JSONMap{"kind": string(ev.Kind), "value": ev.Value, "params": ev.Params},
		ActionsResult: results,
		OccurredAt:    now,
	}
	if actionErr != nil {
		logRow.Error = actionErr.Error()
	}
	if err := e.db.WithContext(ctx).Create(logRow).Error; err != nil {
		e.log.Warn("rule execution log failed", "error", err)
	}
	e.db.WithContext(ctx).Model(&models.Rule{}).Where("id = ?", rule.ID).Updates(map[string]any{
		"last_triggered_at": now,
		"trigger_count":     gorm.Expr("trigger_count + 1"),
	})
}
