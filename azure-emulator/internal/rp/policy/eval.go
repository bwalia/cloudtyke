package policy

// Evaluation of Azure Policy rules: the "if" condition language and the
// small template-expression subset rules use.
//
//   logical:   allOf, anyOf, not
//   operands:  field (type, name, location, kind, id, fullName, tags,
//              tags['x'], tags.x, aliases such as
//              Microsoft.Storage/storageAccounts/minimumTlsVersion and
//              Microsoft.Network/networkSecurityGroups/securityRules[*].access)
//              and value
//   operators: equals, notEquals, like, notLike, match, notMatch, contains,
//              notContains, in, notIn, containsKey, notContainsKey, less,
//              lessOrEquals, greater, greaterOrEquals, exists
//   functions: parameters(), field(), concat(), toLower(), toUpper(),
//              resourceGroup().name/location, subscription().subscriptionId,
//              length(), if(), equals(), not(), contains(), empty()

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// target is the resource being evaluated.
type target struct {
	body   map[string]any // ARM JSON (id, name, type, location, tags, properties, sku, kind)
	typ    string         // full type, e.g. Microsoft.Storage/storageAccounts
	rgName string
	rgLoc  string
	rgTags map[string]any
	subID  string
	params map[string]any
}

func strOf(v any) string { s, _ := v.(string); return s }

// ------------------------------------------------------------ fields and aliases

// fieldValues resolves a policy "field" to one or more values. ok=false means
// the field does not exist on the resource. Aliases with [*] return every
// array member's value and many=true.
func (t *target) fieldValues(field string) (vals []any, ok bool, many bool) {
	f := strings.TrimSpace(field)
	lf := strings.ToLower(f)
	switch {
	case lf == "type":
		return []any{t.typ}, true, false
	case lf == "name":
		return []any{t.body["name"]}, t.body["name"] != nil, false
	case lf == "fullname":
		return []any{t.body["name"]}, true, false
	case lf == "location":
		v, ok := t.body["location"]
		return []any{v}, ok && v != nil, false
	case lf == "kind":
		v, ok := t.body["kind"]
		return []any{v}, ok, false
	case lf == "id":
		return []any{t.body["id"]}, true, false
	case lf == "tags":
		v, ok := t.body["tags"]
		return []any{v}, ok && v != nil, false
	case strings.HasPrefix(lf, "tags[") || strings.HasPrefix(lf, "tags."):
		key := strings.TrimPrefix(f[4:], ".")
		key = strings.Trim(key, "[]'\"")
		tags, _ := t.body["tags"].(map[string]any)
		for k, v := range tags {
			if strings.EqualFold(k, key) {
				return []any{v}, true, false
			}
		}
		return []any{nil}, false, false
	case lf == "identity.type":
		id, _ := t.body["identity"].(map[string]any)
		v, ok := id["type"]
		return []any{v}, ok, false
	}
	// alias: <Namespace>/<type>[/<subtype>...]/<path>
	path := f
	if strings.HasPrefix(strings.ToLower(f), strings.ToLower(t.typ)+"/") {
		path = f[len(t.typ)+1:]
	} else if i := strings.Index(f, "/"); i > 0 && strings.Contains(f[:i], ".") {
		// alias for another type: does not apply to this resource
		return []any{nil}, false, false
	}
	return walk(t.body, path)
}

// walk follows a dotted alias path. Real aliases usually map to
// properties.<path>; sku/identity/kind live at the top level. Array members
// that are child resources keep their fields under "properties" as well.
func walk(body map[string]any, path string) ([]any, bool, bool) {
	parts := strings.Split(path, ".")
	cur := []any{body}
	many := false
	for i, p := range parts {
		star := strings.HasSuffix(p, "[*]")
		name := strings.TrimSuffix(p, "[*]")
		var next []any
		found := false
		for _, c := range cur {
			m, _ := c.(map[string]any)
			if m == nil {
				continue
			}
			v, ok := lookupCI(m, name)
			if !ok {
				if pm, _ := m["properties"].(map[string]any); pm != nil {
					v, ok = lookupCI(pm, name)
				}
			}
			if !ok {
				continue
			}
			found = true
			if star {
				many = true
				if arr, _ := v.([]any); arr != nil {
					next = append(next, arr...)
				}
			} else {
				next = append(next, v)
			}
		}
		if !found {
			if many && i > 0 {
				return nil, true, true // empty array: nothing to check
			}
			return []any{nil}, false, many
		}
		cur = next
	}
	return cur, true, many
}

