#!/usr/bin/env python3
import subprocess
import os
import re
import yaml

SLOTH_BIN = "/Users/Shared/dev/git/sloth/bin/sloth-darwin-arm64"
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
TMP_DIR = os.path.join(REPO_ROOT, ".tmp-slo")
CHARTS_DIR = os.path.join(REPO_ROOT, "domain")

PLACEHOLDER_MAP = {
    "CHARTNAME": '{{ .Chart.Name }}',
    "NAMESPACE": '{{ .Release.Namespace }}',
    "RELEASENAME": '{{ .Release.Name }}',
    "RELEASESERVICE": '{{ .Release.Service }}',
    "TEAM": '{{ .Values.global.team }}',
    "POP": '{{ .Values.global.pop }}',
    "CLUSTERTYPE": '{{ .Values.global.clusterType }}',
    "ENVIRONMENT": '{{ .Values.global.environment }}',
}

PRIORITY_OBJECTIVES = {
    "priority-critical": 99.9,
    "priority-high":     99.5,
    "priority-medium":   99.2,
    "priority-low":      99.0,
}

RUNBOOK_BASE = "https://github.com/treussart/slo-generator/-/blob/main"

SLO_ANCHOR_MAP = {
    "requests-availability": "high-error-rate-availability",
    "ingress-requests-availability": "ingress-high-error-rate-availability",
    "requests-latency": "high-error-rate-latency",
}

def runbook_url(chart_path, short_name, slo_name):
    domain = chart_path.split("/")[0]  # dns or services
    anchor = f"{short_name}-{SLO_ANCHOR_MAP[slo_name]}"
    return f"{RUNBOOK_BASE}/domain/{domain}/runbook.md#{anchor}"

def build_slo_spec(name, part_of, slos):
    spec = {
        "apiVersion": "sloth.slok.dev/v1",
        "kind": "PrometheusServiceLevel",
        "metadata": {
            "name": "CHARTNAME",
            "labels": {
                "app.kubernetes.io/name": "CHARTNAME",
                "app.kubernetes.io/component": "app",
                "app.kubernetes.io/part-of": part_of,
                "prometheus": "infra",
            },
        },
        "spec": {
            "service": "CHARTNAME",
            "labels": {
                "severity": "warning",
                "domain": "domain",
                "namespace": "NAMESPACE",
                "pop": "POP",
                "clusterType": "CLUSTERTYPE",
                "environment": "ENVIRONMENT",
            },
            "slos": slos,
        },
    }
    return spec


def make_availability_slo(job_selector, objective=99.9, metric="handler_http_response_seconds_count", ns_label="namespace", error_codes='5..', rb_url=""):
    return {
        "name": "requests-availability",
        "objective": objective,
        "description": "SLO based on availability for HTTP request responses.",
        "labels": {
            "category": "availability",
            "namespace": "NAMESPACE",
            "pop": "POP",
            "clusterType": "CLUSTERTYPE",
            "environment": "ENVIRONMENT",
        },
        "sli": {
            "events": {
                "errorQuery": f'sum(rate({metric}{{{ns_label}="NAMESPACE",{job_selector},status_code=~"{error_codes}"}}[{{{{.window}}}}]))',
                "totalQuery": f'sum(rate({metric}{{{ns_label}="NAMESPACE",{job_selector}}}[{{{{.window}}}}]))',
            }
        },
        "alerting": {
            "name": "CHARTNAME-HighErrorRateAvailability",
            "labels": {
                "category": "availability",
                "severity": "warning",
                "domain": "domain",
                "namespace": "NAMESPACE",
                "pop": "POP",
                "clusterType": "CLUSTERTYPE",
                "environment": "ENVIRONMENT",
            },
            "annotations": {
                "summary": "High error rate on 'CHARTNAME' requests responses",
                "contact": "TEAM",
                "runbook_url": rb_url,
            },
        },
    }


def make_nginx_availability_slo(objective=99.9, metric="nginx_http_requests_total", status_label="status", error_codes="5..", rb_url=""):
    return {
        "name": "requests-availability",
        "objective": objective,
        "description": "SLO based on availability for HTTP request responses.",
        "labels": {
            "category": "availability",
            "namespace": "NAMESPACE",
            "pop": "POP",
            "clusterType": "CLUSTERTYPE",
            "environment": "ENVIRONMENT",
        },
        "sli": {
            "events": {
                "errorQuery": f'sum(rate({metric}{{namespace="NAMESPACE",job="CHARTNAME",{status_label}=~"{error_codes}"}}[{{{{.window}}}}]))',
                "totalQuery": f'sum(rate({metric}{{namespace="NAMESPACE",job="CHARTNAME"}}[{{{{.window}}}}]))',
            }
        },
        "alerting": {
            "name": "CHARTNAME-HighErrorRateAvailability",
            "labels": {
                "category": "availability",
                "severity": "warning",
                "domain": "domain",
                "namespace": "NAMESPACE",
                "pop": "POP",
                "clusterType": "CLUSTERTYPE",
                "environment": "ENVIRONMENT",
            },
            "annotations": {
                "summary": "High error rate on 'CHARTNAME' requests responses",
                "contact": "TEAM",
                "runbook_url": rb_url,
            },
        },
    }


