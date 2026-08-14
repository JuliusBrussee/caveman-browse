package browse

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JuliusBrussee/caveman/engine"
	"github.com/JuliusBrussee/caveman/engine/tokens"
	"github.com/JuliusBrussee/caveman/mcp"
)

const (
	ToolSnapshot = "browser_snapshot"
	ToolAct      = "browser_act"
	ToolEval     = "browser_eval"
	ToolRecover  = "browser_recover"

	maxSnapshotWaitMS  = 30_000
	maxQueryBytes      = 4 << 10
	maxDataURLBytes    = 1 << 20
	maxActionTextBytes = 1 << 20
)

type Driver interface {
	Snapshot(ctx context.Context, url string, wait time.Duration) ([]byte, error)
	Act(ctx context.Context, req ActionRequest, target Target) (ActionResult, error)
	Eval(ctx context.Context, expression string) (any, error)
	Close() error
}

type Session struct {
	eng    *engine.Engine
	driver Driver
	log    *slog.Logger

	mu      sync.Mutex
	targets map[string]Target
}

type Target struct {
	BackendDOMNodeID int    `json:"backendDOMNodeId"`
	FrameID          string `json:"frameId,omitempty"`
	NodeID           string `json:"nodeId,omitempty"`
}

type ActionRequest struct {
	Action string `json:"action"`
	UID    string `json:"uid"`
	Text   string `json:"text"`
	Option string `json:"option"`
}

type ActionResult struct {
	OK      bool   `json:"ok"`
	UID     string `json:"uid,omitempty"`
	Settled bool   `json:"settled"`
	Note    string `json:"note,omitempty"`
}

func NewSession(eng *engine.Engine, driver Driver, log *slog.Logger) *Session {
	if log == nil {
		log = slog.New(slog.NewTextHandler(discard{}, nil))
	}
	return &Session{eng: eng, driver: driver, log: log, targets: map[string]Target{}}
}

func (s *Session) Close() error {
	if s.driver == nil {
		return nil
	}
	return s.driver.Close()
}

func BrowserTools(s *Session) []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        ToolSnapshot,
			Description: "Compact recovery-backed AX tree. URL navigates; query keeps task matches. Counts inferred.",
			InputSchema: mcp.ObjectSchema(map[string]any{
				"url":         mcp.StringProp("http(s), about:blank, data:text/html"),
				"wait":        map[string]any{"type": "number", "description": "Wait ms 0-30000."},
				"query":       mcp.StringProp("Focus terms."),
				"interactive": map[string]any{"type": "boolean", "description": "Actionable nodes only; hides text."},
			}),
			Handler: s.snapshotTool,
		},
		{
			Name:        ToolAct,
			Description: "Act on latest UID. Resnapshot verifies state.",
			InputSchema: mcp.ObjectSchema(map[string]any{
				"action": mcp.StringProp("click|type|select|scroll|wait"),
				"uid":    mcp.StringProp("Latest UID."),
				"text":   mcp.StringProp("Type/select text."),
				"option": mcp.StringProp("Select value/label."),
			}, "action"),
			Handler: s.actTool,
		},
		{
			Name:        ToolEval,
			Description: "Run JavaScript in page.",
			InputSchema: mcp.ObjectSchema(map[string]any{
				"expression": mcp.StringProp("JavaScript expression."),
			}, "expression"),
			Handler: s.evalTool,
		},
		{
			Name:        ToolRecover,
			Description: "Recover exact raw AX bytes; query narrows.",
			InputSchema: mcp.ObjectSchema(map[string]any{
				"recovery_handle": mcp.StringProp("Recovery handle."),
				"query":           mcp.StringProp("Narrow terms."),
			}, "recovery_handle"),
			Handler:         s.recoverTool,
			ExemptResultCap: true,
		},
	}
}

// snapshotPayload is the accounting carrier for one snapshot. It is rendered as
// text, not JSON — see renderSnapshot.
type snapshotPayload struct {
	UIDs           string
	RecoveryHandle *string
	TokensBefore   int
	ViewTokens     int
	TokensAfter    int
	Ratio          float64
	Basis          string
}