func lookupCI(m map[string]any, k string) (any, bool) {
	if v, ok := m[k]; ok {
		return v, true
	}
	for kk, v := range m {
		if strings.EqualFold(kk, k) {
			return v, true
		}
	}
	return nil, false
}

// ------------------------------------------------------------ conditions

var operators = []string{"equals", "notEquals", "like", "notLike", "match", "matchInsensitively", "notMatch", "notMatchInsensitively", "contains", "notContains", "in", "notIn", "containsKey", "notContainsKey", "less", "lessOrEquals", "greater", "greaterOrEquals", "exists"}

// eval evaluates a policy condition.
func (t *target) eval(c map[string]any) (bool, error) {
	if v, ok := c["allOf"]; ok {
		for _, x := range asList(v) {
			m, _ := x.(map[string]any)
			ok, err := t.eval(m)
			if err != nil || !ok {
				return false, err
			}
		}
		return true, nil
	}
	if v, ok := c["anyOf"]; ok {
		for _, x := range asList(v) {
			m, _ := x.(map[string]any)
			ok, err := t.eval(m)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	if v, ok := c["not"]; ok {
		m, _ := v.(map[string]any)
		ok, err := t.eval(m)
		return !ok, err
	}
	op, want := "", any(nil)
	for _, o := range operators {
		for k, v := range c {
			if strings.EqualFold(k, o) {
				op, want = o, v
			}
		}
	}
	if op == "" {
		return false, fmt.Errorf("condition %v has no supported operator", c)
	}
	want, err := t.expr(want)
	if err != nil {
		return false, err
	}
	if f, ok := c["field"]; ok {
		fs, err := t.expr(f)
		if err != nil {
			return false, err
		}
		vals, exists, many := t.fieldValues(strOf(fs))
		if op == "exists" {
			return exists == truthy(want), nil
		}
		if many {
			// [*] conditions hold only when every array member satisfies them
			for _, v := range vals {
				if !compare(op, v, want) {
					return false, nil
				}
			}
			return true, nil
		}
		var v any
		if len(vals) > 0 {
			v = vals[0]
		}
		return compare(op, v, want), nil
	}
	if v, ok := c["value"]; ok {
		got, err := t.expr(v)
		if err != nil {
			return false, err
		}
		if op == "exists" {
			return (got != nil) == truthy(want), nil
		}
		return compare(op, got, want), nil
	}
	if _, ok := c["count"]; ok {
		return false, fmt.Errorf("count expressions are not supported by the emulator yet")
	}
	return false, fmt.Errorf("condition %v needs field or value", c)
}

func asList(v any) []any { l, _ := v.([]any); return l }

func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true")
	}
	return v != nil
}

func compare(op string, got, want any) bool {
	gs, ws := fmt.Sprint(got), fmt.Sprint(want)
	if got == nil {
		gs = ""
	}
	switch op {
	case "equals":
		return strings.EqualFold(gs, ws)
	case "notEquals":
		return !strings.EqualFold(gs, ws)
	case "like":
		return like(gs, ws)
	case "notLike":
		return !like(gs, ws)
	case "match", "matchInsensitively":
		return match(gs, ws, op == "matchInsensitively")
	case "notMatch", "notMatchInsensitively":
		return !match(gs, ws, op == "notMatchInsensitively")
	case "contains":
		return strings.Contains(strings.ToLower(gs), strings.ToLower(ws))
	case "notContains":
		return !strings.Contains(strings.ToLower(gs), strings.ToLower(ws))
	case "in", "notIn":
		in := false
		for _, w := range asList(want) {
			if strings.EqualFold(gs, fmt.Sprint(w)) {
				in = true
			}
		}
		return in == (op == "in")
	case "containsKey", "notContainsKey":
		m, _ := got.(map[string]any)
		_, has := lookupCI(m, ws)
		return has == (op == "containsKey")
	case "less", "lessOrEquals", "greater", "greaterOrEquals":
		a, e1 := strconv.ParseFloat(gs, 64)
		b, e2 := strconv.ParseFloat(ws, 64)
		var c int
		if e1 == nil && e2 == nil {
			switch {
			case a < b:
				c = -1
			case a > b:
				c = 1
			}
		} else {
			c = strings.Compare(gs, ws)
		}
		switch op {
		case "less":
			return c < 0
		case "lessOrEquals":
			return c <= 0
		case "greater":
			return c > 0
		}
		return c >= 0
	}
	return false
}