def make_ingress_availability_slo(objective=99.9, rb_url=""):
    return {
        "name": "ingress-requests-availability",
        "objective": objective,
        "description": "SLO based on availability for HTTP request responses.",
        "labels": {
            "category": "availability",
            "namespace": "NAMESPACE",
            "pop": "POP",
            "clusterType": "CLUSTERTYPE",
            "environment": "ENVIRONMENT",
        },
        "sli": {
            "events": {
                "errorQuery": 'sum(rate(nginx_ingress_controller_request_duration_seconds_count{exported_namespace="NAMESPACE",ingress="CHARTNAME",status_code=~"5.."}[{{.window}}]))',
                "totalQuery": 'sum(rate(nginx_ingress_controller_request_duration_seconds_count{exported_namespace="NAMESPACE",ingress="CHARTNAME"}[{{.window}}]))',
            }
        },
        "alerting": {
            "name": "CHARTNAME-HighErrorRateAvailability",
            "labels": {
                "category": "availability",
                "severity": "warning",
                "domain": "domain",
                "namespace": "NAMESPACE",
                "pop": "POP",
                "clusterType": "CLUSTERTYPE",
                "environment": "ENVIRONMENT",
            },
            "annotations": {
                "summary": "High error rate on 'CHARTNAME' ingress requests responses",
                "contact": "TEAM",
                "runbook_url": rb_url,
            },
        },
    }


def make_latency_slo(job_selector, objective=99.9, threshold="0.5", rb_url=""):
    return {
        "name": "requests-latency",
        "objective": objective,
        "description": f"SLO for HTTP request latency under {int(float(threshold)*1000)}ms",
        "labels": {
            "category": "latency",
            "namespace": "NAMESPACE",
            "pop": "POP",
            "clusterType": "CLUSTERTYPE",
            "environment": "ENVIRONMENT",
        },
        "sli": {
            "events": {
                "errorQuery": f'(\n  sum(rate(handler_http_response_seconds_count{{namespace="NAMESPACE",{job_selector},status_code=~"2.."}}[{{{{.window}}}}]))\n  -\n  sum(rate(handler_http_response_seconds_bucket{{namespace="NAMESPACE",{job_selector},status_code=~"2..",le="{threshold}"}}[{{{{.window}}}}]))\n)\n',
                "totalQuery": f'sum(rate(handler_http_response_seconds_count{{namespace="NAMESPACE",{job_selector},status_code=~"2.."}}[{{{{.window}}}}]))',
            }
        },
        "alerting": {
            "name": "CHARTNAME-HighErrorRateLatency",
            "labels": {
                "category": "latency",
                "severity": "warning",
                "domain": "domain",
                "namespace": "NAMESPACE",
                "pop": "POP",
                "clusterType": "CLUSTERTYPE",
                "environment": "ENVIRONMENT",
            },
            "annotations": {
                "summary": "High latency rate on 'CHARTNAME' requests responses",
                "contact": "TEAM",
                "runbook_url": rb_url,
            },
        },
    }


def _rb(path, short, slo_name):
    return runbook_url(path, short, slo_name)

CHARTS = {
    "example-1": {
        "path": "example/charts/example-1",
        "part_of": "subDomain",
        "priority": "priority-critical",
        "latency_threshold": "0.1",  # 100ms
        "latency_objective": 80,  # requests-latency SLO (availability/ingress stay at 99.9%)
        "slos": lambda o, lt, o_latency=None, p="example/charts/example-1", s="example-1": [
            make_availability_slo('job="CHARTNAME"', objective=o, rb_url=_rb(p, s, "requests-availability")),
            make_ingress_availability_slo(objective=o, rb_url=_rb(p, s, "ingress-requests-availability")),
            make_latency_slo('job="CHARTNAME"', objective=o_latency or o, threshold=lt, rb_url=_rb(p, s, "requests-latency")),
        ],
    },
    "example-2": {
        "path": "example/charts/example-2",
        "part_of": "subDomain",
        "priority": "priority-high",
        "latency_threshold": "0.5",  # 500ms
        "latency_objective": 80,
        "slos": lambda o, lt, o_latency=None, p="example/charts/example-2", s="example-2": [
            make_availability_slo('job="CHARTNAME"', objective=o, rb_url=_rb(p, s, "requests-availability")),
            make_latency_slo('job="CHARTNAME"', objective=o_latency or o, threshold=lt, rb_url=_rb(p, s, "requests-latency")),
        ],
    },
}


