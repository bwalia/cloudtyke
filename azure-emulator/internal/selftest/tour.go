package selftest

import (
	"fmt"
	"html/template"
	"strings"
)

// tourPage renders /_cloudtyke/selftest?tour=1: one slide per service with
// every step labelled, its expected and actual result, the extra assertions
// and the request / response bodies. Navigate with Back / Next or the arrow
// keys; the slide number is kept in the URL hash.
var tourPage = template.Must(template.New("tour").Funcs(template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"title": func(s string) string {
		if n, ok := sectionNames[s]; ok {
			return n
		}
		return s
	},
	"ok": func(c Call) bool {
		if c.Expect == "" {
			return c.Status < 400
		}
		for _, e := range strings.Split(c.Expect, " or ") {
			if strings.HasPrefix(e, fmt.Sprint(c.Status)) {
				return true
			}
		}
		return false
	},
	"md": func(s string) template.HTML { // render `code` spans from coverage notes
		parts := strings.Split(template.HTMLEscapeString(s), "`")
		var b strings.Builder
		for i, p := range parts {
			if i%2 == 1 {
				b.WriteString("<code>" + p + "</code>")
			} else {
				b.WriteString(p)
			}
		}
		return template.HTML(b.String())
	},
}).Parse(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>CloudTyke service tour</title>
<style>
:root{--bg:#f6f7f9;--card:#fff;--fg:#1d2330;--muted:#5d6678;--line:#e3e6ec;--ok:#1a7f37;--okbg:#e6f4ea;--bad:#cf222e;--badbg:#fdecee;--acc:#0b5cd5}
@media (prefers-color-scheme:dark){:root{--bg:#0f1218;--card:#171b23;--fg:#e6e9ef;--muted:#9aa3b2;--line:#2a303b;--ok:#4ac26b;--okbg:#12261a;--bad:#ff7b72;--badbg:#2d1215;--acc:#6ea8fe}}
*{box-sizing:border-box}body{margin:0;font:14px/1.5 system-ui,-apple-system,Segoe UI,sans-serif;background:var(--bg);color:var(--fg)}
header{position:sticky;top:0;z-index:2;background:var(--bg);border-bottom:1px solid var(--line)}
.top{max-width:1100px;margin:0 auto;padding:12px 20px;display:flex;gap:14px;align-items:center}
.top h1{font-size:15px;margin:0;white-space:nowrap}
.dots{display:flex;gap:4px;flex-wrap:wrap;flex:1}
.dots a{width:14px;height:14px;border-radius:3px;background:var(--okbg);border:1px solid var(--ok);display:block}
.dots a.fail,.dots a.missing,.dots a.stale{background:var(--badbg);border-color:var(--bad)}
.dots a.cur{outline:2px solid var(--acc);outline-offset:1px}
.nav{display:flex;gap:8px}button,.btn{background:var(--card);color:var(--fg);border:1px solid var(--line);border-radius:8px;padding:8px 14px;font:inherit;font-weight:600;cursor:pointer;text-decoration:none}
button.primary{background:var(--fg);color:var(--bg);border-color:var(--fg)}button:disabled{opacity:.35;cursor:default}
main{max-width:1100px;margin:0 auto;padding:24px 20px 80px}
.slide{display:none}.slide.on{display:block}
.kicker{color:var(--muted);font-size:12px;text-transform:uppercase;letter-spacing:.06em}
h2{font-size:24px;margin:2px 0 6px;display:flex;gap:12px;align-items:center}
.pill{font-size:12px;font-weight:700;border-radius:999px;padding:3px 10px}.pill.pass{background:var(--okbg);color:var(--ok)}.pill.fail,.pill.missing,.pill.stale{background:var(--badbg);color:var(--bad)}
.lead{font-size:16px;margin:0 0 6px}.notes{color:var(--muted);margin:0 0 16px}
.stats{display:flex;gap:18px;color:var(--muted);margin-bottom:16px;font-variant-numeric:tabular-nums}.stats b{color:var(--fg)}
.err{background:var(--badbg);color:var(--bad);padding:12px 14px;border-radius:8px;white-space:pre-wrap;margin-bottom:14px}
ol{list-style:none;margin:0;padding:0;background:var(--card);border:1px solid var(--line);border-radius:10px;overflow:hidden}
li{border-top:1px solid var(--line)}li:first-child{border-top:0}
details>summary{display:grid;grid-template-columns:34px 22px 1fr auto;gap:10px;align-items:start;padding:10px 16px;cursor:pointer;list-style:none}
summary::-webkit-details-marker{display:none}
.n{color:var(--muted);font-variant-numeric:tabular-nums;text-align:right}
.label{font-weight:600}.req{display:block;font:12.5px ui-monospace,SFMono-Regular,Menlo,monospace;color:var(--muted);word-break:break-all;margin-top:2px}
.res{white-space:nowrap;font:12.5px ui-monospace,Menlo,monospace;text-align:right}.res .exp{color:var(--muted)}.res .got.good{color:var(--ok)}.res .got.bad{color:var(--bad)}
.ver{grid-column:3/5;color:var(--ok);font-size:13px}.ver div::before{content:"✓ ";font-weight:700}
.bodies{display:grid;grid-template-columns:1fr 1fr;gap:12px;padding:0 16px 14px 82px}
.bodies h4{margin:0 0 4px;font-size:11px;text-transform:uppercase;letter-spacing:.05em;color:var(--muted)}
pre{margin:0;padding:10px;background:var(--bg);border:1px solid var(--line);border-radius:8px;max-height:340px;overflow:auto;font:12px/1.45 ui-monospace,Menlo,monospace;white-space:pre-wrap;word-break:break-all}
code{font:12.5px ui-monospace,Menlo,monospace}h2 code{font-size:22px;font-weight:600}
.hint{color:var(--muted);font-size:12px;margin-top:14px}
@media (max-width:760px){.bodies{grid-template-columns:1fr;padding-left:16px}.top h1{display:none}}
</style></head><body>
<header><div class="top"><h1>CloudTyke tour</h1>
<div class="dots">{{range $i, $r := .R.Results}}<a href="#{{add $i 1}}" class="{{$r.Status}}" title="{{$r.Service}}" data-i="{{$i}}"></a>{{end}}</div>
<div class="nav"><button id="prev">← Back</button><button id="next" class="primary">Next →</button><a class="btn" href="?">Overview</a></div></div></header>
<main>
{{$total := len .R.Results}}{{range $i, $r := .R.Results}}<section class="slide" data-i="{{$i}}">
<div class="kicker">{{add $i 1}} of {{$total}} · {{title $r.Section}}</div>
<h2><code>{{$r.Service}}</code><span class="pill {{$r.Status}}">{{if eq $r.Status "pass"}}working{{else}}{{$r.Status}}{{end}}</span></h2>
{{if $r.Title}}<p class="lead">Checked: {{$r.Title}}</p>{{end}}
{{if $r.Notes}}<p class="notes">Emulated: {{md $r.Notes}}</p>{{end}}
<div class="stats"><span><b>{{len $r.Calls}}</b> HTTP calls</span><span><b>{{$r.DurationMs}}</b> ms</span></div>
{{if $r.Error}}<div class="err">{{$r.Error}}</div>{{end}}
<ol>{{range $j, $c := $r.Calls}}<li><details><summary>
<span class="n">{{add $j 1}}</span><span>{{if ok $c}}✅{{else}}❌{{end}}</span>
<span><span class="label">{{if $c.Step}}{{$c.Step}}{{else}}{{$c.Method}} request{{end}}</span><span class="req">{{$c.Method}} {{$c.URL}}</span></span>
<span class="res">{{if $c.Expect}}<span class="exp">expect {{$c.Expect}} · </span>{{end}}<span class="got {{if ok $c}}good{{else}}bad{{end}}">got {{$c.Status}}{{if $c.Error}} {{$c.Error}}{{end}}</span></span>
{{if $c.Verified}}<span></span><span></span><span class="ver">{{range $c.Verified}}<div>{{.}}</div>{{end}}</span>{{end}}
</summary><div class="bodies"><div><h4>Request</h4><pre>{{if $c.Request}}{{$c.Request}}{{else}}(no body){{end}}</pre></div><div><h4>Response {{$c.Status}}</h4><pre>{{if $c.Response}}{{$c.Response}}{{else}}(no body){{end}}</pre></div></div></details></li>{{end}}</ol>
<p class="hint">Click a step to see its request and response. Use ← → to move between services.</p>
</section>{{end}}
</main>
<script>
const slides=[...document.querySelectorAll('.slide')],dots=[...document.querySelectorAll('.dots a')];
const prev=document.getElementById('prev'),next=document.getElementById('next');
function show(i){i=Math.max(0,Math.min(slides.length-1,i));slides.forEach((s,k)=>s.classList.toggle('on',k===i));
dots.forEach((d,k)=>d.classList.toggle('cur',k===i));prev.disabled=i===0;next.disabled=i===slides.length-1;
if(location.hash!=='#'+(i+1))history.replaceState(null,'','#'+(i+1));window.scrollTo(0,0);return i}
let cur=show((parseInt(location.hash.slice(1))||1)-1);
prev.onclick=()=>cur=show(cur-1);next.onclick=()=>cur=show(cur+1);
dots.forEach(d=>d.onclick=e=>{e.preventDefault();cur=show(+d.dataset.i)});
document.addEventListener('keydown',e=>{if(e.key==='ArrowRight')cur=show(cur+1);if(e.key==='ArrowLeft')cur=show(cur-1)});
</script>
</body></html>`))
