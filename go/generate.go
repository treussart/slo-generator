package main

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/slok/sloth/pkg/common/model"
	sloth "github.com/slok/sloth/pkg/lib"
	yamlv3 "gopkg.in/yaml.v3"
)

var placeholderMap = map[string]string{
	"CHARTNAME":      `{{ .Chart.Name }}`,
	"NAMESPACE":      `{{ .Release.Namespace }}`,
	"RELEASENAME":    `{{ .Release.Name }}`,
	"RELEASESERVICE": `{{ .Release.Service }}`,
	"TEAM":           `{{ .Values.global.team }}`,
	"POP":            `{{ .Values.global.pop }}`,
	"CLUSTERTYPE":    `{{ .Values.global.clusterType }}`,
	"ENVIRONMENT":    `{{ .Values.global.environment }}`,
}

var priorityObjectives = map[string]float64{
	"priority-critical": 99.9,
	"priority-high":     99.5,
	"priority-medium":   99.2,
	"priority-low":      99.0,
}

const runbookBase = "https://github.com/treussart/slo-generator/-/blob/main"

var sloAnchorMap = map[string]string{
	"requests-availability":         "high-error-rate-availability",
	"ingress-requests-availability": "ingress-high-error-rate-availability",
	"requests-latency":              "high-error-rate-latency",
}

func runbookURL(chartPath, shortName, sloName string) string {
	parts := strings.SplitN(chartPath, "/", 2)
	domain := parts[0]
	anchor := shortName + "-" + sloAnchorMap[sloName]
	return fmt.Sprintf("%s/domain/%s/runbook.md#%s", runbookBase, domain, anchor)
}

// SLODef holds the parameters for a single SLO within the Sloth spec.
type SLODef struct {
	Name             string
	Objective        float64
	Description      string
	Category         string
	ErrorQuery       string
	TotalQuery       string
	AlertName        string
	AlertAnnotations map[string]string
}

// ChartConfig mirrors the Python CHARTS dictionary.
type ChartConfig struct {
	Path             string
	PartOf           string
	Priority         string
	LatencyThreshold string  // empty if N/A
	LatencyObjective float64 // 0 means use availability objective
	BuildSLOs        func(objective float64, lt string, oLatency float64) []SLODef
}

func makeAvailabilitySLO(jobSelector string, objective float64, metric, nsLabel, errorCodes, rbURL string) SLODef {
	if metric == "" {
		metric = "handler_http_response_seconds_count"
	}
	if nsLabel == "" {
		nsLabel = "namespace"
	}
	if errorCodes == "" {
		errorCodes = "5.."
	}
	return SLODef{
		Name:        "requests-availability",
		Objective:   objective,
		Description: "SLO based on availability for HTTP request responses.",
		Category:    "availability",
		ErrorQuery:  fmt.Sprintf(`sum(rate(%s{%s="NAMESPACE",%s,status_code=~"%s"}[{{.window}}]))`, metric, nsLabel, jobSelector, errorCodes),
		TotalQuery:  fmt.Sprintf(`sum(rate(%s{%s="NAMESPACE",%s}[{{.window}}]))`, metric, nsLabel, jobSelector),
		AlertName:   "CHARTNAME-HighErrorRateAvailability",
		AlertAnnotations: map[string]string{
			"summary":     "High error rate on 'CHARTNAME' requests responses",
			"contact":     "TEAM",
			"runbook_url": rbURL,
		},
	}
}

func makeNginxAvailabilitySLO(objective float64, rbURL string) SLODef {
	return SLODef{
		Name:        "requests-availability",
		Objective:   objective,
		Description: "SLO based on availability for HTTP request responses.",
		Category:    "availability",
		ErrorQuery:  `sum(rate(nginx_http_requests_total{namespace="NAMESPACE",job="CHARTNAME",status=~"5.."}[{{.window}}]))`,
		TotalQuery:  `sum(rate(nginx_http_requests_total{namespace="NAMESPACE",job="CHARTNAME"}[{{.window}}]))`,
		AlertName:   "CHARTNAME-HighErrorRateAvailability",
		AlertAnnotations: map[string]string{
			"summary":     "High error rate on 'CHARTNAME' requests responses",
			"contact":     "TEAM",
			"runbook_url": rbURL,
		},
	}
}