class LiteralStr(str):
    pass

def literal_str_representer(dumper, data):
    if '\n' in data:
        return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='|')
    return dumper.represent_scalar('tag:yaml.org,2002:str', data)

yaml.add_representer(LiteralStr, literal_str_representer)


def to_literal(obj):
    if isinstance(obj, dict):
        return {k: to_literal(v) for k, v in obj.items()}
    if isinstance(obj, list):
        return [to_literal(v) for v in obj]
    if isinstance(obj, str) and '\n' in obj:
        return LiteralStr(obj)
    return obj


def replace_placeholders_helm(content):
    for placeholder, helm_expr in PLACEHOLDER_MAP.items():
        content = content.replace(placeholder, helm_expr)
    return content


def escape_prometheus_templates(content):
    content = re.sub(
        r'\{\{(\$labels\.[^}]+)\}\}',
        r"{{`{{ \1 }}`}}",
        content
    )
    content = re.sub(
        r'\{\{(\$value[^}]*)\}\}',
        r"{{`{{ \1 }}`}}",
        content
    )
    return content


_AVAILABILITY_RATIO_METRICS = (
    "handler_http_response_seconds_count",
    "nginx_ingress_controller_request_duration_seconds_count",
    "nginx_http_requests_total",
)


def fix_sli_error_ratio_empty_numerator(content):
    """Sloth emits error_total / total. If no series match the error selector (e.g. no 5xx
    labels in TSDB), sum(rate(errors)) is an empty vector and the division yields no series.
    Wrap: (sum(errors) or (sum(total) * 0)) / clamp_min(sum(total), 1e-9) so that:
    - SLI is 0 when there are no 5xx keys (empty numerator case)
    - SLI is 0 when there is no traffic (sum(rate(total)) == 0), avoiding 0/0 NaN on downstream
      slo:period_error_budget_remaining:ratio and Grafana budget columns.
    """
    out = content
    for metric in _AVAILABILITY_RATIO_METRICS:
        esc = re.escape(metric)
        pattern = re.compile(
            rf"\(sum\(rate\({esc}\{{(?P<base>.+),(?P<elabel>status_code|status)=~\"5\.\.\"\}}"
            rf"\[(?P<win>[^\]]+)\]\)\)\)\s*\n\s*/\s*\n\s*"
            rf"\(sum\(rate\({esc}\{{(?P=base)}}\[(?P=win)\]\)\)\)",
            re.MULTILINE,
        )

        def repl(m):
            base = m.group("base")
            elabel = m.group("elabel")
            win = m.group("win")
            err_sel = f"{metric}{{{base},{elabel}=~\"5..\"}}[{win}]"
            tot_sel = f"{metric}{{{base}}}[{win}]"
            return (
                "(\n"
                f"          (sum(rate({err_sel})))\n"
                "          or\n"
                f"          (sum(rate({tot_sel})) * 0)\n"
                "        )\n"
                "        /\n"
                f"        clamp_min((sum(rate({tot_sel}))), 1e-9)"
            )

        out = pattern.sub(repl, out)
    return out


_LATENCY_SLI_RATIO = re.compile(
    r"(?P<ind>[ \t]+)\(\(\s*\n"
    r"\s*sum\(rate\(handler_http_response_seconds_count\{(?P<pre>.+),status_code=~\"2\.\.\"\}"
    rf"\[(?P<win>[^\]]+)\]\)\)\s*\n"
    r"\s*-\s*\n"
    r"\s*sum\(rate\(handler_http_response_seconds_bucket\{(?P=pre),status_code=~\"2\.\.\","
    r'le="(?P<le>[^"]+)"\}\[(?P=win)\]\)\)\s*\n'
    r"\s*\)\s*\n\s*\)\s*\n"
    r"\s*/\s*\n"
    r"\s*\(sum\(rate\(handler_http_response_seconds_count\{(?P=pre),status_code=~\"2\.\.\"\}"
    rf"\[(?P=win)\]\)\)\)",
    re.MULTILINE,
)


def fix_latency_sli_ratio_zero_denominator(content):
    """Latency SLI is (slow_2xx) / (all_2xx). When sum(rate(2xx_count)) is 0, PromQL yields NaN
    and error-budget metrics become NaN. Use clamp_min on the denominator only.
    """
    def repl(m):
        ind = m.group("ind")
        body = ind + "  "
        pre, win, le = m.group("pre"), m.group("win"), m.group("le")
        cnt = f'handler_http_response_seconds_count{{{pre},status_code=~"2.."}}[{win}]'
        bkt = f'handler_http_response_seconds_bucket{{{pre},status_code=~"2..",le="{le}"}}[{win}]'
        return (
            f"{ind}((\n"
            f"{body}sum(rate({cnt}))\n"
            f"{body}-\n"
            f"{body}sum(rate({bkt}))\n"
            f"{ind})\n"
            f"{ind})\n"
            f"{ind}/\n"
            f"{ind}clamp_min((sum(rate({cnt}))), 1e-9)"
        )

    return _LATENCY_SLI_RATIO.sub(repl, content)