func like(s, pattern string) bool {
	re := "^" + strings.ReplaceAll(regexp.QuoteMeta(strings.ToLower(pattern)), `\*`, ".*") + "$"
	ok, _ := regexp.MatchString(re, strings.ToLower(s))
	return ok
}

// match: # = digit, ? = letter, . = any char, anything else literal.
func match(s, pattern string, ci bool) bool {
	if len(s) != len(pattern) {
		return false
	}
	for i := 0; i < len(s); i++ {
		p, c := pattern[i], s[i]
		switch p {
		case '#':
			if c < '0' || c > '9' {
				return false
			}
		case '?':
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
				return false
			}
		case '.':
		default:
			if ci {
				if strings.ToLower(string(p)) != strings.ToLower(string(c)) {
					return false
				}
			} else if p != c {
				return false
			}
		}
	}
	return true
}

// ------------------------------------------------------------ template expressions

// expr resolves "[...]" template expressions inside strings, lists and maps.
func (t *target) expr(v any) (any, error) {
	switch x := v.(type) {
	case string:
		s := strings.TrimSpace(x)
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") && !strings.HasPrefix(s, "[[") {
			p := &parser{s: s[1 : len(s)-1], t: t}
			out, err := p.parse()
			if err != nil {
				return nil, fmt.Errorf("expression %s: %w", s, err)
			}
			return out, nil
		}
		return x, nil
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			r, err := t.expr(e)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	}
	return v, nil
}

type parser struct {
	s   string
	pos int
	t   *target
}

func (p *parser) ws() {
	for p.pos < len(p.s) && p.s[p.pos] == ' ' {
		p.pos++
	}
}

func (p *parser) parse() (any, error) {
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	p.ws()
	if p.pos != len(p.s) {
		return nil, fmt.Errorf("unexpected %q", p.s[p.pos:])
	}
	return v, nil
}

func (p *parser) value() (any, error) {
	p.ws()
	if p.pos >= len(p.s) {
		return nil, fmt.Errorf("unexpected end")
	}
	c := p.s[p.pos]
	var v any
	switch {
	case c == '\'':
		end := strings.IndexByte(p.s[p.pos+1:], '\'')
		if end < 0 {
			return nil, fmt.Errorf("unterminated string")
		}
		v = p.s[p.pos+1 : p.pos+1+end]
		p.pos += end + 2
	case c >= '0' && c <= '9' || c == '-':
		start := p.pos
		p.pos++
		for p.pos < len(p.s) && p.s[p.pos] >= '0' && p.s[p.pos] <= '9' {
			p.pos++
		}
		n, _ := strconv.Atoi(p.s[start:p.pos])
		v = float64(n)
	default:
		start := p.pos
		for p.pos < len(p.s) && (p.s[p.pos] == '_' || p.s[p.pos] >= 'a' && p.s[p.pos] <= 'z' || p.s[p.pos] >= 'A' && p.s[p.pos] <= 'Z') {
			p.pos++
		}
		name := p.s[start:p.pos]
		if name == "" {
			return nil, fmt.Errorf("unexpected %q", p.s[p.pos:])
		}
		if name == "true" || name == "false" {
			v = name == "true"
			break
		}
		p.ws()
		if p.pos >= len(p.s) || p.s[p.pos] != '(' {
			return nil, fmt.Errorf("expected ( after %s", name)
		}
		p.pos++
		var args []any
		for {
			p.ws()
			if p.pos < len(p.s) && p.s[p.pos] == ')' {
				p.pos++
				break
			}
			a, err := p.value()
			if err != nil {
				return nil, err
			}
			args = append(args, a)
			p.ws()
			if p.pos < len(p.s) && p.s[p.pos] == ',' {
				p.pos++
			}
		}
		r, err := p.t.call(name, args)
		if err != nil {
			return nil, err
		}
		v = r
	}
	// property access: .name or ['name']
	for p.pos < len(p.s) && (p.s[p.pos] == '.' || p.s[p.pos] == '[') {
		if p.s[p.pos] == '[' {
			p.pos++
			k, err := p.value()
			if err != nil {
				return nil, err
			}
			p.ws()
			if p.pos >= len(p.s) || p.s[p.pos] != ']' {
				return nil, fmt.Errorf("expected ]")
			}
			p.pos++
			m, _ := v.(map[string]any)
			v, _ = lookupCI(m, fmt.Sprint(k))
			continue
		}
		p.pos++
		start := p.pos
		for p.pos < len(p.s) && (p.s[p.pos] == '_' || p.s[p.pos] >= 'a' && p.s[p.pos] <= 'z' || p.s[p.pos] >= 'A' && p.s[p.pos] <= 'Z' || p.s[p.pos] >= '0' && p.s[p.pos] <= '9') {
			p.pos++
		}
		m, _ := v.(map[string]any)
		v, _ = lookupCI(m, p.s[start:p.pos])
	}
	return v, nil
}

func (t *target) call(name string, args []any) (any, error) {
	arg := func(i int) string {
		if i < len(args) {
			return fmt.Sprint(args[i])
		}
		return ""
	}
	switch strings.ToLower(name) {
	case "parameters":
		v, ok := t.params[arg(0)]
		if !ok {
			return nil, fmt.Errorf("parameter '%s' is not defined", arg(0))
		}
		return v, nil
	case "field":
		vals, ok, many := t.fieldValues(arg(0))
		if !ok {
			return nil, nil
		}
		if many {
			return vals, nil
		}
		return vals[0], nil
	case "concat":
		if len(args) > 0 {
			if _, isList := args[0].([]any); isList {
				var out []any
				for _, a := range args {
					out = append(out, asList(a)...)
				}
				return out, nil
			}
		}
		var b strings.Builder
		for _, a := range args {
			b.WriteString(fmt.Sprint(a))
		}
		return b.String(), nil
	case "tolower":
		return strings.ToLower(arg(0)), nil
	case "toupper":
		return strings.ToUpper(arg(0)), nil
	case "resourcegroup":
		tags := t.rgTags
		if tags == nil {
			tags = map[string]any{}
		}
		return map[string]any{"name": t.rgName, "location": t.rgLoc, "tags": tags}, nil
	case "subscription":
		return map[string]any{"subscriptionId": t.subID, "id": "/subscriptions/" + t.subID}, nil
	case "length":
		if l, ok := args[0].([]any); ok {
			return float64(len(l)), nil
		}
		if m, ok := args[0].(map[string]any); ok {
			return float64(len(m)), nil
		}
		return float64(len(arg(0))), nil
	case "empty":
		if len(args) == 0 || args[0] == nil {
			return true, nil
		}
		switch x := args[0].(type) {
		case []any:
			return len(x) == 0, nil
		case map[string]any:
			return len(x) == 0, nil
		}
		return arg(0) == "", nil
	case "if":
		if len(args) < 3 {
			return nil, fmt.Errorf("if() needs 3 arguments")
		}
		if truthy(args[0]) {
			return args[1], nil
		}
		return args[2], nil
	case "equals":
		return strings.EqualFold(arg(0), arg(1)), nil
	case "not":
		return !truthy(args[0]), nil
	case "contains":
		if l, ok := args[0].([]any); ok {
			for _, x := range l {
				if strings.EqualFold(fmt.Sprint(x), arg(1)) {
					return true, nil
				}
			}
			return false, nil
		}
		return strings.Contains(strings.ToLower(arg(0)), strings.ToLower(arg(1))), nil
	}
	return nil, fmt.Errorf("function %s() is not supported by the emulator", name)
}