func makeIngressAvailabilitySLO(objective float64, rbURL string) SLODef {
	return SLODef{
		Name:        "ingress-requests-availability",
		Objective:   objective,
		Description: "SLO based on availability for HTTP request responses.",
		Category:    "availability",
		ErrorQuery:  `sum(rate(nginx_ingress_controller_request_duration_seconds_count{exported_namespace="NAMESPACE",ingress="CHARTNAME",status_code=~"5.."}[{{.window}}]))`,
		TotalQuery:  `sum(rate(nginx_ingress_controller_request_duration_seconds_count{exported_namespace="NAMESPACE",ingress="CHARTNAME"}[{{.window}}]))`,
		AlertName:   "CHARTNAME-HighErrorRateAvailability",
		AlertAnnotations: map[string]string{
			"summary":     "High error rate on 'CHARTNAME' ingress requests responses",
			"contact":     "TEAM",
			"runbook_url": rbURL,
		},
	}
}

func makeLatencySLO(jobSelector string, objective float64, threshold, rbURL string) SLODef {
	ms := int(math.Round(parseFloat(threshold) * 1000))
	errQ := fmt.Sprintf("(\n  sum(rate(handler_http_response_seconds_count{namespace=\"NAMESPACE\",%s,status_code=~\"2..\"}[{{.window}}]))\n  -\n  sum(rate(handler_http_response_seconds_bucket{namespace=\"NAMESPACE\",%s,status_code=~\"2..\",le=\"%s\"}[{{.window}}]))\n)\n", jobSelector, jobSelector, threshold)
	totQ := fmt.Sprintf(`sum(rate(handler_http_response_seconds_count{namespace="NAMESPACE",%s,status_code=~"2.."}[{{.window}}]))`, jobSelector)
	return SLODef{
		Name:        "requests-latency",
		Objective:   objective,
		Description: fmt.Sprintf("SLO for HTTP request latency under %dms", ms),
		Category:    "latency",
		ErrorQuery:  errQ,
		TotalQuery:  totQ,
		AlertName:   "CHARTNAME-HighErrorRateLatency",
		AlertAnnotations: map[string]string{
			"summary":     "High latency rate on 'CHARTNAME' requests responses",
			"contact":     "TEAM",
			"runbook_url": rbURL,
		},
	}
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

func rb(path, short, sloName string) string {
	return runbookURL(path, short, sloName)
}

var charts = map[string]ChartConfig{
	"example-1": {
		Path: "example/charts/example-1", PartOf: "domain", Priority: "priority-critical",
		LatencyThreshold: "0.1", LatencyObjective: 80,
		BuildSLOs: func(o float64, lt string, oLat float64) []SLODef {
			p, s := "example/charts/example-1", "example-1"
			return []SLODef{
				makeAvailabilitySLO(`job="CHARTNAME"`, o, "", "", "", rb(p, s, "requests-availability")),
				makeIngressAvailabilitySLO(o, rb(p, s, "ingress-requests-availability")),
				makeLatencySLO(`job="CHARTNAME"`, oLat, lt, rb(p, s, "requests-latency")),
			}
		},
	},
	"example-2": {
		Path: "example/charts/example-2", PartOf: "domain", Priority: "priority-high",
		LatencyThreshold: "0.5", LatencyObjective: 80,
		BuildSLOs: func(o float64, lt string, oLat float64) []SLODef {
			p, s := "example/charts/example-2", "example-2"
			return []SLODef{
				makeAvailabilitySLO(`job=~"CHARTNAME-(req|resp)"`, o, "", "", "", rb(p, s, "requests-availability")),
				makeLatencySLO(`job=~"CHARTNAME-(req|resp)"`, oLat, lt, rb(p, s, "requests-latency")),
			}
		},
	},
}

// chartOrder preserves processing order (matches the Python dict insertion order).
var chartOrder = []string{
	"example-1", "example-2",
}

// YAML structs for the Sloth raw spec (version: prometheus/v1).
type slothRawSpec struct {
	Version string            `yaml:"version"`
	Service string            `yaml:"service"`
	Labels  map[string]string `yaml:"labels"`
	SLOs    []slothRawSLO     `yaml:"slos"`
}

type slothRawSLO struct {
	Name        string            `yaml:"name"`
	Objective   float64           `yaml:"objective"`
	Description string            `yaml:"description"`
	Labels      map[string]string `yaml:"labels"`
	SLI         slothRawSLI       `yaml:"sli"`
	Alerting    slothRawAlerting  `yaml:"alerting"`
}

type slothRawSLI struct {
	Events slothRawEvents `yaml:"events"`
}

type slothRawEvents struct {
	ErrorQuery string `yaml:"error_query"`
	TotalQuery string `yaml:"total_query"`
}

type slothRawAlerting struct {
	Name        string            `yaml:"name"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
	PageAlert   slothRawAlert     `yaml:"page_alert"`
	TicketAlert slothRawAlert     `yaml:"ticket_alert"`
}

type slothRawAlert struct {
	Labels map[string]string `yaml:"labels"`
}

func buildSlothSpec(partOf string, slos []SLODef) string {
	spec := slothRawSpec{
		Version: "prometheus/v1",
		Service: "CHARTNAME",
		Labels: map[string]string{
			"severity":    "warning",
			"domain":      "domain",
			"namespace":   "NAMESPACE",
			"pop":         "POP",
			"clusterType": "CLUSTERTYPE",
			"environment": "ENVIRONMENT",
		},
	}
	for _, s := range slos {
		spec.SLOs = append(spec.SLOs, slothRawSLO{
			Name:        s.Name,
			Objective:   s.Objective,
			Description: s.Description,
			Labels: map[string]string{
				"category":    s.Category,
				"namespace":   "NAMESPACE",
				"pop":         "POP",
				"clusterType": "CLUSTERTYPE",
				"environment": "ENVIRONMENT",
			},
			SLI: slothRawSLI{
				Events: slothRawEvents{
					ErrorQuery: s.ErrorQuery,
					TotalQuery: s.TotalQuery,
				},
			},
			Alerting: slothRawAlerting{
				Name: s.AlertName,
				Labels: map[string]string{
					"category":    s.Category,
					"severity":    "warning",
					"domain":      "domain",
					"namespace":   "NAMESPACE",
					"pop":         "POP",
					"clusterType": "CLUSTERTYPE",
					"environment": "ENVIRONMENT",
				},
				Annotations: s.AlertAnnotations,
				PageAlert:   slothRawAlert{Labels: map[string]string{"sloth_severity": "page"}},
				TicketAlert: slothRawAlert{Labels: map[string]string{"sloth_severity": "ticket"}},
			},
		})
	}
	out, err := yamlv3.Marshal(spec)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal sloth spec: %v", err))
	}
	return string(out)
}

// --- Post-processing functions (ported from Python) ---

func replacePlaceholdersHelm(content string) string {
	for placeholder, helmExpr := range placeholderMap {
		content = strings.ReplaceAll(content, placeholder, helmExpr)
	}
	return content
}

var (
	reLabels = regexp.MustCompile(`\{\{(\$labels\.[^}]+)\}\}`)
	reValue  = regexp.MustCompile(`\{\{(\$value[^}]*)\}\}`)
)

func escapePrometheusTemplates(content string) string {
	content = reLabels.ReplaceAllString(content, "{{`{{ $1 }}`}}")
	content = reValue.ReplaceAllString(content, "{{`{{ $1 }}`}}")
	return content
}

var availabilityRatioMetrics = []string{
	"handler_http_response_seconds_count",
	"nginx_ingress_controller_request_duration_seconds_count",
	"nginx_http_requests_total",
}

func fixSLIErrorRatioEmptyNumerator(content string) string {
	out := content
	for _, metric := range availabilityRatioMetrics {
		esc := regexp.QuoteMeta(metric)
		pattern := regexp.MustCompile(
			`\(sum\(rate\(` + esc + `\{(?P<base>.+),(?P<elabel>status_code|status)=~"5\.\."\}` +
				`\[(?P<win>[^\]]+)\]\)\)\)\s*\n\s*/\s*\n\s*` +
				`\(sum\(rate\(` + esc + `\{(?P<base2>.+)\}\[(?P<win2>[^\]]+)\]\)\)\)`,
		)
		out = pattern.ReplaceAllStringFunc(out, func(match string) string {
			sub := pattern.FindStringSubmatch(match)
			if len(sub) < 6 {
				return match
			}
			base := sub[1]
			elabel := sub[2]
			win := sub[3]
			errSel := fmt.Sprintf("%s{%s,%s=~\"5..\"}[%s]", metric, base, elabel, win)
			totSel := fmt.Sprintf("%s{%s}[%s]", metric, base, win)
			return fmt.Sprintf("(\n          (sum(rate(%s)))\n          or\n          (sum(rate(%s)) * 0)\n        )\n        /\n        clamp_min((sum(rate(%s))), 1e-9)", errSel, totSel, totSel)
		})
	}
	return out
}

var latencySLIRatio = regexp.MustCompile(
	`(?P<ind>[ \t]+)\(\(\s*\n` +
		`\s*sum\(rate\(handler_http_response_seconds_count\{(?P<pre>.+),status_code=~"2\.\."\}` +
		`\[(?P<win>[^\]]+)\]\)\)\s*\n` +
		`\s*-\s*\n` +
		`\s*sum\(rate\(handler_http_response_seconds_bucket\{(?P<pre2>.+),status_code=~"2\.\.",` +
		`le="(?P<le>[^"]+)"\}\[(?P<win2>[^\]]+)\]\)\)\s*\n` +
		`\s*\)\s*\n\s*\)\s*\n` +
		`\s*/\s*\n` +
		`\s*\(sum\(rate\(handler_http_response_seconds_count\{(?P<pre3>.+),status_code=~"2\.\."\}` +
		`\[(?P<win3>[^\]]+)\]\)\)\)`,
)

func fixLatencySLIRatioZeroDenominator(content string) string {
	return latencySLIRatio.ReplaceAllStringFunc(content, func(match string) string {
		sub := latencySLIRatio.FindStringSubmatch(match)
		if len(sub) < 8 {
			return match
		}
		ind := sub[1]
		body := ind + "  "
		pre := sub[2]
		win := sub[3]
		le := sub[5]
		cnt := fmt.Sprintf(`handler_http_response_seconds_count{%s,status_code=~"2.."}[%s]`, pre, win)
		bkt := fmt.Sprintf(`handler_http_response_seconds_bucket{%s,status_code=~"2..",le="%s"}[%s]`, pre, le, win)
		return fmt.Sprintf("%s((\n%ssum(rate(%s))\n%s-\n%ssum(rate(%s))\n%s)\n%s)\n%s/\n%sclamp_min((sum(rate(%s))), 1e-9)",
			ind, body, cnt, body, body, bkt, ind, ind, ind, ind, cnt)
	})
}

func addHelmMetadata(content, partOf string) string {
	annotationsBlock := "  annotations:\n" +
		"    saas.product.com/team: {{ .Values.global.team }}\n" +
		"    saas.product.com/environment: {{ .Values.global.environment }}\n" +
		"    saas.product.com/pop: {{ .Values.global.pop }}\n"
	extraLabels := "    app.kubernetes.io/instance: {{ .Release.Name }}\n"

	content = strings.Replace(content,
		"metadata:\n  labels:\n",
		"metadata:\n"+annotationsBlock+"  labels:\n", 1)
	content = strings.Replace(content,
		"    app.kubernetes.io/managed-by: sloth\n",
		"    app.kubernetes.io/managed-by: sloth\n"+extraLabels, 1)
	content = strings.Replace(content,
		"  name: {{ .Chart.Name }}\n",
		"  name: {{ .Chart.Name }}-slo\n", 1)
	return content
}

var reAlertGroupStart = regexp.MustCompile(`^  - name: sloth-slo-alerts-`)
var reNonAlertGroupStart = regexp.MustCompile(`^  - name: sloth-slo-(?:sli|meta)`)

func wrapAlertGroups(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inAlertGroup := false
	for _, line := range lines {
		if reAlertGroupStart.MatchString(line) {
			result = append(result, "  {{- if .Values.slo.alerting.enabled }}")
			result = append(result, line)
			inAlertGroup = true
		} else if inAlertGroup && reNonAlertGroupStart.MatchString(line) {
			result = append(result, "  {{- end }}")
			result = append(result, line)
			inAlertGroup = false
		} else {
			result = append(result, line)
		}
	}
	if inAlertGroup {
		result = append(result, "  {{- end }}")
	}
	return strings.Join(result, "\n")
}

func addSLODefaultToValues(valuesPath string) {
	data, err := os.ReadFile(valuesPath)
	if err != nil {
		return
	}
	content := string(data)
	sloBlock := "slo:\n  enabled: true\n  alerting:\n    enabled: false"
	if strings.Contains(content, "slo:") {
		re := regexp.MustCompile(`slo:\n(?:  enabled: (?:true|false)\n)?(?:  alerting:\n    enabled: (?:true|false))?`)
		content = re.ReplaceAllString(content, sloBlock)
		os.WriteFile(valuesPath, []byte(content), 0644)
		return
	}
	content = strings.TrimRight(content, "\n") + "\n" + sloBlock + "\n"
	os.WriteFile(valuesPath, []byte(content), 0644)
}

func processChart(ctx context.Context, gen *sloth.PrometheusSLOGenerator, name string, config ChartConfig, repoRoot string) bool {
	objective := priorityObjectives[config.Priority]
	lt := config.LatencyThreshold
	if lt == "" {
		lt = "0.5"
	}
	oLatency := config.LatencyObjective
	if oLatency == 0 {
		oLatency = objective
	}

	slos := config.BuildSLOs(objective, lt, oLatency)
	ltDisplay := "N/A"
	if config.LatencyThreshold != "" {
		ltDisplay = fmt.Sprintf("%dms", int(math.Round(parseFloat(config.LatencyThreshold)*1000)))
	}
	fmt.Printf("Processing %s (priority=%s, objective=%.1f%%, latency=%s)...\n", name, config.Priority, objective, ltDisplay)

	spec := buildSlothSpec(config.PartOf, slos)

	result, err := gen.GenerateFromRaw(ctx, []byte(spec))
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return false
	}
	fmt.Println("  sloth generate OK")

	k8sMeta := model.K8sMeta{
		Name: "CHARTNAME",
		Labels: map[string]string{
			"app.kubernetes.io/name":      "CHARTNAME",
			"app.kubernetes.io/component": "SLO",
			"app.kubernetes.io/part-of":   config.PartOf,
			"prometheus":                  "infra",
		},
	}

	var buf bytes.Buffer
	if err := gen.WriteResultAsK8sPrometheusOperator(ctx, k8sMeta, *result, &buf); err != nil {
		fmt.Printf("  ERROR writing result: %v\n", err)
		return false
	}
	content := buf.String()

	content = escapePrometheusTemplates(content)
	content = replacePlaceholdersHelm(content)
	content = addHelmMetadata(content, config.PartOf)
	content = fixSLIErrorRatioEmptyNumerator(content)
	content = fixLatencySLIRatioZeroDenominator(content)
	content = wrapAlertGroups(content)
	content = "{{- if .Values.slo.enabled }}\n" + strings.TrimRight(content, "\n") + "\n{{- end }}\n"

	chartsDir := filepath.Join(repoRoot, "domain")
	finalPath := filepath.Join(chartsDir, config.Path, "templates", "prometheus-rules-slo.yaml")
	if err := os.WriteFile(finalPath, []byte(content), 0644); err != nil {
		fmt.Printf("  ERROR writing file: %v\n", err)
		return false
	}

	valuesPath := filepath.Join(chartsDir, config.Path, "values.yaml")
	addSLODefaultToValues(valuesPath)

	fmt.Printf("  Written to %s\n", finalPath)
	return true
}