def add_helm_metadata(content, part_of):
    annotations_block = (
        "  annotations:\n"
        "    saas.product.com/team: {{ .Values.global.team }}\n"
        "    saas.product.com/environment: {{ .Values.global.environment }}\n"
        "    saas.product.com/pop: {{ .Values.global.pop }}\n"
    )
    extra_labels = (
        "    app.kubernetes.io/instance: {{ .Release.Name }}\n"
    )

    content = content.replace(
        "metadata:\n  labels:\n",
        "metadata:\n" + annotations_block + "  labels:\n"
    )
    content = content.replace(
        "    app.kubernetes.io/managed-by: sloth\n",
        "    app.kubernetes.io/managed-by: sloth\n" + extra_labels
    )
    content = content.replace(
        "  name: {{ .Chart.Name }}\n",
        "  name: {{ .Chart.Name }}-slo\n",
        1
    )
    return content


def wrap_alert_groups(content):
    lines = content.split('\n')
    result = []
    in_alert_group = False
    for line in lines:
        if re.match(r'  - name: sloth-slo-alerts-', line):
            result.append('  {{- if .Values.slo.alerting.enabled }}')
            result.append(line)
            in_alert_group = True
        elif in_alert_group and re.match(r'  - name: sloth-slo-(?!alerts)', line):
            result.append('  {{- end }}')
            result.append(line)
            in_alert_group = False
        else:
            result.append(line)
    if in_alert_group:
        result.append('  {{- end }}')
    return '\n'.join(result)


def add_slo_default_to_values(values_path):
    if not os.path.exists(values_path):
        return
    with open(values_path, 'r') as f:
        content = f.read()
    slo_block = 'slo:\n  enabled: true\n  alerting:\n    enabled: false'
    if 'slo:' in content:
        content = re.sub(
            r'slo:\n(?:  enabled: (?:true|false)\n)?(?:  alerting:\n    enabled: (?:true|false))?',
            slo_block,
            content
        )
        with open(values_path, 'w') as f:
            f.write(content)
        return
    content = content.rstrip('\n') + '\n' + slo_block + '\n'
    with open(values_path, 'w') as f:
        f.write(content)


def process_chart(name, config):
    priority = config["priority"]
    objective = PRIORITY_OBJECTIVES[priority]
    lt = config.get("latency_threshold", "0.5")
    o_latency = config.get("latency_objective")
    slos = config["slos"](objective, lt, o_latency)
    lt_display = f"{int(float(lt)*1000)}ms" if lt else "N/A"
    print(f"Processing {name} (priority={priority}, objective={objective}%, latency={lt_display})...")

    spec = build_slo_spec(name, config["part_of"], slos)
    spec = to_literal(spec)

    input_path = os.path.join(TMP_DIR, f"{name}-input.yaml")
    output_path = os.path.join(TMP_DIR, f"{name}-output.yaml")

    with open(input_path, 'w') as f:
        yaml.dump(spec, f, default_flow_style=False, allow_unicode=True, sort_keys=False)

    result = subprocess.run(
        [SLOTH_BIN, "generate", "-i", input_path, "-o", output_path],
        capture_output=True, text=True
    )
    if result.returncode != 0:
        print(f"  ERROR: {result.stderr}")
        return False

    print(f"  sloth generate OK")

    with open(output_path, 'r') as f:
        content = f.read()

    content = escape_prometheus_templates(content)
    content = replace_placeholders_helm(content)
    content = add_helm_metadata(content, config["part_of"])
    content = fix_sli_error_ratio_empty_numerator(content)
    content = fix_latency_sli_ratio_zero_denominator(content)
    content = wrap_alert_groups(content)
    content = '{{- if .Values.slo.enabled }}\n' + content.rstrip('\n') + '\n{{- end }}\n'

    final_path = os.path.join(CHARTS_DIR, config["path"], "templates", "prometheus-rules-slo.yaml")
    with open(final_path, 'w') as f:
        f.write(content)

    values_path = os.path.join(CHARTS_DIR, config["path"], "values.yaml")
    add_slo_default_to_values(values_path)

    print(f"  Written to {final_path}")
    return True


if __name__ == "__main__":
    os.makedirs(TMP_DIR, exist_ok=True)
    results = {}
    for name, config in CHARTS.items():
        results[name] = process_chart(name, config)

    print("\n--- Summary ---")
    for name, ok in results.items():
        status = "OK" if ok else "FAILED"
        print(f"  {name}: {status}")