func (s *Session) snapshotTool(args json.RawMessage) mcp.ToolResult {
	var a struct {
		URL         string  `json:"url"`
		WaitMS      float64 `json:"wait"`
		Query       string  `json:"query"`
		Interactive bool    `json:"interactive"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return mcp.ToolError("cave_invalid_arguments", "snapshot: invalid arguments")
	}
	if code, message := validateSnapshotArgs(a.URL, a.WaitMS, a.Query); code != "" {
		return mcp.ToolError(code, message)
	}
	if s.driver == nil || s.eng == nil {
		return mcp.ToolError("cave_browser_unavailable", "browser session unavailable")
	}
	wait := time.Duration(a.WaitMS * float64(time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), snapshotTimeout(wait))
	defer cancel()

	raw, err := s.driver.Snapshot(ctx, a.URL, wait)
	if err != nil {
		s.log.Warn("browser snapshot failed", "err", err)
		return mcp.ToolError("cave_browser_snapshot_failed", "snapshot failed")
	}
	res, err := s.eng.Compress(raw, engine.Options{Mode: engine.ModeCompress, Type: engine.TypeA11y, Query: a.Query})
	if err != nil {
		s.log.Warn("a11y compress failed", "err", err)
		return mcp.ToolError("cave_browser_snapshot_failed", "snapshot recovery unavailable")
	}
	if res.RecoveryHandle == "" {
		// Pass-through: the engine did not produce a recovery-backed uid view
		// (a tree that did not get smaller, or no CCR store). The uid map is this
		// tool's contract, not a side effect of a compression ratio, so we must
		// NOT (a) dump the raw AX tree into `uids` — hundreds of KB of raw JSON is
		// strictly worse than not using Browse — nor (b) wipe the prior page's uid
		// cache. Fail closed and keep prior targets so acting stays predictable
		// (issue #140).
		s.log.Warn("a11y snapshot did not compress; refusing raw-tree pass-through",
			"ratio", res.Ratio, "tokens_before", res.TokensBefore)
		return mcp.ToolError("cave_browser_snapshot_uncompressed",
			"snapshot did not compress to a uid view; raw tree withheld and prior uids retained")
	}
	h := res.RecoveryHandle
	targets, err := s.targetsForHandle(h)
	if err != nil {
		s.log.Warn("a11y uid metadata unavailable", "err", err)
		return mcp.ToolError("cave_browser_snapshot_failed", "snapshot uid map unavailable; prior uids retained")
	}
	view := string(res.Output)
	viewTokens := res.TokensAfter
	if a.Interactive {
		if narrowed := keepInteractive(view); narrowed != view {
			view = narrowed
			viewTokens = tokens.Default().Count([]byte(view))
		}
	}
	payload, text := finalizeSnapshotPayload(snapshotPayload{
		UIDs:           view,
		RecoveryHandle: &h,
		TokensBefore:   res.TokensBefore,
		ViewTokens:     viewTokens,
		Basis:          res.Basis,
	})
	if payload.TokensAfter >= payload.TokensBefore {
		return mcp.ToolError("cave_browser_snapshot_uncompressed",
			"agent-visible snapshot was not smaller than raw AX; prior uids retained")
	}
	s.replaceTargets(targets)
	return mcp.ToolRawText(text)
}

func snapshotTimeout(wait time.Duration) time.Duration {
	base := 15 * time.Second
	if wait > 0 {
		base += wait
	}
	return base
}

func (s *Session) targetsForHandle(handle string) (map[string]Target, error) {
	meta, err := s.eng.RetrieveMetadata(handle)
	if err != nil {
		return nil, err
	}
	if len(meta) == 0 {
		return nil, errors.New("empty a11y recovery metadata")
	}
	var decoded struct {
		UIDs map[string]Target `json:"uids"`
	}
	if err := json.Unmarshal(meta, &decoded); err != nil {
		return nil, err
	}
	if decoded.UIDs == nil {
		decoded.UIDs = map[string]Target{}
	}
	return decoded.UIDs, nil
}

// renderSnapshot writes the agent-visible payload: the compact AX view plus one
// trailing accounting line carrying every honesty field.
//
// The view is TEXT, not a JSON envelope. renderAXRecords already strconv.Quote's
// every accessible name, so wrapping the result in JSON escaped each one a second
// time (`\"Cancel order\"`) and turned every line break into `\n`. Measured on a
// 200-row operations view with the same o200k counter this file reports with:
// 6,800 raw vs 7,650 JSON-wrapped tokens — a 12.5% tax on the payload, plus 49
// tokens for the envelope keys themselves. Playwright and agent-browser both
// return bare text for the same reason.
func renderSnapshot(p snapshotPayload) string {
	handle := ""
	if p.RecoveryHandle != nil {
		handle = *p.RecoveryHandle
	}
	return p.UIDs + "\ncaveman before=" + strconv.Itoa(p.TokensBefore) +
		" view=" + strconv.Itoa(p.ViewTokens) +
		" after=" + strconv.Itoa(p.TokensAfter) +
		" ratio=" + strconv.FormatFloat(p.Ratio, 'f', 4, 64) +
		" basis=" + p.Basis +
		" handle=" + handle
}

// finalizeSnapshotPayload settles the self-referential accounting: the line
// states the delivered token count, and stating it changes that count. Ratio is
// rounded to the 4 decimals actually printed so the struct never claims more
// precision than the agent can see; tokens_before/after stay exact.
func finalizeSnapshotPayload(payload snapshotPayload) (snapshotPayload, string) {
	counter := tokens.Default()
	for range 8 {
		text := renderSnapshot(payload)
		delivered := counter.Count([]byte(text))
		ratio := 0.0
		if payload.TokensBefore > 0 && delivered < payload.TokensBefore {
			ratio = math.Round(float64(payload.TokensBefore-delivered)/float64(payload.TokensBefore)*1e4) / 1e4
		}
		if payload.TokensAfter == delivered && payload.Ratio == ratio {
			return payload, text
		}
		payload.TokensAfter = delivered
		payload.Ratio = ratio
	}
	return payload, renderSnapshot(payload)
}

// keepInteractive narrows the compact AX view to uid-bearing lines plus the
// ancestor lines that place them — the predicate `agent-browser snapshot -i`
// sells, computed here from the uid handles shouldExposeAXUID already assigned.
//
// It is a flag and never a default: an actionable-only view hides the text an
// agent has to READ, so `query` remains the recommended way to shrink a page. If
// no uid survives (an all-text page), the full view is returned unchanged —
// failing open to the whole page beats returning an empty one.
func keepInteractive(view string) string {
	lines := strings.Split(view, "\n")
	keep := make([]bool, len(lines))
	found := false
	for i, line := range lines {
		indent := axIndent(line)
		if !strings.HasPrefix(line[indent:], "[") {
			continue
		}
		keep[i] = true
		found = true
		for j, want := i-1, indent-2; j >= 0 && want >= 0; j-- {
			if axIndent(lines[j]) == want {
				keep[j] = true
				want -= 2
			}
		}
	}
	if !found {
		return view
	}
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if keep[i] {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// axIndent counts the two-space-per-depth indent renderAXRecords writes.
func axIndent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

func validateSnapshotArgs(rawURL string, waitMS float64, query string) (string, string) {
	if math.IsNaN(waitMS) || math.IsInf(waitMS, 0) || waitMS < 0 || waitMS > maxSnapshotWaitMS {
		return "cave_invalid_arguments", "snapshot: wait must be between 0 and 30000 milliseconds"
	}
	if len(query) > maxQueryBytes {
		return "cave_invalid_arguments", "snapshot: query too large"
	}
	if rawURL == "" {
		return "", ""
	}
	if len(rawURL) > maxDataURLBytes {
		return "cave_invalid_arguments", "snapshot: url too large"
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" {
		return "cave_invalid_arguments", "snapshot: absolute URL required"
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		if parsed.Host == "" {
			return "cave_invalid_arguments", "snapshot: URL host required"
		}
		return "", ""
	case "about":
		if strings.EqualFold(rawURL, "about:blank") {
			return "", ""
		}
	case "data":
		if strings.HasPrefix(strings.ToLower(rawURL), "data:text/html,") ||
			strings.HasPrefix(strings.ToLower(rawURL), "data:text/html;") {
			return "", ""
		}
	}
	return "cave_browser_url_denied", "snapshot: URL scheme denied; use http(s), about:blank, or data:text/html"
}

func (s *Session) replaceTargets(targets map[string]Target) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.targets = map[string]Target{}
	for uid, target := range targets {
		s.targets[uid] = target
	}
}

func (s *Session) LoadTargets(targets map[string]Target) {
	s.replaceTargets(targets)
}

func (s *Session) TargetsSnapshot() map[string]Target {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]Target, len(s.targets))
	for uid, target := range s.targets {
		out[uid] = target
	}
	return out
}

func (s *Session) actTool(args json.RawMessage) mcp.ToolResult {
	var req ActionRequest
	if err := json.Unmarshal(args, &req); err != nil || req.Action == "" {
		return mcp.ToolError("cave_invalid_arguments", "act: missing action")
	}
	if len(req.Text) > maxActionTextBytes || len(req.Option) > maxActionTextBytes {
		return mcp.ToolError("cave_invalid_arguments", "act: text too large")
	}
	if s.driver == nil {
		return mcp.ToolError("cave_browser_unavailable", "browser session unavailable")
	}
	if req.Action == "wait" {
		time.Sleep(250 * time.Millisecond)
		return mcp.ToolText(ActionResult{OK: true, UID: req.UID, Settled: true})
	}
	if req.Action != "click" && req.Action != "type" && req.Action != "select" && req.Action != "scroll" {
		return mcp.ToolError("cave_unknown_action", "unsupported browser action")
	}
	if req.UID == "" {
		return mcp.ToolError("cave_invalid_arguments", "act: uid required")
	}
	if req.Action == "type" && req.Text == "" {
		return mcp.ToolError("cave_invalid_arguments", "act: text required for type")
	}
	target, ok := s.lookupTarget(req.UID)
	if !ok {
		return mcp.ToolError("cave_unknown_uid", "no element found for uid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.driver.Act(ctx, req, target)
	if err != nil {
		s.log.Warn("browser action failed", "action", req.Action, "uid", req.UID, "err", err)
		return mcp.ToolError("cave_browser_action_failed", "action failed")
	}
	if !res.OK {
		return mcp.ToolError("cave_browser_action_failed", "action was not completed")
	}
	res.UID = req.UID
	return mcp.ToolText(res)
}

func (s *Session) lookupTarget(uid string) (Target, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	target, ok := s.targets[uid]
	return target, ok
}

func (s *Session) evalTool(args json.RawMessage) mcp.ToolResult {
	var a struct {
		Expression string `json:"expression"`
	}
	if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.Expression) == "" || len(a.Expression) > maxActionTextBytes {
		return mcp.ToolError("cave_invalid_arguments", "eval: missing expression")
	}
	if s.driver == nil {
		return mcp.ToolError("cave_browser_unavailable", "browser session unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := s.driver.Eval(ctx, a.Expression)
	if err != nil {
		s.log.Warn("browser eval failed", "err", err)
		return mcp.ToolError("cave_browser_eval_failed", "eval failed")
	}
	return mcp.ToolText(map[string]any{"result": result})
}

func (s *Session) recoverTool(args json.RawMessage) mcp.ToolResult {
	var a struct {
		RecoveryHandle string `json:"recovery_handle"`
		Query          string `json:"query"`
	}
	if err := json.Unmarshal(args, &a); err != nil || a.RecoveryHandle == "" || len(a.RecoveryHandle) > 512 || len(a.Query) > maxQueryBytes {
		return mcp.ToolError("cave_invalid_arguments", "recover: missing recovery_handle")
	}
	if s.eng == nil {
		return mcp.ToolError("cave_browser_unavailable", "browser recovery unavailable")
	}
	original, err := s.eng.RetrieveQuery(a.RecoveryHandle, a.Query)
	if err != nil {
		return mcp.ToolError("cave_unknown_handle", "no original found for handle")
	}
	return mcp.ToolRawText(string(original))
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
