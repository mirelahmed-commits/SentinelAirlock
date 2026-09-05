package fleet

import (
	"html/template"
	"net/http"
	"strings"
	"time"
)

// indexPage renders the fleet inventory: summary counts plus a table of
// every known Sentinel, polling the JSON API every few seconds. No desired-
// policy/drift column is shown -- drift requires comparing reported state
// against a centrally-declared desired policy, which does not exist yet
// (Prompt 14A). Showing a fake or always-zero "Drifted" count would be
// dishonest, so it is omitted rather than stubbed.
var indexPage = template.Must(template.New("index").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>Airlock Fleet</title>
<style>
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#0b1220;color:#e5e7eb;margin:0;padding:28px 32px}
h1{font-size:20px;letter-spacing:.04em;margin:0 0 4px;text-transform:uppercase}
.sub{color:#94a3b8;font-size:13px;margin-bottom:20px}
.stats{display:flex;gap:14px;margin-bottom:22px}
.stat{background:#101b2b;border:1px solid #28364a;border-radius:10px;padding:14px 20px;min-width:110px}
.stat .n{font-size:26px;font-weight:750}
.stat .l{font-size:11px;color:#94a3b8;text-transform:uppercase;letter-spacing:.06em}
.stat.active .n{color:#5eead4}
.stat.offline .n{color:#fca5a5}
table{width:100%;border-collapse:collapse;background:#101b2b;border:1px solid #28364a;border-radius:10px;overflow:hidden}
th,td{text-align:left;padding:9px 14px;font-size:13px;border-top:1px solid #1e293b}
th{color:#94a3b8;font-size:11px;text-transform:uppercase;letter-spacing:.05em;border-top:none}
tr:hover td{background:#131f33}
a{color:#93c5fd;text-decoration:none}
.badge{border-radius:999px;padding:2px 9px;font-size:11px;font-weight:800;letter-spacing:.03em}
.badge.active{background:#052e2b;color:#99f6e4;border:1px solid #0f766e}
.badge.offline{background:#3b1318;color:#fecaca;border:1px solid #991b1b}
.mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px}
.empty{padding:30px;text-align:center;color:#94a3b8}
</style></head>
<body>
<h1>Airlock Fleet</h1>
<div class="sub">Control plane for Sentinel inventory and health. Not in the filesystem-policy decision path -- Sentinels enforce locally whether or not this page can reach them.</div>
<div class="stats">
  <div class="stat active"><div class="n" id="active">-</div><div class="l">Active</div></div>
  <div class="stat offline"><div class="n" id="offline">-</div><div class="l">Offline</div></div>
</div>
<div id="table-wrap"></div>
<script>
function esc(v){return String(v==null?"":v).replace(/[&<>"']/g,function(c){return {"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"}[c];});}
function ago(iso){if(!iso)return "never";var d=new Date(iso);if(isNaN(d.getTime()))return "never";var s=Math.max(0,Math.round((Date.now()-d.getTime())/1000));if(s<60)return s+"s ago";if(s<3600)return Math.round(s/60)+"m ago";if(s<86400)return Math.round(s/3600)+"h ago";return Math.round(s/86400)+"d ago";}
function render(d){
  document.getElementById("active").textContent=d.active;
  document.getElementById("offline").textContent=d.offline;
  var wrap=document.getElementById("table-wrap");
  if(!d.sentinels||!d.sentinels.length){wrap.innerHTML="<div class='empty'>No Sentinels enrolled yet. Start one with: airlock sentinel --repo . --fleet http://&lt;this-host&gt; --background</div>";return;}
  var rows=d.sentinels.map(function(sv){
    var policy=(sv.policy_id||"-")+(sv.policy_version?"@"+sv.policy_version:"");
    return "<tr><td><a href='/fleet/sentinels/"+encodeURIComponent(sv.sentinel_id)+"'>"+esc(sv.sentinel_id.slice(0,8))+"</a></td>"+
      "<td><span class='badge "+(sv.health==="ACTIVE"?"active":"offline")+"'>"+sv.health+"</span></td>"+
      "<td class='mono'>"+esc(sv.repo_path||"-")+"</td>"+
      "<td>"+esc(policy)+"</td>"+
      "<td>"+ago(sv.last_heartbeat)+"</td></tr>";
  }).join("");
  wrap.innerHTML="<table><tr><th>Sentinel</th><th>Status</th><th>Repository</th><th>Policy</th><th>Heartbeat</th></tr>"+rows+"</table>";
}
function load(){fetch("/api/fleet/sentinels",{cache:"no-store"}).then(function(r){return r.json();}).then(render).catch(function(){});}
load();setInterval(load,3000);
</script>
</body></html>`))

var detailPage = template.Must(template.New("detail").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>Sentinel {{.SentinelID}}</title>
<style>
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#0b1220;color:#e5e7eb;margin:0;padding:28px 32px}
a{color:#93c5fd;text-decoration:none}
h1{font-size:18px;margin:16px 0 18px}
.grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px}
.field{background:#101b2b;border:1px solid #28364a;border-radius:10px;padding:12px 14px}
.k{font-size:10px;text-transform:uppercase;letter-spacing:.07em;color:#94a3b8}
.v{font-size:14px;font-weight:650;margin-top:4px;word-break:break-all}
.badge{border-radius:999px;padding:2px 9px;font-size:11px;font-weight:800}
.badge.active{background:#052e2b;color:#99f6e4;border:1px solid #0f766e}
.badge.offline{background:#3b1318;color:#fecaca;border:1px solid #991b1b}
</style></head>
<body>
<a href="/">&larr; Fleet inventory</a>
<h1>Sentinel <span class="badge {{if eq .Health "ACTIVE"}}active{{else}}offline{{end}}">{{.Health}}</span></h1>
<div class="grid">
  <div class="field"><div class="k">Sentinel ID</div><div class="v">{{.SentinelID}}</div></div>
  <div class="field"><div class="k">Machine ID</div><div class="v">{{.MachineID}}</div></div>
  <div class="field"><div class="k">Repository</div><div class="v">{{.RepoPath}}</div></div>
  <div class="field"><div class="k">Hostname</div><div class="v">{{.Hostname}}</div></div>
  <div class="field"><div class="k">Platform</div><div class="v">{{.Platform}}</div></div>
  <div class="field"><div class="k">Sentinel version</div><div class="v">{{.SentinelVersion}}</div></div>
  <div class="field"><div class="k">Current session</div><div class="v">{{.SessionID}}</div></div>
  <div class="field"><div class="k">Policy</div><div class="v">{{if .PolicyID}}{{.PolicyID}}{{if .PolicyVersion}}@{{.PolicyVersion}}{{end}}{{else}}-{{end}}</div></div>
  <div class="field"><div class="k">Policy hash</div><div class="v">{{if .PolicyHash}}{{.PolicyHash}}{{else}}-{{end}}</div></div>
  <div class="field"><div class="k">Enrolled</div><div class="v">{{.EnrolledAt}}</div></div>
  <div class="field"><div class="k">Started</div><div class="v">{{.StartedAt}}</div></div>
  <div class="field"><div class="k">Last heartbeat</div><div class="v">{{.LastHeartbeat}}</div></div>
  <div class="field"><div class="k">Last event</div><div class="v">{{if .LastEventAt}}{{.LastEventAt}}{{else}}-{{end}}</div></div>
  <div class="field"><div class="k">Allowed</div><div class="v">{{.AllowCount}}</div></div>
  <div class="field"><div class="k">Denied</div><div class="v">{{.DenyCount}}</div></div>
  <div class="field"><div class="k">Reverted</div><div class="v">{{.RevertedCount}}</div></div>
  <div class="field"><div class="k">Revert failed</div><div class="v">{{.RevertFailedCount}}</div></div>
</div>
</body></html>`))

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = indexPage.Execute(w, nil)
}

func (s *Server) handleDetailPage(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/fleet/sentinels/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	rec, ok := s.store.Get(id)
	if !ok {
		http.Error(w, "sentinel not found", http.StatusNotFound)
		return
	}
	view := SentinelView{Record: rec, Health: Health(rec, time.Now().UTC())}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = detailPage.Execute(w, view)
}

// FormatAge renders how long ago t was, or "never" for a zero time. Used by
// `airlock fleet list`/`status` so the CLI table matches the web UI's
// freshness wording.
func FormatAge(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return d.Round(time.Second).String() + " ago"
	case d < time.Hour:
		return d.Round(time.Minute).String() + " ago"
	default:
		return d.Round(time.Hour).String() + " ago"
	}
}
