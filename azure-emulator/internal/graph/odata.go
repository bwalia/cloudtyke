package graph

import (
	"fmt"
	"regexp"
	"strings"
)

// filterFn evaluates a parsed $filter against an object.
type filterFn func(map[string]any) bool

var (
	reEq         = regexp.MustCompile(`(?i)^([\w/.]+)\s+(eq|ne)\s+(.+)$`)
	reStartsWith = regexp.MustCompile(`(?i)^startswith\(\s*([\w/.]+)\s*,\s*'((?:[^']|'')*)'\s*\)$`)
	reAny        = regexp.MustCompile(`(?i)^([\w]+)/any\(\s*(\w+)\s*:\s*(.+)\)$`)
	reIn         = regexp.MustCompile(`(?i)^([\w/.]+)\s+in\s+\((.*)\)$`)
)

// parseFilter supports the subset of OData used by Terraform providers and
// common scripts: eq/ne, startswith(), in (...), collection/any(x: x eq 'v'),
// combined with and/or and an optional leading not.
func parseFilter(expr string) (filterFn, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return func(map[string]any) bool { return true }, nil
	}
	// strip wrapping parentheses
	for strings.HasPrefix(expr, "(") && matchingParen(expr) == len(expr)-1 {
		expr = strings.TrimSpace(expr[1 : len(expr)-1])
	}
	if parts := splitTop(expr, " or "); len(parts) > 1 {
		return combine(parts, false)
	}
	if parts := splitTop(expr, " and "); len(parts) > 1 {
		return combine(parts, true)
	}
	if l := strings.ToLower(expr); strings.HasPrefix(l, "not ") || strings.HasPrefix(l, "not(") {
		inner, err := parseFilter(strings.TrimSpace(expr[3:]))
		if err != nil {
			return nil, err
		}
		return func(o map[string]any) bool { return !inner(o) }, nil
	}
	if m := reStartsWith.FindStringSubmatch(expr); m != nil {
		field, val := m[1], strings.ReplaceAll(m[2], "''", "'")
		return func(o map[string]any) bool {
			s, _ := lookup(o, field).(string)
			return strings.HasPrefix(strings.ToLower(s), strings.ToLower(val))
		}, nil
	}
	if m := reAny.FindStringSubmatch(expr); m != nil {
		field, v, inner := m[1], m[2], strings.TrimSpace(m[3])
		// rewrite the lambda variable: "x eq 'a'" -> "__item eq 'a'", "x/prop eq 'a'" -> "__item/prop eq 'a'"
		inner = regexp.MustCompile(`(^|[\s(,])`+regexp.QuoteMeta(v)+`([\s/)])`).ReplaceAllString(inner, "${1}__item${2}")
		innerFn, err := parseFilter(inner)
		if err != nil {
			return nil, err
		}
		return func(o map[string]any) bool {
			arr, _ := lookup(o, field).([]any)
			for _, it := range arr {
				if innerFn(map[string]any{"__item": it}) {
					return true
				}
			}
			return false
		}, nil
	}
	if m := reIn.FindStringSubmatch(expr); m != nil {
		field := m[1]
		var vals []string
		for _, v := range strings.Split(m[2], ",") {
			vals = append(vals, strings.Trim(strings.TrimSpace(v), "'"))
		}
		return func(o map[string]any) bool {
			s := fmt.Sprint(lookup(o, field))
			for _, v := range vals {
				if strings.EqualFold(s, v) {
					return true
				}
			}
			return false
		}, nil
	}
	if m := reEq.FindStringSubmatch(expr); m != nil {
		field, op, raw := m[1], strings.ToLower(m[2]), strings.TrimSpace(m[3])
		var want any
		switch {
		case strings.HasPrefix(raw, "'") && strings.HasSuffix(raw, "'"):
			want = strings.ReplaceAll(raw[1:len(raw)-1], "''", "'")
		case strings.EqualFold(raw, "true"):
			want = true
		case strings.EqualFold(raw, "false"):
			want = false
		case strings.EqualFold(raw, "null"):
			want = nil
		default:
			want = raw
		}
		return func(o map[string]any) bool {
			got := lookup(o, field)
			eq := false
			switch w := want.(type) {
			case string:
				s, ok := got.(string)
				eq = ok && strings.EqualFold(s, w)
			case nil:
				eq = got == nil
			default:
				eq = got == w
			}
			if op == "ne" {
				return !eq
			}
			return eq
		}, nil
	}
	return nil, fmt.Errorf("unsupported filter clause %q", expr)
}

func combine(parts []string, and bool) (filterFn, error) {
	var fns []filterFn
	for _, p := range parts {
		f, err := parseFilter(p)
		if err != nil {
			return nil, err
		}
		fns = append(fns, f)
	}
	return func(o map[string]any) bool {
		for _, f := range fns {
			if f(o) != and {
				return !and
			}
		}
		return and
	}, nil
}

// lookup resolves a/b style property paths.
func lookup(o map[string]any, path string) any {
	var cur any = o
	for _, p := range strings.Split(path, "/") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		found := false
		for k, v := range m {
			if strings.EqualFold(k, p) {
				cur, found = v, true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return cur
}

// splitTop splits on sep (case-insensitive) outside quotes and parentheses.
func splitTop(s, sep string) []string {
	var out []string
	depth, inQuote, last := 0, false, 0
	ls := strings.ToLower(s)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			inQuote = !inQuote
		case '(':
			if !inQuote {
				depth++
			}
		case ')':
			if !inQuote {
				depth--
			}
		}
		if !inQuote && depth == 0 && strings.HasPrefix(ls[i:], sep) {
			out = append(out, s[last:i])
			last = i + len(sep)
			i += len(sep) - 1
		}
	}
	return append(out, s[last:])
}

func matchingParen(s string) int {
	depth, inQuote := 0, false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			inQuote = !inQuote
		case '(':
			if !inQuote {
				depth++
			}
		case ')':
			if !inQuote {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}
	return -1
}

// project applies $select.
func project(o map[string]any, sel string) map[string]any {
	if sel == "" {
		return o
	}
	out := map[string]any{}
	if t, ok := o["@odata.type"]; ok {
		out["@odata.type"] = t
	}
	out["id"] = o["id"]
	for _, f := range strings.Split(sel, ",") {
		f = strings.TrimSpace(f)
		for k, v := range o {
			if strings.EqualFold(k, f) {
				out[k] = v
			}
		}
	}
	return out
}

// public strips internal (underscore-prefixed) fields.
func public(o map[string]any) map[string]any {
	out := make(map[string]any, len(o))
	for k, v := range o {
		if !strings.HasPrefix(k, "_") {
			out[k] = v
		}
	}
	return out
}
